package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeMediaQuality(t *testing.T) {
	bucket, fallbacks, warnings := NormalizeMediaQuality("")
	require.Equal(t, "auto", bucket)
	require.Equal(t, []string{"quality:empty->auto"}, fallbacks)
	require.Empty(t, warnings)

	bucket, fallbacks, warnings = NormalizeMediaQuality("hd")
	require.Equal(t, "high", bucket)
	require.Empty(t, fallbacks)
	require.Empty(t, warnings)

	bucket, fallbacks, warnings = NormalizeMediaQuality("provider-special")
	require.Equal(t, "unknown", bucket)
	require.Empty(t, fallbacks)
	require.Equal(t, []string{"quality:unknown:provider-special"}, warnings)
}

func TestNormalizeMediaResolution(t *testing.T) {
	width, height, bucket, aspectRatio, fallbacks, warnings := NormalizeMediaResolution("1280*720")
	require.Equal(t, 1280, width)
	require.Equal(t, 720, height)
	require.Equal(t, "720p", bucket)
	require.Equal(t, "16:9", aspectRatio)
	require.Empty(t, fallbacks)
	require.Empty(t, warnings)

	width, height, bucket, aspectRatio, fallbacks, warnings = NormalizeMediaResolution("hd")
	require.Zero(t, width)
	require.Zero(t, height)
	require.Equal(t, "720p", bucket)
	require.Equal(t, "unknown", aspectRatio)
	require.Empty(t, fallbacks)
	require.Empty(t, warnings)

	width, height, bucket, aspectRatio, fallbacks, warnings = NormalizeMediaResolution("")
	require.Zero(t, width)
	require.Zero(t, height)
	require.Equal(t, "unknown", bucket)
	require.Equal(t, "unknown", aspectRatio)
	require.Equal(t, []string{"resolution:empty->unknown"}, fallbacks)
	require.Empty(t, warnings)
}

func TestBuildMediaOther(t *testing.T) {
	other := BuildMediaOther(MediaBillingDimensions{
		Modality:            MediaModalityVideo,
		ModelName:           "wan2.6-i2v",
		UpstreamModelName:   "wan2.6-i2v",
		Provider:            "ali",
		BillingMode:         MediaBillingModeVideoDuration,
		SizeRaw:             "1280*720",
		Width:               1280,
		Height:              720,
		ResolutionBucket:    "720p",
		AspectRatio:         "16:9",
		DurationSeconds:     5,
		HasImageInput:       true,
		ReferenceImageCount: 1,
		ProviderUsage:       map[string]float64{"duration": 5},
		Fallbacks:           []string{"x:y"},
	}, 0.3, MediaUnitSecond, 5)

	require.Equal(t, true, other["media_billing"])
	require.Equal(t, MediaModalityVideo, other["media_modality"])
	require.Equal(t, "wan2.6-i2v", other["media_model_name"])
	require.Equal(t, "720p", other["media_resolution_bucket"])
	require.Equal(t, "16:9", other["media_aspect_ratio"])
	require.Equal(t, 5.0, other["media_duration_seconds"])
	require.Equal(t, true, other["media_has_image_input"])
	require.Equal(t, 1, other["media_reference_image_count"])
	require.Equal(t, 0.3, other["media_unit_price"])
	require.Equal(t, MediaUnitSecond, other["media_unit"])
	require.Equal(t, 5.0, other["media_multiplier"])
	require.Equal(t, map[string]float64{"duration": 5}, other["media_provider_usage"])
	require.Equal(t, []string{"x:y"}, other["media_fallbacks"])
}
