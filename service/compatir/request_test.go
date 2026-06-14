package compatir

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/dto"
)

func TestCompatIRChatRequestToResponsesRequest(t *testing.T) {
	stream := true
	parallel := false
	maxTokens := uint(128)
	req := &dto.GeneralOpenAIRequest{
		Model: "chat-model",
		Messages: []dto.Message{
			{Role: "system", Content: "be concise"},
			{Role: "user", Content: "hello"},
			{
				Role:    "assistant",
				Content: "",
				ToolCalls: mustRaw(t, []dto.ToolCallRequest{
					{
						ID:   "call_0",
						Type: ToolTypeFunction,
						Function: dto.FunctionRequest{
							Name:      "lookup",
							Arguments: `{"q":"x"}`,
						},
					},
				}),
			},
			{Role: "tool", ToolCallId: "call_0", Content: `{"answer":"y"}`},
		},
		Stream:               &stream,
		MaxTokens:            &maxTokens,
		ParallelTooCalls:     &parallel,
		PromptCacheKey:       "cache-key",
		PromptCacheRetention: json.RawMessage(`{"type":"ephemeral"}`),
		ReasoningEffort:      "medium",
		ResponseFormat: &dto.ResponseFormat{
			Type:       "json_schema",
			JsonSchema: json.RawMessage(`{"name":"answer","schema":{"type":"object"},"strict":true}`),
		},
		Tools: []dto.ToolCallRequest{
			{
				Type: ToolTypeFunction,
				Function: dto.FunctionRequest{
					Name:        "lookup",
					Description: "lookup data",
					Parameters:  map[string]any{"type": "object"},
				},
			},
		},
		ToolChoice: map[string]any{"type": "function", "function": map[string]any{"name": "lookup"}},
	}

	ir, err := FromChatRequest(req)
	if err != nil {
		t.Fatalf("FromChatRequest error: %v", err)
	}
	if ir.Instructions != "be concise" {
		t.Fatalf("instructions = %q", ir.Instructions)
	}
	if len(ir.Messages) != 3 {
		t.Fatalf("messages = %+v", ir.Messages)
	}
	if ir.Messages[1].ToolCalls[0].Name != "lookup" || ir.Messages[2].ToolCallID != "call_0" {
		t.Fatalf("tool messages = %+v", ir.Messages)
	}

	responsesReq, err := ToResponsesRequest(ir)
	if err != nil {
		t.Fatalf("ToResponsesRequest error: %v", err)
	}
	if responsesReq.Model != "chat-model" || responsesReq.MaxOutputTokens == nil || *responsesReq.MaxOutputTokens != 128 {
		t.Fatalf("responses request = %+v", responsesReq)
	}
	if string(responsesReq.Instructions) != `"be concise"` {
		t.Fatalf("instructions raw = %s", responsesReq.Instructions)
	}
	if string(responsesReq.ParallelToolCalls) != "false" {
		t.Fatalf("parallel_tool_calls = %s", responsesReq.ParallelToolCalls)
	}
	if responsesReq.PromptCacheKey == nil || string(responsesReq.PromptCacheKey) != `"cache-key"` {
		t.Fatalf("prompt_cache_key = %s", responsesReq.PromptCacheKey)
	}
	if responsesReq.Text == nil {
		t.Fatal("expected response_format to map to responses text format")
	}
	var text map[string]any
	if err := json.Unmarshal(responsesReq.Text, &text); err != nil {
		t.Fatalf("text format json: %v", err)
	}
	format, _ := text["format"].(map[string]any)
	if format["type"] != "json_schema" || format["name"] != "answer" {
		t.Fatalf("text format = %+v", text)
	}

	var input []map[string]any
	if err := json.Unmarshal(responsesReq.Input, &input); err != nil {
		t.Fatalf("input json: %v", err)
	}
	if len(input) != 4 {
		t.Fatalf("input = %+v", input)
	}
	if input[2]["type"] != "function_call" || input[3]["type"] != "function_call_output" {
		t.Fatalf("input = %+v", input)
	}
}

func TestCompatIRChatRequestConvertsLegacyFunctionsAndFunctionCall(t *testing.T) {
	req := &dto.GeneralOpenAIRequest{
		Model:    "chat-model",
		Messages: []dto.Message{{Role: "user", Content: "hello"}},
		Functions: json.RawMessage(`[
			{"name":"lookup","description":"lookup data","parameters":{"type":"object"}}
		]`),
		FunctionCall: json.RawMessage(`{"name":"lookup"}`),
	}

	ir, err := FromChatRequest(req)
	if err != nil {
		t.Fatalf("FromChatRequest error: %v", err)
	}
	responsesReq, err := ToResponsesRequest(ir)
	if err != nil {
		t.Fatalf("ToResponsesRequest error: %v", err)
	}

	var tools []map[string]any
	if err := json.Unmarshal(responsesReq.Tools, &tools); err != nil {
		t.Fatalf("tools json: %v", err)
	}
	if len(tools) != 1 || tools[0]["type"] != ToolTypeFunction || tools[0]["name"] != "lookup" {
		t.Fatalf("tools = %+v", tools)
	}

	var toolChoice map[string]any
	if err := json.Unmarshal(responsesReq.ToolChoice, &toolChoice); err != nil {
		t.Fatalf("tool_choice json: %v", err)
	}
	if toolChoice["type"] != ToolTypeFunction || toolChoice["name"] != "lookup" {
		t.Fatalf("tool_choice = %+v", toolChoice)
	}
}

func TestCompatIRChatRequestRejectsUnsupportedFields(t *testing.T) {
	req := &dto.GeneralOpenAIRequest{
		Model:    "chat-model",
		Messages: []dto.Message{{Role: "user", Content: "hello"}},
		Stop:     []any{"\n"},
	}
	if _, err := FromChatRequest(req); err == nil {
		t.Fatal("expected stop to be rejected")
	}

	req.Stop = nil
	req.FunctionCall = json.RawMessage(`"lookup"`)
	if _, err := FromChatRequest(req); err == nil {
		t.Fatal("expected unsupported function_call to be rejected")
	}
}

func TestCompatIRChatRequestToResponsesPreservesImageDetailAndFileFields(t *testing.T) {
	msg := dto.Message{Role: "user"}
	msg.SetMediaContent([]dto.MediaContent{
		{Type: dto.ContentTypeText, Text: "inspect"},
		{Type: dto.ContentTypeImageURL, ImageUrl: &dto.MessageImageUrl{Url: "https://example.test/a.png", Detail: "low"}},
		{Type: dto.ContentTypeFile, File: map[string]any{
			"file_id":   "file_123",
			"file_data": "data:application/pdf;base64,AAAA",
			"filename":  "a.pdf",
			"file_url":  "https://example.test/a.pdf",
		}},
	})
	req := &dto.GeneralOpenAIRequest{
		Model:    "chat-model",
		Messages: []dto.Message{msg},
	}

	ir, err := FromChatRequest(req)
	if err != nil {
		t.Fatalf("FromChatRequest error: %v", err)
	}
	responsesReq, err := ToResponsesRequest(ir)
	if err != nil {
		t.Fatalf("ToResponsesRequest error: %v", err)
	}

	var input []map[string]any
	if err := json.Unmarshal(responsesReq.Input, &input); err != nil {
		t.Fatalf("input json: %v", err)
	}
	content, ok := input[0]["content"].([]any)
	if !ok {
		t.Fatalf("content = %+v", input[0]["content"])
	}
	image, _ := content[1].(map[string]any)
	if image["type"] != "input_image" || image["image_url"] != "https://example.test/a.png" || image["detail"] != "low" {
		t.Fatalf("image part = %+v", image)
	}
	file, _ := content[2].(map[string]any)
	if file["type"] != "input_file" || file["file_id"] != "file_123" || file["file_data"] == "" || file["filename"] != "a.pdf" || file["file_url"] == "" {
		t.Fatalf("file part = %+v", file)
	}
	if _, nested := file["file"]; nested {
		t.Fatalf("input_file should not use nested file object: %+v", file)
	}
}

func TestCompatIRResponsesRequestToChatRequest(t *testing.T) {
	stream := true
	req := &dto.OpenAIResponsesRequest{
		Model:        "responses-model",
		Input:        json.RawMessage(`[{"role":"user","content":[{"type":"input_text","text":"hello"},{"type":"input_image","image_url":"https://example.test/a.png"}]},{"type":"function_call","call_id":"call_0","name":"lookup","arguments":{"q":"x"}},{"type":"function_call_output","call_id":"call_0","output":"done"}]`),
		Instructions: json.RawMessage(`"be concise"`),
		Stream:       &stream,
		Tools:        json.RawMessage(`[{"type":"function","name":"lookup","description":"lookup data","parameters":{"type":"object"}}]`),
		ToolChoice:   json.RawMessage(`{"type":"function","name":"lookup"}`),
	}

	ir, err := FromResponsesRequest(req)
	if err != nil {
		t.Fatalf("FromResponsesRequest error: %v", err)
	}
	if ir.Instructions != "be concise" || len(ir.Messages) != 3 {
		t.Fatalf("ir = %+v", ir)
	}
	if ir.Messages[0].Content[0].Text != "hello" || ir.Messages[0].Content[1].ImageURL == nil {
		t.Fatalf("content = %+v", ir.Messages[0].Content)
	}

	chatReq, err := ToChatRequest(ir)
	if err != nil {
		t.Fatalf("ToChatRequest error: %v", err)
	}
	if chatReq.Model != "responses-model" || chatReq.Stream == nil || !*chatReq.Stream {
		t.Fatalf("chat request = %+v", chatReq)
	}
	if len(chatReq.Messages) != 4 {
		t.Fatalf("chat messages = %+v", chatReq.Messages)
	}
	if chatReq.Messages[0].Role != "system" || chatReq.Messages[0].StringContent() != "be concise" {
		t.Fatalf("system message = %+v", chatReq.Messages[0])
	}
	if len(chatReq.Messages[2].ParseToolCalls()) != 1 || chatReq.Messages[3].ToolCallId != "call_0" {
		t.Fatalf("tool messages = %+v", chatReq.Messages)
	}
}

func TestCompatIRResponsesRequestRejectsStatefulFieldsForChat(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{
		Model:              "responses-model",
		Input:              json.RawMessage(`"hello"`),
		PreviousResponseID: "resp_prev",
	}

	if _, err := FromResponsesRequest(req); err == nil {
		t.Fatal("expected previous_response_id to be rejected")
	}
}

func TestCompatIRResponsesRequestRejectsBuiltInToolsForChat(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{
		Model: "responses-model",
		Input: json.RawMessage(`"hello"`),
		Tools: json.RawMessage(`[
			{"type":"web_search_preview"}
		]`),
	}

	if _, err := FromResponsesRequest(req); err == nil {
		t.Fatal("expected built-in Responses tool to be rejected")
	}
}

func mustRaw(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
