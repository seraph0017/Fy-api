package helper

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetAndValidateRequestConvertsTextOnlyImageChatToImageGeneration(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{
		"model":"gpt-image-2",
		"messages":[
			{"role":"system","content":"Use a flat illustration style."},
			{"role":"user","content":[{"type":"text","text":"Draw a red teapot on a blue table."}]}
		],
		"size":"1024x1024",
		"quality":"high",
		"n":2,
		"stream":true
	}`)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	request, err := GetAndValidateRequest(c, types.RelayFormatOpenAI)

	require.NoError(t, err)
	imageRequest, ok := request.(*dto.ImageRequest)
	require.True(t, ok)
	assert.Equal(t, "gpt-image-2", imageRequest.Model)
	assert.Equal(t, "Use a flat illustration style.\nDraw a red teapot on a blue table.", imageRequest.Prompt)
	assert.Equal(t, "1024x1024", imageRequest.Size)
	assert.Equal(t, "high", imageRequest.Quality)
	require.NotNil(t, imageRequest.N)
	assert.Equal(t, uint(2), *imageRequest.N)
	require.NotNil(t, imageRequest.Stream)
	assert.True(t, *imageRequest.Stream)
	assert.Equal(t, relayconstant.RelayModeImagesGenerations, c.GetInt("relay_mode"))
	assert.Equal(t, "/v1/images/generations", c.Request.URL.Path)
	c.Set(string(constant.ContextKeyOriginalModel), imageRequest.Model)
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatOpenAI, request, nil)
	require.NoError(t, err)
	assert.Equal(t, relayconstant.RelayModeImagesGenerations, relayInfo.RelayMode)
	assert.Equal(t, types.RelayFormat(types.RelayFormatOpenAIImage), relayInfo.RelayFormat)

	storage, err := common.GetBodyStorage(c)
	require.NoError(t, err)
	replayedBody, err := storage.Bytes()
	require.NoError(t, err)
	var replayed dto.ImageRequest
	require.NoError(t, common.Unmarshal(replayedBody, &replayed))
	assert.Equal(t, imageRequest.Model, replayed.Model)
	assert.Equal(t, imageRequest.Prompt, replayed.Prompt)
}

func TestGetAndValidateRequestRejectsMultimodalImageChatCompat(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{
		"model":"gpt-image-2",
		"messages":[
			{"role":"user","content":[
				{"type":"text","text":"Edit this product photo."},
				{"type":"image_url","image_url":{"url":"https://example.com/photo.png"}}
			]}
		]
	}`)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	request, err := GetAndValidateRequest(c, types.RelayFormatOpenAI)

	require.Error(t, err)
	assert.Nil(t, request)
	assert.Contains(t, err.Error(), "multimodal")
}
