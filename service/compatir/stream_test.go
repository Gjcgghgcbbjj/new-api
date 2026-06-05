package compatir

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/dto"
)

func TestCompatIRChatStreamToResponsesPreservesTextAndToolCalls(t *testing.T) {
	converter := NewChatToResponsesStreamConverter(ChatToResponsesStreamOptions{
		ResponseID: "resp_1",
		MessageID:  "msg_1",
		Model:      "chat-model",
		CreatedAt:  123,
	})

	startEvents := converter.StartEvents()
	assertEventTypes(t, startEvents, "response.created", "response.output_item.added", "response.content_part.added")

	content := "Let me check."
	idx := 0
	chunkEvents := converter.EventsFromChatChunk(&dto.ChatCompletionsStreamResponse{
		Model:   "chat-model",
		Created: 123,
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{
				Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
					Content: &content,
					ToolCalls: []dto.ToolCallResponse{
						{
							Index: &idx,
							ID:    "call_1",
							Type:  ToolTypeFunction,
							Function: dto.FunctionResponse{
								Name:      "get_",
								Arguments: `{"city":"`,
							},
						},
					},
				},
			},
		},
	})
	assertEventTypes(t, chunkEvents, "response.output_text.delta")

	converter.EventsFromChatChunk(&dto.ChatCompletionsStreamResponse{
		Usage: &dto.Usage{PromptTokens: 2, CompletionTokens: 3, TotalTokens: 5},
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{
				Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
					ToolCalls: []dto.ToolCallResponse{
						{
							Index: &idx,
							Function: dto.FunctionResponse{
								Name:      "weather",
								Arguments: `Shanghai"}`,
							},
						},
					},
				},
			},
		},
	})

	finalEvents, usage := converter.FinalEvents()
	assertEventTypes(t, finalEvents,
		"response.output_text.done",
		"response.content_part.done",
		"response.output_item.done",
		"response.output_item.added",
		"response.output_item.done",
		"response.completed",
	)
	if usage == nil || usage.InputTokens != 2 || usage.OutputTokens != 3 || usage.TotalTokens != 5 {
		t.Fatalf("usage = %+v", usage)
	}

	toolEvent := finalEvents[3]
	item := payloadMap(t, toolEvent.Payload, "item")
	if item["call_id"] != "call_1" || item["name"] != "get_weather" || item["arguments"] != `{"city":"Shanghai"}` {
		t.Fatalf("tool event item = %+v", item)
	}
	completed := payloadMap(t, finalEvents[5].Payload, "response")
	if completed["output_text"] != "Let me check." {
		t.Fatalf("completed response = %+v", completed)
	}
}

func TestCompatIRResponsesStreamToChatPreservesTextAndToolDeltas(t *testing.T) {
	converter := NewResponsesToChatStreamConverter(ResponsesToChatStreamOptions{
		ResponseID: "chat_1",
		Model:      "responses-model",
		CreatedAt:  123,
	})

	chunks := converter.ChunksFromResponsesEvent(&dto.ResponsesStreamResponse{
		Type: "response.created",
		Response: &dto.OpenAIResponsesResponse{
			CreatedAt: 123,
			Model:     "responses-model",
		},
	})
	if len(chunks) != 0 {
		t.Fatalf("created chunks = %+v", chunks)
	}

	chunks = converter.ChunksFromResponsesEvent(&dto.ResponsesStreamResponse{
		Type:  "response.output_text.delta",
		Delta: "Let me check.",
	})
	if len(chunks) != 2 {
		t.Fatalf("text chunks = %+v", chunks)
	}
	if chunks[0].Choices[0].Delta.Role != "assistant" || chunks[1].Choices[0].Delta.GetContentString() != "Let me check." {
		t.Fatalf("text chunks = %+v", chunks)
	}

	chunks = converter.ChunksFromResponsesEvent(&dto.ResponsesStreamResponse{
		Type: "response.output_item.done",
		Item: &dto.ResponsesOutput{
			ID:        "fc_1",
			Type:      OutputTypeFunctionCall,
			Status:    "completed",
			CallId:    "call_1",
			Name:      "get_weather",
			Arguments: json.RawMessage(`{"city":"Shanghai"}`),
		},
	})
	if len(chunks) != 1 {
		t.Fatalf("tool chunks = %+v", chunks)
	}
	tool := chunks[0].Choices[0].Delta.ToolCalls[0]
	if tool.ID != "call_1" || tool.Function.Name != "get_weather" || tool.Function.Arguments != `{"city":"Shanghai"}` {
		t.Fatalf("tool delta = %+v", tool)
	}

	chunks = converter.ChunksFromResponsesEvent(&dto.ResponsesStreamResponse{
		Type: "response.completed",
		Response: &dto.OpenAIResponsesResponse{
			CreatedAt: 123,
			Model:     "responses-model",
			Usage:     &dto.Usage{InputTokens: 2, OutputTokens: 3, TotalTokens: 5},
		},
	})
	if len(chunks) != 1 || chunks[0].Choices[0].FinishReason == nil || *chunks[0].Choices[0].FinishReason != "tool_calls" {
		t.Fatalf("completed chunks = %+v", chunks)
	}
	usage := converter.Usage()
	if usage == nil || usage.PromptTokens != 2 || usage.CompletionTokens != 3 || usage.TotalTokens != 5 {
		t.Fatalf("usage = %+v", usage)
	}
}

func TestCompatIRResponsesStreamFunctionCallArgumentsDelta(t *testing.T) {
	converter := NewResponsesToChatStreamConverter(ResponsesToChatStreamOptions{
		ResponseID: "chat_1",
		Model:      "responses-model",
		CreatedAt:  123,
	})

	chunks := converter.ChunksFromResponsesEvent(&dto.ResponsesStreamResponse{
		Type: "response.output_item.added",
		Item: &dto.ResponsesOutput{
			ID:     "fc_1",
			Type:   OutputTypeFunctionCall,
			CallId: "call_1",
			Name:   "get_weather",
		},
	})
	if len(chunks) != 2 {
		t.Fatalf("tool start chunks = %+v", chunks)
	}
	firstTool := chunks[1].Choices[0].Delta.ToolCalls[0]
	if firstTool.ID != "call_1" || firstTool.Function.Name != "get_weather" {
		t.Fatalf("first tool delta = %+v", firstTool)
	}

	chunks = converter.ChunksFromResponsesEvent(&dto.ResponsesStreamResponse{
		Type:   "response.function_call_arguments.delta",
		ItemID: "fc_1",
		Delta:  `{"city":"Shanghai"}`,
	})
	if len(chunks) != 1 {
		t.Fatalf("arguments chunks = %+v", chunks)
	}
	tool := chunks[0].Choices[0].Delta.ToolCalls[0]
	if tool.ID != "call_1" || tool.Function.Name != "" || tool.Function.Arguments != `{"city":"Shanghai"}` {
		t.Fatalf("arguments delta = %+v", tool)
	}
}

func assertEventTypes(t *testing.T, events []ResponsesStreamEvent, want ...string) {
	t.Helper()
	if len(events) != len(want) {
		t.Fatalf("event count = %d, want %d: %+v", len(events), len(want), events)
	}
	for i := range want {
		if events[i].Type != want[i] {
			t.Fatalf("event[%d] = %q, want %q: %+v", i, events[i].Type, want[i], events)
		}
	}
}

func payloadMap(t *testing.T, payload map[string]any, key string) map[string]any {
	t.Helper()
	value, ok := payload[key].(map[string]any)
	if !ok {
		t.Fatalf("payload[%q] = %+v", key, payload[key])
	}
	return value
}
