package deepseek

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// Fy-api overlay: bridge OpenAI Responses clients to DeepSeek upstreams that
// only implement OpenAI-compatible chat/completions.
func deepSeekResponsesToChatCompletionsRequest(request dto.OpenAIResponsesRequest) (*dto.GeneralOpenAIRequest, error) {
	if request.Model == "" {
		return nil, errors.New("model is required")
	}
	if request.PreviousResponseID != "" || len(request.Conversation) > 0 || len(request.Prompt) > 0 {
		return nil, errors.New("deepseek responses-to-chat bridge does not support previous_response_id, conversation, or prompt")
	}
	if request.MaxToolCalls != nil {
		return nil, errors.New("deepseek responses-to-chat bridge does not support max_tool_calls")
	}

	messages, err := deepSeekResponsesInputToMessages(request)
	if err != nil {
		return nil, err
	}
	tools, err := deepSeekResponsesToolsToChatTools(request.Tools)
	if err != nil {
		return nil, err
	}
	toolChoice, err := deepSeekResponsesToolChoiceToChatToolChoice(request.ToolChoice)
	if err != nil {
		return nil, err
	}

	chatRequest := &dto.GeneralOpenAIRequest{
		Model:                request.Model,
		Messages:             messages,
		Stream:               request.Stream,
		StreamOptions:        request.StreamOptions,
		MaxCompletionTokens:  request.MaxOutputTokens,
		Temperature:          request.Temperature,
		TopP:                 request.TopP,
		ServiceTier:          deepSeekStringRaw(request.ServiceTier),
		Store:                request.Store,
		PromptCacheRetention: request.PromptCacheRetention,
		Metadata:             request.Metadata,
		User:                 request.User,
		EnableThinking:       request.EnableThinking,
		Tools:                tools,
		ToolChoice:           toolChoice,
	}
	if request.Reasoning != nil {
		chatRequest.ReasoningEffort = request.Reasoning.Effort
	}
	return chatRequest, nil
}

func deepSeekResponsesInputToMessages(request dto.OpenAIResponsesRequest) ([]dto.Message, error) {
	messages := make([]dto.Message, 0)
	if len(request.Instructions) > 0 && common.GetJsonType(request.Instructions) != "null" {
		instructions, err := deepSeekResponsesInstructionsText(request.Instructions)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(instructions) != "" {
			msg := dto.Message{Role: "system"}
			msg.SetStringContent(instructions)
			messages = append(messages, msg)
		}
	}

	if len(request.Input) == 0 || common.GetJsonType(request.Input) == "null" {
		return messages, nil
	}

	switch common.GetJsonType(request.Input) {
	case "string":
		var text string
		if err := common.Unmarshal(request.Input, &text); err != nil {
			return nil, err
		}
		msg := dto.Message{Role: "user"}
		msg.SetStringContent(text)
		messages = append(messages, msg)
	case "array":
		var inputs []map[string]any
		if err := common.Unmarshal(request.Input, &inputs); err != nil {
			return nil, err
		}
		for _, input := range inputs {
			msg, ok, err := deepSeekResponsesInputItemToMessage(input)
			if err != nil {
				return nil, err
			}
			if ok {
				messages = append(messages, msg)
			}
		}
	default:
		return nil, fmt.Errorf("deepseek responses-to-chat bridge does not support input type %s", common.GetJsonType(request.Input))
	}
	return messages, nil
}

func deepSeekResponsesToolsToChatTools(raw []byte) ([]dto.ToolCallRequest, error) {
	if len(raw) == 0 || common.GetJsonType(raw) == "null" {
		return nil, nil
	}
	var tools []map[string]any
	if err := common.Unmarshal(raw, &tools); err != nil {
		return nil, err
	}
	chatTools := make([]dto.ToolCallRequest, 0, len(tools))
	for _, tool := range tools {
		toolType, _ := tool["type"].(string)
		if toolType != "function" {
			return nil, fmt.Errorf("deepseek responses-to-chat bridge does not support tool type %s", toolType)
		}
		name, _ := tool["name"].(string)
		if strings.TrimSpace(name) == "" {
			return nil, errors.New("deepseek responses-to-chat bridge requires function tool name")
		}
		description, _ := tool["description"].(string)
		chatTools = append(chatTools, dto.ToolCallRequest{
			Type: "function",
			Function: dto.FunctionRequest{
				Name:        name,
				Description: description,
				Parameters:  tool["parameters"],
			},
		})
	}
	return chatTools, nil
}

func deepSeekResponsesToolChoiceToChatToolChoice(raw []byte) (any, error) {
	if len(raw) == 0 || common.GetJsonType(raw) == "null" {
		return nil, nil
	}
	switch common.GetJsonType(raw) {
	case "string":
		var choice string
		if err := common.Unmarshal(raw, &choice); err != nil {
			return nil, err
		}
		return choice, nil
	case "object":
		var choice map[string]any
		if err := common.Unmarshal(raw, &choice); err != nil {
			return nil, err
		}
		if choice["type"] == "function" {
			name, _ := choice["name"].(string)
			if strings.TrimSpace(name) == "" {
				return nil, errors.New("deepseek responses-to-chat bridge requires function tool_choice name")
			}
			return map[string]any{
				"type": "function",
				"function": map[string]any{
					"name": name,
				},
			}, nil
		}
		return choice, nil
	default:
		return nil, fmt.Errorf("deepseek responses-to-chat bridge does not support tool_choice type %s", common.GetJsonType(raw))
	}
}

func deepSeekResponsesInstructionsText(raw []byte) (string, error) {
	switch common.GetJsonType(raw) {
	case "string":
		var text string
		if err := common.Unmarshal(raw, &text); err != nil {
			return "", err
		}
		return text, nil
	case "array":
		var parts []map[string]any
		if err := common.Unmarshal(raw, &parts); err != nil {
			return "", err
		}
		var sb strings.Builder
		for _, part := range parts {
			if text, _ := part["text"].(string); text != "" {
				if sb.Len() > 0 {
					sb.WriteString("\n")
				}
				sb.WriteString(text)
			}
		}
		return sb.String(), nil
	default:
		return "", fmt.Errorf("deepseek responses-to-chat bridge does not support instructions type %s", common.GetJsonType(raw))
	}
}

func deepSeekResponsesInputItemToMessage(input map[string]any) (dto.Message, bool, error) {
	inputType, _ := input["type"].(string)
	if inputType == "function_call_output" {
		return deepSeekResponsesFunctionCallOutputToToolMessage(input)
	}
	if inputType != "" && inputType != "message" {
		return dto.Message{}, false, fmt.Errorf("deepseek responses-to-chat bridge does not support input item type %s", inputType)
	}
	role, _ := input["role"].(string)
	role = strings.TrimSpace(role)
	if role == "" {
		role = "user"
	}
	msg := dto.Message{Role: role}

	contentRaw, err := common.Marshal(input["content"])
	if err != nil {
		return dto.Message{}, false, err
	}
	switch common.GetJsonType(contentRaw) {
	case "string":
		var text string
		if err := common.Unmarshal(contentRaw, &text); err != nil {
			return dto.Message{}, false, err
		}
		msg.SetStringContent(text)
	case "array":
		contents, err := deepSeekResponsesContentToChatContent(contentRaw)
		if err != nil {
			return dto.Message{}, false, err
		}
		msg.SetMediaContent(contents)
	case "":
		msg.SetStringContent("")
	default:
		return dto.Message{}, false, fmt.Errorf("deepseek responses-to-chat bridge does not support input content type %s", common.GetJsonType(contentRaw))
	}
	return msg, true, nil
}

func deepSeekResponsesFunctionCallOutputToToolMessage(input map[string]any) (dto.Message, bool, error) {
	callID, _ := input["call_id"].(string)
	if strings.TrimSpace(callID) == "" {
		return dto.Message{}, false, errors.New("deepseek responses-to-chat bridge requires function_call_output call_id")
	}
	output := ""
	switch v := input["output"].(type) {
	case string:
		output = v
	default:
		outputBytes, _ := common.Marshal(v)
		output = string(outputBytes)
	}
	msg := dto.Message{Role: "tool", ToolCallId: callID}
	msg.SetStringContent(output)
	return msg, true, nil
}

func deepSeekResponsesContentToChatContent(raw []byte) ([]dto.MediaContent, error) {
	var parts []map[string]any
	if err := common.Unmarshal(raw, &parts); err != nil {
		return nil, err
	}
	contents := make([]dto.MediaContent, 0, len(parts))
	for _, part := range parts {
		switch part["type"] {
		case "input_text", "output_text", "text":
			text, _ := part["text"].(string)
			contents = append(contents, dto.MediaContent{
				Type: dto.ContentTypeText,
				Text: text,
			})
		case "input_image":
			imageURL := ""
			switch v := part["image_url"].(type) {
			case string:
				imageURL = v
			case map[string]any:
				imageURL, _ = v["url"].(string)
			}
			contents = append(contents, dto.MediaContent{
				Type:     dto.ContentTypeImageURL,
				ImageUrl: &dto.MessageImageUrl{Url: imageURL},
			})
		default:
			return nil, fmt.Errorf("deepseek responses-to-chat bridge does not support content item type %v", part["type"])
		}
	}
	return contents, nil
}

func deepSeekChatCompletionsToResponsesHandler(c *gin.Context, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}

	var chatResponse dto.OpenAITextResponse
	if err := common.Unmarshal(responseBody, &chatResponse); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if oaiError := chatResponse.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}

	responsesResponse := deepSeekChatCompletionsToResponsesResponse(chatResponse)
	responsesBody, err := common.Marshal(responsesResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	service.IOCopyBytesGracefully(c, resp, responsesBody)
	return &chatResponse.Usage, nil
}

func deepSeekChatCompletionsToResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	usage := &dto.Usage{}
	state := &deepSeekResponsesStreamState{
		CreatedAt: int(time.Now().Unix()),
		ToolCalls: make(map[int]*deepSeekResponsesStreamToolCall),
	}

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		var chatChunk dto.ChatCompletionsStreamResponse
		if err := common.UnmarshalJsonStr(data, &chatChunk); err != nil {
			sr.Error(err)
			return
		}
		state.Init(c, chatChunk)
		if chatChunk.Usage != nil {
			*usage = *chatChunk.Usage
		}
		for _, choice := range chatChunk.Choices {
			delta := choice.Delta.GetContentString()
			if delta == "" {
				delta = choice.Delta.GetReasoningContent()
			}
			if delta == "" {
				state.HandleToolCallDeltas(c, choice.Delta.ToolCalls)
				continue
			}
			state.HandleTextDelta(c, delta)
			state.HandleToolCallDeltas(c, choice.Delta.ToolCalls)
		}
	})

	if usage.CompletionTokens == 0 && state.OutputText.Len() > 0 {
		usage.CompletionTokens = service.CountTextToken(state.OutputText.String(), state.ModelName)
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	if usage.InputTokens == 0 {
		usage.InputTokens = usage.PromptTokens
	}
	if usage.OutputTokens == 0 {
		usage.OutputTokens = usage.CompletionTokens
	}

	completed := state.CompletedResponse(*usage)
	state.SendDoneEvents(c, &completed)
	helper.Done(c)
	return usage, nil
}

type deepSeekResponsesStreamToolCall struct {
	Index     int
	ID        string
	CallID    string
	Name      string
	Arguments strings.Builder
}

type deepSeekResponsesStreamState struct {
	ResponseID      string
	ModelName       string
	CreatedAt       int
	TextOutputIndex *int
	TextItemStarted bool
	OutputText      strings.Builder
	ToolCalls       map[int]*deepSeekResponsesStreamToolCall
	NextOutputIndex int
	ResponseWasSent bool
}

func (s *deepSeekResponsesStreamState) Init(c *gin.Context, chunk dto.ChatCompletionsStreamResponse) {
	if s.ResponseWasSent {
		return
	}
	s.ResponseID = chunk.Id
	s.ModelName = chunk.Model
	if chunk.Created > 0 {
		s.CreatedAt = int(chunk.Created)
	}
	deepSeekSendResponsesStreamEvent(c, dto.ResponsesStreamResponse{
		Type: "response.created",
		Response: &dto.OpenAIResponsesResponse{
			ID:        s.ResponseID,
			Object:    "response",
			CreatedAt: s.CreatedAt,
			Status:    mustJSONRawString("in_progress"),
			Model:     s.ModelName,
		},
	})
	s.ResponseWasSent = true
}

func (s *deepSeekResponsesStreamState) HandleTextDelta(c *gin.Context, delta string) {
	if !s.TextItemStarted {
		index := s.NextOutputIndex
		s.NextOutputIndex++
		s.TextOutputIndex = &index
		s.TextItemStarted = true
		deepSeekSendResponsesStreamEvent(c, dto.ResponsesStreamResponse{
			Type:        "response.output_item.added",
			OutputIndex: common.GetPointer(index),
			Item: &dto.ResponsesOutput{
				Type:    "message",
				ID:      s.TextItemID(),
				Status:  "in_progress",
				Role:    "assistant",
				Content: []dto.ResponsesOutputContent{},
			},
		})
		deepSeekSendResponsesStreamEvent(c, dto.ResponsesStreamResponse{
			Type:         "response.content_part.added",
			OutputIndex:  common.GetPointer(index),
			ContentIndex: common.GetPointer(0),
			ItemID:       s.TextItemID(),
		})
	}
	s.OutputText.WriteString(delta)
	deepSeekSendResponsesStreamEvent(c, dto.ResponsesStreamResponse{
		Type:         "response.output_text.delta",
		Delta:        delta,
		OutputIndex:  common.GetPointer(*s.TextOutputIndex),
		ContentIndex: common.GetPointer(0),
		ItemID:       s.TextItemID(),
	})
}

func (s *deepSeekResponsesStreamState) HandleToolCallDeltas(c *gin.Context, toolCalls []dto.ToolCallResponse) {
	for _, toolCall := range toolCalls {
		index := 0
		if toolCall.Index != nil {
			index = *toolCall.Index
		}
		stateToolCall := s.ToolCalls[index]
		if stateToolCall == nil {
			outputIndex := s.NextOutputIndex
			s.NextOutputIndex++
			callID := strings.TrimSpace(toolCall.ID)
			if callID == "" {
				callID = fmt.Sprintf("call_%s_%d", s.ResponseID, index)
			}
			stateToolCall = &deepSeekResponsesStreamToolCall{
				Index:  outputIndex,
				ID:     callID,
				CallID: callID,
			}
			s.ToolCalls[index] = stateToolCall
			deepSeekSendResponsesStreamEvent(c, dto.ResponsesStreamResponse{
				Type:        "response.output_item.added",
				OutputIndex: common.GetPointer(outputIndex),
				Item: &dto.ResponsesOutput{
					Type:   "function_call",
					ID:     stateToolCall.ID,
					Status: "in_progress",
					CallId: stateToolCall.CallID,
				},
			})
		}
		if toolCall.ID != "" {
			stateToolCall.ID = toolCall.ID
			stateToolCall.CallID = toolCall.ID
		}
		if toolCall.Function.Name != "" {
			stateToolCall.Name = toolCall.Function.Name
		}
		if toolCall.Function.Arguments != "" {
			stateToolCall.Arguments.WriteString(toolCall.Function.Arguments)
			deepSeekSendResponsesStreamEvent(c, dto.ResponsesStreamResponse{
				Type:        "response.function_call_arguments.delta",
				Delta:       toolCall.Function.Arguments,
				OutputIndex: common.GetPointer(stateToolCall.Index),
				ItemID:      stateToolCall.ID,
			})
		}
	}
}

func (s *deepSeekResponsesStreamState) CompletedResponse(usage dto.Usage) dto.OpenAIResponsesResponse {
	message := dto.Message{
		Role:    "assistant",
		Content: s.OutputText.String(),
	}
	if len(s.ToolCalls) > 0 {
		toolCalls := make([]dto.ToolCallRequest, 0, len(s.ToolCalls))
		for _, toolCall := range s.ToolCalls {
			toolCalls = append(toolCalls, dto.ToolCallRequest{
				ID:   toolCall.CallID,
				Type: "function",
				Function: dto.FunctionRequest{
					Name:      toolCall.Name,
					Arguments: toolCall.Arguments.String(),
				},
			})
		}
		message.SetToolCalls(toolCalls)
	}
	return deepSeekChatCompletionsToResponsesResponse(dto.OpenAITextResponse{
		Id:      s.ResponseID,
		Object:  "chat.completion",
		Created: s.CreatedAt,
		Model:   s.ModelName,
		Choices: []dto.OpenAITextResponseChoice{
			{
				Index:        0,
				Message:      message,
				FinishReason: "stop",
			},
		},
		Usage: usage,
	})
}

func (s *deepSeekResponsesStreamState) SendDoneEvents(c *gin.Context, completed *dto.OpenAIResponsesResponse) {
	if s.TextItemStarted && s.TextOutputIndex != nil {
		deepSeekSendResponsesStreamEvent(c, dto.ResponsesStreamResponse{
			Type:         "response.output_text.done",
			Delta:        s.OutputText.String(),
			OutputIndex:  common.GetPointer(*s.TextOutputIndex),
			ContentIndex: common.GetPointer(0),
			ItemID:       s.TextItemID(),
		})
	}
	for _, output := range completed.Output {
		outputIndex := 0
		if output.Type == "message" && s.TextOutputIndex != nil {
			outputIndex = *s.TextOutputIndex
		} else if output.Type == "function_call" {
			if toolCall := s.FindToolCallByCallID(output.CallId); toolCall != nil {
				outputIndex = toolCall.Index
				deepSeekSendResponsesStreamEvent(c, dto.ResponsesStreamResponse{
					Type:        "response.function_call_arguments.done",
					Delta:       toolCall.Arguments.String(),
					OutputIndex: common.GetPointer(outputIndex),
					ItemID:      toolCall.ID,
				})
			}
		}
		item := output
		deepSeekSendResponsesStreamEvent(c, dto.ResponsesStreamResponse{
			Type:        "response.output_item.done",
			OutputIndex: common.GetPointer(outputIndex),
			Item:        &item,
		})
	}
	deepSeekSendResponsesStreamEvent(c, dto.ResponsesStreamResponse{
		Type:     "response.completed",
		Response: completed,
	})
}

func (s *deepSeekResponsesStreamState) FindToolCallByCallID(callID string) *deepSeekResponsesStreamToolCall {
	for _, toolCall := range s.ToolCalls {
		if toolCall.CallID == callID {
			return toolCall
		}
	}
	return nil
}

func (s *deepSeekResponsesStreamState) TextItemID() string {
	return fmt.Sprintf("msg_%s_0", s.ResponseID)
}

func deepSeekSendResponsesStreamEvent(c *gin.Context, event dto.ResponsesStreamResponse) {
	data, err := common.Marshal(event)
	if err != nil {
		return
	}
	helper.ResponseChunkData(c, event, string(data))
}

func deepSeekChatCompletionsToResponsesResponse(chatResponse dto.OpenAITextResponse) dto.OpenAIResponsesResponse {
	output := make([]dto.ResponsesOutput, 0, len(chatResponse.Choices))
	for _, choice := range chatResponse.Choices {
		toolCalls := choice.Message.ParseToolCalls()
		for _, toolCall := range toolCalls {
			callID := strings.TrimSpace(toolCall.ID)
			if callID == "" {
				callID = fmt.Sprintf("call_%s_%d", chatResponse.Id, choice.Index)
			}
			arguments, _ := common.Marshal(toolCall.Function.Arguments)
			output = append(output, dto.ResponsesOutput{
				Type:      "function_call",
				ID:        callID,
				Status:    "completed",
				CallId:    callID,
				Name:      toolCall.Function.Name,
				Arguments: arguments,
			})
		}
		messageText := choice.Message.StringContent()
		if messageText == "" {
			messageText = choice.Message.GetReasoningContent()
		}
		if messageText == "" && len(toolCalls) > 0 {
			continue
		}
		output = append(output, dto.ResponsesOutput{
			Type:   "message",
			ID:     fmt.Sprintf("msg_%s_%d", chatResponse.Id, choice.Index),
			Status: "completed",
			Role:   "assistant",
			Content: []dto.ResponsesOutputContent{
				{
					Type: "output_text",
					Text: messageText,
				},
			},
		})
	}

	createdAt := int(time.Now().Unix())
	switch created := chatResponse.Created.(type) {
	case float64:
		createdAt = int(created)
	case int:
		createdAt = created
	case int64:
		createdAt = int(created)
	}

	status, _ := common.Marshal("completed")
	return dto.OpenAIResponsesResponse{
		ID:        chatResponse.Id,
		Object:    "response",
		CreatedAt: createdAt,
		Status:    status,
		Model:     chatResponse.Model,
		Output:    output,
		Usage: &dto.Usage{
			PromptTokens:           chatResponse.Usage.PromptTokens,
			CompletionTokens:       chatResponse.Usage.CompletionTokens,
			TotalTokens:            chatResponse.Usage.TotalTokens,
			PromptTokensDetails:    chatResponse.Usage.PromptTokensDetails,
			CompletionTokenDetails: chatResponse.Usage.CompletionTokenDetails,
			InputTokens:            chatResponse.Usage.PromptTokens,
			OutputTokens:           chatResponse.Usage.CompletionTokens,
			InputTokensDetails:     &chatResponse.Usage.PromptTokensDetails,
		},
	}
}

func mustJSONRawString(value string) []byte {
	raw, _ := common.Marshal(value)
	return raw
}

func deepSeekStringRaw(value string) []byte {
	if value == "" {
		return nil
	}
	raw, _ := common.Marshal(value)
	return raw
}
