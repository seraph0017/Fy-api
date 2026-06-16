package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatUserLogsRedactsInternalCostEstimateFields(t *testing.T) {
	logs := []*Log{
		{
			ChannelName: "internal-channel",
			Other: common.MapToJsonStr(map[string]interface{}{
				"pipeline":                          "seedance2_720p_mediakit_1080p",
				"safe_key":                          "keep",
				"provider_cost_estimate_quota":      123,
				"generation_cost_estimate_quota":    100,
				"enhance_cost_estimate_quota":       23,
				"user_billed_quota":                 456,
				"gross_profit_estimate_quota":       333,
				"gross_margin_estimate":             0.73,
				"cost_price_version":                "2026-06-15",
				"rmb_per_usd":                       7.3,
				"seedance_usage_source":             "provider_usage_estimated",
				"seedance_billable_tokens":          250000,
				"seedance_estimated_tokens":         108000,
				"generation_cost_estimate_rmb":      12.75,
				"enhance_cost_estimate_rmb":         0.125,
				"provider_cost_estimate_rmb":        12.875,
				"enhance_billing_version":           "2026-06-15",
				"enhance_base_price_rmb_per_minute": 0.75,
				"enhance_billing_coefficient":       2,
				"actual_duration_seconds":           5,
				"provider_cost_estimate_details": []interface{}{
					map[string]interface{}{
						"formula_key":  "seedance_token_price",
						"formula_text": "cost_rmb = unit_price_rmb_per_million_tokens * billable_tokens / 1000000",
						"variables": map[string]interface{}{
							"billable_tokens": 250000,
						},
						"coefficients": map[string]interface{}{
							"quota_per_unit": 500000,
						},
						"cost_quota": 873288,
					},
				},
			}),
		},
	}

	formatUserLogs(logs, 0)

	assert.Empty(t, logs[0].ChannelName)
	var other map[string]interface{}
	require.NoError(t, common.Unmarshal([]byte(logs[0].Other), &other))
	assert.Equal(t, "seedance2_720p_mediakit_1080p", other["pipeline"])
	assert.Equal(t, "keep", other["safe_key"])

	for _, key := range []string{
		"provider_cost_estimate_quota",
		"generation_cost_estimate_quota",
		"enhance_cost_estimate_quota",
		"user_billed_quota",
		"gross_profit_estimate_quota",
		"gross_margin_estimate",
		"cost_price_version",
		"rmb_per_usd",
		"seedance_usage_source",
		"seedance_billable_tokens",
		"seedance_estimated_tokens",
		"generation_cost_estimate_rmb",
		"enhance_cost_estimate_rmb",
		"provider_cost_estimate_rmb",
		"enhance_billing_version",
		"enhance_base_price_rmb_per_minute",
		"enhance_billing_coefficient",
		"actual_duration_seconds",
		"provider_cost_estimate_details",
	} {
		assert.NotContains(t, other, key)
	}
}
