package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskPrivateDataSeedanceEnhanceRoundTrip(t *testing.T) {
	in := TaskPrivateData{
		UpstreamTaskID: "generation-1",
		SeedanceEnhance: &SeedanceEnhancePipeline{
			Pipeline:                 "seedance_720_to_1080_enhance",
			Status:                   "generation_submitted",
			RequestedResolution:      "1080p",
			GenerationResolution:     "720p",
			EnhanceTargetResolution:  "1080p",
			MatchedGenerationPolicy:  "static-prompt-low-cost-720p",
			CostPriceVersion:         "2026-06-15",
			RMBPerUSD:                7.3,
			GenerationUsageSource:    "provider_usage_estimated",
			GenerationBillableTokens: 250000,
			GenerationCostRMB:        12.75,
			GenerationCostQuota:      873288,
			EnhanceBillingVersion:    "2026-06-15",
			EnhanceToolVersion:       "standard",
			EnhanceScene:             "aigc",
			EnhanceOutputResolution:  "1080p",
			EnhanceOutputFPS:         30,
			EnhanceProviderTaskType:  "enhance-video",
			EnhanceCostRMB:           0.125,
			EnhanceCostQuota:         8562,
			PipelineProviderCost:     881850,
			UserBilledQuota:          1000000,
			GrossProfitQuota:         118150,
			GrossMargin:              0.11815,
			GenerationCostDetail: &MediaCostEstimateDetail{
				Component:      "generation",
				FormulaKey:     "seedance_token_price",
				FormulaVersion: "2026-06-15",
				FormulaText:    "cost_rmb = unit_price_rmb_per_million_tokens * billable_tokens / 1000000; cost_quota = round(cost_rmb / rmb_per_usd * quota_per_unit)",
				Variables: map[string]interface{}{
					"billable_tokens": float64(250000),
				},
				Coefficients: map[string]interface{}{
					"unit_price_rmb_per_million_tokens": float64(51),
					"rmb_per_usd":                       float64(7.3),
					"quota_per_unit":                    float64(500000),
				},
				CostRMB:      12.75,
				CostQuota:    873288,
				RMBPerUSD:    7.3,
				QuotaPerUnit: float64(500000),
			},
			EnhanceCostDetail: &MediaCostEstimateDetail{
				Component:      "enhance",
				FormulaKey:     "mediakit_enhance_video",
				FormulaVersion: "2026-06-15",
				FormulaText:    "cost_rmb = base_price_rmb_per_minute * billing_coefficient * duration_minutes; cost_quota = round(cost_rmb / rmb_per_usd * quota_per_unit)",
				Variables: map[string]interface{}{
					"duration_seconds": float64(5),
				},
				Coefficients: map[string]interface{}{
					"base_price_rmb_per_minute": float64(0.75),
					"billing_coefficient":       float64(2),
					"rmb_per_usd":               float64(7.3),
					"quota_per_unit":            float64(500000),
				},
				CostRMB:      0.125,
				CostQuota:    8562,
				RMBPerUSD:    7.3,
				QuotaPerUnit: float64(500000),
			},
			Analysis: VideoRequestAnalysis{
				MotionClass:        "static_or_low_motion",
				ReferenceCount:     1,
				AnalysisConfidence: 0.8,
			},
		},
		SeedanceAssetPrepare: &SeedanceAssetPrepareData{
			References: []SeedanceAssetReference{
				{
					AssetID:       "asset-001",
					URI:           "asset://asset-001",
					CleanupStatus: "deleted",
					CleanupAt:     123,
				},
			},
		},
	}
	b, err := common.Marshal(in)
	require.NoError(t, err)
	var out TaskPrivateData
	require.NoError(t, common.Unmarshal(b, &out))
	require.NotNil(t, out.SeedanceEnhance)
	assert.Equal(t, in.SeedanceEnhance.MatchedGenerationPolicy, out.SeedanceEnhance.MatchedGenerationPolicy)
	assert.Equal(t, in.SeedanceEnhance.Analysis.MotionClass, out.SeedanceEnhance.Analysis.MotionClass)
	assert.Equal(t, in.SeedanceEnhance.GenerationUsageSource, out.SeedanceEnhance.GenerationUsageSource)
	assert.Equal(t, in.SeedanceEnhance.GenerationBillableTokens, out.SeedanceEnhance.GenerationBillableTokens)
	assert.Equal(t, in.SeedanceEnhance.GenerationCostQuota, out.SeedanceEnhance.GenerationCostQuota)
	assert.Equal(t, in.SeedanceEnhance.EnhanceToolVersion, out.SeedanceEnhance.EnhanceToolVersion)
	assert.Equal(t, in.SeedanceEnhance.EnhanceOutputResolution, out.SeedanceEnhance.EnhanceOutputResolution)
	assert.Equal(t, in.SeedanceEnhance.EnhanceCostQuota, out.SeedanceEnhance.EnhanceCostQuota)
	assert.Equal(t, in.SeedanceEnhance.EnhanceProviderTaskType, out.SeedanceEnhance.EnhanceProviderTaskType)
	assert.Equal(t, in.SeedanceEnhance.PipelineProviderCost, out.SeedanceEnhance.PipelineProviderCost)
	assert.Equal(t, in.SeedanceEnhance.GrossProfitQuota, out.SeedanceEnhance.GrossProfitQuota)
	assert.Equal(t, in.SeedanceEnhance.GrossMargin, out.SeedanceEnhance.GrossMargin)
	require.NotNil(t, out.SeedanceEnhance.GenerationCostDetail)
	require.NotNil(t, out.SeedanceEnhance.EnhanceCostDetail)
	assert.Equal(t, "seedance_token_price", out.SeedanceEnhance.GenerationCostDetail.FormulaKey)
	assert.Equal(t, "mediakit_enhance_video", out.SeedanceEnhance.EnhanceCostDetail.FormulaKey)
	assert.Equal(t, "cost_rmb = unit_price_rmb_per_million_tokens * billable_tokens / 1000000; cost_quota = round(cost_rmb / rmb_per_usd * quota_per_unit)", out.SeedanceEnhance.GenerationCostDetail.FormulaText)
	assert.Equal(t, "cost_rmb = base_price_rmb_per_minute * billing_coefficient * duration_minutes; cost_quota = round(cost_rmb / rmb_per_usd * quota_per_unit)", out.SeedanceEnhance.EnhanceCostDetail.FormulaText)
	assert.Equal(t, float64(250000), out.SeedanceEnhance.GenerationCostDetail.Variables["billable_tokens"])
	assert.Equal(t, float64(2), out.SeedanceEnhance.EnhanceCostDetail.Coefficients["billing_coefficient"])
	assert.Equal(t, float64(500000), out.SeedanceEnhance.GenerationCostDetail.Coefficients["quota_per_unit"])
	assert.Equal(t, float64(500000), out.SeedanceEnhance.GenerationCostDetail.QuotaPerUnit)
	require.NotNil(t, out.SeedanceAssetPrepare)
	require.Len(t, out.SeedanceAssetPrepare.References, 1)
	assert.Equal(t, "deleted", out.SeedanceAssetPrepare.References[0].CleanupStatus)
}
