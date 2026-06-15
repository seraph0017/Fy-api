package service

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testVideoPipelineConfig = `
version: 1
defaults:
  enabled: true
strategies:
  - name: seedance2_720p_mediakit_1080p
    enabled: true
    lifecycle:
      match:
        relay_mode: video_submit
        models:
          - doubao-seedance-2-0-260128
          - doubao-seedance-2-0-fast-260128
        requested_resolutions:
          - 1080p
      rollout:
        traffic_percent: 100
        request_override_metadata:
          force_keys:
            - fy_enhance_force
          bypass_keys:
            - fy_enhance_bypass
`

func useVideoPipelineConfig(t *testing.T, body string) {
	t.Helper()
	resetVideoPipelineConfigForTest()
	t.Cleanup(resetVideoPipelineConfigForTest)
	path := filepath.Join(t.TempDir(), "video-pipeline.yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	require.NoError(t, LoadVideoPipelineConfigFromFile(path))
}

func TestAnalyzeVideoRequest_StaticPromptAndReferences(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:          "doubao-seedance-2-0-260128",
		Prompt:         "固定镜头的产品展示，海报感",
		Size:           "1080x1920",
		InputReference: "https://example.com/a.png",
		Images:         []string{"https://example.com/b.png"},
		Metadata: map[string]interface{}{
			"storyboard": true,
			"shot_count": 3,
		},
	}

	analysis, err := AnalyzeVideoRequest(req)
	require.NoError(t, err)
	assert.Equal(t, "1080p", analysis.RequestedResolution)
	assert.Equal(t, "9:16", analysis.Ratio)
	assert.Equal(t, 2, analysis.ReferenceCount)
	assert.True(t, analysis.Storyboard)
	assert.Equal(t, 3, analysis.ShotCount)
	assert.Equal(t, "static_or_low_motion", analysis.MotionClass)
}

func TestAnalyzeVideoRequest_CountsReferenceContentWithoutDoubleCountingImage(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:  "doubao-seedance-2-0-260128",
		Prompt: "参考素材生成视频",
		Size:   "1920x1080",
		Image:  "https://example.com/single.png",
		Images: []string{"https://example.com/single.png"},
		Metadata: map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type": "image_url",
					"role": "reference_image",
					"image_url": map[string]interface{}{
						"url": "https://example.com/style.png",
					},
				},
				map[string]interface{}{
					"type": "video_url",
					"role": "reference_video",
					"video_url": map[string]interface{}{
						"url": "https://example.com/motion.mp4",
					},
				},
			},
		},
	}

	analysis, err := AnalyzeVideoRequest(req)
	require.NoError(t, err)

	assert.Equal(t, 3, analysis.ReferenceCount)
	assert.True(t, analysis.HasVideoReference)
}

func TestBuildVideoPipelinePlan_KeepsSeedance2ForStoryboardRequests(t *testing.T) {
	useVideoPipelineConfig(t, testVideoPipelineConfig)

	req := relaycommon.TaskSubmitReq{
		Model:  "doubao-seedance-2-0-260128",
		Prompt: "一组多镜头故事版",
		Size:   "1920x1080",
		Metadata: map[string]interface{}{
			"storyboard": true,
			"shot_count": 2,
		},
	}
	info := &relaycommon.RelayInfo{UserId: 7, TokenId: 9, OriginModelName: req.Model, RelayMode: relayconstant.RelayModeVideoSubmit}

	plan, err := BuildVideoPipelinePlan(nil, info, req)
	require.NoError(t, err)
	require.NotNil(t, plan)
	assert.Equal(t, "dynamic-default-seedance-2-720p", plan.MatchedGenerationPolicy)
	assert.Equal(t, "doubao-seedance-2-0-260128", plan.Generation.Model)
	assert.Equal(t, "720p", plan.Generation.Resolution)
	require.NotNil(t, plan.Enhance)
	assert.Equal(t, "1080p", plan.Enhance.Resolution)
}

func TestBuildVideoPipelinePlan_DefaultConfigDisablesStrategies(t *testing.T) {
	resetVideoPipelineConfigForTest()
	t.Cleanup(resetVideoPipelineConfigForTest)

	req := relaycommon.TaskSubmitReq{
		Model:  "doubao-seedance-2-0-260128",
		Prompt: "固定镜头的产品展示",
		Size:   "1920x1080",
	}
	info := &relaycommon.RelayInfo{OriginModelName: req.Model, RelayMode: relayconstant.RelayModeVideoSubmit}

	plan, err := BuildVideoPipelinePlan(nil, info, req)
	require.NoError(t, err)
	assert.Nil(t, plan)
}

func TestBuildVideoPipelinePlan_IgnoresRequestForceUnlessAllowed(t *testing.T) {
	useVideoPipelineConfig(t, `
version: 1
defaults:
  enabled: true
strategies:
  - name: seedance2_720p_mediakit_1080p
    enabled: true
    lifecycle:
      match:
        relay_mode: video_submit
        models:
          - doubao-seedance-2-0-260128
        requested_resolutions:
          - 1080p
      rollout:
        traffic_percent: 0
        request_override_metadata: {}
`)

	req := relaycommon.TaskSubmitReq{
		Model:  "doubao-seedance-2-0-260128",
		Prompt: "固定镜头的产品展示",
		Size:   "1920x1080",
		Metadata: map[string]interface{}{
			"fy_enhance_force": true,
		},
	}

	plan, err := BuildVideoPipelinePlan(nil, &relaycommon.RelayInfo{OriginModelName: req.Model, RelayMode: relayconstant.RelayModeVideoSubmit}, req)
	require.NoError(t, err)
	assert.Nil(t, plan)
}

func TestBuildVideoPipelinePlan_UsesRuntimeConfigMatchAndOverrides(t *testing.T) {
	useVideoPipelineConfig(t, `
version: 1
defaults:
  enabled: true
strategies:
  - name: seedance2_720p_mediakit_1080p
    enabled: true
    lifecycle:
      match:
        relay_mode: video_submit
        models:
          - doubao-seedance-2-0-260128
        requested_resolutions:
          - 1080p
      rollout:
        traffic_percent: 0
        request_override_metadata:
          force_keys:
            - fy_enhance_force
          bypass_keys:
            - fy_enhance_bypass
`)

	req := relaycommon.TaskSubmitReq{
		Model:  "doubao-seedance-2-0-260128",
		Prompt: "固定镜头的产品展示",
		Size:   "1920x1080",
		Metadata: map[string]interface{}{
			"fy_enhance_force": true,
		},
	}
	info := &relaycommon.RelayInfo{OriginModelName: req.Model, RelayMode: relayconstant.RelayModeVideoSubmit, RequestId: "req-force"}

	plan, err := BuildVideoPipelinePlan(nil, info, req)
	require.NoError(t, err)
	require.NotNil(t, plan)
	assert.Equal(t, "seedance2_720p_mediakit_1080p", plan.StrategyName)
	assert.Equal(t, "720p", plan.Generation.Resolution)

	req.Metadata["fy_enhance_bypass"] = true
	plan, err = BuildVideoPipelinePlan(nil, info, req)
	require.NoError(t, err)
	assert.Nil(t, plan)
}

func TestLoadVideoPipelineConfigFromFileRejectsInvalidTrafficPercent(t *testing.T) {
	resetVideoPipelineConfigForTest()
	t.Cleanup(resetVideoPipelineConfigForTest)
	path := filepath.Join(t.TempDir(), "video-pipeline.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
version: 1
defaults:
  enabled: true
strategies:
  - name: seedance2_720p_mediakit_1080p
    enabled: true
    lifecycle:
      match:
        relay_mode: video_submit
      rollout:
        traffic_percent: 101
`), 0o600))

	err := LoadVideoPipelineConfigFromFile(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "traffic_percent")
}

func TestApplyVideoPipelineSubmitSnapshotFallsBackToRelayInfoPlan(t *testing.T) {
	plan := &VideoPipelinePlan{
		StrategyName:            VideoPipelineNameSeedanceEnhance,
		StrategyVersion:         "v1",
		UserRequestedModel:      "doubao-seedance-2-0-260128",
		RequestedResolution:     "1080p",
		RequestedRatio:          "16:9",
		MatchedGenerationPolicy: "dynamic-default-seedance-2-720p",
		MatchedEnhancePolicy:    "enhance-720-to-1080-standard",
		Generation: VideoGenerationPlan{
			Provider:   VideoPipelineProviderDoubaoVideo,
			Model:      "doubao-seedance-2-0-260128",
			Resolution: "720p",
		},
		Enhance: &VideoEnhancePlan{
			Provider:   VideoPipelineProviderVolcengineMediaKit,
			Resolution: "1080p",
		},
		Fallback: VideoPipelineFallback{EnhanceFailed: VideoPipelineFallbackReturnGeneration},
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 87},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{
			VideoPipelinePlan: plan,
		},
	}
	task := &model.Task{Quota: 123, PrivateData: model.TaskPrivateData{UpstreamTaskID: "generation-1"}}

	ApplyVideoPipelineSubmitSnapshot(nil, task, info)

	require.NotNil(t, task.PrivateData.SeedanceEnhance)
	assert.Equal(t, VideoPipelineNameSeedanceEnhance, task.PrivateData.SeedanceEnhance.Pipeline)
	assert.Equal(t, VideoPipelineStatusGenerationSubmitted, task.PrivateData.SeedanceEnhance.Status)
	assert.Equal(t, "720p", task.PrivateData.SeedanceEnhance.GenerationResolution)
	assert.Equal(t, "1080p", task.PrivateData.SeedanceEnhance.EnhanceTargetResolution)
	assert.Equal(t, "generation-1", task.PrivateData.SeedanceEnhance.GenerationTaskID)
}

func TestApplyVideoPipelineSubmitSnapshotSeparatesUserBillingFromProviderCost(t *testing.T) {
	plan := &VideoPipelinePlan{
		StrategyName:            VideoPipelineNameSeedanceEnhance,
		StrategyVersion:         "v1",
		UserRequestedModel:      "doubao-seedance-2-0-260128",
		RequestedResolution:     "1080p",
		RequestedRatio:          "16:9",
		MatchedGenerationPolicy: "dynamic-default-seedance-2-720p",
		Generation: VideoGenerationPlan{
			Provider:   VideoPipelineProviderDoubaoVideo,
			Model:      "doubao-seedance-2-0-260128",
			Resolution: "720p",
		},
		Enhance: &VideoEnhancePlan{
			Provider:   VideoPipelineProviderVolcengineMediaKit,
			Resolution: "1080p",
		},
		Fallback: VideoPipelineFallback{EnhanceFailed: VideoPipelineFallbackReturnGeneration},
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 87},
		PriceData: types.PriceData{
			UsePrice:   true,
			ModelPrice: 0.13698630137,
			OtherRatios: map[string]float64{
				"seedance_1080p": 46.0 / 28.0,
			},
			GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1},
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{
			VideoPipelinePlan: plan,
		},
	}
	task := &model.Task{Quota: 112524, PrivateData: model.TaskPrivateData{UpstreamTaskID: "generation-1"}}

	ApplyVideoPipelineSubmitSnapshot(nil, task, info)

	require.NotNil(t, task.PrivateData.SeedanceEnhance)
	assert.Equal(t, 112524, task.PrivateData.SeedanceEnhance.UserBilledQuota)
	assert.Equal(t, 68493, task.PrivateData.SeedanceEnhance.GenerationCostQuota)
}

func TestApplyVideoPipelineSubmitSnapshotRebuildsPlanFromRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	useVideoPipelineConfig(t, testVideoPipelineConfig)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/v1/videos", nil)
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Model:   "doubao-seedance-2-0-260128",
		Prompt:  "雨夜跑车镜头推进",
		Seconds: "5",
		Size:    "1920x1080",
	})
	info := &relaycommon.RelayInfo{OriginModelName: "doubao-seedance-2-0-260128", RelayMode: relayconstant.RelayModeVideoSubmit, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
	task := &model.Task{Quota: 123, PrivateData: model.TaskPrivateData{UpstreamTaskID: "generation-2"}}

	ApplyVideoPipelineSubmitSnapshot(c, task, info)

	require.NotNil(t, task.PrivateData.SeedanceEnhance)
	assert.Equal(t, "720p", task.PrivateData.SeedanceEnhance.GenerationResolution)
	assert.Equal(t, "1080p", task.PrivateData.SeedanceEnhance.EnhanceTargetResolution)
	assert.Equal(t, "generation-2", task.PrivateData.SeedanceEnhance.GenerationTaskID)
}

func TestAnalyzeVideoRequest_ResolutionSizeAllowsMetadataRatio(t *testing.T) {
	analysis, err := AnalyzeVideoRequest(relaycommon.TaskSubmitReq{
		Model:  "doubao-seedance-2-0-260128",
		Prompt: "test",
		Size:   "1080p",
		Metadata: map[string]interface{}{
			"ratio": "9:16",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "1080p", analysis.RequestedResolution)
	assert.Equal(t, "9:16", analysis.Ratio)
}

func TestMediaKitClientSubmitAndQuery(t *testing.T) {
	var submitted bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		switch r.URL.Path {
		case "/api/v1/tools/enhance-video":
			submitted = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"success":true,"task_id":"enhance-1","request_id":"req-1"}`))
		case "/api/v1/tasks/enhance-1":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"success":true,"task_id":"enhance-1","status":"completed","result":{"duration":5,"fps":30,"resolution":"1080p","video_url":"https://cdn.example.com/enhanced.mp4"},"request_id":"req-2"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := &MediaKitClient{BaseURL: srv.URL, APIKey: "test-key", Client: srv.Client()}
	submit, err := client.SubmitEnhanceVideo(t.Context(), MediaKitSubmitRequest{VideoURL: "https://cdn.example.com/gen.mp4", Scene: "aigc", ToolVersion: "standard", Resolution: "1080p"})
	require.NoError(t, err)
	assert.True(t, submitted)
	assert.Equal(t, "enhance-1", submit.TaskID)

	task, err := client.GetEnhanceTask(t.Context(), "enhance-1")
	require.NoError(t, err)
	assert.Equal(t, "completed", task.Status)
	assert.Equal(t, "https://cdn.example.com/enhanced.mp4", task.Result.VideoURL)
}

func TestAdvanceVideoPipelineIfNeeded_SubmitThenComplete(t *testing.T) {
	truncate(t)
	t.Setenv("VOLCENGINE_MEDIAKIT_API_KEY", "test-key")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/tools/enhance-video":
			_, _ = w.Write([]byte(`{"success":true,"task_id":"enhance-1","request_id":"req-1"}`))
		case "/api/v1/tasks/enhance-1":
			_, _ = w.Write([]byte(`{"success":true,"task_id":"enhance-1","status":"completed","result":{"duration":5,"fps":30,"resolution":"1080p","video_url":"https://cdn.example.com/enhanced.mp4"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	t.Setenv("VOLCENGINE_MEDIAKIT_BASE_URL", srv.URL)

	task := &model.Task{
		TaskID:    "task_pipeline_e2e",
		UserId:    1,
		ChannelId: 1,
		Status:    model.TaskStatusInProgress,
		Progress:  "50%",
		Data:      []byte(`{}`),
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "generation-1",
			SeedanceEnhance: &model.SeedanceEnhancePipeline{
				Pipeline:                VideoPipelineNameSeedanceEnhance,
				Status:                  VideoPipelineStatusGenerationSubmitted,
				RequestedResolution:     "1080p",
				GenerationResolution:    "720p",
				EnhanceTargetResolution: "1080p",
				EnhanceProvider:         VideoPipelineProviderVolcengineMediaKit,
			},
		},
	}
	require.NoError(t, model.DB.Create(task).Error)

	handled, err := AdvanceVideoPipelineIfNeeded(t.Context(), task, &relaycommon.TaskInfo{
		Status: model.TaskStatusSuccess,
		Url:    "https://cdn.example.com/generated.mp4",
	}, []byte(`{"status":"succeeded"}`))
	require.NoError(t, err)
	assert.True(t, handled)

	var reloaded model.Task
	require.NoError(t, model.DB.Where("task_id = ?", task.TaskID).First(&reloaded).Error)
	require.NotNil(t, reloaded.PrivateData.SeedanceEnhance)
	assert.Equal(t, "enhance-1", reloaded.PrivateData.SeedanceEnhance.EnhanceTaskID)
	assert.EqualValues(t, model.TaskStatusInProgress, reloaded.Status)

	handled, err = AdvanceVideoPipelineIfNeeded(t.Context(), &reloaded, &relaycommon.TaskInfo{
		Status: model.TaskStatusInProgress,
	}, nil)
	require.NoError(t, err)
	assert.True(t, handled)

	var done model.Task
	require.NoError(t, model.DB.Where("task_id = ?", task.TaskID).First(&done).Error)
	assert.EqualValues(t, model.TaskStatusSuccess, done.Status)
	assert.Equal(t, "https://cdn.example.com/enhanced.mp4", done.PrivateData.ResultURL)
	assert.Equal(t, "enhance_succeeded", done.PrivateData.SeedanceEnhance.Status)
	assert.Equal(t, int(5*0.025*common.QuotaPerUnit), done.PrivateData.SeedanceEnhance.EnhanceCostQuota)
}

func TestHydrateVideoPipelinePrivateDataMergesStaleTaskSnapshot(t *testing.T) {
	truncate(t)

	task := &model.Task{
		TaskID:    "task_pipeline_hydrate",
		UserId:    1,
		ChannelId: 1,
		Status:    model.TaskStatusInProgress,
		Progress:  "50%",
		Data:      []byte(`{}`),
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "generation-1",
			SeedanceEnhance: &model.SeedanceEnhancePipeline{
				Pipeline:                VideoPipelineNameSeedanceEnhance,
				Status:                  VideoPipelineStatusGenerationSubmitted,
				RequestedResolution:     "1080p",
				GenerationResolution:    "720p",
				EnhanceTargetResolution: "1080p",
				EnhanceProvider:         VideoPipelineProviderVolcengineMediaKit,
			},
		},
	}
	require.NoError(t, model.DB.Create(task).Error)

	stale := *task
	stale.PrivateData.SeedanceEnhance = nil

	hydrateVideoPipelinePrivateData(&stale)

	require.NotNil(t, stale.PrivateData.SeedanceEnhance)
	assert.Equal(t, VideoPipelineNameSeedanceEnhance, stale.PrivateData.SeedanceEnhance.Pipeline)
	assert.Equal(t, "generation-1", stale.PrivateData.UpstreamTaskID)
}
