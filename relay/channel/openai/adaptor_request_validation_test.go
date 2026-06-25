package openai

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newTestContext() *gin.Context {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	return c
}

func newRelayInfo(model string) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeOpenAI,
			UpstreamModelName: model,
		},
	}
}

// --- Test: Reasoning parameter stripping ---

func TestConvertOpenAIRequest_StripsReasoningForNonReasoningModels(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		model           string
		expectStripped  bool
	}{
		{"gpt-5.2 strips reasoning", "gpt-5.2", true},
		{"gpt-5.1 strips reasoning", "gpt-5.1", true},
		{"gpt-5.3 strips reasoning", "gpt-5.3", true},
		{"gpt-5.4-mini strips reasoning", "gpt-5.4-mini", true},
		{"gpt-4o strips reasoning", "gpt-4o", true},
		{"gpt-4o-mini strips reasoning", "gpt-4o-mini", true},
		{"gpt-5 keeps reasoning", "gpt-5", false},
		{"gpt-5.4 keeps reasoning", "gpt-5.4", false},
		{"gpt-5.5 keeps reasoning", "gpt-5.5", false},
		{"o3 keeps reasoning", "o3", false},
		{"o4-mini keeps reasoning", "o4-mini", false},
		{"o1 keeps reasoning", "o1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			adaptor := &Adaptor{ChannelType: constant.ChannelTypeOpenAI}
			info := newRelayInfo(tt.model)
			request := &dto.GeneralOpenAIRequest{
				Model:           tt.model,
				ReasoningEffort: "medium",
				Reasoning:       json.RawMessage(`{"effort":"medium"}`),
			}

			got, err := adaptor.ConvertOpenAIRequest(newTestContext(), info, request)
			require.NoError(t, err)

			result := got.(*dto.GeneralOpenAIRequest)
			if tt.expectStripped {
				assert.Empty(t, result.ReasoningEffort, "ReasoningEffort should be stripped")
				assert.Nil(t, result.Reasoning, "Reasoning should be stripped")
			} else {
				assert.NotEmpty(t, result.ReasoningEffort, "ReasoningEffort should be kept")
			}
		})
	}
}

// --- Test: max_output_tokens minimum enforcement ---

func TestConvertOpenAIRequest_EnforcesMinMaxTokens(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                   string
		maxTokens              *uint
		maxCompletionTokens    *uint
		expectMaxTokens        *uint
		expectMaxCompletion    *uint
	}{
		{
			"max_tokens=1 raised to 16",
			lo.ToPtr(uint(1)), nil,
			lo.ToPtr(uint(16)), nil,
		},
		{
			"max_completion_tokens=5 raised to 16",
			nil, lo.ToPtr(uint(5)),
			nil, lo.ToPtr(uint(16)),
		},
		{
			"max_tokens=15 raised to 16",
			lo.ToPtr(uint(15)), nil,
			lo.ToPtr(uint(16)), nil,
		},
		{
			"max_tokens=16 unchanged",
			lo.ToPtr(uint(16)), nil,
			lo.ToPtr(uint(16)), nil,
		},
		{
			"max_tokens=100 unchanged",
			lo.ToPtr(uint(100)), nil,
			lo.ToPtr(uint(100)), nil,
		},
		{
			"nil max_tokens unchanged",
			nil, nil,
			nil, nil,
		},
		{
			"max_tokens=0 pointer unchanged",
			lo.ToPtr(uint(0)), nil,
			lo.ToPtr(uint(0)), nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			adaptor := &Adaptor{ChannelType: constant.ChannelTypeOpenAI}
			info := newRelayInfo("gpt-4o")
			request := &dto.GeneralOpenAIRequest{
				Model:               "gpt-4o",
				MaxTokens:           tt.maxTokens,
				MaxCompletionTokens: tt.maxCompletionTokens,
			}

			got, err := adaptor.ConvertOpenAIRequest(newTestContext(), info, request)
			require.NoError(t, err)

			result := got.(*dto.GeneralOpenAIRequest)
			assert.Equal(t, tt.expectMaxTokens, result.MaxTokens)
			assert.Equal(t, tt.expectMaxCompletion, result.MaxCompletionTokens)
		})
	}
}

func TestConvertOpenAIResponsesRequest_EnforcesMinMaxOutputTokens(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		maxOutput       *uint
		expectMaxOutput *uint
	}{
		{"1 raised to 16", lo.ToPtr(uint(1)), lo.ToPtr(uint(16))},
		{"15 raised to 16", lo.ToPtr(uint(15)), lo.ToPtr(uint(16))},
		{"16 unchanged", lo.ToPtr(uint(16)), lo.ToPtr(uint(16))},
		{"100 unchanged", lo.ToPtr(uint(100)), lo.ToPtr(uint(100))},
		{"nil unchanged", nil, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			adaptor := &Adaptor{ChannelType: constant.ChannelTypeOpenAI}
			info := newRelayInfo("gpt-4o")
			request := dto.OpenAIResponsesRequest{
				Model:           "gpt-4o",
				MaxOutputTokens: tt.maxOutput,
			}

			got, err := adaptor.ConvertOpenAIResponsesRequest(newTestContext(), info, request)
			require.NoError(t, err)

			result := got.(dto.OpenAIResponsesRequest)
			assert.Equal(t, tt.expectMaxOutput, result.MaxOutputTokens)
		})
	}
}

// --- Test: tool_calls truncation ---

func TestConvertOpenAIRequest_TruncatesToolCalls(t *testing.T) {
	t.Parallel()

	makeToolCalls := func(n int) json.RawMessage {
		calls := make([]dto.ToolCallRequest, n)
		for i := range calls {
			calls[i] = dto.ToolCallRequest{
				ID:   "call_" + string(rune('a'+i%26)),
				Type: "function",
				Function: dto.FunctionRequest{
					Name: "test_func",
				},
			}
		}
		data, _ := json.Marshal(calls)
		return data
	}

	t.Run("130 tool_calls truncated to 128", func(t *testing.T) {
		t.Parallel()
		adaptor := &Adaptor{ChannelType: constant.ChannelTypeOpenAI}
		info := newRelayInfo("gpt-4o")
		request := &dto.GeneralOpenAIRequest{
			Model: "gpt-4o",
			Messages: []dto.Message{
				{Role: "user", Content: "hello"},
				{Role: "assistant", ToolCalls: makeToolCalls(130)},
			},
		}

		got, err := adaptor.ConvertOpenAIRequest(newTestContext(), info, request)
		require.NoError(t, err)

		result := got.(*dto.GeneralOpenAIRequest)
		toolCalls := result.Messages[1].ParseToolCalls()
		assert.Len(t, toolCalls, 128)
	})

	t.Run("128 tool_calls unchanged", func(t *testing.T) {
		t.Parallel()
		adaptor := &Adaptor{ChannelType: constant.ChannelTypeOpenAI}
		info := newRelayInfo("gpt-4o")
		request := &dto.GeneralOpenAIRequest{
			Model: "gpt-4o",
			Messages: []dto.Message{
				{Role: "assistant", ToolCalls: makeToolCalls(128)},
			},
		}

		got, err := adaptor.ConvertOpenAIRequest(newTestContext(), info, request)
		require.NoError(t, err)

		result := got.(*dto.GeneralOpenAIRequest)
		toolCalls := result.Messages[0].ParseToolCalls()
		assert.Len(t, toolCalls, 128)
	})

	t.Run("50 tool_calls unchanged", func(t *testing.T) {
		t.Parallel()
		adaptor := &Adaptor{ChannelType: constant.ChannelTypeOpenAI}
		info := newRelayInfo("gpt-4o")
		request := &dto.GeneralOpenAIRequest{
			Model: "gpt-4o",
			Messages: []dto.Message{
				{Role: "assistant", ToolCalls: makeToolCalls(50)},
			},
		}

		got, err := adaptor.ConvertOpenAIRequest(newTestContext(), info, request)
		require.NoError(t, err)

		result := got.(*dto.GeneralOpenAIRequest)
		toolCalls := result.Messages[0].ParseToolCalls()
		assert.Len(t, toolCalls, 50)
	})

	t.Run("nil tool_calls unchanged", func(t *testing.T) {
		t.Parallel()
		adaptor := &Adaptor{ChannelType: constant.ChannelTypeOpenAI}
		info := newRelayInfo("gpt-4o")
		request := &dto.GeneralOpenAIRequest{
			Model: "gpt-4o",
			Messages: []dto.Message{
				{Role: "user", Content: "hello"},
			},
		}

		got, err := adaptor.ConvertOpenAIRequest(newTestContext(), info, request)
		require.NoError(t, err)

		result := got.(*dto.GeneralOpenAIRequest)
		assert.Nil(t, result.Messages[0].ToolCalls)
	})
}

// --- Test: IsOpenAIModelSupportReasoning helper ---

func TestIsOpenAIModelSupportReasoning(t *testing.T) {
	t.Parallel()
	tests := []struct {
		model  string
		expect bool
	}{
		{"o1", true},
		{"o1-mini", true},
		{"o1-preview", true},
		{"o3", true},
		{"o3-mini", true},
		{"o4-mini", true},
		{"gpt-5", true},
		{"gpt-5.4", true},
		{"gpt-5.5", true},
		{"gpt-5.6", true},
		{"gpt-5.1", false},
		{"gpt-5.2", false},
		{"gpt-5.3", false},
		{"gpt-5-mini", false},
		{"gpt-5.4-mini", false},
		{"gpt-4o", false},
		{"gpt-4o-mini", false},
		{"gpt-4", false},
		{"claude-3-opus", false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expect, dto.IsOpenAIModelSupportReasoning(tt.model))
		})
	}
}

// --- Test: IsOpenAIImageModel helper ---

func TestIsOpenAIImageModel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		model  string
		expect bool
	}{
		{"gpt-image-1", true},
		{"gpt-image-1-mini", true},
		{"gpt-image-1.5", true},
		{"gpt-image-2", true},
		{"chatgpt-image-latest", true},
		{"CHATGPT-IMAGE-LATEST", true},
		{"gpt-4o", false},
		{"gpt-5", false},
		{"dall-e-3", false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expect, dto.IsOpenAIImageModel(tt.model))
		})
	}
}

func TestConvertOpenAIResponsesRequest_StripsReasoningForNonReasoningModels(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		model          string
		expectStripped bool
	}{
		{"gpt-5.2 strips reasoning", "gpt-5.2", true},
		{"gpt-4o strips reasoning", "gpt-4o", true},
		{"gpt-5.5 keeps reasoning", "gpt-5.5", false},
		{"o3 keeps reasoning", "o3", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			adaptor := &Adaptor{ChannelType: constant.ChannelTypeOpenAI}
			info := newRelayInfo(tt.model)
			request := dto.OpenAIResponsesRequest{
				Model:     tt.model,
				Reasoning: &dto.Reasoning{Effort: "medium"},
			}

			got, err := adaptor.ConvertOpenAIResponsesRequest(newTestContext(), info, request)
			require.NoError(t, err)

			result := got.(dto.OpenAIResponsesRequest)
			if tt.expectStripped {
				assert.Nil(t, result.Reasoning)
			} else {
				assert.NotNil(t, result.Reasoning)
			}
		})
	}
}
