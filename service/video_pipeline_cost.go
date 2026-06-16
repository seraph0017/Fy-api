package service

import (
	"math"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

func applySeedanceGenerationCostSnapshot(p *model.SeedanceEnhancePipeline, req relaycommon.TaskSubmitReq, taskResult *relaycommon.TaskInfo) {
	if p == nil {
		return
	}
	// Fy-api overlay: this snapshot is an estimated provider-cost view only;
	// user billing remains owned by the normal task billing pipeline.
	input := seedanceCostInputFromRequest(p, req)
	if taskResult != nil {
		if taskResult.Resolution != "" {
			input.Resolution = taskResult.Resolution
			input.OutputWidth, input.OutputHeight = videoResolutionPixels(taskResult.Resolution, p.Ratio)
		}
		if taskResult.DurationSeconds > 0 {
			input.OutputVideoSeconds = taskResult.DurationSeconds
			if p.Analysis.HasVideoReference {
				input.InputVideoSeconds = taskResult.DurationSeconds
			}
		}
		if taskResult.FPS > 0 {
			input.OutputFPS = taskResult.FPS
		}
		if taskResult.CompletionTokens > 0 {
			input.BillableTokens = int64(taskResult.CompletionTokens)
			input.UsageSource = "provider_usage_estimated"
		}
	}

	result := ResolveSeedanceGenerationCost(p.GenerationModel, input)
	p.CostPriceVersion = result.Version
	p.RMBPerUSD = result.RMBPerUSD
	p.GenerationUsageSource = result.UsageSource
	p.GenerationBillableTokens = result.BillableTokens
	p.GenerationEstimatedTokens = result.EstimatedTokens
	p.GenerationMinimumTokensApplied = result.MinimumTokensApplied
	p.GenerationCostRMB = result.CostRMB
	p.GenerationCostQuota = result.CostQuota
	p.GenerationCostDetail = mediaCostEstimateDetail("generation", p.GenerationProvider, p.GenerationModel, result)
}

func applyMediaKitEnhanceCostSnapshot(p *model.SeedanceEnhancePipeline, res *MediaKitTaskResponse) {
	if p == nil || res == nil {
		return
	}
	// Fy-api overlay: record official MediaKit list-price estimates separately
	// from TraceNex user-facing quota settlement.
	toolVersion := strings.TrimSpace(res.Result.ToolVersion)
	if toolVersion == "" {
		toolVersion = strings.TrimSpace(p.EnhanceToolVersion)
	}
	if toolVersion == "" {
		toolVersion = "standard"
	}
	resolution := strings.TrimSpace(res.Result.Resolution)
	if resolution == "" {
		resolution = strings.TrimSpace(p.EnhanceOutputResolution)
	}
	if resolution == "" {
		resolution = strings.TrimSpace(p.EnhanceTargetResolution)
	}
	fps := res.Result.FPS
	if fps <= 0 {
		fps = p.ActualFPS
	}
	if fps <= 0 {
		fps = 30
	}
	providerTaskType := strings.TrimSpace(res.TaskType)
	taskClass := "normal"
	taskClassSource := "default"

	result := ResolveMediaKitEnhanceCost(toolVersion, resolution, fps, res.Result.Duration)
	p.CostPriceVersion = result.Version
	p.RMBPerUSD = result.RMBPerUSD
	p.EnhanceBillingVersion = result.Version
	p.EnhanceToolVersion = toolVersion
	p.EnhanceOutputResolution = resolution
	p.EnhanceOutputFPS = fps
	p.EnhanceBasePriceRMBPerMinute = result.EnhanceBasePriceRMBPerMin
	p.EnhanceBillingCoefficient = result.BillingCoefficient
	p.EnhanceProviderTaskType = providerTaskType
	p.EnhanceTaskClass = taskClass
	p.EnhanceTaskClassSource = taskClassSource
	p.EnhanceCostRMB = result.CostRMB
	p.EnhanceCostQuota = result.CostQuota
	p.EnhanceCostDetail = mediaCostEstimateDetail("enhance", p.EnhanceProvider, "mediakit-enhance-video", result)
}

func updatePipelineCostTotals(p *model.SeedanceEnhancePipeline) {
	if p == nil {
		return
	}
	p.PipelineProviderCost = p.GenerationCostQuota + p.EnhanceCostQuota
	if p.UserBilledQuota > 0 {
		p.GrossProfitQuota = p.UserBilledQuota - p.PipelineProviderCost
		p.GrossMargin = float64(p.GrossProfitQuota) / float64(p.UserBilledQuota)
	}
}

func seedanceCostInputFromRequest(p *model.SeedanceEnhancePipeline, req relaycommon.TaskSubmitReq) MediaProviderCostInput {
	width, height := videoResolutionPixels(p.GenerationResolution, p.Ratio)
	outputSeconds := videoDurationSeconds(req)
	if outputSeconds <= 0 {
		outputSeconds = 5
	}
	inputVideoSeconds := 0.0
	if p.Analysis.HasVideoReference {
		inputVideoSeconds = outputSeconds
	}
	return MediaProviderCostInput{
		Model:              p.GenerationModel,
		Provider:           p.GenerationProvider,
		Resolution:         p.GenerationResolution,
		InputClass:         seedanceInputClass(p.Analysis.HasVideoReference),
		InputVideoSeconds:  inputVideoSeconds,
		OutputVideoSeconds: outputSeconds,
		OutputWidth:        width,
		OutputHeight:       height,
		OutputFPS:          videoFPSFromRequest(req),
		UsageSource:        "param_estimated",
	}
}

func seedanceInputClass(hasVideoReference bool) string {
	if hasVideoReference {
		return "video"
	}
	return "image"
}

func videoDurationSeconds(req relaycommon.TaskSubmitReq) float64 {
	if req.Seconds != "" {
		if v, err := strconv.ParseFloat(req.Seconds, 64); err == nil {
			return v
		}
	}
	if req.Duration > 0 {
		return float64(req.Duration)
	}
	return 0
}

func videoFPSFromRequest(req relaycommon.TaskSubmitReq) float64 {
	if req.Metadata != nil {
		for _, key := range []string{"fps", "frames_per_second", "framespersecond"} {
			if v, ok := req.Metadata[key]; ok {
				switch t := v.(type) {
				case int:
					return float64(t)
				case int64:
					return float64(t)
				case float64:
					return t
				case string:
					if parsed, err := strconv.ParseFloat(t, 64); err == nil {
						return parsed
					}
				}
			}
		}
	}
	return 24
}

func videoResolutionPixels(resolution, ratio string) (float64, float64) {
	landscape := ratio != "9:16"
	switch strings.ToLower(strings.TrimSpace(resolution)) {
	case "1080p":
		if landscape {
			return 1920, 1080
		}
		return 1080, 1920
	default:
		if landscape {
			return 1280, 720
		}
		return 720, 1280
	}
}

func roundCostForLog(v float64) float64 {
	return math.Round(v*1_000_000) / 1_000_000
}

func mediaCostEstimateDetail(component, provider, modelName string, result MediaProviderCostResult) *model.MediaCostEstimateDetail {
	return &model.MediaCostEstimateDetail{
		Component:      component,
		Provider:       provider,
		Model:          modelName,
		FormulaKey:     result.FormulaKey,
		FormulaVersion: result.FormulaVersion,
		FormulaText:    result.FormulaText,
		Variables:      result.Variables,
		Coefficients:   result.Coefficients,
		UsageSource:    result.UsageSource,
		CostRMB:        result.CostRMB,
		CostQuota:      result.CostQuota,
		Currency:       "RMB",
		RMBPerUSD:      result.RMBPerUSD,
		QuotaPerUnit:   result.QuotaPerUnit,
		Confidence:     result.Confidence,
	}
}
