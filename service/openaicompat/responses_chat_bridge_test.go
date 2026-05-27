package openaicompat

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/dto"
)

func TestResponsesRequestToChatCompletionsRequest(t *testing.T) {
	stream := true
	req := &dto.OpenAIResponsesRequest{
		Model:        "codex-chat",
		Input:        json.RawMessage(`"hello"`),
		Instructions: json.RawMessage(`"be brief"`),
		Stream:       &stream,
		Tools: json.RawMessage(`[
			{
				"type": "function",
				"name": "shell",
				"description": "run command",
				"parameters": {"type": "object"}
			}
		]`),
	}

	chatReq, err := ResponsesRequestToChatCompletionsRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chatReq.Model != "codex-chat" {
		t.Fatalf("model = %q", chatReq.Model)
	}
	if chatReq.Stream == nil || !*chatReq.Stream {
		t.Fatal("stream was not preserved")
	}
	if len(chatReq.Messages) != 2 {
		t.Fatalf("messages len = %d", len(chatReq.Messages))
	}
	if chatReq.Messages[0].Role != "system" || chatReq.Messages[0].StringContent() != "be brief" {
		t.Fatalf("system message = %+v", chatReq.Messages[0])
	}
	if chatReq.Messages[1].Role != "user" || chatReq.Messages[1].StringContent() != "hello" {
		t.Fatalf("user message = %+v", chatReq.Messages[1])
	}
	if len(chatReq.Tools) != 1 || chatReq.Tools[0].Function.Name != "shell" {
		t.Fatalf("tools = %+v", chatReq.Tools)
	}
}

func TestChatCompletionsResponseToResponsesResponse(t *testing.T) {
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
					Content: "world",
				},
				FinishReason: "stop",
			},
		},
		Usage: dto.Usage{
			PromptTokens:     2,
			CompletionTokens: 3,
			TotalTokens:      5,
		},
	}

	resp, usage, err := ChatCompletionsResponseToResponsesResponse(chatResp, &dto.OpenAIResponsesRequest{Model: "alias"}, "resp_1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID != "resp_1" || resp.Object != "response" || resp.Model != "chat-model" {
		t.Fatalf("response metadata = %+v", resp)
	}
	if resp.OutputText != "world" {
		t.Fatalf("output_text = %q", resp.OutputText)
	}
	if len(resp.Output) != 1 || resp.Output[0].Content[0].Text != "world" {
		t.Fatalf("output = %+v", resp.Output)
	}
	if usage.InputTokens != 2 || usage.OutputTokens != 3 || usage.TotalTokens != 5 {
		t.Fatalf("usage = %+v", usage)
	}
}
