package service

import (
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

type generationPolicyMatch struct {
	Name       string
	Reason     string
	Generation VideoGenerationPlan
}

func selectGenerationPolicy(info *relaycommon.RelayInfo, req relaycommon.TaskSubmitReq, analysis model.VideoRequestAnalysis) generationPolicyMatch {
	modelName := req.Model
	if modelName == "" && info != nil {
		modelName = info.OriginModelName
	}
	if modelName == "" {
		modelName = "doubao-seedance-2-0-260128"
	}
	base := VideoGenerationPlan{
		Provider:   VideoPipelineProviderDoubaoVideo,
		Model:      modelName,
		Resolution: "720p",
		Ratio:      analysis.Ratio,
		Mapper:     "seedance_basic_mapper",
	}
	if info != nil && info.ChannelMeta != nil && info.ChannelId > 0 {
		base.ChannelID = info.ChannelId
	}
	return generationPolicyMatch{Name: "dynamic-default-seedance-2-720p", Reason: "default_1080p_pipeline", Generation: base}
}

func selectEnhancePolicy(analysis model.VideoRequestAnalysis, generation VideoGenerationPlan) *VideoEnhancePlan {
	if analysis.RequestedResolution == generation.Resolution {
		return nil
	}
	if analysis.RequestedResolution == "1080p" && generation.Resolution == "720p" {
		return &VideoEnhancePlan{
			Provider:    VideoPipelineProviderVolcengineMediaKit,
			Scene:       "aigc",
			ToolVersion: "standard",
			Resolution:  "1080p",
			FPS:         "keep_source",
		}
	}
	return nil
}
