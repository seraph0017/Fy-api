package deepseek

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestApplyDeepSeekV4OpenAIThinkingSuffix(t *testing.T) {
	t.Parallel()

	t.Run("nothink alias maps to disabled thinking", func(t *testing.T) {
		t.Parallel()

		info := &relaycommon.RelayInfo{
			ChannelMeta: &relaycommon.ChannelMeta{
				UpstreamModelName: "deepseek-v4-pro-nothink",
			},
		}
		request := &dto.GeneralOpenAIRequest{Model: "deepseek-v4-pro-nothink"}

		err := applyDeepSeekV4OpenAIThinkingSuffix(info, request)
		require.NoError(t, err)
		require.Equal(t, "deepseek-v4-pro", request.Model)
		require.Equal(t, "deepseek-v4-pro", info.UpstreamModelName)
		require.Empty(t, request.ReasoningEffort)
		require.Empty(t, info.ReasoningEffort)

		var thinking map[string]string
		err = common.Unmarshal(request.THINKING, &thinking)
		require.NoError(t, err)
		require.Equal(t, "disabled", thinking["type"])
	})

	t.Run("max suffix keeps max effort", func(t *testing.T) {
		t.Parallel()

		info := &relaycommon.RelayInfo{
			ChannelMeta: &relaycommon.ChannelMeta{
				UpstreamModelName: "deepseek-v4-flash-max",
			},
		}
		request := &dto.GeneralOpenAIRequest{Model: "deepseek-v4-flash-max"}

		err := applyDeepSeekV4OpenAIThinkingSuffix(info, request)
		require.NoError(t, err)
		require.Equal(t, "deepseek-v4-flash", request.Model)
		require.Equal(t, "deepseek-v4-flash", info.UpstreamModelName)
		require.Equal(t, "max", request.ReasoningEffort)
		require.Equal(t, "max", info.ReasoningEffort)

		var thinking map[string]string
		err = common.Unmarshal(request.THINKING, &thinking)
		require.NoError(t, err)
		require.Equal(t, "enabled", thinking["type"])
	})
}

func TestApplyDeepSeekV4ClaudeThinkingSuffix(t *testing.T) {
	t.Parallel()

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "deepseek-v4-pro-nothinking",
		},
	}
	request := &dto.ClaudeRequest{Model: "deepseek-v4-pro-nothinking"}

	err := applyDeepSeekV4ClaudeThinkingSuffix(info, request)
	require.NoError(t, err)
	require.Equal(t, "deepseek-v4-pro", request.Model)
	require.Equal(t, "deepseek-v4-pro", info.UpstreamModelName)
	require.NotNil(t, request.Thinking)
	require.Equal(t, "disabled", request.Thinking.Type)
	require.Nil(t, request.OutputConfig)
}

func TestNormalizeDeepSeekOpenAIRequestForUpstream(t *testing.T) {
	t.Parallel()

	request := &dto.GeneralOpenAIRequest{
		Messages: []dto.Message{
			{Role: "developer"},
			{Role: "latest_reminder"},
			{Role: "unknown_role"},
			{Role: ""},
		},
	}

	normalizeDeepSeekOpenAIRequestForUpstream(request)

	require.Equal(t, "system", request.Messages[0].Role)
	require.Equal(t, "latest_reminder", request.Messages[1].Role)
	require.Equal(t, "user", request.Messages[2].Role)
	require.Equal(t, "user", request.Messages[3].Role)
}

func TestDeepSeekConvertClaudeRequestUsesOpenAIChatBridge(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		IsStream:    true,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:       "https://upstream.example",
			UpstreamModelName:    "deepseek-v4-pro-nothink",
			SupportStreamOptions: true,
		},
	}
	req := &dto.ClaudeRequest{
		Model:     "deepseek-v4-pro-nothink",
		MaxTokens: common.GetPointer[uint](128),
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "hello"},
		},
	}

	converted, err := (&Adaptor{}).ConvertClaudeRequest(ctx, info, req)
	require.NoError(t, err)
	chatReq, ok := converted.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Equal(t, "deepseek-v4-pro", chatReq.Model)
	require.Equal(t, types.RelayFormatOpenAI, info.FinalRequestRelayFormat)
	require.Equal(t, "deepseek-v4-pro", info.UpstreamModelName)
	require.NotNil(t, chatReq.StreamOptions)
	require.True(t, chatReq.StreamOptions.IncludeUsage)

	var thinking map[string]string
	err = common.Unmarshal(chatReq.THINKING, &thinking)
	require.NoError(t, err)
	require.Equal(t, "disabled", thinking["type"])

	url, err := (&Adaptor{}).GetRequestURL(info)
	require.NoError(t, err)
	require.Equal(t, "https://upstream.example/v1/chat/completions", url)
}

func TestDeepSeekConvertResponsesRequestUsesOpenAIChatBridge(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	stream := true
	maxOutputTokens := uint(256)
	input := common.StringToByteSlice(`[{"role":"user","content":[{"type":"input_text","text":"hello"}]}]`)
	instructions := common.StringToByteSlice(`"be concise"`)
	tools := common.StringToByteSlice(`[{"type":"function","name":"shell","description":"run shell","parameters":{"type":"object"}}]`)
	toolChoice := common.StringToByteSlice(`{"type":"function","name":"shell"}`)
	info := &relaycommon.RelayInfo{
		RelayMode:   constant.RelayModeResponses,
		RelayFormat: types.RelayFormatOpenAIResponses,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "deepseek-v4-pro-nothink",
		},
	}
	req := dto.OpenAIResponsesRequest{
		Model:           "deepseek-v4-pro-nothink",
		Input:           input,
		Instructions:    instructions,
		Tools:           tools,
		ToolChoice:      toolChoice,
		Stream:          &stream,
		MaxOutputTokens: &maxOutputTokens,
	}

	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(ctx, info, req)
	require.NoError(t, err)
	chatReq, ok := converted.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Equal(t, "deepseek-v4-pro", chatReq.Model)
	require.Len(t, chatReq.Messages, 2)
	require.Equal(t, "system", chatReq.Messages[0].Role)
	require.Equal(t, "be concise", chatReq.Messages[0].StringContent())
	require.Equal(t, "user", chatReq.Messages[1].Role)
	require.True(t, chatReq.Messages[1].IsStringContent() || len(chatReq.Messages[1].ParseContent()) > 0)
	require.Len(t, chatReq.Tools, 1)
	require.Equal(t, "shell", chatReq.Tools[0].Function.Name)
	require.Equal(t, map[string]any{"type": "function", "function": map[string]any{"name": "shell"}}, chatReq.ToolChoice)
	require.True(t, *chatReq.Stream)
	require.Equal(t, maxOutputTokens, *chatReq.MaxCompletionTokens)
	require.Equal(t, types.RelayFormatOpenAI, info.FinalRequestRelayFormat)

	var thinking map[string]string
	err = common.Unmarshal(chatReq.THINKING, &thinking)
	require.NoError(t, err)
	require.Equal(t, "disabled", thinking["type"])
}

func TestDeepSeekConvertResponsesRequestNormalizesUnsupportedRoles(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{
		RelayMode:   constant.RelayModeResponses,
		RelayFormat: types.RelayFormatOpenAIResponses,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "deepseek-v4-pro-nothink",
		},
	}
	req := dto.OpenAIResponsesRequest{
		Model: "deepseek-v4-pro-nothink",
		Input: common.StringToByteSlice(`[
			{"type":"message","role":"developer","content":[{"type":"input_text","text":"follow project rules"}]},
			{"type":"message","role":"unknown_role","content":"fallback to user"},
			{"type":"message","role":"latest_reminder","content":"keep concise"}
		]`),
	}

	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(ctx, info, req)
	require.NoError(t, err)
	chatReq, ok := converted.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Len(t, chatReq.Messages, 3)
	require.Equal(t, "system", chatReq.Messages[0].Role)
	require.Equal(t, "user", chatReq.Messages[1].Role)
	require.Equal(t, "latest_reminder", chatReq.Messages[2].Role)
}

func TestDeepSeekConvertResponsesRequestFlattensNamespaceTools(t *testing.T) {
	t.Parallel()

	req := dto.OpenAIResponsesRequest{
		Model: "deepseek-v4-pro-nothink",
		Input: common.StringToByteSlice(`"hello"`),
		Tools: common.StringToByteSlice(`[
			{
				"type":"namespace",
				"name":"mcp__calendar",
				"description":"Calendar tools.",
				"tools":[
					{
						"type":"function",
						"name":"create_event",
						"description":"Create an event",
						"parameters":{"type":"object","properties":{"title":{"type":"string"}}}
					}
				]
			}
		]`),
		ToolChoice: common.StringToByteSlice(`{"type":"function","namespace":"mcp__calendar","name":"create_event"}`),
	}

	chatReq, err := deepSeekResponsesToChatCompletionsRequest(req)
	require.NoError(t, err)
	require.Len(t, chatReq.Tools, 1)
	require.Equal(t, "function", chatReq.Tools[0].Type)
	require.Equal(t, "mcp__calendar___create_event", chatReq.Tools[0].Function.Name)
	require.Equal(t, "Create an event", chatReq.Tools[0].Function.Description)
	require.Equal(t, map[string]any{
		"type": "function",
		"function": map[string]any{
			"name": "mcp__calendar___create_event",
		},
	}, chatReq.ToolChoice)
}

func TestDeepSeekResponsesBridgeRejectsUnsupportedStatefulFeatures(t *testing.T) {
	t.Parallel()

	req := dto.OpenAIResponsesRequest{
		Model:              "deepseek-v4-pro-nothink",
		Input:              common.StringToByteSlice(`"hello"`),
		PreviousResponseID: "resp_123",
	}

	_, err := deepSeekResponsesToChatCompletionsRequest(req)
	require.ErrorContains(t, err, "previous_response_id")
}

func TestDeepSeekResponsesBridgeRejectsUnsupportedBuiltInTools(t *testing.T) {
	t.Parallel()

	req := dto.OpenAIResponsesRequest{
		Model: "deepseek-v4-pro-nothink",
		Input: common.StringToByteSlice(`"hello"`),
		Tools: common.StringToByteSlice(`[
			{"type":"web_search_preview"},
			{"type":"file_search"},
			{"type":"tool_search"},
			{"type":"image_generation"},
			{"type":"custom"}
		]`),
	}

	chatReq, err := deepSeekResponsesToChatCompletionsRequest(req)
	require.NoError(t, err)
	require.Empty(t, chatReq.Tools)
	require.Nil(t, chatReq.ToolChoice)
}

func TestDeepSeekResponsesBridgeKeepsSupportedToolsWhenMixedWithUnsupportedOnes(t *testing.T) {
	t.Parallel()

	req := dto.OpenAIResponsesRequest{
		Model: "deepseek-v4-pro-nothink",
		Input: common.StringToByteSlice(`"hello"`),
		Tools: common.StringToByteSlice(`[
			{"type":"web_search_preview"},
			{"type":"namespace","name":"mcp__calendar","tools":[{"type":"function","name":"create_event","parameters":{"type":"object"}}]},
			{"type":"file_search"},
			{"type":"function","name":"shell","description":"run shell","parameters":{"type":"object"}}
		]`),
		ToolChoice: common.StringToByteSlice(`{"type":"function","namespace":"mcp__calendar","name":"create_event"}`),
	}

	chatReq, err := deepSeekResponsesToChatCompletionsRequest(req)
	require.NoError(t, err)
	require.Len(t, chatReq.Tools, 2)
	require.Equal(t, "mcp__calendar___create_event", chatReq.Tools[0].Function.Name)
	require.Equal(t, "shell", chatReq.Tools[1].Function.Name)
	require.Equal(t, map[string]any{
		"type": "function",
		"function": map[string]any{
			"name": "mcp__calendar___create_event",
		},
	}, chatReq.ToolChoice)
}

func TestDeepSeekResponsesFunctionCallOutputToChatToolMessage(t *testing.T) {
	t.Parallel()

	req := dto.OpenAIResponsesRequest{
		Model: "deepseek-v4-pro-nothink",
		Input: common.StringToByteSlice(`[
			{"type":"message","role":"user","content":"run pwd"},
			{"type":"function_call_output","call_id":"call_123","output":"{\"pwd\":\"/tmp\"}"}
		]`),
	}

	chatReq, err := deepSeekResponsesToChatCompletionsRequest(req)
	require.NoError(t, err)
	require.Len(t, chatReq.Messages, 2)
	require.Equal(t, "tool", chatReq.Messages[1].Role)
	require.Equal(t, "call_123", chatReq.Messages[1].ToolCallId)
	require.JSONEq(t, `{"pwd":"/tmp"}`, chatReq.Messages[1].StringContent())
}

func TestDeepSeekResponsesFunctionCallHistoryToChatAssistantMessage(t *testing.T) {
	t.Parallel()

	req := dto.OpenAIResponsesRequest{
		Model: "deepseek-v4-pro-nothink",
		Input: common.StringToByteSlice(`[
			{"type":"message","role":"user","content":"run pwd"},
			{"type":"function_call","call_id":"call_123","name":"shell","arguments":"{\"cmd\":\"pwd\"}"},
			{"type":"function_call_output","call_id":"call_123","output":"{\"pwd\":\"/tmp\"}"},
			{"type":"reasoning","summary":[{"type":"summary_text","text":"irrelevant hidden state"}]}
		]`),
	}

	chatReq, err := deepSeekResponsesToChatCompletionsRequest(req)
	require.NoError(t, err)
	require.Len(t, chatReq.Messages, 3)
	require.Equal(t, "assistant", chatReq.Messages[1].Role)
	require.Equal(t, "", chatReq.Messages[1].StringContent())
	toolCalls := chatReq.Messages[1].ParseToolCalls()
	require.Len(t, toolCalls, 1)
	require.Equal(t, "call_123", toolCalls[0].ID)
	require.Equal(t, "shell", toolCalls[0].Function.Name)
	require.JSONEq(t, `{"cmd":"pwd"}`, toolCalls[0].Function.Arguments)
	require.Equal(t, "tool", chatReq.Messages[2].Role)
	require.Equal(t, "call_123", chatReq.Messages[2].ToolCallId)
}

func TestDeepSeekChatCompletionsToResponsesResponse(t *testing.T) {
	t.Parallel()

	resp := deepSeekChatCompletionsToResponsesResponse(dto.OpenAITextResponse{
		Id:      "chatcmpl_123",
		Model:   "deepseek-v4-pro",
		Created: float64(1234),
		Choices: []dto.OpenAITextResponseChoice{
			{
				Index: 0,
				Message: dto.Message{
					Role:    "assistant",
					Content: "hello back",
				},
				FinishReason: "stop",
			},
		},
		Usage: dto.Usage{
			PromptTokens:     3,
			CompletionTokens: 4,
			TotalTokens:      7,
		},
	})

	require.Equal(t, "response", resp.Object)
	require.Equal(t, 1234, resp.CreatedAt)
	require.Len(t, resp.Output, 1)
	require.Equal(t, "message", resp.Output[0].Type)
	require.Equal(t, "output_text", resp.Output[0].Content[0].Type)
	require.Equal(t, "hello back", resp.Output[0].Content[0].Text)
	require.NotNil(t, resp.Usage)
	require.Equal(t, 3, resp.Usage.InputTokens)
	require.Equal(t, 4, resp.Usage.OutputTokens)
	require.Equal(t, 7, resp.Usage.TotalTokens)
}

func TestDeepSeekChatCompletionsToolCallToResponsesResponse(t *testing.T) {
	t.Parallel()

	message := dto.Message{Role: "assistant"}
	message.SetToolCalls([]dto.ToolCallRequest{
		{
			ID:   "call_123",
			Type: "function",
			Function: dto.FunctionRequest{
				Name:      "shell",
				Arguments: `{"cmd":"pwd"}`,
			},
		},
	})
	resp := deepSeekChatCompletionsToResponsesResponse(dto.OpenAITextResponse{
		Id:    "chatcmpl_123",
		Model: "deepseek-v4-pro",
		Choices: []dto.OpenAITextResponseChoice{
			{
				Index:        0,
				Message:      message,
				FinishReason: "tool_calls",
			},
		},
	})

	require.Len(t, resp.Output, 1)
	require.Equal(t, "function_call", resp.Output[0].Type)
	require.Equal(t, "call_123", resp.Output[0].CallId)
	require.Equal(t, "shell", resp.Output[0].Name)
	var arguments string
	err := common.Unmarshal(resp.Output[0].Arguments, &arguments)
	require.NoError(t, err)
	require.JSONEq(t, `{"cmd":"pwd"}`, arguments)
}

func TestDeepSeekChatCompletionsNamespacedToolCallToResponsesResponse(t *testing.T) {
	t.Parallel()

	message := dto.Message{Role: "assistant"}
	message.SetToolCalls([]dto.ToolCallRequest{
		{
			ID:   "call_123",
			Type: "function",
			Function: dto.FunctionRequest{
				Name:      "mcp__calendar___create_event",
				Arguments: `{"title":"standup"}`,
			},
		},
	})
	resp := deepSeekChatCompletionsToResponsesResponse(dto.OpenAITextResponse{
		Id:    "chatcmpl_123",
		Model: "deepseek-v4-pro",
		Choices: []dto.OpenAITextResponseChoice{
			{
				Index:        0,
				Message:      message,
				FinishReason: "tool_calls",
			},
		},
	})

	require.Len(t, resp.Output, 1)
	require.Equal(t, "function_call", resp.Output[0].Type)
	require.Equal(t, "mcp__calendar", resp.Output[0].Namespace)
	require.Equal(t, "create_event", resp.Output[0].Name)
}

func TestDeepSeekResponsesStreamStateAccumulatesToolCalls(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	state := &deepSeekResponsesStreamState{
		ResponseID:      "resp_123",
		ModelName:       "deepseek-v4-pro",
		CreatedAt:       1234,
		ToolCalls:       make(map[int]*deepSeekResponsesStreamToolCall),
		NextOutputIndex: 1,
	}
	toolIndex := 0
	state.HandleToolCallDeltas(ctx, []dto.ToolCallResponse{
		{
			Index: &toolIndex,
			ID:    "call_123",
			Type:  "function",
			Function: dto.FunctionResponse{
				Name:      "shell",
				Arguments: `{"cmd":`,
			},
		},
		{
			Index: &toolIndex,
			Type:  "function",
			Function: dto.FunctionResponse{
				Arguments: `"pwd"}`,
			},
		},
	})

	completed := state.CompletedResponse(dto.Usage{})
	require.Len(t, completed.Output, 1)
	require.Equal(t, "function_call", completed.Output[0].Type)
	require.Equal(t, "call_123", completed.Output[0].CallId)
	require.Equal(t, "shell", completed.Output[0].Name)
	var arguments string
	err := common.Unmarshal(completed.Output[0].Arguments, &arguments)
	require.NoError(t, err)
	require.JSONEq(t, `{"cmd":"pwd"}`, arguments)
}
