package gemini

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/relayconvert"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/reasoning"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

type Adaptor struct {
}

func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	if len(request.Contents) > 0 {
		for i, content := range request.Contents {
			if i == 0 {
				if request.Contents[0].Role == "" {
					request.Contents[0].Role = "user"
				}
			}
			for _, part := range content.Parts {
				if part.FileData != nil {
					if part.FileData.MimeType == "" && strings.Contains(part.FileData.FileUri, "www.youtube.com") {
						part.FileData.MimeType = "video/webm"
					}
				}
			}
		}
	}
	return request, nil
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, req *dto.ClaudeRequest) (any, error) {
	adaptor := openai.Adaptor{}
	oaiReq, err := adaptor.ConvertClaudeRequest(c, info, req)
	if err != nil {
		return nil, err
	}
	return a.ConvertOpenAIRequest(c, info, oaiReq.(*dto.GeneralOpenAIRequest))
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	if isGeminiImagePreviewModel(info.UpstreamModelName) {
		return a.convertGeminiImagePreviewRequest(c, info, request)
	}
	if !strings.HasPrefix(info.UpstreamModelName, "imagen") {
		return nil, errors.New("not supported model for image generation, only imagen models are supported")
	}

	// convert size to aspect ratio but allow user to specify aspect ratio
	aspectRatio := "1:1" // default aspect ratio
	size := strings.TrimSpace(request.Size)
	if size != "" {
		if strings.Contains(size, ":") {
			aspectRatio = size
		} else {
			switch size {
			case "256x256", "512x512", "1024x1024":
				aspectRatio = "1:1"
			case "1536x1024":
				aspectRatio = "3:2"
			case "1024x1536":
				aspectRatio = "2:3"
			case "1024x1792":
				aspectRatio = "9:16"
			case "1792x1024":
				aspectRatio = "16:9"
			}
		}
	}

	// build gemini imagen request
	geminiRequest := dto.GeminiImageRequest{
		Instances: []dto.GeminiImageInstance{
			{
				Prompt: request.Prompt,
			},
		},
		Parameters: dto.GeminiImageParameters{
			SampleCount:      int(lo.FromPtrOr(request.N, uint(1))),
			AspectRatio:      aspectRatio,
			PersonGeneration: "allow_adult", // default allow adult
		},
	}

	// Set imageSize when quality parameter is specified
	// Map quality parameter to imageSize (only supported by Standard and Ultra models)
	// quality values: auto, high, medium, low (for gpt-image-1), hd, standard (for dall-e-3)
	// imageSize values: 1K (default), 2K
	// https://ai.google.dev/gemini-api/docs/imagen
	// https://platform.openai.com/docs/api-reference/images/create
	if request.Quality != "" {
		imageSize := "1K" // default
		switch request.Quality {
		case "hd", "high":
			imageSize = "2K"
		case "2K":
			imageSize = "2K"
		case "standard", "medium", "low", "auto", "1K":
			imageSize = "1K"
		default:
			// unknown quality value, default to 1K
			imageSize = "1K"
		}
		geminiRequest.Parameters.ImageSize = imageSize
	}

	return geminiRequest, nil
}

func isGeminiImagePreviewModel(model string) bool {
	return strings.HasPrefix(model, "gemini-3-pro-image") ||
		strings.HasPrefix(model, "gemini-3.1-flash-image") ||
		strings.HasPrefix(model, "gemini-2.5-flash-image")
}

// Fy-api overlay: route OpenAI image-compatible requests for Gemini
// image-preview models through generateContent, which returns inlineData image
// parts instead of Imagen predictions.
func (a *Adaptor) convertGeminiImagePreviewRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	parts := []dto.GeminiPart{}
	if strings.TrimSpace(request.Prompt) != "" {
		parts = append(parts, dto.GeminiPart{Text: request.Prompt})
	}

	if info.RelayMode == constant.RelayModeImagesEdits {
		hasInputImage := false
		if isMultipartFormRequest(c) {
			if imageParts, err := readMultipartImageParts(c, "image", "image[]"); err != nil {
				return nil, err
			} else if len(imageParts) > 0 {
				hasInputImage = true
				parts = append(parts, imageParts...)
			}
			if maskParts, err := readMultipartImageParts(c, "mask"); err != nil {
				return nil, err
			} else if len(maskParts) > 0 {
				parts = append(parts, dto.GeminiPart{Text: "Use the next image as the edit mask."})
				parts = append(parts, maskParts...)
			}
		}
		if imageParts, err := rawImagePartsFromJSON(request.Image); err != nil {
			return nil, err
		} else if len(imageParts) > 0 {
			hasInputImage = true
			parts = append(parts, imageParts...)
		}
		if imageParts, err := rawImagePartsFromJSON(request.Images); err != nil {
			return nil, err
		} else if len(imageParts) > 0 {
			hasInputImage = true
			parts = append(parts, imageParts...)
		}
		if maskParts, err := rawImagePartsFromJSON(request.Mask); err != nil {
			return nil, err
		} else if len(maskParts) > 0 {
			parts = append(parts, dto.GeminiPart{Text: "Use the next image as the edit mask."})
			parts = append(parts, maskParts...)
		} else if request.Mask != nil && len(request.Mask) > 0 {
			parts = append(parts, dto.GeminiPart{Text: "Apply the provided mask."})
		}
		if !hasInputImage {
			return nil, errors.New("image is required")
		}
	}

	if len(parts) == 0 {
		return nil, errors.New("prompt is required")
	}

	geminiReq := &dto.GeminiChatRequest{
		Contents: []dto.GeminiChatContent{
			{
				Role:  "user",
				Parts: parts,
			},
		},
		GenerationConfig: dto.GeminiChatGenerationConfig{
			ResponseModalities: []string{"TEXT", "IMAGE"},
		},
	}
	if request.N != nil && *request.N > 0 {
		geminiReq.GenerationConfig.CandidateCount = lo.ToPtr(int(*request.N))
	}

	config := processGeminiImageSizeParameters(strings.TrimSpace(request.Size), request.Quality)
	if config.AspectRatio != "" || config.ImageSize != "" {
		imageConfig := make(map[string]any)
		if config.AspectRatio != "" {
			imageConfig["aspectRatio"] = config.AspectRatio
		}
		if config.ImageSize != "" {
			imageConfig["imageSize"] = config.ImageSize
		}
		geminiReq.GenerationConfig.ImageConfig, _ = common.Marshal(imageConfig)
	}
	return geminiReq, nil
}

func isMultipartFormRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	return strings.Contains(c.Request.Header.Get("Content-Type"), "multipart/form-data")
}

func readMultipartImageParts(c *gin.Context, fieldNames ...string) ([]dto.GeminiPart, error) {
	form, err := common.ParseMultipartFormReusable(c)
	if err != nil {
		return nil, fmt.Errorf("failed to parse image edit form request: %w", err)
	}

	fileHeaders := make([]*multipart.FileHeader, 0)
	for _, key := range fieldNames {
		fileHeaders = append(fileHeaders, form.File[key]...)
	}
	if len(fileHeaders) == 0 {
		return nil, nil
	}

	parts := make([]dto.GeminiPart, 0, len(fileHeaders))
	for _, fileHeader := range fileHeaders {
		file, err := fileHeader.Open()
		if err != nil {
			return nil, fmt.Errorf("failed to open image file: %w", err)
		}

		fileBytes, err := io.ReadAll(file)
		_ = file.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read image file: %w", err)
		}

		mimeType := fileHeader.Header.Get("Content-Type")
		if mimeType == "" || mimeType == "application/octet-stream" {
			mimeType = http.DetectContentType(fileBytes)
		}
		if !strings.HasPrefix(mimeType, "image/") {
			mimeType = "image/png"
		}

		parts = append(parts, dto.GeminiPart{
			InlineData: &dto.GeminiInlineData{
				MimeType: mimeType,
				Data:     base64.StdEncoding.EncodeToString(fileBytes),
			},
		})
	}

	return parts, nil
}

func rawImagePartsFromJSON(raw json.RawMessage) ([]dto.GeminiPart, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	var encoded string
	if err := common.Unmarshal(raw, &encoded); err == nil {
		return geminiImagePartFromEncodedString(encoded)
	}

	var encodedList []string
	if err := common.Unmarshal(raw, &encodedList); err != nil {
		return nil, fmt.Errorf("invalid image payload: %w", err)
	}
	parts := make([]dto.GeminiPart, 0, len(encodedList))
	for _, encoded := range encodedList {
		imageParts, err := geminiImagePartFromEncodedString(encoded)
		if err != nil {
			return nil, err
		}
		parts = append(parts, imageParts...)
	}
	return parts, nil
}

func geminiImagePartFromEncodedString(encoded string) ([]dto.GeminiPart, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return nil, nil
	}

	mimeType, cleanBase64, err := service.DecodeBase64FileData(encoded)
	if err != nil {
		return nil, err
	}
	return []dto.GeminiPart{
		{
			InlineData: &dto.GeminiInlineData{
				MimeType: mimeType,
				Data:     cleanBase64,
			},
		},
	}, nil
}

type geminiImageConfig struct {
	AspectRatio string
	ImageSize   string
}

func processGeminiImageSizeParameters(size, quality string) geminiImageConfig {
	config := geminiImageConfig{}
	switch size {
	case "1536x1024":
		config.AspectRatio = "3:2"
	case "1024x1536":
		config.AspectRatio = "2:3"
	case "1024x1792":
		config.AspectRatio = "9:16"
	case "1792x1024":
		config.AspectRatio = "16:9"
	case "2048x2048":
		config.ImageSize = "2K"
	case "4096x4096":
		config.ImageSize = "4K"
	default:
		if strings.Contains(size, ":") {
			config.AspectRatio = size
		}
	}

	switch strings.ToLower(strings.TrimSpace(quality)) {
	case "hd", "high", "2k":
		config.ImageSize = "2K"
	case "4k":
		config.ImageSize = "4K"
	case "standard", "medium", "low", "auto", "1k":
		config.ImageSize = "1K"
	}

	return config
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {

}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {

	if model_setting.GetGeminiSettings().ThinkingAdapterEnabled &&
		!model_setting.ShouldPreserveThinkingSuffix(info.OriginModelName) {
		// 新增逻辑：处理 -thinking-<budget> 格式
		if strings.Contains(info.UpstreamModelName, "-thinking-") {
			parts := strings.Split(info.UpstreamModelName, "-thinking-")
			info.UpstreamModelName = parts[0]
		} else if strings.HasSuffix(info.UpstreamModelName, "-thinking") { // 旧的适配
			info.UpstreamModelName = strings.TrimSuffix(info.UpstreamModelName, "-thinking")
		} else if strings.HasSuffix(info.UpstreamModelName, "-nothinking") {
			info.UpstreamModelName = strings.TrimSuffix(info.UpstreamModelName, "-nothinking")
		} else if baseModel, level, ok := reasoning.TrimEffortSuffix(info.UpstreamModelName); ok && level != "" {
			info.UpstreamModelName = baseModel
		}
	}

	version := model_setting.GetGeminiVersionSetting(info.UpstreamModelName)

	// Fy-api overlay: 原生 Gemini pass-through 入口（/v1beta/... 或 /v1/...）必须沿用
	// 客户端 URL 中的版本号，否则后台 VersionSettings 会把 /v1beta 强制改写成 /v1，
	// 导致 gemini-3-pro-image-preview 等只在 v1beta 暴露的模型返回
	// "is not found for API version v1"。仅在 RelayModeGemini 下生效，
	// 不影响 OpenAI/Claude 兼容入口对版本的统一管理。
	if info.RelayMode == constant.RelayModeGemini {
		if strings.HasPrefix(info.RequestURLPath, "/v1beta/") {
			version = "v1beta"
		} else if strings.HasPrefix(info.RequestURLPath, "/v1/") {
			version = "v1"
		}
	}

	if strings.HasPrefix(info.UpstreamModelName, "imagen") {
		return fmt.Sprintf("%s/%s/models/%s:predict", info.ChannelBaseUrl, version, info.UpstreamModelName), nil
	}

	if strings.HasPrefix(info.UpstreamModelName, "text-embedding") ||
		strings.HasPrefix(info.UpstreamModelName, "embedding") ||
		strings.HasPrefix(info.UpstreamModelName, "gemini-embedding") {
		action := "embedContent"
		if info.IsGeminiBatchEmbedding {
			action = "batchEmbedContents"
		}
		return fmt.Sprintf("%s/%s/models/%s:%s", info.ChannelBaseUrl, version, info.UpstreamModelName, action), nil
	}

	action := "generateContent"
	if info.IsStream {
		action = "streamGenerateContent?alt=sse"
		if info.RelayMode == constant.RelayModeGemini {
			info.DisablePing = true
		}
	}
	return fmt.Sprintf("%s/%s/models/%s:%s", info.ChannelBaseUrl, version, info.UpstreamModelName, action), nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	if isGeminiImagePreviewModel(info.UpstreamModelName) &&
		(info.RelayMode == relayconstant.RelayModeImagesGenerations || info.RelayMode == relayconstant.RelayModeImagesEdits) {
		req.Set("Content-Type", "application/json")
	}
	req.Set("x-goog-api-key", info.ApiKey)
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}

	geminiRequest, err := CovertOpenAI2Gemini(c, *request, info)
	if err != nil {
		return nil, err
	}

	return geminiRequest, nil
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	if request.Input == nil {
		return nil, errors.New("input is required")
	}

	inputs := request.ParseInput()
	if len(inputs) == 0 {
		return nil, errors.New("input is empty")
	}
	// We always build a batch-style payload with `requests`, so ensure we call the
	// batch endpoint upstream to avoid payload/endpoint mismatches.
	info.IsGeminiBatchEmbedding = true
	// process all inputs
	geminiRequests := make([]map[string]interface{}, 0, len(inputs))
	for _, input := range inputs {
		geminiRequest := map[string]interface{}{
			"model": fmt.Sprintf("models/%s", info.UpstreamModelName),
			"content": dto.GeminiChatContent{
				Parts: []dto.GeminiPart{
					{
						Text: input,
					},
				},
			},
		}

		// set specific parameters for different models
		// https://ai.google.dev/api/embeddings?hl=zh-cn#method:-models.embedcontent
		switch info.UpstreamModelName {
		case "text-embedding-004", "gemini-embedding-exp-03-07", "gemini-embedding-001":
			// Only newer models introduced after 2024 support OutputDimensionality
			dimensions := lo.FromPtrOr(request.Dimensions, 0)
			if dimensions > 0 {
				geminiRequest["outputDimensionality"] = dimensions
			}
		}
		geminiRequests = append(geminiRequests, geminiRequest)
	}

	return map[string]interface{}{
		"requests": geminiRequests,
	}, nil
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	request, err := preprocessGeminiOpenAIResponsesRequest(request)
	if err != nil {
		return nil, err
	}

	chatRequest, err := relayconvert.ResponsesRequestToChatCompletionsRequest(&request)
	if err != nil {
		return nil, err
	}

	return a.ConvertOpenAIRequest(c, info, chatRequest)
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	if info.RelayMode == constant.RelayModeResponses {
		if info.IsStream {
			return GeminiResponsesStreamHandler(c, info, resp)
		}
		return GeminiResponsesHandler(c, info, resp)
	}

	if info.RelayMode == constant.RelayModeGemini {
		if strings.Contains(info.RequestURLPath, ":embedContent") ||
			strings.Contains(info.RequestURLPath, ":batchEmbedContents") {
			return NativeGeminiEmbeddingHandler(c, resp, info)
		}
		if info.IsStream {
			return GeminiTextGenerationStreamHandler(c, info, resp)
		} else {
			return GeminiTextGenerationHandler(c, info, resp)
		}
	}

	if strings.HasPrefix(info.UpstreamModelName, "imagen") {
		return GeminiImageHandler(c, info, resp)
	}
	if isGeminiImagePreviewModel(info.UpstreamModelName) {
		return ChatImageHandler(c, info, resp)
	}

	// check if the model is an embedding model
	if strings.HasPrefix(info.UpstreamModelName, "text-embedding") ||
		strings.HasPrefix(info.UpstreamModelName, "embedding") ||
		strings.HasPrefix(info.UpstreamModelName, "gemini-embedding") {
		return GeminiEmbeddingHandler(c, info, resp)
	}

	if info.IsStream {
		return GeminiChatStreamHandler(c, info, resp)
	} else {
		return GeminiChatHandler(c, info, resp)
	}

}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
