package service

import (
	"hash/fnv"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"

	"github.com/gin-gonic/gin"
)

const (
	videoPipelineContextKey = "fy_video_pipeline_plan"

	VideoPipelineNameSeedanceEnhance       = "seedance2_720p_mediakit_1080p"
	VideoPipelineStatusGenerationSubmitted = "generation_submitted"

	VideoPipelineFallbackReturnGeneration = "return_generation_result"

	VideoPipelineProviderDoubaoVideo        = "doubao-video"
	VideoPipelineProviderVolcengineMediaKit = "volcengine-mediakit"
)

type VideoPipelinePlan struct {
	StrategyName            string
	StrategyVersion         string
	Analysis                model.VideoRequestAnalysis
	UserRequestedModel      string
	RequestedResolution     string
	RequestedRatio          string
	Generation              VideoGenerationPlan
	Enhance                 *VideoEnhancePlan
	Fallback                VideoPipelineFallback
	MatchedGenerationPolicy string
	MatchedEnhancePolicy    string
	MatchReason             string
	MappedFields            []string
	DroppedFields           []string
}

type VideoGenerationPlan struct {
	Provider   string
	Model      string
	Resolution string
	Ratio      string
	ChannelID  int
	Mapper     string
}

type VideoEnhancePlan struct {
	Provider    string
	Scene       string
	ToolVersion string
	Resolution  string
	FPS         string
}

type VideoPipelineFallback struct {
	EnhanceFailed    string
	GenerationFailed string
}

func StoreVideoPipelinePlan(c *gin.Context, plan *VideoPipelinePlan) {
	if c == nil || plan == nil {
		return
	}
	c.Set(videoPipelineContextKey, plan)
}

func AttachVideoPipelinePlan(info *relaycommon.RelayInfo, plan *VideoPipelinePlan) {
	if info == nil || info.TaskRelayInfo == nil || plan == nil {
		return
	}
	info.TaskRelayInfo.VideoPipelinePlan = plan
}

func GetVideoPipelinePlan(c *gin.Context) (*VideoPipelinePlan, bool) {
	if c == nil {
		return nil, false
	}
	v, ok := c.Get(videoPipelineContextKey)
	if !ok {
		return nil, false
	}
	plan, ok := v.(*VideoPipelinePlan)
	return plan, ok && plan != nil
}

func BuildVideoPipelinePlan(c *gin.Context, info *relaycommon.RelayInfo, req relaycommon.TaskSubmitReq) (*VideoPipelinePlan, error) {
	analysis, err := AnalyzeVideoRequest(req)
	if err != nil {
		return nil, err
	}
	strategy, ok := matchVideoPipelineStrategy(info, req, analysis)
	if !ok {
		return nil, nil
	}
	if metadataAnyBool(req.Metadata, strategy.Lifecycle.Rollout.RequestOverrideMetadata.BypassKeys) {
		return nil, nil
	}
	force := metadataAnyBool(req.Metadata, strategy.Lifecycle.Rollout.RequestOverrideMetadata.ForceKeys)
	if !force && !rolloutMatched(info, analysis, strategy.Lifecycle.Rollout.TrafficPercent) {
		return nil, nil
	}

	gen := selectGenerationPolicy(info, req, analysis)
	enhance := selectEnhancePolicy(analysis, gen.Generation)
	plan := &VideoPipelinePlan{
		StrategyName:        strategy.Name,
		StrategyVersion:     "v1",
		Analysis:            analysis,
		UserRequestedModel:  req.Model,
		RequestedResolution: analysis.RequestedResolution,
		RequestedRatio:      analysis.Ratio,
		Generation:          gen.Generation,
		Enhance:             enhance,
		Fallback: VideoPipelineFallback{
			EnhanceFailed:    VideoPipelineFallbackReturnGeneration,
			GenerationFailed: "fail_task",
		},
		MatchedGenerationPolicy: gen.Name,
		MatchReason:             gen.Reason,
		MappedFields:            mappedFieldsForMapper(gen.Generation.Mapper),
		DroppedFields:           droppedFieldsForMapper(gen.Generation.Mapper, req.Metadata),
	}
	if enhance != nil {
		plan.MatchedEnhancePolicy = "enhance-720-to-1080-standard"
	} else {
		plan.MatchedEnhancePolicy = "no-enhance-when-target-met"
	}
	return plan, nil
}

func ApplyVideoPipelineSubmitSnapshot(c *gin.Context, task *model.Task, info *relaycommon.RelayInfo) {
	plan, ok := GetVideoPipelinePlan(c)
	if !ok && info != nil && info.TaskRelayInfo != nil {
		plan, ok = info.TaskRelayInfo.VideoPipelinePlan.(*VideoPipelinePlan)
	}
	if !ok && c != nil {
		req, err := relaycommon.GetTaskRequest(c)
		if err == nil {
			if rebuilt, rebuildErr := BuildVideoPipelinePlan(c, info, req); rebuildErr == nil && rebuilt != nil {
				plan = rebuilt
				ok = true
			}
		}
	}
	if !ok || task == nil || plan == nil {
		return
	}
	p := &model.SeedanceEnhancePipeline{
		Pipeline:                plan.StrategyName,
		Status:                  VideoPipelineStatusGenerationSubmitted,
		RequestedResolution:     plan.RequestedResolution,
		GenerationResolution:    plan.Generation.Resolution,
		Ratio:                   plan.RequestedRatio,
		Strategy:                plan.StrategyName,
		StrategyVersion:         plan.StrategyVersion,
		Fallback:                plan.Fallback.EnhanceFailed,
		Analysis:                plan.Analysis,
		MatchedGenerationPolicy: plan.MatchedGenerationPolicy,
		MatchedEnhancePolicy:    plan.MatchedEnhancePolicy,
		MatchReason:             plan.MatchReason,
		MappedFields:            plan.MappedFields,
		DroppedFields:           plan.DroppedFields,
		UserRequestedModel:      plan.UserRequestedModel,
		GenerationProvider:      plan.Generation.Provider,
		GenerationModel:         plan.Generation.Model,
		GenerationTaskID:        task.PrivateData.UpstreamTaskID,
		EnhanceProvider:         "",
		GenerationCostQuota:     estimateVideoPipelineGenerationCostQuota(task, info),
		UserBilledQuota:         task.Quota,
	}
	if plan.Enhance != nil {
		p.EnhanceProvider = plan.Enhance.Provider
		p.EnhanceTargetResolution = plan.Enhance.Resolution
	}
	if info != nil && info.ChannelMeta != nil {
		p.GenerationChannelID = info.ChannelId
	}
	task.PrivateData.SeedanceEnhance = p
}

func estimateVideoPipelineGenerationCostQuota(task *model.Task, info *relaycommon.RelayInfo) int {
	if info != nil && info.PriceData.UsePrice && info.PriceData.ModelPrice > 0 {
		cost := info.PriceData.ModelPrice * common.QuotaPerUnit * info.PriceData.GroupRatioInfo.GroupRatio
		for key, ratio := range info.PriceData.OtherRatios {
			if key == "seedance_1080p" {
				continue
			}
			if ratio > 0 {
				cost *= ratio
			}
		}
		if cost > 0 {
			return int(cost)
		}
	}
	if task == nil || task.Quota <= 0 || info == nil {
		return 0
	}
	if ratio := info.PriceData.OtherRatios["seedance_1080p"]; ratio > 0 {
		return int(float64(task.Quota) / ratio)
	}
	return task.Quota
}

func matchVideoPipelineStrategy(info *relaycommon.RelayInfo, req relaycommon.TaskSubmitReq, analysis model.VideoRequestAnalysis) (VideoPipelineStrategyConfig, bool) {
	cfg := GetVideoPipelineConfig()
	if cfg == nil || !cfg.Defaults.Enabled {
		return VideoPipelineStrategyConfig{}, false
	}
	for _, strategy := range cfg.Strategies {
		if !strategy.Enabled {
			continue
		}
		if !isSeedancePipelineStrategyName(strategy.Name) {
			continue
		}
		if !relayModeMatchesStrategy(info, strategy.Lifecycle.Match.RelayMode) {
			continue
		}
		if !isSeedancePipelineCandidate(info, req, analysis, strategy) {
			continue
		}
		return strategy, true
	}
	return VideoPipelineStrategyConfig{}, false
}

func isSeedancePipelineCandidate(info *relaycommon.RelayInfo, req relaycommon.TaskSubmitReq, analysis model.VideoRequestAnalysis, strategy VideoPipelineStrategyConfig) bool {
	modelName := req.Model
	if modelName == "" && info != nil {
		modelName = info.OriginModelName
	}
	if !containsString(strategy.Lifecycle.Match.Models, modelName) {
		return false
	}
	return containsString(strategy.Lifecycle.Match.RequestedResolutions, analysis.RequestedResolution)
}

func isSeedancePipelineStrategyName(name string) bool {
	return name == VideoPipelineNameSeedanceEnhance
}

func relayModeMatchesStrategy(info *relaycommon.RelayInfo, relayModeName string) bool {
	expected, ok := videoPipelineRelayModeFromName(relayModeName)
	if !ok || expected == relayconstant.RelayModeUnknown {
		return ok
	}
	if info == nil {
		return false
	}
	return info.RelayMode == expected
}

func rolloutMatched(info *relaycommon.RelayInfo, analysis model.VideoRequestAnalysis, percent int) bool {
	if percent <= 0 {
		return false
	}
	if percent >= 100 {
		return true
	}
	key := analysis.RequestedResolution
	if info != nil {
		key = info.RequestId
		if key == "" {
			key = strconv.Itoa(info.UserId) + ":" + strconv.Itoa(info.TokenId) + ":" + info.OriginModelName + ":" + analysis.RequestedResolution
		}
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return int(h.Sum32()%10000) < percent*100
}

func metadataAnyBool(metadata map[string]interface{}, keys []string) bool {
	for _, key := range keys {
		if metadataBool(metadata, key) {
			return true
		}
	}
	return false
}

func containsString(items []string, want string) bool {
	want = strings.TrimSpace(want)
	if want == "" {
		return false
	}
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item), want) {
			return true
		}
	}
	return false
}
