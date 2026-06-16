package service

import (
	"math"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
)

func TestResolveSeedanceGenerationCostUsesProviderCompletionTokens(t *testing.T) {
	got := ResolveSeedanceGenerationCost("doubao-seedance-2-0-260128", MediaProviderCostInput{
		InputClass:     "image",
		Resolution:     "720p",
		BillableTokens: 1_000_000,
		UsageSource:    "provider_usage_estimated",
	})

	assert.Equal(t, mediaProviderCostPriceVersion, got.Version)
	assert.Equal(t, int64(1_000_000), got.BillableTokens)
	assert.Equal(t, 46.0, got.UnitPriceRMB)
	assert.Equal(t, 46.0, got.CostRMB)
	assert.Equal(t, RMBToQuota(46.0), got.CostQuota)
	assert.Equal(t, common.QuotaPerUnit, got.QuotaPerUnit)
	assert.Equal(t, "provider_usage_estimated", got.UsageSource)
	assert.Equal(t, "seedance_token_price", got.FormulaKey)
	assert.Equal(t, mediaProviderCostPriceVersion, got.FormulaVersion)
	assert.True(t, strings.Contains(got.FormulaText, "cost_quota = round(cost_rmb / rmb_per_usd * quota_per_unit)"))
	assert.Equal(t, int64(1_000_000), got.Variables["billable_tokens"])
	assert.Equal(t, 46.0, got.Coefficients["unit_price_rmb_per_million_tokens"])
	assert.Equal(t, mediaProviderRMBPerUSD, got.Coefficients["rmb_per_usd"])
	assert.Equal(t, got.QuotaPerUnit, got.Coefficients["quota_per_unit"])
	assert.Equal(t, "provider_usage_estimated", got.Confidence)
}

func TestResolveSeedanceGenerationCostSnapshotsQuotaPerUnit(t *testing.T) {
	oldQuotaPerUnit := common.QuotaPerUnit
	defer func() {
		common.QuotaPerUnit = oldQuotaPerUnit
	}()
	common.QuotaPerUnit = 600_000

	got := ResolveSeedanceGenerationCost("doubao-seedance-2-0-260128", MediaProviderCostInput{
		InputClass:     "image",
		Resolution:     "720p",
		BillableTokens: 1_000_000,
	})

	assert.Equal(t, 600_000.0, got.QuotaPerUnit)
	assert.Equal(t, 600_000.0, got.Coefficients["quota_per_unit"])
	assert.Equal(t, int(math.Round(46.0/mediaProviderRMBPerUSD*600_000.0)), got.CostQuota)
}

func TestResolveSeedanceGenerationCostUses1080pTier(t *testing.T) {
	got := ResolveSeedanceGenerationCost("doubao-seedance-2-0-260128", MediaProviderCostInput{
		InputClass:     "video",
		Resolution:     "1080p",
		BillableTokens: 1_000_000,
	})

	assert.Equal(t, 31.0, got.UnitPriceRMB)
	assert.Equal(t, 31.0, got.CostRMB)
}

func TestResolveSeedanceGenerationCostFastVideoInput(t *testing.T) {
	got := ResolveSeedanceGenerationCost("doubao-seedance-2-0-fast-260128", MediaProviderCostInput{
		InputClass:     "video",
		BillableTokens: 500_000,
	})

	assert.Equal(t, 22.0, got.UnitPriceRMB)
	assert.Equal(t, 11.0, got.CostRMB)
}

func TestResolveSeedanceGenerationCostMiniTiers(t *testing.T) {
	noVideo := ResolveSeedanceGenerationCost("doubao-seedance-2-0-mini-260128", MediaProviderCostInput{
		InputClass:     "image",
		BillableTokens: 1_000_000,
	})
	video := ResolveSeedanceGenerationCost("doubao-seedance-2.0-mini-260128", MediaProviderCostInput{
		InputClass:     "video",
		BillableTokens: 1_000_000,
	})

	assert.Equal(t, 23.0, noVideo.UnitPriceRMB)
	assert.Equal(t, 23.0, noVideo.CostRMB)
	assert.Equal(t, 14.0, video.UnitPriceRMB)
	assert.Equal(t, 14.0, video.CostRMB)
}

func TestResolveSeedanceGenerationCostEstimatesTokens(t *testing.T) {
	got := ResolveSeedanceGenerationCost("doubao-seedance-2-0-260128", MediaProviderCostInput{
		InputClass:         "image",
		Resolution:         "720p",
		OutputVideoSeconds: 5,
		OutputWidth:        1280,
		OutputHeight:       720,
		OutputFPS:          24,
	})

	assert.Equal(t, "param_estimated", got.UsageSource)
	assert.Equal(t, int64(108000), got.EstimatedTokens)
	assert.Equal(t, int64(108000), got.BillableTokens)
	assert.InDelta(t, 4.968, got.CostRMB, 0.000001)
	assert.Equal(t, int64(108000), got.Variables["estimated_tokens"])
	assert.Equal(t, int64(108000), got.Variables["billable_tokens"])
	assert.Equal(t, "estimated", got.Confidence)
}

func TestResolveMediaKitEnhanceCostStandard1080p(t *testing.T) {
	got := ResolveMediaKitEnhanceCost("standard", "1080p", 30, 90)

	assert.Equal(t, 0.75, got.EnhanceBasePriceRMBPerMin)
	assert.Equal(t, 2.0, got.BillingCoefficient)
	assert.Equal(t, 2.25, got.CostRMB)
	assert.Equal(t, RMBToQuota(2.25), got.CostQuota)
	assert.Equal(t, common.QuotaPerUnit, got.QuotaPerUnit)
	assert.Equal(t, "mediakit_enhance_video", got.FormulaKey)
	assert.True(t, strings.Contains(got.FormulaText, "cost_quota = round(cost_rmb / rmb_per_usd * quota_per_unit)"))
	assert.Equal(t, 90.0, got.Variables["duration_seconds"])
	assert.Equal(t, 1.5, got.Variables["duration_minutes"])
	assert.Equal(t, 0.75, got.Coefficients["base_price_rmb_per_minute"])
	assert.Equal(t, 2.0, got.Coefficients["billing_coefficient"])
	assert.Equal(t, got.QuotaPerUnit, got.Coefficients["quota_per_unit"])
}

func TestResolveMediaKitEnhanceCostProfessionalHighFPS(t *testing.T) {
	got := ResolveMediaKitEnhanceCost("professional", "1080p", 60, 60)

	assert.Equal(t, 0.75, got.EnhanceBasePriceRMBPerMin)
	assert.Equal(t, 40.0, got.BillingCoefficient)
	assert.Equal(t, 30.0, got.CostRMB)
	assert.Equal(t, RMBToQuota(30.0), got.CostQuota)
}
