package service

import (
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

const (
	mediaProviderCostPriceVersion = "2026-06-15"
	mediaProviderRMBPerUSD        = ratio_setting.USD2RMB

	seedance2BaseInputVideoPriceRMBPerMillionTokens   = 28.0
	seedance2BaseInputNoVideoPriceRMBPerMillionTokens = 46.0
	seedance2HDInputVideoPriceRMBPerMillionTokens     = 31.0
	seedance2HDInputNoVideoPriceRMBPerMillionTokens   = 51.0
	seedance2FastInputVideoPriceRMBPerMillionTokens   = 22.0
	seedance2FastInputNoVideoPriceRMBPerMillionTokens = 37.0
	seedance2MiniInputVideoPriceRMBPerMillionTokens   = 14.0
	seedance2MiniInputNoVideoPriceRMBPerMillionTokens = 23.0

	mediaKitEnhanceBasePriceRMBPerMinute = 0.75
)

type MediaProviderCostInput struct {
	Component          string
	Provider           string
	Model              string
	ToolVersion        string
	Resolution         string
	FPS                float64
	InputClass         string
	TaskClass          string
	InputVideoSeconds  float64
	OutputVideoSeconds float64
	OutputWidth        float64
	OutputHeight       float64
	OutputFPS          float64
	BillableTokens     int64
	UsageSource        string
}

type MediaProviderCostResult struct {
	Version                   string
	CostRMB                   float64
	CostQuota                 int
	UnitPriceRMB              float64
	Unit                      string
	RMBPerUSD                 float64
	QuotaPerUnit              float64
	UsageSource               string
	EstimatedTokens           int64
	BillableTokens            int64
	MinimumTokensApplied      bool
	BillingCoefficient        float64
	EnhanceBasePriceRMBPerMin float64
	FormulaKey                string
	FormulaVersion            string
	FormulaText               string
	Variables                 map[string]interface{}
	Coefficients              map[string]interface{}
	Confidence                string
}

func RMBToQuota(rmb float64) int {
	return rmbToQuotaWithUnit(rmb, common.QuotaPerUnit)
}

func rmbToQuotaWithUnit(rmb float64, quotaPerUnit float64) int {
	if rmb <= 0 {
		return 0
	}
	return int(math.Round(rmb / mediaProviderRMBPerUSD * quotaPerUnit))
}

func ResolveSeedanceGenerationCost(modelName string, input MediaProviderCostInput) MediaProviderCostResult {
	normalized := normalizeSeedanceModelName(modelName)
	quotaPerUnit := common.QuotaPerUnit
	unitPrice := seedance2BaseInputVideoPriceRMBPerMillionTokens
	if normalized == "doubao-seedance-2.0-fast" {
		if strings.Contains(strings.ToLower(input.InputClass), "video") {
			unitPrice = seedance2FastInputVideoPriceRMBPerMillionTokens
		} else {
			unitPrice = seedance2FastInputNoVideoPriceRMBPerMillionTokens
		}
	} else if normalized == "doubao-seedance-2.0-mini" {
		if strings.Contains(strings.ToLower(input.InputClass), "video") {
			unitPrice = seedance2MiniInputVideoPriceRMBPerMillionTokens
		} else {
			unitPrice = seedance2MiniInputNoVideoPriceRMBPerMillionTokens
		}
	} else {
		if strings.EqualFold(strings.TrimSpace(input.Resolution), "1080p") {
			if strings.Contains(strings.ToLower(input.InputClass), "video") {
				unitPrice = seedance2HDInputVideoPriceRMBPerMillionTokens
			} else {
				unitPrice = seedance2HDInputNoVideoPriceRMBPerMillionTokens
			}
		} else if strings.Contains(strings.ToLower(input.InputClass), "video") {
			unitPrice = seedance2BaseInputVideoPriceRMBPerMillionTokens
		} else {
			unitPrice = seedance2BaseInputNoVideoPriceRMBPerMillionTokens
		}
	}

	billableTokens := input.BillableTokens
	if billableTokens <= 0 {
		estimated := estimateSeedanceTokens(input.InputVideoSeconds, input.OutputVideoSeconds, input.OutputWidth, input.OutputHeight, input.OutputFPS)
		billableTokens = estimated
		costRMB := unitPrice * float64(billableTokens) / 1_000_000
		return MediaProviderCostResult{
			Version:              mediaProviderCostPriceVersion,
			CostRMB:              costRMB,
			CostQuota:            rmbToQuotaWithUnit(costRMB, quotaPerUnit),
			UnitPriceRMB:         unitPrice,
			Unit:                 "rmb_per_million_tokens",
			RMBPerUSD:            mediaProviderRMBPerUSD,
			QuotaPerUnit:         quotaPerUnit,
			UsageSource:          "param_estimated",
			EstimatedTokens:      estimated,
			BillableTokens:       billableTokens,
			MinimumTokensApplied: false,
			FormulaKey:           "seedance_token_price",
			FormulaVersion:       mediaProviderCostPriceVersion,
			FormulaText:          "cost_rmb = unit_price_rmb_per_million_tokens * billable_tokens / 1000000; cost_quota = round(cost_rmb / rmb_per_usd * quota_per_unit)",
			Variables: map[string]interface{}{
				"input_video_seconds":  input.InputVideoSeconds,
				"output_video_seconds": input.OutputVideoSeconds,
				"output_width":         input.OutputWidth,
				"output_height":        input.OutputHeight,
				"output_fps":           input.OutputFPS,
				"estimated_tokens":     estimated,
				"billable_tokens":      billableTokens,
			},
			Coefficients: map[string]interface{}{
				"unit_price_rmb_per_million_tokens": unitPrice,
				"price_tier":                        seedancePriceTier(normalized, input.Resolution, input.InputClass),
				"rmb_per_usd":                       mediaProviderRMBPerUSD,
				"quota_per_unit":                    quotaPerUnit,
			},
			Confidence: "estimated",
		}
	}

	costRMB := unitPrice * float64(billableTokens) / 1_000_000
	return MediaProviderCostResult{
		Version:        mediaProviderCostPriceVersion,
		CostRMB:        costRMB,
		CostQuota:      rmbToQuotaWithUnit(costRMB, quotaPerUnit),
		UnitPriceRMB:   unitPrice,
		Unit:           "rmb_per_million_tokens",
		RMBPerUSD:      mediaProviderRMBPerUSD,
		QuotaPerUnit:   quotaPerUnit,
		UsageSource:    usageSourceOrDefault(input.UsageSource, "provider_usage_estimated"),
		BillableTokens: billableTokens,
		FormulaKey:     "seedance_token_price",
		FormulaVersion: mediaProviderCostPriceVersion,
		FormulaText:    "cost_rmb = unit_price_rmb_per_million_tokens * billable_tokens / 1000000; cost_quota = round(cost_rmb / rmb_per_usd * quota_per_unit)",
		Variables: map[string]interface{}{
			"billable_tokens": billableTokens,
		},
		Coefficients: map[string]interface{}{
			"unit_price_rmb_per_million_tokens": unitPrice,
			"price_tier":                        seedancePriceTier(normalized, input.Resolution, input.InputClass),
			"rmb_per_usd":                       mediaProviderRMBPerUSD,
			"quota_per_unit":                    quotaPerUnit,
		},
		Confidence: "provider_usage_estimated",
	}
}

func ResolveMediaKitEnhanceCost(toolVersion, resolution string, fps float64, durationSeconds float64) MediaProviderCostResult {
	quotaPerUnit := common.QuotaPerUnit
	coefficient := mediaKitEnhanceBillingCoefficient(toolVersion, resolution, fps)
	basePrice := mediaKitEnhanceBasePriceRMBPerMinute
	costRMB := basePrice * coefficient * math.Max(durationSeconds, 0) / 60.0
	return MediaProviderCostResult{
		Version:                   mediaProviderCostPriceVersion,
		CostRMB:                   costRMB,
		CostQuota:                 rmbToQuotaWithUnit(costRMB, quotaPerUnit),
		UnitPriceRMB:              basePrice,
		Unit:                      "rmb_per_minute",
		RMBPerUSD:                 mediaProviderRMBPerUSD,
		QuotaPerUnit:              quotaPerUnit,
		UsageSource:               "provider_usage_estimated",
		BillingCoefficient:        coefficient,
		EnhanceBasePriceRMBPerMin: basePrice,
		FormulaKey:                "mediakit_enhance_video",
		FormulaVersion:            mediaProviderCostPriceVersion,
		FormulaText:               "cost_rmb = base_price_rmb_per_minute * billing_coefficient * duration_minutes; cost_quota = round(cost_rmb / rmb_per_usd * quota_per_unit)",
		Variables: map[string]interface{}{
			"duration_seconds": durationSeconds,
			"duration_minutes": durationSeconds / 60.0,
			"tool_version":     toolVersion,
			"resolution":       resolution,
			"fps":              fps,
		},
		Coefficients: map[string]interface{}{
			"base_price_rmb_per_minute": basePrice,
			"billing_coefficient":       coefficient,
			"rmb_per_usd":               mediaProviderRMBPerUSD,
			"quota_per_unit":            quotaPerUnit,
		},
		Confidence: "provider_usage_estimated",
	}
}

func estimateSeedanceTokens(inputVideoSeconds, outputVideoSeconds, width, height, fps float64) int64 {
	if width <= 0 || height <= 0 || fps <= 0 {
		return 0
	}
	total := (math.Max(inputVideoSeconds, 0) + math.Max(outputVideoSeconds, 0)) * width * height * fps / 1024.0
	return int64(math.Round(total))
}

func mediaKitEnhanceBillingCoefficient(toolVersion, resolution string, fps float64) float64 {
	resolution = strings.ToLower(strings.TrimSpace(resolution))
	if resolution == "" {
		resolution = "1080p"
	}
	highFPS := fps > 30
	switch strings.ToLower(strings.TrimSpace(toolVersion)) {
	case "professional":
		switch resolution {
		case "720p":
			if highFPS {
				return 20
			}
			return 10
		case "1080p":
			if highFPS {
				return 40
			}
			return 20
		case "2k":
			if highFPS {
				return 80
			}
			return 40
		default:
			if highFPS {
				return 160
			}
			return 80
		}
	default:
		switch resolution {
		case "720p":
			if highFPS {
				return 2
			}
			return 1
		case "1080p":
			if highFPS {
				return 4
			}
			return 2
		case "2k":
			if highFPS {
				return 8
			}
			return 4
		default:
			if highFPS {
				return 16
			}
			return 8
		}
	}
}

func normalizeSeedanceModelName(modelName string) string {
	modelName = strings.ToLower(strings.TrimSpace(modelName))
	switch {
	case strings.Contains(modelName, "seedance-2-0-mini"), strings.Contains(modelName, "seedance-2.0-mini"):
		return "doubao-seedance-2.0-mini"
	case strings.Contains(modelName, "seedance-2-0-fast"), strings.Contains(modelName, "seedance-2.0-fast"):
		return "doubao-seedance-2.0-fast"
	case strings.Contains(modelName, "seedance-2-0"), strings.Contains(modelName, "seedance-2.0"):
		return "doubao-seedance-2.0"
	default:
		return modelName
	}
}

func seedancePriceTier(normalizedModel, resolution, inputClass string) string {
	tier := "720p"
	if strings.EqualFold(strings.TrimSpace(resolution), "1080p") {
		tier = "1080p"
	}
	if strings.Contains(strings.ToLower(inputClass), "video") {
		tier += ":video"
	} else {
		tier += ":no_video"
	}
	if normalizedModel == "doubao-seedance-2.0-fast" || normalizedModel == "doubao-seedance-2.0-mini" {
		return normalizedModel + ":" + tier
	}
	return "doubao-seedance-2.0:" + tier
}

func usageSourceOrDefault(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
