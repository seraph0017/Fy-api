package relay

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsResponsesEventStreamContentType(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		want        bool
	}{
		{name: "plain", contentType: "text/event-stream", want: true},
		{name: "mixed case with charset", contentType: "Text/Event-Stream; charset=utf-8", want: true},
		{name: "json", contentType: "application/json", want: false},
		{name: "empty", contentType: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isResponsesEventStreamContentType(tt.contentType))
		})
	}
}

func TestChatCompletionsViaResponsesRetriesWithOriginalRequestJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set(common.RequestIdKey, "chat-responses-retry-test")

	adaptor := &chatResponsesRetryAdaptor{}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeOpenAI,
			ChannelBaseUrl:    "https://api.openai.test",
			ApiKey:            "sk-test",
			UpstreamModelName: "gpt-test",
		},
		OriginModelName: "gpt-test",
		RelayMode:       relayconstant.RelayModeChatCompletions,
		RequestURLPath:  "/v1/chat/completions",
		RelayFormat:     types.RelayFormatOpenAI,
	}
	request := &dto.GeneralOpenAIRequest{
		Model: "gpt-test",
		Messages: []dto.Message{
			{Role: "user", Content: "hello"},
		},
	}

	usage, apiErr := chatCompletionsViaResponses(c, info, adaptor, request)

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 2, adaptor.doRequestCalls)
	require.Len(t, adaptor.requestBodies, 2)
	assert.Contains(t, adaptor.requestBodies[0], "encrypted_content")
	assert.NotContains(t, adaptor.requestBodies[1], "encrypted_content")
}

type chatResponsesRetryAdaptor struct {
	channel.Adaptor
	doRequestCalls int
	requestBodies  []string
}

func (a *chatResponsesRetryAdaptor) Init(_ *relaycommon.RelayInfo) {}

func (a *chatResponsesRetryAdaptor) GetRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return "https://api.openai.test/v1/responses", nil
}

func (a *chatResponsesRetryAdaptor) SetupRequestHeader(_ *gin.Context, _ *http.Header, _ *relaycommon.RelayInfo) error {
	return nil
}

func (a *chatResponsesRetryAdaptor) ConvertOpenAIRequest(_ *gin.Context, _ *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	return request, nil
}

func (a *chatResponsesRetryAdaptor) ConvertRerankRequest(_ *gin.Context, _ int, request dto.RerankRequest) (any, error) {
	return request, nil
}

func (a *chatResponsesRetryAdaptor) ConvertEmbeddingRequest(_ *gin.Context, _ *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return request, nil
}

func (a *chatResponsesRetryAdaptor) ConvertAudioRequest(_ *gin.Context, _ *relaycommon.RelayInfo, _ dto.AudioRequest) (io.Reader, error) {
	return nil, nil
}

func (a *chatResponsesRetryAdaptor) ConvertImageRequest(_ *gin.Context, _ *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	return request, nil
}

func (a *chatResponsesRetryAdaptor) ConvertOpenAIResponsesRequest(_ *gin.Context, _ *relaycommon.RelayInfo, _ dto.OpenAIResponsesRequest) (any, error) {
	return map[string]any{
		"model": "gpt-test",
		"input": []any{
			map[string]any{"role": "user", "content": "hello"},
			map[string]any{"type": "reasoning", "encrypted_content": "gAAA-stale"},
		},
	}, nil
}

func (a *chatResponsesRetryAdaptor) DoRequest(_ *gin.Context, _ *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	a.doRequestCalls++
	raw, err := io.ReadAll(requestBody)
	if err != nil {
		return nil, err
	}
	a.requestBodies = append(a.requestBodies, string(raw))
	if a.doRequestCalls == 1 {
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"The encrypted content gAAA-stale could not be verified. Reason: Encrypted content could not be decrypted or parsed.","type":"invalid_request_error","code":"invalid_request_error"}}`)),
			Header:     http.Header{"Content-Type": []string{"application/json"}},
		}, nil
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"id":"resp_1","model":"gpt-test","output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}, nil
}

func (a *chatResponsesRetryAdaptor) DoResponse(_ *gin.Context, _ *http.Response, _ *relaycommon.RelayInfo) (any, *types.NewAPIError) {
	return &dto.Usage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2}, nil
}

func (a *chatResponsesRetryAdaptor) GetModelList() []string {
	return nil
}

func (a *chatResponsesRetryAdaptor) GetChannelName() string {
	return "chat-responses-retry-test"
}

func (a *chatResponsesRetryAdaptor) ConvertClaudeRequest(_ *gin.Context, _ *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	return request, nil
}

func (a *chatResponsesRetryAdaptor) ConvertGeminiRequest(_ *gin.Context, _ *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	return request, nil
}
