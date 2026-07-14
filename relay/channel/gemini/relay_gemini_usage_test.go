package gemini

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestGeminiAdaptorSupportsGeminiImagePreviewModel(t *testing.T) {
	t.Parallel()

	adaptor := &Adaptor{}
	require.True(t, isGeminiImagePreviewModel("gemini-3-pro-image-preview"))
	require.True(t, isGeminiImagePreviewModel("gemini-3-pro-image"))
	require.True(t, isGeminiImagePreviewModel("gemini-3.1-flash-image-preview"))
	require.True(t, isGeminiImagePreviewModel("nano-banana-pro-preview"))

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	got, err := adaptor.convertGeminiImagePreviewRequest(c, &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeGemini,
			UpstreamModelName: "gemini-3-pro-image-preview",
		},
		RelayMode: relayconstant.RelayModeImagesGenerations,
	}, dto.ImageRequest{
		Model:   "gpt-image-1",
		Prompt:  "a red panda",
		N:       common.GetPointer(uint(2)),
		Size:    "1024x1792",
		Quality: "high",
	})
	require.NoError(t, err)

	req, ok := got.(*dto.GeminiChatRequest)
	require.True(t, ok)
	require.Len(t, req.Contents, 1)
	require.Len(t, req.Contents[0].Parts, 1)
	require.Equal(t, "a red panda", req.Contents[0].Parts[0].Text)
	require.Equal(t, []string{"TEXT", "IMAGE"}, req.GenerationConfig.ResponseModalities)
	require.NotNil(t, req.GenerationConfig.CandidateCount)
	require.Equal(t, 2, *req.GenerationConfig.CandidateCount)
	var extra map[string]any
	require.NoError(t, common.Unmarshal(req.GenerationConfig.ImageConfig, &extra))
	require.Equal(t, "9:16", extra["aspectRatio"])
	require.Equal(t, "2K", extra["imageSize"])
}

func TestGeminiAdaptorMapsSquareImagePreviewGenerationSize(t *testing.T) {
	t.Parallel()

	adaptor := &Adaptor{}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	got, err := adaptor.convertGeminiImagePreviewRequest(c, &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeGemini,
			UpstreamModelName: "gemini-3-pro-image-preview",
		},
	}, dto.ImageRequest{
		Model:  "gemini-3-pro-image-preview",
		Prompt: "a white document cover",
		Size:   "1024x1024",
	})
	require.NoError(t, err)

	req, ok := got.(*dto.GeminiChatRequest)
	require.True(t, ok)
	var imageConfig map[string]any
	require.NoError(t, common.Unmarshal(req.GenerationConfig.ImageConfig, &imageConfig))
	require.Equal(t, "1:1", imageConfig["aspectRatio"])
}

func TestGeminiAdaptorConvertsJsonEditImageToInlineData(t *testing.T) {
	t.Parallel()

	adaptor := &Adaptor{}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil)
	c.Request.Header.Set("Content-Type", "application/json")

	got, err := adaptor.convertGeminiImagePreviewRequest(c, &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesEdits,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeGemini,
			UpstreamModelName: "gemini-3-pro-image-preview",
		},
	}, dto.ImageRequest{
		Model:  "gpt-image-1",
		Prompt: "edit this image",
		Image:  json.RawMessage(`"data:image/png;base64,aGVsbG8="`),
	})
	require.NoError(t, err)

	req, ok := got.(*dto.GeminiChatRequest)
	require.True(t, ok)
	require.Len(t, req.Contents, 1)
	require.Len(t, req.Contents[0].Parts, 2)
	require.Equal(t, "edit this image", req.Contents[0].Parts[0].Text)
	require.NotNil(t, req.Contents[0].Parts[1].InlineData)
	require.Equal(t, "image/png", req.Contents[0].Parts[1].InlineData.MimeType)
}

func TestGeminiAdaptorConvertsMultipartEditImageToInlineData(t *testing.T) {
	t.Parallel()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "gemini-2.5-flash-image"))
	require.NoError(t, writer.WriteField("prompt", "turn the center into a gold star"))
	require.NoError(t, writer.WriteField("n", "1"))
	require.NoError(t, writer.WriteField("size", "1024x1024"))
	imagePart, err := writer.CreateFormFile("image", "input.png")
	require.NoError(t, err)
	_, err = imagePart.Write([]byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	})
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	adaptor := &Adaptor{}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())

	got, err := adaptor.convertGeminiImagePreviewRequest(c, &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesEdits,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeGemini,
			UpstreamModelName: "gemini-2.5-flash-image",
		},
	}, dto.ImageRequest{
		Model:  "gemini-2.5-flash-image",
		Prompt: "turn the center into a gold star",
		N:      common.GetPointer(uint(1)),
		Size:   "1024x1024",
	})
	require.NoError(t, err)

	req, ok := got.(*dto.GeminiChatRequest)
	require.True(t, ok)
	require.Len(t, req.Contents, 1)
	require.Len(t, req.Contents[0].Parts, 2)
	require.Equal(t, "turn the center into a gold star", req.Contents[0].Parts[0].Text)
	require.NotNil(t, req.Contents[0].Parts[1].InlineData)
	require.Equal(t, "image/png", req.Contents[0].Parts[1].InlineData.MimeType)
	require.NotEmpty(t, req.Contents[0].Parts[1].InlineData.Data)
	var imageConfig map[string]any
	require.NoError(t, common.Unmarshal(req.GenerationConfig.ImageConfig, &imageConfig))
	require.Equal(t, "1:1", imageConfig["aspectRatio"])
}

func TestGeminiAdaptorSetsJSONContentTypeForMultipartImagePreviewEdit(t *testing.T) {
	t.Parallel()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "gemini-2.5-flash-image"))
	require.NoError(t, writer.WriteField("prompt", "turn the center into a gold star"))
	imagePart, err := writer.CreateFormFile("image", "input.png")
	require.NoError(t, err)
	_, err = imagePart.Write([]byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	})
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	adaptor := &Adaptor{}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())

	headers := http.Header{}
	err = adaptor.SetupRequestHeader(c, &headers, &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesEdits,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-2.5-flash-image",
			ApiKey:            "test-key",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "application/json", headers.Get("Content-Type"))
}

func TestGeminiAdaptorConvertsMultipartEditImagesAndMaskToInlineData(t *testing.T) {
	t.Parallel()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "gemini-2.5-flash-image"))
	require.NoError(t, writer.WriteField("prompt", "combine these images and use the mask"))
	for _, fileName := range []string{"input.png", "input2.png"} {
		imagePart, err := writer.CreateFormFile("image", fileName)
		require.NoError(t, err)
		_, err = imagePart.Write([]byte{
			0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
			0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		})
		require.NoError(t, err)
	}
	maskPart, err := writer.CreateFormFile("mask", "mask.png")
	require.NoError(t, err)
	_, err = maskPart.Write([]byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	})
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	adaptor := &Adaptor{}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())

	got, err := adaptor.convertGeminiImagePreviewRequest(c, &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesEdits,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeGemini,
			UpstreamModelName: "gemini-2.5-flash-image",
		},
	}, dto.ImageRequest{
		Model:  "gemini-2.5-flash-image",
		Prompt: "combine these images and use the mask",
	})
	require.NoError(t, err)

	req, ok := got.(*dto.GeminiChatRequest)
	require.True(t, ok)
	require.Len(t, req.Contents, 1)
	require.Len(t, req.Contents[0].Parts, 5)
	require.Equal(t, "combine these images and use the mask", req.Contents[0].Parts[0].Text)
	require.NotNil(t, req.Contents[0].Parts[1].InlineData)
	require.NotNil(t, req.Contents[0].Parts[2].InlineData)
	require.Equal(t, "Use the next image as the edit mask.", req.Contents[0].Parts[3].Text)
	require.NotNil(t, req.Contents[0].Parts[4].InlineData)
}

func TestGeminiAdaptorConvertsJsonEditImageURLToInlineData(t *testing.T) {
	t.Parallel()

	// Serve a tiny valid PNG.
	testData := filepath.Join(os.TempDir(), "testimage.png")
	require.NoError(t, os.WriteFile(testData, []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	}, 0644))
	defer os.Remove(testData)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, testData)
	}))
	defer srv.Close()

	adaptor := &Adaptor{}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil)
	c.Request.Header.Set("Content-Type", "application/json")

	imageStr := `"` + srv.URL + `/image.png"` + ""
	got, err := adaptor.convertGeminiImagePreviewRequest(c, &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesEdits,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeGemini,
			UpstreamModelName: "gemini-3-pro-image-preview",
		},
	}, dto.ImageRequest{
		Model:  "gpt-image-1",
		Prompt: "edit this image from url",
		Image:  json.RawMessage(imageStr),
	})
	require.NoError(t, err)

	req, ok := got.(*dto.GeminiChatRequest)
	require.True(t, ok)
	require.Len(t, req.Contents[0].Parts, 2)
	require.Equal(t, "edit this image from url", req.Contents[0].Parts[0].Text)
	require.NotNil(t, req.Contents[0].Parts[1].InlineData)
	require.NotEmpty(t, req.Contents[0].Parts[1].InlineData.Data)
}

func TestGeminiAdaptorConvertsJsonEditImagesArrayURLsToInlineData(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte{
			0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
			0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		})
	}))
	defer srv.Close()

	adaptor := &Adaptor{}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil)
	c.Request.Header.Set("Content-Type", "application/json")

	got, err := adaptor.convertGeminiImagePreviewRequest(c, &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesEdits,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeGemini,
			UpstreamModelName: "gemini-2.5-flash-image",
		},
	}, dto.ImageRequest{
		Model:  "gemini-2.5-flash-image",
		Prompt: "combine two images from urls",
		Images: json.RawMessage(fmt.Sprintf(`["%s/1.png","%s/2.png"]`, srv.URL, srv.URL)),
	})
	require.NoError(t, err)

	req, ok := got.(*dto.GeminiChatRequest)
	require.True(t, ok)
	require.Len(t, req.Contents[0].Parts, 3)
	require.Equal(t, "combine two images from urls", req.Contents[0].Parts[0].Text)
	require.NotNil(t, req.Contents[0].Parts[1].InlineData)
	require.NotEmpty(t, req.Contents[0].Parts[1].InlineData.Data)
	require.NotNil(t, req.Contents[0].Parts[2].InlineData)
	require.NotEmpty(t, req.Contents[0].Parts[2].InlineData.Data)
}

func TestGeminiAdaptorConvertsJsonEditImageURLObjectsToInlineData(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte{
			0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
			0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		})
	}))
	defer srv.Close()

	adaptor := &Adaptor{}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil)
	c.Request.Header.Set("Content-Type", "application/json")

	got, err := adaptor.convertGeminiImagePreviewRequest(c, &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesEdits,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeGemini,
			UpstreamModelName: "gemini-3-pro-image-preview",
		},
	}, dto.ImageRequest{
		Model:  "gemini-3-pro-image-preview",
		Prompt: "edit image from OpenAI image URL object",
		Images: json.RawMessage(fmt.Sprintf(`[{"image_url":"%s/object.png"}]`, srv.URL)),
	})
	require.NoError(t, err)

	req, ok := got.(*dto.GeminiChatRequest)
	require.True(t, ok)
	require.Len(t, req.Contents[0].Parts, 2)
	require.Equal(t, "edit image from OpenAI image URL object", req.Contents[0].Parts[0].Text)
	require.NotNil(t, req.Contents[0].Parts[1].InlineData)
	require.NotEmpty(t, req.Contents[0].Parts[1].InlineData.Data)
}

func TestGeminiAdaptorRejectsInvalidImageURL(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	adaptor := &Adaptor{}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil)
	c.Request.Header.Set("Content-Type", "application/json")

	_, err := adaptor.convertGeminiImagePreviewRequest(c, &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesEdits,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeGemini,
			UpstreamModelName: "gemini-3-pro-image-preview",
		},
	}, dto.ImageRequest{
		Model:  "gpt-image-1",
		Prompt: "edit image at bad url",
		Image:  json.RawMessage(fmt.Sprintf(`"%s/notfound.png"`, srv.URL)),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "status 404")
}

func TestGeminiAdaptorConvertsJsonEditImagesArrayToInlineData(t *testing.T) {
	t.Parallel()

	adaptor := &Adaptor{}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil)
	c.Request.Header.Set("Content-Type", "application/json")

	got, err := adaptor.convertGeminiImagePreviewRequest(c, &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesEdits,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeGemini,
			UpstreamModelName: "gemini-2.5-flash-image",
		},
	}, dto.ImageRequest{
		Model:  "gemini-2.5-flash-image",
		Prompt: "combine these images",
		Images: json.RawMessage(`["data:image/png;base64,aGVsbG8=","data:image/png;base64,d29ybGQ="]`),
	})
	require.NoError(t, err)

	req, ok := got.(*dto.GeminiChatRequest)
	require.True(t, ok)
	require.Len(t, req.Contents, 1)
	require.Len(t, req.Contents[0].Parts, 3)
	require.Equal(t, "combine these images", req.Contents[0].Parts[0].Text)
	require.NotNil(t, req.Contents[0].Parts[1].InlineData)
	require.NotNil(t, req.Contents[0].Parts[2].InlineData)
}

func TestGeminiAdaptorDoResponseRoutesImagePreviewToChatImageHandler(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-3-pro-image-preview:generateContent", nil)

	info := &relaycommon.RelayInfo{
		RelayFormat:    types.RelayFormatOpenAIImage,
		RelayMode:      relayconstant.RelayModeImagesGenerations,
		RequestURLPath: "/v1/images/generations",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeGemini,
			UpstreamModelName: "gemini-3-pro-image-preview",
		},
	}
	usage, err := (&Adaptor{}).DoResponse(c, &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader([]byte(`{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"aGVsbG8="}}]}}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}`))),
	}, info)
	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"b64_json":"aGVsbG8="`)
	require.Contains(t, w.Body.String(), `"usage":`)
	require.Contains(t, w.Body.String(), `"input_tokens":1`)
	require.Contains(t, w.Body.String(), `"output_tokens":1`)
	require.Contains(t, w.Body.String(), `"total_tokens":2`)
}

func TestGeminiChatHandlerCompletionTokensExcludeToolUsePromptTokens(t *testing.T) {
	t.Parallel()

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	info := &relaycommon.RelayInfo{
		RelayFormat:     types.RelayFormatGemini,
		OriginModelName: "gemini-3-flash-preview",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3-flash-preview",
		},
	}

	payload := dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{
			{
				Content: dto.GeminiChatContent{
					Role: "model",
					Parts: []dto.GeminiPart{
						{Text: "ok"},
					},
				},
			},
		},
		UsageMetadata: dto.GeminiUsageMetadata{
			PromptTokenCount:        151,
			ToolUsePromptTokenCount: 18329,
			CandidatesTokenCount:    1089,
			ThoughtsTokenCount:      1120,
			TotalTokenCount:         20689,
		},
	}

	body, err := common.Marshal(payload)
	require.NoError(t, err)

	resp := &http.Response{
		Body: io.NopCloser(bytes.NewReader(body)),
	}

	usage, newAPIError := GeminiChatHandler(c, info, resp)
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 18480, usage.PromptTokens)
	require.Equal(t, 2209, usage.CompletionTokens)
	require.Equal(t, 20689, usage.TotalTokens)
	require.Equal(t, 1120, usage.CompletionTokenDetails.ReasoningTokens)
}

func TestGeminiStreamHandlerCompletionTokensExcludeToolUsePromptTokens(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	oldStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 300
	t.Cleanup(func() {
		constant.StreamingTimeout = oldStreamingTimeout
	})

	info := &relaycommon.RelayInfo{
		OriginModelName: "gemini-3-flash-preview",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3-flash-preview",
		},
	}

	chunk := dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{
			{
				Content: dto.GeminiChatContent{
					Role: "model",
					Parts: []dto.GeminiPart{
						{Text: "partial"},
					},
				},
			},
		},
		UsageMetadata: dto.GeminiUsageMetadata{
			PromptTokenCount:        151,
			ToolUsePromptTokenCount: 18329,
			CandidatesTokenCount:    1089,
			ThoughtsTokenCount:      1120,
			TotalTokenCount:         20689,
		},
	}

	chunkData, err := common.Marshal(chunk)
	require.NoError(t, err)

	streamBody := []byte("data: " + string(chunkData) + "\n" + "data: [DONE]\n")
	resp := &http.Response{
		Body: io.NopCloser(bytes.NewReader(streamBody)),
	}

	usage, newAPIError := geminiStreamHandler(c, info, resp, func(_ string, _ *dto.GeminiChatResponse) bool {
		return true
	})
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 18480, usage.PromptTokens)
	require.Equal(t, 2209, usage.CompletionTokens)
	require.Equal(t, 20689, usage.TotalTokens)
	require.Equal(t, 1120, usage.CompletionTokenDetails.ReasoningTokens)
}

func TestGeminiTextGenerationHandlerPromptTokensIncludeToolUsePromptTokens(t *testing.T) {
	t.Parallel()

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-3-flash-preview:generateContent", nil)

	info := &relaycommon.RelayInfo{
		OriginModelName: "gemini-3-flash-preview",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3-flash-preview",
		},
	}

	payload := dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{
			{
				Content: dto.GeminiChatContent{
					Role: "model",
					Parts: []dto.GeminiPart{
						{Text: "ok"},
					},
				},
			},
		},
		UsageMetadata: dto.GeminiUsageMetadata{
			PromptTokenCount:        151,
			ToolUsePromptTokenCount: 18329,
			CandidatesTokenCount:    1089,
			ThoughtsTokenCount:      1120,
			TotalTokenCount:         20689,
		},
	}

	body, err := common.Marshal(payload)
	require.NoError(t, err)

	resp := &http.Response{
		Body: io.NopCloser(bytes.NewReader(body)),
	}

	usage, newAPIError := GeminiTextGenerationHandler(c, info, resp)
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 18480, usage.PromptTokens)
	require.Equal(t, 2209, usage.CompletionTokens)
	require.Equal(t, 20689, usage.TotalTokens)
	require.Equal(t, 1120, usage.CompletionTokenDetails.ReasoningTokens)
}

func TestGeminiChatHandlerUsesEstimatedPromptTokensWhenUsagePromptMissing(t *testing.T) {
	t.Parallel()

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	info := &relaycommon.RelayInfo{
		RelayFormat:     types.RelayFormatGemini,
		OriginModelName: "gemini-3-flash-preview",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3-flash-preview",
		},
	}
	info.SetEstimatePromptTokens(20)

	payload := dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{
			{
				Content: dto.GeminiChatContent{
					Role: "model",
					Parts: []dto.GeminiPart{
						{Text: "ok"},
					},
				},
			},
		},
		UsageMetadata: dto.GeminiUsageMetadata{
			PromptTokenCount:        0,
			ToolUsePromptTokenCount: 0,
			CandidatesTokenCount:    90,
			ThoughtsTokenCount:      10,
			TotalTokenCount:         110,
		},
	}

	body, err := common.Marshal(payload)
	require.NoError(t, err)

	resp := &http.Response{
		Body: io.NopCloser(bytes.NewReader(body)),
	}

	usage, newAPIError := GeminiChatHandler(c, info, resp)
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 20, usage.PromptTokens)
	require.Equal(t, 100, usage.CompletionTokens)
	require.Equal(t, 110, usage.TotalTokens)
}

func TestGeminiStreamHandlerUsesEstimatedPromptTokensWhenUsagePromptMissing(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	oldStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 300
	t.Cleanup(func() {
		constant.StreamingTimeout = oldStreamingTimeout
	})

	info := &relaycommon.RelayInfo{
		OriginModelName: "gemini-3-flash-preview",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3-flash-preview",
		},
	}
	info.SetEstimatePromptTokens(20)

	chunk := dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{
			{
				Content: dto.GeminiChatContent{
					Role: "model",
					Parts: []dto.GeminiPart{
						{Text: "partial"},
					},
				},
			},
		},
		UsageMetadata: dto.GeminiUsageMetadata{
			PromptTokenCount:        0,
			ToolUsePromptTokenCount: 0,
			CandidatesTokenCount:    90,
			ThoughtsTokenCount:      10,
			TotalTokenCount:         110,
		},
	}

	chunkData, err := common.Marshal(chunk)
	require.NoError(t, err)

	streamBody := []byte("data: " + string(chunkData) + "\n" + "data: [DONE]\n")
	resp := &http.Response{
		Body: io.NopCloser(bytes.NewReader(streamBody)),
	}

	usage, newAPIError := geminiStreamHandler(c, info, resp, func(_ string, _ *dto.GeminiChatResponse) bool {
		return true
	})
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 20, usage.PromptTokens)
	require.Equal(t, 100, usage.CompletionTokens)
	require.Equal(t, 110, usage.TotalTokens)
}

func TestGeminiTextGenerationHandlerUsesEstimatedPromptTokensWhenUsagePromptMissing(t *testing.T) {
	t.Parallel()

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-3-flash-preview:generateContent", nil)

	info := &relaycommon.RelayInfo{
		OriginModelName: "gemini-3-flash-preview",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3-flash-preview",
		},
	}
	info.SetEstimatePromptTokens(20)

	payload := dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{
			{
				Content: dto.GeminiChatContent{
					Role: "model",
					Parts: []dto.GeminiPart{
						{Text: "ok"},
					},
				},
			},
		},
		UsageMetadata: dto.GeminiUsageMetadata{
			PromptTokenCount:        0,
			ToolUsePromptTokenCount: 0,
			CandidatesTokenCount:    90,
			ThoughtsTokenCount:      10,
			TotalTokenCount:         110,
		},
	}

	body, err := common.Marshal(payload)
	require.NoError(t, err)

	resp := &http.Response{
		Body: io.NopCloser(bytes.NewReader(body)),
	}

	usage, newAPIError := GeminiTextGenerationHandler(c, info, resp)
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 20, usage.PromptTokens)
	require.Equal(t, 100, usage.CompletionTokens)
	require.Equal(t, 110, usage.TotalTokens)
}

// Fy-api overlay: 回归测试，确保原生 Gemini pass-through 入口
// 沿用客户端 URL 中的 v1 / v1beta，不会被后台 VersionSettings 强制改写。
func TestGeminiAdaptorGetRequestURLPreservesNativeVersion(t *testing.T) {
	t.Parallel()

	const baseURL = "https://generativelanguage.googleapis.com"

	cases := []struct {
		name        string
		relayMode   int
		requestPath string
		modelName   string
		wantURL     string
	}{
		{
			name:        "native v1beta is preserved for image-preview",
			relayMode:   relayconstant.RelayModeGemini,
			requestPath: "/v1beta/models/gemini-3-pro-image-preview:generateContent",
			modelName:   "gemini-3-pro-image-preview",
			wantURL:     baseURL + "/v1beta/models/gemini-3-pro-image-preview:generateContent",
		},
		{
			name:        "native v1 is preserved",
			relayMode:   relayconstant.RelayModeGemini,
			requestPath: "/v1/models/gemini-1.0-pro:generateContent",
			modelName:   "gemini-1.0-pro",
			wantURL:     baseURL + "/v1/models/gemini-1.0-pro:generateContent",
		},
		{
			name:        "imagen native v1beta is preserved",
			relayMode:   relayconstant.RelayModeGemini,
			requestPath: "/v1beta/models/imagen-3.0-generate-002:predict",
			modelName:   "imagen-3.0-generate-002",
			wantURL:     baseURL + "/v1beta/models/imagen-3.0-generate-002:predict",
		},
		{
			name:        "non-native relay mode falls back to model_setting",
			relayMode:   relayconstant.RelayModeChatCompletions,
			requestPath: "/v1/chat/completions",
			modelName:   "gemini-3-pro-image-preview", // 默认 map 中钉为 v1beta
			wantURL:     baseURL + "/v1beta/models/gemini-3-pro-image-preview:generateContent",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			a := &Adaptor{}
			info := &relaycommon.RelayInfo{
				RelayMode:      tc.relayMode,
				RequestURLPath: tc.requestPath,
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelBaseUrl:    baseURL,
					UpstreamModelName: tc.modelName,
				},
			}

			gotURL, err := a.GetRequestURL(info)
			require.NoError(t, err)
			require.Equal(t, tc.wantURL, gotURL)
		})
	}
}

func TestGeminiChatHandlerMissingUsageMetadataBuildsEstimatedBillingUsage(t *testing.T) {
	t.Parallel()

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	info := &relaycommon.RelayInfo{
		RelayFormat:     types.RelayFormatGemini,
		OriginModelName: "gemini-3-flash-preview",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3-flash-preview",
		},
	}
	info.SetEstimatePromptTokens(20)

	body := []byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]}}]}`)
	resp := &http.Response{
		Body: io.NopCloser(bytes.NewReader(body)),
	}

	usage, newAPIError := GeminiChatHandler(c, info, resp)
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 20, usage.PromptTokens)
	require.NotNil(t, usage.BillingUsage)
	require.True(t, usage.BillingUsage.Estimated)
	require.Equal(t, dto.BillingUsageSourceGeminiChat, usage.BillingUsage.Source)
	require.Equal(t, dto.BillingUsageSemanticGemini, usage.BillingUsage.Semantic)
	require.NotNil(t, usage.BillingUsage.GeminiUsageMetadata)
	require.Equal(t, usage.PromptTokens, usage.BillingUsage.GeminiUsageMetadata.PromptTokenCount)
	require.Equal(t, usage.CompletionTokens, usage.BillingUsage.GeminiUsageMetadata.CandidatesTokenCount)
	require.True(t, common.GetContextKeyBool(c, constant.ContextKeyLocalCountTokens))
}

func TestGeminiStreamHandlerPromptOnlyUsageMetadataEstimatesCompletionTokens(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	oldStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 300
	t.Cleanup(func() {
		constant.StreamingTimeout = oldStreamingTimeout
	})

	info := &relaycommon.RelayInfo{
		OriginModelName: "gemini-3-flash-preview",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3-flash-preview",
		},
	}
	info.SetEstimatePromptTokens(20)

	// Simulates a client aborting the stream before the final chunk: text was
	// streamed but the last observed usageMetadata only carries prompt tokens.
	chunk := dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{
			{
				Content: dto.GeminiChatContent{
					Role: "model",
					Parts: []dto.GeminiPart{
						{Text: "partial streamed answer before disconnect"},
					},
				},
			},
		},
		UsageMetadata: dto.GeminiUsageMetadata{
			PromptTokenCount: 151,
			TotalTokenCount:  151,
		},
	}

	chunkData, err := common.Marshal(chunk)
	require.NoError(t, err)

	streamBody := []byte("data: " + string(chunkData) + "\n" + "data: [DONE]\n")
	resp := &http.Response{
		Body: io.NopCloser(bytes.NewReader(streamBody)),
	}

	usage, newAPIError := geminiStreamHandler(c, info, resp, func(_ string, _ *dto.GeminiChatResponse) bool {
		return true
	})
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 151, usage.PromptTokens)
	require.Greater(t, usage.CompletionTokens, 0)
	require.Equal(t, usage.PromptTokens+usage.CompletionTokens, usage.TotalTokens)
	require.NotNil(t, usage.BillingUsage)
	require.True(t, usage.BillingUsage.Estimated)
	require.NotNil(t, usage.BillingUsage.GeminiUsageMetadata)
	require.Equal(t, usage.CompletionTokens, usage.BillingUsage.GeminiUsageMetadata.CandidatesTokenCount)
}

func TestGeminiChatHandlerPromptOnlyUsageMetadataEstimatesCompletionTokens(t *testing.T) {
	t.Parallel()

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	info := &relaycommon.RelayInfo{
		RelayFormat:     types.RelayFormatGemini,
		OriginModelName: "gemini-3-flash-preview",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3-flash-preview",
		},
	}

	payload := dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{
			{
				Content: dto.GeminiChatContent{
					Role: "model",
					Parts: []dto.GeminiPart{
						{Text: "answer text without candidate token count"},
					},
				},
			},
		},
		UsageMetadata: dto.GeminiUsageMetadata{
			PromptTokenCount: 151,
			TotalTokenCount:  151,
		},
	}

	body, err := common.Marshal(payload)
	require.NoError(t, err)

	resp := &http.Response{
		Body: io.NopCloser(bytes.NewReader(body)),
	}

	usage, newAPIError := GeminiChatHandler(c, info, resp)
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 151, usage.PromptTokens)
	require.Greater(t, usage.CompletionTokens, 0)
	require.Equal(t, usage.PromptTokens+usage.CompletionTokens, usage.TotalTokens)
	require.NotNil(t, usage.BillingUsage)
	require.True(t, usage.BillingUsage.Estimated)
}

func TestGeminiStreamHandlerEmptyUsageMetadataBuildsEstimatedBillingUsage(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	oldStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 300
	t.Cleanup(func() {
		constant.StreamingTimeout = oldStreamingTimeout
	})

	info := &relaycommon.RelayInfo{
		OriginModelName: "gemini-3-flash-preview",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3-flash-preview",
		},
	}
	info.SetEstimatePromptTokens(20)

	streamBody := []byte("data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"partial\"}]}}],\"usageMetadata\":{}}\n" + "data: [DONE]\n")
	resp := &http.Response{
		Body: io.NopCloser(bytes.NewReader(streamBody)),
	}

	usage, newAPIError := geminiStreamHandler(c, info, resp, func(_ string, _ *dto.GeminiChatResponse) bool {
		return true
	})
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 20, usage.PromptTokens)
	require.NotNil(t, usage.BillingUsage)
	require.True(t, usage.BillingUsage.Estimated)
	require.Equal(t, dto.BillingUsageSourceGeminiChat, usage.BillingUsage.Source)
	require.NotNil(t, usage.BillingUsage.GeminiUsageMetadata)
	require.Equal(t, usage.PromptTokens, usage.BillingUsage.GeminiUsageMetadata.PromptTokenCount)
	require.Equal(t, usage.CompletionTokens, usage.BillingUsage.GeminiUsageMetadata.CandidatesTokenCount)
	require.True(t, common.GetContextKeyBool(c, constant.ContextKeyLocalCountTokens))
}
