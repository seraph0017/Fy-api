package tencent

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsTencentVODImageGeneration(t *testing.T) {
	for _, baseURL := range []string{
		"https://gateway.vod-qcloud.com",
		"https://vod.tencentcloudapi.com/",
	} {
		info := &relaycommon.RelayInfo{
			RelayMode:   relayconstant.RelayModeImagesGenerations,
			ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: baseURL},
		}
		assert.True(t, isTencentVODImageGeneration(info), baseURL)
	}

	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeImagesGenerations,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://aiart.tencentcloudapi.com"},
	}
	assert.False(t, isTencentVODImageGeneration(info))
	info.RelayMode = relayconstant.RelayModeChatCompletions
	info.ChannelMeta.ChannelBaseUrl = "https://gateway.vod-qcloud.com"
	assert.False(t, isTencentVODImageGeneration(info))
}

func TestTencentVODImageRequestFromOpenAI(t *testing.T) {
	n := uint(3)
	request := dto.ImageRequest{
		Model:        "gpt-image-2",
		Prompt:       "a glass tea house in the mountains",
		N:            &n,
		Size:         "1536x1024",
		Quality:      "high",
		Background:   json.RawMessage(`"transparent"`),
		OutputFormat: json.RawMessage(`"png"`),
		Images: json.RawMessage(`[
			"https://example.com/reference.png",
			{"image_url":{"url":"data:image/png;base64,aGVsbG8="}}
		]`),
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{ApiKey: "1500044236|sid|skey"},
	}

	got, err := tencentVODImageRequestFromOpenAI(request, info)
	require.NoError(t, err)
	assert.Equal(t, int64(1500044236), got.SubAppID)
	assert.Equal(t, "OG", got.ModelName)
	assert.Equal(t, "image2_high", got.ModelVersion)
	assert.Equal(t, uint(3), got.OutputConfig.OutputImageCount)
	assert.Equal(t, "png", got.OutputConfig.OutputFormat)
	require.Len(t, got.FileInfos, 2)
	assert.Equal(t, tencentVODImageInputFile{Type: "Url", URL: "https://example.com/reference.png"}, got.FileInfos[0])
	assert.Equal(t, tencentVODImageInputFile{Type: "Base64", Base64: "aGVsbG8="}, got.FileInfos[1])

	var extInfo map[string]string
	require.NoError(t, common.UnmarshalJsonStr(got.ExtInfo, &extInfo))
	var additional map[string]any
	require.NoError(t, common.UnmarshalJsonStr(extInfo["AdditionalParameters"], &additional))
	assert.Equal(t, "1536x1024", additional["size"])
	assert.Equal(t, "transparent", additional["background"])
}

func TestTencentVODImageRequestDefaultsAndValidation(t *testing.T) {
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ApiKey: "123|sid|skey"}}
	request := dto.ImageRequest{Prompt: "hello", Quality: "auto"}
	got, err := tencentVODImageRequestFromOpenAI(request, info)
	require.NoError(t, err)
	assert.Equal(t, "image2_medium", got.ModelVersion)
	assert.Equal(t, uint(1), got.OutputConfig.OutputImageCount)

	var extInfo map[string]string
	require.NoError(t, common.UnmarshalJsonStr(got.ExtInfo, &extInfo))
	assert.Contains(t, extInfo["AdditionalParameters"], `"size":"auto"`)

	nine := uint(9)
	_, err = tencentVODImageRequestFromOpenAI(dto.ImageRequest{Prompt: "hello", N: &nine}, info)
	assert.ErrorContains(t, err, "between 1 and 8")
	_, err = tencentVODImageRequestFromOpenAI(dto.ImageRequest{Prompt: "hello", Size: "512x512"}, info)
	assert.ErrorContains(t, err, "655360-8294400")
	_, err = tencentVODImageRequestFromOpenAI(dto.ImageRequest{Prompt: "hello", OutputFormat: json.RawMessage(`"webp"`)}, info)
	assert.ErrorContains(t, err, "png or jpeg")
}

func TestAdaptorConvertImageRequestUsesTencentVODForGatewayHost(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: "https://gateway.vod-qcloud.com",
			ApiKey:         "123|sid|skey",
		},
	}
	converted, err := (&Adaptor{}).ConvertImageRequest(
		gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New()),
		info,
		dto.ImageRequest{Prompt: "hello", Quality: "low"},
	)
	require.NoError(t, err)
	request, ok := converted.(*tencentVODImageRequest)
	require.True(t, ok)
	assert.Equal(t, "image2_low", request.ModelVersion)
}

func TestTencentVODEndpointAndSignatureUseActionSpecificHost(t *testing.T) {
	const (
		baseURL   = "https://gateway.vod-qcloud.com"
		secretID  = "sid"
		secretKey = "skey"
		timestamp = int64(1700000000)
	)
	body := []byte(`{"TaskId":"task-1","SubAppId":1500044236}`)

	tests := []struct {
		name     string
		action   string
		wantHost string
	}{
		{name: "submit through AIGC gateway", action: tencentVODSubmitAction, wantHost: tencentVODGatewayHost},
		{name: "poll through public VOD API", action: tencentVODDescribeAction, wantHost: tencentVODLegacyHost},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoint, err := tencentVODEndpoint(baseURL, tt.action)
			require.NoError(t, err)
			req, err := newTencentVODRequest(t.Context(), endpoint, tt.action, body, secretID, secretKey, timestamp)
			require.NoError(t, err)

			assert.Equal(t, tt.wantHost, req.URL.Host)
			assert.Equal(t, tt.action, req.Header.Get("X-TC-Action"))
			assert.Equal(t, buildTencentTC3Authorization(tencentTC3SignInput{
				SecretID:  secretID,
				SecretKey: secretKey,
				Service:   tencentVODService,
				Host:      tt.wantHost,
				Action:    tt.action,
				Timestamp: timestamp,
				Payload:   body,
			}), req.Header.Get("Authorization"))
		})
	}
}

func TestTencentVODDoRequestSubmitsPollsAndConverts(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := atomic.AddInt32(&calls, 1)
		assert.Contains(t, r.Header.Get("Authorization"), "/vod/tc3_request")
		assert.Equal(t, tencentVODVersion, r.Header.Get("X-TC-Version"))
		switch call {
		case 1:
			assert.Equal(t, tencentVODSubmitAction, r.Header.Get("X-TC-Action"))
			_, _ = w.Write([]byte(`{"Response":{"TaskId":"1500044236-AigcImageTask-task-1"}}`))
		case 2:
			assert.Equal(t, tencentVODDescribeAction, r.Header.Get("X-TC-Action"))
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			assert.JSONEq(t, `{"TaskId":"1500044236-AigcImageTask-task-1","SubAppId":1500044236}`, string(body))
			_, _ = w.Write([]byte(`{"Response":{"AigcImageTask":{"ErrCode":0,"Status":"FINISH","Output":{"FileInfos":[{"FileUrl":"https://example.com/final.png"}]}}}}`))
		default:
			t.Errorf("unexpected call %d", call)
		}
	}))
	defer server.Close()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	common.SetContextKey(c, constant.ContextKeyChannelKey, "1500044236|sid|skey")
	service.InitHttpClient()

	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeImagesGenerations,
		StartTime:   time.Unix(1700000000, 0),
		Request:     &dto.ImageRequest{ResponseFormat: "url"},
		ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: server.URL},
	}
	resp, err := (&Adaptor{}).doTencentVODImageRequest(c, info, strings.NewReader(`{"SubAppId":1500044236}`))
	require.NoError(t, err)
	usage, apiErr := writeTencentVODImageResponse(c, resp, info)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, int32(2), atomic.LoadInt32(&calls))
	assert.JSONEq(t, `{"created":1700000000,"data":[{"url":"https://example.com/final.png","b64_json":"","revised_prompt":""}]}`, recorder.Body.String())
}

func TestTencentVODImageTaskFailure(t *testing.T) {
	done, err := tencentVODImageTaskDone([]byte(`{
		"Response":{"AigcImageTask":{"ErrCode":1001,"ErrCodeExt":"moderation blocked","Status":"FAIL"}}
	}`))
	assert.False(t, done)
	assert.ErrorContains(t, err, "moderation blocked")
}

func TestTencentVODImageTaskAcceptsTopLevelSDKShape(t *testing.T) {
	done, err := tencentVODImageTaskDone([]byte(`{
		"AigcImageTask":{"ErrCode":0,"Status":"FINISH","Output":{"FileInfos":[{"FileUrl":"https://example.com/image.png"}]}}
	}`))
	require.NoError(t, err)
	assert.True(t, done)
}
