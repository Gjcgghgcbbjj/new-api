package compatir

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/dto"
)

func TestCompatIRChatResponseToResponsesResponsePreservesTextAndToolCalls(t *testing.T) {
	chatResp := &dto.OpenAITextResponse{
		Id:      "chatcmpl_1",
		Object:  "chat.completion",
		Created: float64(123),
		Model:   "chat-model",
		Choices: []dto.OpenAITextResponseChoice{
			{
				Index: 0,
				Message: dto.Message{
					Role:    "assistant",
					Content: "Let me check.",
					ToolCalls: mustRaw(t, []dto.ToolCallRequest{
						{
							ID:   "call_1",
							Type: ToolTypeFunction,
							Function: dto.FunctionRequest{
								Name:      "get_weather",
								Arguments: `{"city":"Shanghai"}`,
							},
						},
					}),
				},
				FinishReason: "tool_calls",
			},
		},
		Usage: dto.Usage{PromptTokens: 2, CompletionTokens: 3, TotalTokens: 5},
	}

	ir, err := FromChatResponse(chatResp)
	if err != nil {
		t.Fatalf("FromChatResponse error: %v", err)
	}
	responsesResp, usage, err := ToResponsesResponse(ir, &dto.OpenAIResponsesRequest{Model: "alias"}, "resp_1")
	if err != nil {
		t.Fatalf("ToResponsesResponse error: %v", err)
	}
	if responsesResp.ID != "resp_1" || responsesResp.OutputText != "Let me check." {
		t.Fatalf("responses response = %+v", responsesResp)
	}
	if usage.InputTokens != 2 || usage.OutputTokens != 3 || usage.TotalTokens != 5 {
		t.Fatalf("usage = %+v", usage)
	}
	if len(responsesResp.Output) != 2 {
		t.Fatalf("output = %+v", responsesResp.Output)
	}
	if responsesResp.Output[0].Type != OutputTypeMessage || responsesResp.Output[1].Type != OutputTypeFunctionCall {
		t.Fatalf("output = %+v", responsesResp.Output)
	}
	if responsesResp.Output[1].CallId != "call_1" || responsesResp.Output[1].Name != "get_weather" {
		t.Fatalf("tool output = %+v", responsesResp.Output[1])
	}
}

func TestCompatIRResponsesResponseToChatResponsePreservesTextAndToolCalls(t *testing.T) {
	responsesResp := &dto.OpenAIResponsesResponse{
		ID:        "resp_1",
		Object:    "response",
		CreatedAt: 123,
		Model:     "responses-model",
		Output: []dto.ResponsesOutput{
			{
				Type:   OutputTypeMessage,
				ID:     "msg_1",
				Status: "completed",
				Role:   "assistant",
				Content: []dto.ResponsesOutputContent{
					{Type: "output_text", Text: "Let me check."},
				},
			},
			{
				Type:      OutputTypeFunctionCall,
				ID:        "fc_1",
				Status:    "completed",
				CallId:    "call_1",
				Name:      "get_weather",
				Arguments: json.RawMessage(`{"city":"Shanghai"}`),
			},
		},
		Usage: &dto.Usage{InputTokens: 2, OutputTokens: 3, TotalTokens: 5},
	}

	ir, err := FromResponsesResponse(responsesResp)
	if err != nil {
		t.Fatalf("FromResponsesResponse error: %v", err)
	}
	chatResp, usage, err := ToChatResponse(ir, "chat_1")
	if err != nil {
		t.Fatalf("ToChatResponse error: %v", err)
	}
	if chatResp.Id != "chat_1" || chatResp.Model != "responses-model" {
		t.Fatalf("chat response = %+v", chatResp)
	}
	if usage.PromptTokens != 2 || usage.CompletionTokens != 3 || usage.TotalTokens != 5 {
		t.Fatalf("usage = %+v", usage)
	}
	choice := chatResp.Choices[0]
	if choice.FinishReason != "tool_calls" || choice.Message.StringContent() != "Let me check." {
		t.Fatalf("choice = %+v", choice)
	}
	toolCalls := choice.Message.ParseToolCalls()
	if len(toolCalls) != 1 || toolCalls[0].ID != "call_1" || toolCalls[0].Function.Arguments != `{"city":"Shanghai"}` {
		t.Fatalf("tool calls = %+v", toolCalls)
	}
}

func TestCompatIRResponsesResponseToChatResponseToolOnly(t *testing.T) {
	responsesResp := &dto.OpenAIResponsesResponse{
		ID:        "resp_1",
		CreatedAt: 123,
		Model:     "responses-model",
		Output: []dto.ResponsesOutput{
			{
				Type:      OutputTypeFunctionCall,
				ID:        "fc_1",
				Status:    "completed",
				CallId:    "call_1",
				Name:      "get_weather",
				Arguments: json.RawMessage(`{"city":"Shanghai"}`),
			},
		},
	}

	ir, err := FromResponsesResponse(responsesResp)
	if err != nil {
		t.Fatalf("FromResponsesResponse error: %v", err)
	}
	chatResp, _, err := ToChatResponse(ir, "chat_1")
	if err != nil {
		t.Fatalf("ToChatResponse error: %v", err)
	}
	choice := chatResp.Choices[0]
	if choice.FinishReason != "tool_calls" || choice.Message.Content != "" {
		t.Fatalf("choice = %+v", choice)
	}
	if len(choice.Message.ParseToolCalls()) != 1 {
		t.Fatalf("tool calls = %+v", choice.Message.ParseToolCalls())
	}
}
