package service

import (
	"context"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

func AdvanceVideoPipelineIfNeeded(ctx context.Context, task *model.Task, taskResult *relaycommon.TaskInfo, responseBody []byte) (bool, error) {
	if task == nil || task.PrivateData.SeedanceEnhance == nil || taskResult == nil {
		return false, nil
	}
	p := task.PrivateData.SeedanceEnhance
	if p.EnhanceProvider == "" || p.EnhanceTargetResolution == "" {
		return false, nil
	}
	if taskResult.Status != model.TaskStatusSuccess && p.EnhanceTaskID == "" {
		return false, nil
	}
	snap := task.Snapshot()
	now := time.Now().Unix()

	if p.GenerationVideoURL == "" && taskResult.Url != "" {
		p.GenerationVideoURL = taskResult.Url
	}
	if taskResult.Status == model.TaskStatusSuccess {
		// Fy-api overlay: prefer upstream reported Seedance usage for the
		// internal provider-cost estimate before the MediaKit phase starts.
		applySeedanceGenerationCostSnapshot(p, relaycommon.TaskSubmitReq{}, taskResult)
		updatePipelineCostTotals(p)
	}
	if len(responseBody) > 0 {
		task.Data = redactVideoResponseBody(responseBody)
	}

	client := NewMediaKitClientFromEnv()
	if p.EnhanceTaskID == "" {
		toolVersion := p.EnhanceToolVersion
		if toolVersion == "" {
			toolVersion = "standard"
		}
		scene := p.EnhanceScene
		if scene == "" {
			scene = "aigc"
		}
		resolution := p.EnhanceTargetResolution
		if resolution == "" {
			resolution = p.EnhanceOutputResolution
		}
		submit, err := client.SubmitEnhanceVideo(ctx, MediaKitSubmitRequest{
			VideoURL:    p.GenerationVideoURL,
			Scene:       scene,
			ToolVersion: toolVersion,
			Resolution:  resolution,
		})
		if err != nil {
			return completePipelineWithFallback(ctx, task, snap.Status, fmt.Sprintf("submit enhance failed: %s", err.Error()))
		}
		p.EnhanceScene = scene
		p.EnhanceToolVersion = toolVersion
		p.EnhanceOutputResolution = resolution
		p.EnhanceTaskID = submit.TaskID
		p.EnhanceRequestID = submit.RequestID
		p.Status = "enhance_submitted"
		task.Status = model.TaskStatusInProgress
		task.Progress = "70%"
		if task.StartTime == 0 {
			task.StartTime = now
		}
		won, err := task.UpdateWithStatus(snap.Status)
		if err != nil || !won {
			return true, err
		}
		return true, nil
	}

	res, err := client.GetEnhanceTask(ctx, p.EnhanceTaskID)
	if err != nil {
		p.EnhanceError = err.Error()
		p.Status = "enhance_running"
		task.Status = model.TaskStatusInProgress
		task.Progress = "85%"
		_, updateErr := task.UpdateWithStatus(snap.Status)
		return true, updateErr
	}
	switch res.Status {
	case "completed", "success", "succeeded":
		// Fy-api overlay: record MediaKit list-price estimate on task private
		// data when the enhancement phase finishes.
		p.Status = "enhance_succeeded"
		p.EnhancedVideoURL = res.Result.VideoURL
		p.ActualDurationSeconds = res.Result.Duration
		p.ActualFPS = res.Result.FPS
		applyMediaKitEnhanceCostSnapshot(p, res)
		updatePipelineCostTotals(p)
		task.PrivateData.ResultURL = res.Result.VideoURL
		task.Status = model.TaskStatusSuccess
		task.Progress = taskcommon.ProgressComplete
		if task.FinishTime == 0 {
			task.FinishTime = now
		}
	case "failed", "failure":
		return completePipelineWithFallback(ctx, task, snap.Status, "enhance failed")
	default:
		p.Status = "enhance_running"
		task.Status = model.TaskStatusInProgress
		task.Progress = "85%"
	}
	won, err := task.UpdateWithStatus(snap.Status)
	if err != nil || !won {
		return true, err
	}
	return true, nil
}

func hydrateVideoPipelinePrivateData(task *model.Task) {
	if task == nil || task.ID == 0 || task.PrivateData.SeedanceEnhance != nil {
		return
	}
	var current model.Task
	if err := model.DB.Select("private_data").Where("id = ?", task.ID).First(&current).Error; err != nil {
		return
	}
	if current.PrivateData.SeedanceEnhance == nil {
		return
	}
	task.PrivateData.SeedanceEnhance = current.PrivateData.SeedanceEnhance
}

func completePipelineWithFallback(_ context.Context, task *model.Task, oldStatus model.TaskStatus, reason string) (bool, error) {
	p := task.PrivateData.SeedanceEnhance
	p.Status = "enhance_failed_fallback"
	p.EnhanceError = reason
	task.PrivateData.ResultURL = p.GenerationVideoURL
	task.Status = model.TaskStatusSuccess
	task.Progress = taskcommon.ProgressComplete
	task.FailReason = ""
	if task.FinishTime == 0 {
		task.FinishTime = time.Now().Unix()
	}
	if task.PrivateData.ResultURL == "" {
		task.PrivateData.ResultURL = taskcommon.BuildProxyURL(task.TaskID)
	}
	won, err := task.UpdateWithStatus(oldStatus)
	if err != nil || !won {
		return true, err
	}
	common.SysLog(fmt.Sprintf("video pipeline fallback task=%s reason=%s", task.TaskID, reason))
	return true, nil
}
