package service

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

const (
	MediaModalityImage = "image"
	MediaModalityVideo = "video"

	MediaBillingModeFixedImage      = "fixed_image"
	MediaBillingModeResolutionImage = "resolution_image"
	MediaBillingModeTokenRate       = "token_rate"
	MediaBillingModeVideoDuration   = "video_duration"
	MediaBillingModeExpression      = "expression"

	MediaUnitImage   = "image"
	MediaUnitSecond  = "second"
	MediaUnitToken1M = "token_1m"
	MediaUnitRequest = "request"
)

type MediaBillingDimensions struct {
	Modality string `json:"modality,omitempty"`

	ModelName         string `json:"model_name,omitempty"`
	UpstreamModelName string `json:"upstream_model_name,omitempty"`
	Provider          string `json:"provider,omitempty"`

	BillingMode string `json:"billing_mode,omitempty"`

	ImageCount int `json:"image_count,omitempty"`

	QualityRaw    string `json:"quality_raw,omitempty"`
	QualityBucket string `json:"quality_bucket,omitempty"`

	SizeRaw          string `json:"size_raw,omitempty"`
	Width            int    `json:"width,omitempty"`
	Height           int    `json:"height,omitempty"`
	ResolutionBucket string `json:"resolution_bucket,omitempty"`
	AspectRatio      string `json:"aspect_ratio,omitempty"`

	DurationSeconds float64 `json:"duration_seconds,omitempty"`

	HasImageInput       bool `json:"has_image_input,omitempty"`
	HasVideoInput       bool `json:"has_video_input,omitempty"`
	ReferenceImageCount int  `json:"reference_image_count,omitempty"`
	ReferenceVideoCount int  `json:"reference_video_count,omitempty"`

	ProviderUsage map[string]float64 `json:"provider_usage,omitempty"`

	Fallbacks []string `json:"fallbacks,omitempty"`
	Warnings  []string `json:"warnings,omitempty"`
}

type MediaBillingEstimate struct {
	Dimensions MediaBillingDimensions `json:"dimensions"`

	UnitPrice float64 `json:"unit_price,omitempty"`
	Unit      string  `json:"unit,omitempty"`

	Multiplier     float64 `json:"multiplier,omitempty"`
	EstimatedQuota int     `json:"estimated_quota,omitempty"`

	OtherRatios map[string]float64 `json:"other_ratios,omitempty"`
	Other       map[string]any     `json:"other,omitempty"`
}

func NormalizeMediaQuality(raw string) (bucket string, fallbacks []string, warnings []string) {
	trimmed := strings.TrimSpace(strings.ToLower(raw))
	switch trimmed {
	case "":
		return "auto", []string{"quality:empty->auto"}, nil
	case "auto":
		return "auto", nil, nil
	case "low", "standard-low":
		return "low", nil, nil
	case "medium", "standard", "normal", "balanced":
		return "medium", nil, nil
	case "high", "hd", "ultra", "best":
		return "high", nil, nil
	default:
		return "unknown", nil, []string{fmt.Sprintf("quality:unknown:%s", raw)}
	}
}

var mediaSizePattern = regexp.MustCompile(`(?i)(\d+)\s*[x*×]\s*(\d+)`)

func NormalizeMediaResolution(raw string) (width int, height int, bucket string, aspectRatio string, fallbacks []string, warnings []string) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, 0, "unknown", "unknown", []string{"resolution:empty->unknown"}, nil
	}
	lower := strings.ToLower(strings.ReplaceAll(trimmed, " ", ""))
	aliases := map[string]string{
		"480p":  "480p",
		"480":   "480p",
		"720p":  "720p",
		"720":   "720p",
		"hd":    "720p",
		"1080p": "1080p",
		"1080":  "1080p",
		"fhd":   "1080p",
		"2k":    "2k",
		"1440p": "2k",
		"4k":    "4k",
		"2160p": "4k",
	}
	if b, ok := aliases[lower]; ok {
		return 0, 0, b, "unknown", nil, nil
	}

	matches := mediaSizePattern.FindStringSubmatch(trimmed)
	if len(matches) != 3 {
		return 0, 0, "unknown", "unknown", nil, []string{fmt.Sprintf("resolution:unknown:%s", raw)}
	}
	w, errW := strconv.Atoi(matches[1])
	h, errH := strconv.Atoi(matches[2])
	if errW != nil || errH != nil || w <= 0 || h <= 0 {
		return 0, 0, "unknown", "unknown", nil, []string{fmt.Sprintf("resolution:invalid:%s", raw)}
	}
	return w, h, ResolutionBucketFromDimensions(w, h), AspectRatioFromDimensions(w, h), nil, nil
}

func ResolutionBucketFromDimensions(width int, height int) string {
	if width <= 0 || height <= 0 {
		return "unknown"
	}
	shortSide := width
	if height < shortSide {
		shortSide = height
	}
	longSide := width
	if height > longSide {
		longSide = height
	}
	if width == height {
		return "square"
	}
	switch {
	case shortSide <= 540:
		return "480p"
	case shortSide <= 800:
		return "720p"
	case shortSide <= 1200:
		return "1080p"
	case longSide <= 2560:
		return "2k"
	case longSide <= 4096:
		return "4k"
	default:
		return "custom"
	}
}

func AspectRatioFromDimensions(width int, height int) string {
	if width <= 0 || height <= 0 {
		return "unknown"
	}
	g := gcd(width, height)
	if g <= 0 {
		return "unknown"
	}
	w := width / g
	h := height / g
	common := map[string]bool{
		"1:1":  true,
		"16:9": true,
		"9:16": true,
		"4:3":  true,
		"3:4":  true,
		"3:2":  true,
		"2:3":  true,
	}
	ratio := fmt.Sprintf("%d:%d", w, h)
	if common[ratio] {
		return ratio
	}
	return "custom"
}

func gcd(a int, b int) int {
	a = int(math.Abs(float64(a)))
	b = int(math.Abs(float64(b)))
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

func BuildMediaOther(d MediaBillingDimensions, unitPrice float64, unit string, multiplier float64) map[string]any {
	other := map[string]any{
		"media_billing": true,
	}
	addString := func(key, val string) {
		if val != "" {
			other[key] = val
		}
	}
	addNumber := func(key string, val float64) {
		if val > 0 {
			other[key] = val
		}
	}
	addInt := func(key string, val int) {
		if val > 0 {
			other[key] = val
		}
	}
	addString("media_modality", d.Modality)
	addString("media_model_name", d.ModelName)
	addString("media_upstream_model_name", d.UpstreamModelName)
	addString("media_provider", d.Provider)
	addString("media_billing_mode", d.BillingMode)
	addString("media_quality_raw", d.QualityRaw)
	addString("media_quality_bucket", d.QualityBucket)
	addString("media_size_raw", d.SizeRaw)
	addInt("media_width", d.Width)
	addInt("media_height", d.Height)
	addString("media_resolution_bucket", d.ResolutionBucket)
	addString("media_aspect_ratio", d.AspectRatio)
	addNumber("media_duration_seconds", d.DurationSeconds)
	addInt("media_image_count", d.ImageCount)
	if d.HasImageInput {
		other["media_has_image_input"] = true
	}
	if d.HasVideoInput {
		other["media_has_video_input"] = true
	}
	addInt("media_reference_image_count", d.ReferenceImageCount)
	addInt("media_reference_video_count", d.ReferenceVideoCount)
	addNumber("media_unit_price", unitPrice)
	addString("media_unit", unit)
	addNumber("media_multiplier", multiplier)
	if len(d.Fallbacks) > 0 {
		other["media_fallbacks"] = d.Fallbacks
	}
	if len(d.Warnings) > 0 {
		other["media_warnings"] = d.Warnings
	}
	if len(d.ProviderUsage) > 0 {
		other["media_provider_usage"] = d.ProviderUsage
	}
	return other
}

func MergeMediaOther(dst map[string]any, src map[string]any) map[string]any {
	if dst == nil {
		dst = make(map[string]any)
	}
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
