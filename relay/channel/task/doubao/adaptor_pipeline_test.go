package doubao

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildRequestBodyAppliesVideoPipelinePlan(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("SEEDANCE_PIPELINE_ENABLED", "true")
	t.Setenv("SEEDANCE_PIPELINE_TRAFFIC_PERCENT", "100")

	body := `{"model":"doubao-seedance-2-0-260128","prompt":"固定镜头的产品展示","seconds":"5","size":"1920x1080","metadata":{"fy_enhance_force":true}}`
	req := httptest.NewRequest("POST", "/v1/videos", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	info := &relaycommon.RelayInfo{
		OriginModelName: "doubao-seedance-2-0-260128",
		ChannelMeta:     &relaycommon.ChannelMeta{},
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
	}
	adaptor := &TaskAdaptor{}
	taskErr := adaptor.ValidateRequestAndSetAction(c, info)
	require.Nil(t, taskErr)

	reader, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	raw, err := io.ReadAll(reader)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(raw, &payload))

	assert.Equal(t, "doubao-seedance-2-0-260128", payload["model"])
	assert.Equal(t, "720p", payload["resolution"])
	plan, ok := service.GetVideoPipelinePlan(c)
	require.True(t, ok)
	assert.Equal(t, "dynamic-default-seedance-2-720p", plan.MatchedGenerationPolicy)
}

func TestBuildRequestBodyMapsReferenceMediaRoles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("SEEDANCE_PIPELINE_ENABLED", "true")
	t.Setenv("SEEDANCE_PIPELINE_TRAFFIC_PERCENT", "100")

	body := `{
		"model":"doubao-seedance-2-0-260128",
		"prompt":"参考素材生成视频",
		"seconds":"5",
		"size":"1920x1080",
		"image":"https://example.com/single.png",
		"images":["https://example.com/second.png"],
		"media":[{"type":"video","url":"https://example.com/motion.mp4"}],
		"metadata":{"fy_enhance_force":true}
	}`
	req := httptest.NewRequest("POST", "/v1/videos", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	info := &relaycommon.RelayInfo{
		OriginModelName: "doubao-seedance-2-0-260128",
		ChannelMeta:     &relaycommon.ChannelMeta{},
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
	}
	adaptor := &TaskAdaptor{}
	taskErr := adaptor.ValidateRequestAndSetAction(c, info)
	require.Nil(t, taskErr)

	reader, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	raw, err := io.ReadAll(reader)
	require.NoError(t, err)
	var payload struct {
		Content []ContentItem `json:"content"`
	}
	require.NoError(t, common.Unmarshal(raw, &payload))

	require.Len(t, payload.Content, 4)
	assert.Equal(t, "image_url", payload.Content[0].Type)
	assert.Equal(t, "reference_image", payload.Content[0].Role)
	assert.Equal(t, "https://example.com/single.png", payload.Content[0].ImageURL.URL)
	assert.Equal(t, "image_url", payload.Content[1].Type)
	assert.Equal(t, "reference_image", payload.Content[1].Role)
	assert.Equal(t, "https://example.com/second.png", payload.Content[1].ImageURL.URL)
	assert.Equal(t, "video_url", payload.Content[2].Type)
	assert.Equal(t, "reference_video", payload.Content[2].Role)
	assert.Equal(t, "https://example.com/motion.mp4", payload.Content[2].VideoURL.URL)
	assert.Equal(t, "text", payload.Content[3].Type)
}

func TestEstimateBillingChargesRequestedSeedance1080pTier(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := `{"model":"doubao-seedance-2-0-260128","prompt":"test","seconds":"5","size":"1920x1080"}`
	req := httptest.NewRequest("POST", "/v1/videos", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	info := &relaycommon.RelayInfo{
		OriginModelName: "doubao-seedance-2-0-260128",
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
	}
	adaptor := &TaskAdaptor{}
	taskErr := adaptor.ValidateRequestAndSetAction(c, info)
	require.Nil(t, taskErr)

	ratios := adaptor.EstimateBilling(c, info)

	require.NotNil(t, ratios)
	assert.InDelta(t, 46.0/28.0, ratios["seedance_1080p"], 0.000001)
}

func TestConvertToOpenAIVideoHidesInternalPipelineMetadata(t *testing.T) {
	task := &model.Task{
		TaskID:    "task_public",
		Status:    model.TaskStatusSuccess,
		Progress:  "100%",
		CreatedAt: 100,
		UpdatedAt: 200,
		Properties: model.Properties{
			OriginModelName: "doubao-seedance-2-0-260128",
		},
		PrivateData: model.TaskPrivateData{
			ResultURL: "https://cdn.example.com/enhanced.mp4",
			SeedanceEnhance: &model.SeedanceEnhancePipeline{
				Pipeline:             service.VideoPipelineNameSeedanceEnhance,
				Status:               "enhance_succeeded",
				GenerationResolution: "720p",
			},
		},
		Data: []byte(`{"status":"succeeded","content":{"video_url":"https://cdn.example.com/generated.mp4"}}`),
	}

	raw, err := (&TaskAdaptor{}).ConvertToOpenAIVideo(task)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, common.Unmarshal(raw, &got))
	metadata, ok := got["metadata"].(map[string]any)
	require.True(t, ok)

	assert.Equal(t, "https://cdn.example.com/enhanced.mp4", metadata["url"])
	assert.NotContains(t, metadata, "pipeline")
	assert.NotContains(t, metadata, "pipeline_status")
	assert.NotContains(t, metadata, "generation_resolution")
}
