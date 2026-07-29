package tencent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

const (
	tencentVODGatewayHost    = "gateway.vod-qcloud.com"
	tencentVODLegacyHost     = "vod.tencentcloudapi.com"
	tencentVODService        = "vod"
	tencentVODVersion        = "2018-07-17"
	tencentVODSubmitAction   = "CreateAigcImageTask"
	tencentVODDescribeAction = "DescribeTaskDetail"
	tencentVODPollInterval   = 5 * time.Second
	tencentVODPollTimeout    = 10 * time.Minute
	tencentVODModelName      = "OG"
)

type tencentVODImageRequest struct {
	SubAppID     int64                       `json:"SubAppId"`
	ModelName    string                      `json:"ModelName"`
	ModelVersion string                      `json:"ModelVersion"`
	FileInfos    []tencentVODImageInputFile  `json:"FileInfos,omitempty"`
	Prompt       string                      `json:"Prompt"`
	OutputConfig tencentVODImageOutputConfig `json:"OutputConfig"`
	ExtInfo      string                      `json:"ExtInfo,omitempty"`
}

type tencentVODImageInputFile struct {
	Type          string `json:"Type"`
	URL           string `json:"Url,omitempty"`
	Base64        string `json:"Base64,omitempty"`
	ReferenceType string `json:"ReferenceType,omitempty"`
}

type tencentVODImageOutputConfig struct {
	StorageMode      string `json:"StorageMode"`
	Resolution       string `json:"Resolution,omitempty"`
	AspectRatio      string `json:"AspectRatio,omitempty"`
	OutputImageCount uint   `json:"OutputImageCount,omitempty"`
	OutputFormat     string `json:"OutputFormat,omitempty"`
}

type tencentVODAPIError struct {
	Code    string `json:"Code,omitempty"`
	Message string `json:"Message,omitempty"`
}

type tencentVODSubmitResponse struct {
	TaskID   string              `json:"TaskId,omitempty"`
	Error    *tencentVODAPIError `json:"Error,omitempty"`
	Response struct {
		TaskID string              `json:"TaskId,omitempty"`
		Error  *tencentVODAPIError `json:"Error,omitempty"`
	} `json:"Response,omitempty"`
}

type tencentVODDescribeRequest struct {
	TaskID   string `json:"TaskId"`
	SubAppID int64  `json:"SubAppId"`
}

type tencentVODDescribeResponse struct {
	AigcImageTask *tencentVODImageTask `json:"AigcImageTask,omitempty"`
	Error         *tencentVODAPIError  `json:"Error,omitempty"`
	Response      struct {
		AigcImageTask *tencentVODImageTask `json:"AigcImageTask,omitempty"`
		Error         *tencentVODAPIError  `json:"Error,omitempty"`
	} `json:"Response"`
}

type tencentVODImageTask struct {
	ErrCode    int                       `json:"ErrCode,omitempty"`
	ErrCodeExt string                    `json:"ErrCodeExt,omitempty"`
	Message    string                    `json:"Message,omitempty"`
	Status     string                    `json:"Status,omitempty"`
	TaskID     string                    `json:"TaskId,omitempty"`
	Output     tencentVODImageTaskOutput `json:"Output,omitempty"`
}

type tencentVODImageTaskOutput struct {
	FileInfos []tencentVODImageOutputFile `json:"FileInfos,omitempty"`
}

type tencentVODImageOutputFile struct {
	FileURL string `json:"FileUrl,omitempty"`
}

func isTencentVODImageGeneration(info *relaycommon.RelayInfo) bool {
	if info == nil || info.RelayMode != relayconstant.RelayModeImagesGenerations || info.ChannelMeta == nil {
		return false
	}
	u, err := url.Parse(info.ChannelBaseUrl)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == tencentVODGatewayHost || host == tencentVODLegacyHost
}

func tencentVODImageRequestFromOpenAI(request dto.ImageRequest, info *relaycommon.RelayInfo) (*tencentVODImageRequest, error) {
	if strings.TrimSpace(request.Prompt) == "" {
		return nil, errors.New("prompt is required")
	}

	subAppID, err := tencentVODSubAppID(info)
	if err != nil {
		return nil, err
	}
	quality := strings.ToLower(strings.TrimSpace(request.Quality))
	if quality == "" || quality == "auto" || quality == "standard" {
		quality = "medium"
	}
	if quality == "hd" {
		quality = "high"
	}
	if quality != "low" && quality != "medium" && quality != "high" {
		return nil, fmt.Errorf("unsupported Tencent VOD image quality %q", request.Quality)
	}

	imageCount := uint(1)
	if request.N != nil {
		imageCount = *request.N
	}
	if imageCount < 1 || imageCount > 8 {
		return nil, fmt.Errorf("Tencent VOD gpt-image-2 supports n between 1 and 8")
	}

	extraParameters := make(map[string]any)
	size := strings.TrimSpace(request.Size)
	if size == "" {
		size = "auto"
	}
	if err := validateTencentVODImageSize(size); err != nil {
		return nil, err
	}
	extraParameters["size"] = size

	background, err := rawJSONString(request.Background)
	if err != nil {
		return nil, fmt.Errorf("invalid background: %w", err)
	}
	if background != "" && background != "auto" {
		extraParameters["background"] = background
	}

	outputFormat, err := rawJSONString(request.OutputFormat)
	if err != nil {
		return nil, fmt.Errorf("invalid output_format: %w", err)
	}
	if outputFormat != "" && outputFormat != "png" && outputFormat != "jpeg" {
		return nil, fmt.Errorf("Tencent VOD gpt-image-2 output_format must be png or jpeg")
	}

	fileInfos, err := tencentVODImageInputs(request.Images, request.Image)
	if err != nil {
		return nil, err
	}
	extInfo, err := buildTencentVODExtInfo(extraParameters)
	if err != nil {
		return nil, err
	}

	return &tencentVODImageRequest{
		SubAppID:     subAppID,
		ModelName:    tencentVODModelName,
		ModelVersion: "image2_" + quality,
		FileInfos:    fileInfos,
		Prompt:       request.Prompt,
		OutputConfig: tencentVODImageOutputConfig{
			StorageMode:      "Temporary",
			OutputImageCount: imageCount,
			OutputFormat:     outputFormat,
		},
		ExtInfo: extInfo,
	}, nil
}

func tencentVODSubAppID(info *relaycommon.RelayInfo) (int64, error) {
	if info == nil || info.ChannelMeta == nil {
		return 0, errors.New("Tencent VOD channel metadata is missing")
	}
	apiKey := strings.TrimPrefix(strings.TrimSpace(info.ChannelMeta.ApiKey), "Bearer ")
	appID, _, _, err := parseTencentConfig(apiKey)
	if err != nil {
		return 0, fmt.Errorf("invalid Tencent VOD key, expected SubAppId|SecretId|SecretKey: %w", err)
	}
	return appID, nil
}

func validateTencentVODImageSize(size string) error {
	if size == "auto" {
		return nil
	}
	parts := strings.Split(strings.ToLower(size), "x")
	if len(parts) != 2 {
		return fmt.Errorf("Tencent VOD gpt-image-2 size must be auto or WIDTHxHEIGHT")
	}
	width, err := strconv.Atoi(parts[0])
	if err != nil {
		return fmt.Errorf("invalid Tencent VOD image width: %w", err)
	}
	height, err := strconv.Atoi(parts[1])
	if err != nil {
		return fmt.Errorf("invalid Tencent VOD image height: %w", err)
	}
	pixels := int64(width) * int64(height)
	if width <= 0 || height <= 0 || width%16 != 0 || height%16 != 0 || max(width, height) > 3840 || pixels < 655360 || pixels > 8294400 {
		return fmt.Errorf("Tencent VOD gpt-image-2 size must use multiples of 16, longest edge <= 3840, and 655360-8294400 total pixels")
	}
	return nil
}

func buildTencentVODExtInfo(parameters map[string]any) (string, error) {
	additional, err := common.Marshal(parameters)
	if err != nil {
		return "", err
	}
	extInfo, err := common.Marshal(map[string]string{"AdditionalParameters": string(additional)})
	if err != nil {
		return "", err
	}
	return string(extInfo), nil
}

func rawJSONString(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var value string
	if err := common.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	return strings.ToLower(strings.TrimSpace(value)), nil
}

func tencentVODImageInputs(rawValues ...json.RawMessage) ([]tencentVODImageInputFile, error) {
	inputs := make([]tencentVODImageInputFile, 0)
	for _, raw := range rawValues {
		if len(raw) == 0 || string(raw) == "null" {
			continue
		}
		var value any
		if err := common.Unmarshal(raw, &value); err != nil {
			return nil, fmt.Errorf("invalid Tencent VOD image input: %w", err)
		}
		if err := appendTencentVODImageInputs(&inputs, value); err != nil {
			return nil, err
		}
	}
	if len(inputs) > 16 {
		return nil, fmt.Errorf("Tencent VOD gpt-image-2 supports at most 16 reference images")
	}
	return inputs, nil
}

func appendTencentVODImageInputs(inputs *[]tencentVODImageInputFile, value any) error {
	switch typed := value.(type) {
	case string:
		input, err := tencentVODImageInputFromString(typed)
		if err != nil {
			return err
		}
		*inputs = append(*inputs, input)
	case []any:
		for _, item := range typed {
			if err := appendTencentVODImageInputs(inputs, item); err != nil {
				return err
			}
		}
	case map[string]any:
		for _, key := range []string{"url", "image_url", "base64", "b64_json"} {
			if child, ok := typed[key]; ok {
				if nested, ok := child.(map[string]any); ok {
					child = nested["url"]
				}
				return appendTencentVODImageInputs(inputs, child)
			}
		}
		return errors.New("Tencent VOD image input object must contain url, image_url, base64, or b64_json")
	default:
		return fmt.Errorf("unsupported Tencent VOD image input type %T", value)
	}
	return nil
}

func tencentVODImageInputFromString(value string) (tencentVODImageInputFile, error) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return tencentVODImageInputFile{Type: "Url", URL: value}, nil
	}
	if strings.HasPrefix(value, "data:") {
		comma := strings.Index(value, ",")
		if comma < 0 || comma == len(value)-1 {
			return tencentVODImageInputFile{}, errors.New("invalid data URL in Tencent VOD image input")
		}
		return tencentVODImageInputFile{Type: "Base64", Base64: value[comma+1:]}, nil
	}
	return tencentVODImageInputFile{Type: "Base64", Base64: value}, nil
}

func (a *Adaptor) doTencentVODImageRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	body, err := io.ReadAll(requestBody)
	if err != nil {
		return nil, fmt.Errorf("read Tencent VOD image request body failed: %w", err)
	}
	apiKey := tencentAIArtAPIKey(c, info)
	subAppID, secretID, secretKey, err := parseTencentConfig(apiKey)
	if err != nil {
		return nil, fmt.Errorf("invalid Tencent VOD key, expected SubAppId|SecretId|SecretKey: %w", err)
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), tencentVODPollTimeout)
	defer cancel()

	submitBody, err := a.tencentVODPost(ctx, c, info, tencentVODSubmitAction, body, secretID, secretKey)
	if err != nil {
		return nil, err
	}
	taskID, err := parseTencentVODSubmitTaskID(submitBody)
	if err != nil {
		return nil, err
	}

	ticker := time.NewTicker(tencentVODPollInterval)
	defer ticker.Stop()
	for {
		describeBody, err := common.Marshal(tencentVODDescribeRequest{TaskID: taskID, SubAppID: subAppID})
		if err != nil {
			return nil, err
		}
		resultBody, err := a.tencentVODPost(ctx, c, info, tencentVODDescribeAction, describeBody, secretID, secretKey)
		if err != nil {
			return nil, err
		}
		if done, err := tencentVODImageTaskDone(resultBody); done || err != nil {
			if err != nil {
				return nil, err
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewReader(resultBody)),
			}, nil
		}

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("Tencent VOD image task %s timed out after %s", taskID, tencentVODPollTimeout)
		case <-ticker.C:
		}
	}
}

func (a *Adaptor) tencentVODPost(ctx context.Context, c *gin.Context, info *relaycommon.RelayInfo, action string, body []byte, secretID, secretKey string) ([]byte, error) {
	endpoint, err := tencentVODEndpoint(info.ChannelBaseUrl, action)
	if err != nil {
		return nil, err
	}
	req, err := newTencentVODRequest(ctx, endpoint, action, body, secretID, secretKey, common.GetTimestamp())
	if err != nil {
		return nil, err
	}

	resp, err := channel.DoRequest(c, req, info)
	if err != nil {
		return nil, err
	}
	defer service.CloseResponseBodyGracefully(resp)
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Tencent VOD %s failed with status %d: %s", action, resp.StatusCode, string(responseBody))
	}
	return responseBody, nil
}

func tencentVODEndpoint(baseURL, action string) (string, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/"
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid Tencent VOD base URL %q", baseURL)
	}
	// Fy-api overlay: vod-gateway accepts AIGC submission but rejects the
	// standard DescribeTaskDetail action. Poll through the public VOD API.
	if action == tencentVODDescribeAction && strings.EqualFold(u.Hostname(), tencentVODGatewayHost) {
		u.Scheme = "https"
		u.Host = tencentVODLegacyHost
		u.Path = "/"
		u.RawPath = ""
		u.RawQuery = ""
		u.Fragment = ""
	}
	return u.String(), nil
}

func newTencentVODRequest(ctx context.Context, endpoint, action string, body []byte, secretID, secretKey string, timestamp int64) (*http.Request, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	host := u.Host
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-TC-Action", action)
	req.Header.Set("X-TC-Version", tencentVODVersion)
	req.Header.Set("X-TC-Timestamp", strconv.FormatInt(timestamp, 10))
	req.Header.Set("Authorization", buildTencentTC3Authorization(tencentTC3SignInput{
		SecretID:  secretID,
		SecretKey: secretKey,
		Service:   tencentVODService,
		Host:      host,
		Action:    action,
		Timestamp: timestamp,
		Payload:   body,
	}))
	return req, nil
}

func parseTencentVODSubmitTaskID(body []byte) (string, error) {
	var result tencentVODSubmitResponse
	if err := common.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("decode Tencent VOD submit response failed: %w", err)
	}
	if result.Error != nil {
		return "", fmt.Errorf("%s: %s", result.Error.Code, result.Error.Message)
	}
	if result.Response.Error != nil {
		return "", fmt.Errorf("%s: %s", result.Response.Error.Code, result.Response.Error.Message)
	}
	if result.TaskID != "" {
		return result.TaskID, nil
	}
	if result.Response.TaskID != "" {
		return result.Response.TaskID, nil
	}
	return "", errors.New("Tencent VOD submit response missing TaskId")
}

func tencentVODImageTaskDone(body []byte) (bool, error) {
	var result tencentVODDescribeResponse
	if err := common.Unmarshal(body, &result); err != nil {
		return false, fmt.Errorf("decode Tencent VOD task response failed: %w", err)
	}
	if apiErr := result.apiError(); apiErr != nil {
		return false, fmt.Errorf("%s: %s", apiErr.Code, apiErr.Message)
	}
	task := result.imageTask()
	if task == nil {
		return false, errors.New("Tencent VOD task response missing AigcImageTask")
	}
	switch strings.ToUpper(strings.TrimSpace(task.Status)) {
	case "FINISH", "SUCCESS", "SUCCEEDED":
		if task.ErrCode != 0 {
			return false, tencentVODTaskError(task)
		}
		return true, nil
	case "FAIL", "FAILED", "ERROR":
		return false, tencentVODTaskError(task)
	default:
		return false, nil
	}
}

func tencentVODTaskError(task *tencentVODImageTask) error {
	message := strings.TrimSpace(task.Message)
	if message == "" {
		message = strings.TrimSpace(task.ErrCodeExt)
	}
	if message == "" {
		message = "Tencent VOD image task failed"
	}
	if task.ErrCode != 0 {
		return fmt.Errorf("Tencent VOD image task failed (%d): %s", task.ErrCode, message)
	}
	return errors.New(message)
}

func writeTencentVODImageResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	service.CloseResponseBodyGracefully(resp)

	var result tencentVODDescribeResponse
	if err := common.Unmarshal(responseBody, &result); err != nil {
		return nil, types.NewOpenAIError(fmt.Errorf("decode Tencent VOD image response failed: %w", err), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if apiErr := result.apiError(); apiErr != nil {
		return nil, types.WithOpenAIError(types.OpenAIError{
			Message: apiErr.Message,
			Type:    "tencent_vod_error",
			Code:    apiErr.Code,
		}, http.StatusBadGateway)
	}
	task := result.imageTask()
	if task == nil {
		return nil, types.NewOpenAIError(errors.New("Tencent VOD image response missing AigcImageTask"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}

	imageURLs := make([]string, 0, len(task.Output.FileInfos))
	for _, file := range task.Output.FileInfos {
		if strings.TrimSpace(file.FileURL) != "" {
			imageURLs = append(imageURLs, file.FileURL)
		}
	}
	if len(imageURLs) == 0 {
		return nil, types.NewOpenAIError(errors.New("Tencent VOD image response contains no images"), types.ErrorCodeEmptyResponse, http.StatusBadGateway)
	}

	wantsBase64 := true
	if info != nil {
		if imageReq, ok := info.Request.(*dto.ImageRequest); ok {
			wantsBase64 = imageReq.ResponseFormat != "url"
		}
		if info.PriceData.UsePrice {
			info.PriceData.AddOtherRatio("n", float64(len(imageURLs)))
		}
	}
	imageResponse := dto.ImageResponse{Created: common.GetTimestamp(), Data: make([]dto.ImageData, 0, len(imageURLs))}
	if info != nil && !info.StartTime.IsZero() {
		imageResponse.Created = info.StartTime.Unix()
	}
	for _, imageURL := range imageURLs {
		if wantsBase64 {
			_, b64, err := service.GetImageFromUrl(imageURL)
			if err != nil {
				return nil, types.NewOpenAIError(fmt.Errorf("download Tencent VOD image failed: %w", err), types.ErrorCodeBadResponse, http.StatusBadGateway)
			}
			imageResponse.Data = append(imageResponse.Data, dto.ImageData{B64Json: b64})
		} else {
			imageResponse.Data = append(imageResponse.Data, dto.ImageData{Url: imageURL})
		}
	}

	jsonResponse, err := common.Marshal(imageResponse)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(http.StatusOK)
	_, _ = c.Writer.Write(jsonResponse)
	return &dto.Usage{}, nil
}

func (r *tencentVODDescribeResponse) imageTask() *tencentVODImageTask {
	if r == nil {
		return nil
	}
	if r.Response.AigcImageTask != nil {
		return r.Response.AigcImageTask
	}
	return r.AigcImageTask
}

func (r *tencentVODDescribeResponse) apiError() *tencentVODAPIError {
	if r == nil {
		return nil
	}
	if r.Response.Error != nil {
		return r.Response.Error
	}
	return r.Error
}
