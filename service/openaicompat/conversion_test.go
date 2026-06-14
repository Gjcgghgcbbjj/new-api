package openaicompat

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
)

type testResponsesPolicy struct {
	enabled         bool
	modelPatterns   []string
	excludePatterns []string
	matchTarget     string
}

func (p testResponsesPolicy) IsChannelEnabled(channelID int, channelType int) bool {
	return p.enabled
}

func (p testResponsesPolicy) GetModelPatterns() []string {
	return p.modelPatterns
}

func (p testResponsesPolicy) GetExcludePatterns() []string {
	return p.excludePatterns
}

func (p testResponsesPolicy) GetMatchTarget() string {
	return p.matchTarget
}

func TestResponsesResponseToChatCompletionsResponse(t *testing.T) {
	tests := []struct {
		name         string
		resp         *dto.OpenAIResponsesResponse
		wantContent  string
		wantFinish   string
		wantToolCall bool
	}{
		{
			name: "text and function call",
			resp: &dto.OpenAIResponsesResponse{
				CreatedAt: 123,
				Model:     "gpt-4.1",
				Output: []dto.ResponsesOutput{
					{
						Type: "message",
						Role: "assistant",
						Content: []dto.ResponsesOutputContent{
							{Type: "output_text", Text: "hello "},
							{Type: "output_text", Text: "world"},
						},
					},
					{
						Type:      "function_call",
						ID:        "fc_1",
						CallId:    "call_1",
						Name:      "lookup_weather",
						Arguments: []byte(`{"city":"shanghai"}`),
					},
				},
				Usage: &dto.Usage{
					InputTokens:  3,
					OutputTokens: 4,
					TotalTokens:  7,
				},
			},
			wantContent:  "hello world",
			wantFinish:   "tool_calls",
			wantToolCall: true,
		},
		{
			name: "text only",
			resp: &dto.OpenAIResponsesResponse{
				CreatedAt: 456,
				Model:     "gpt-4.1-mini",
				Output: []dto.ResponsesOutput{
					{
						Type: "message",
						Role: "assistant",
						Content: []dto.ResponsesOutputContent{
							{Type: "output_text", Text: "plain text"},
						},
					},
				},
			},
			wantContent: "plain text",
			wantFinish:  "stop",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, usage, err := ResponsesResponseToChatCompletionsResponse(tt.resp, "chatcmpl_test")
			if err != nil {
				t.Fatalf("ResponsesResponseToChatCompletionsResponse() error = %v", err)
			}
			if got.Id != "chatcmpl_test" {
				t.Fatalf("Id = %q, want %q", got.Id, "chatcmpl_test")
			}
			if got.Object != "chat.completion" {
				t.Fatalf("Object = %q, want %q", got.Object, "chat.completion")
			}
			if got.Model != tt.resp.Model {
				t.Fatalf("Model = %q, want %q", got.Model, tt.resp.Model)
			}
			if len(got.Choices) != 1 {
				t.Fatalf("len(Choices) = %d, want 1", len(got.Choices))
			}

			choice := got.Choices[0]
			if choice.Message.Role != "assistant" {
				t.Fatalf("Message.Role = %q, want %q", choice.Message.Role, "assistant")
			}
			if choice.Message.Content != tt.wantContent {
				t.Fatalf("Message.Content = %v, want %q", choice.Message.Content, tt.wantContent)
			}
			if choice.FinishReason != tt.wantFinish {
				t.Fatalf("FinishReason = %q, want %q", choice.FinishReason, tt.wantFinish)
			}

			toolCalls := choice.Message.ParseToolCalls()
			if tt.wantToolCall {
				if len(toolCalls) != 1 {
					t.Fatalf("len(toolCalls) = %d, want 1", len(toolCalls))
				}
				if toolCalls[0].ID != "call_1" {
					t.Fatalf("toolCalls[0].ID = %q, want %q", toolCalls[0].ID, "call_1")
				}
				if toolCalls[0].Type != "function" {
					t.Fatalf("toolCalls[0].Type = %q, want %q", toolCalls[0].Type, "function")
				}
				if toolCalls[0].Function.Name != "lookup_weather" {
					t.Fatalf("toolCalls[0].Function.Name = %q, want %q", toolCalls[0].Function.Name, "lookup_weather")
				}
				if toolCalls[0].Function.Arguments != `{"city":"shanghai"}` {
					t.Fatalf("toolCalls[0].Function.Arguments = %q", toolCalls[0].Function.Arguments)
				}
			} else if len(toolCalls) != 0 {
				t.Fatalf("len(toolCalls) = %d, want 0", len(toolCalls))
			}

			if tt.resp.Usage != nil {
				if usage.PromptTokens != 3 || usage.CompletionTokens != 4 || usage.TotalTokens != 7 {
					t.Fatalf("usage = %+v, want prompt=3 completion=4 total=7", usage)
				}
				if got.Usage.PromptTokens != 3 || got.Usage.CompletionTokens != 4 || got.Usage.TotalTokens != 7 {
					t.Fatalf("got.Usage = %+v, want prompt=3 completion=4 total=7", got.Usage)
				}
			}
		})
	}
}

func TestResponsesFinishReason(t *testing.T) {
	tests := []struct {
		name         string
		resp         *dto.OpenAIResponsesResponse
		hasToolCalls bool
		want         string
	}{
		{
			name:         "tool calls",
			resp:         &dto.OpenAIResponsesResponse{Status: mustRawJSON(t, "completed")},
			hasToolCalls: true,
			want:         "tool_calls",
		},
		{
			name: "max output tokens",
			resp: &dto.OpenAIResponsesResponse{
				Status:            mustRawJSON(t, "incomplete"),
				IncompleteDetails: &dto.IncompleteDetails{Reasoning: "max_output_tokens"},
			},
			want: "length",
		},
		{
			name: "content filter",
			resp: &dto.OpenAIResponsesResponse{
				Status:            mustRawJSON(t, "incomplete"),
				IncompleteDetails: &dto.IncompleteDetails{Reasoning: "content_filter"},
			},
			want: "content_filter",
		},
		{
			name: "default stop",
			resp: &dto.OpenAIResponsesResponse{Status: mustRawJSON(t, "completed")},
			want: "stop",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResponsesFinishReason(tt.resp, tt.hasToolCalls); got != tt.want {
				t.Fatalf("ResponsesFinishReason() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestChatCompletionsRequestToResponsesRequestLegacyFunctions(t *testing.T) {
	req := &dto.GeneralOpenAIRequest{
		Model: "gpt-4.1",
		Messages: []dto.Message{
			{Role: "user", Content: "hello"},
		},
		Functions: mustRawJSON(t, []map[string]any{
			{
				"name":        "lookup_weather",
				"description": "Lookup weather",
				"parameters":  map[string]any{"type": "object"},
			},
		}),
		FunctionCall: mustRawJSON(t, map[string]any{"name": "lookup_weather"}),
	}

	got, err := ChatCompletionsRequestToResponsesRequest(req)
	if err != nil {
		t.Fatalf("ChatCompletionsRequestToResponsesRequest() error = %v", err)
	}

	var tools []map[string]any
	mustUnmarshal(t, got.Tools, &tools)
	if len(tools) != 1 {
		t.Fatalf("len(tools) = %d, want 1", len(tools))
	}
	if tools[0]["type"] != "function" {
		t.Fatalf("tools[0].type = %v, want function", tools[0]["type"])
	}
	if tools[0]["name"] != "lookup_weather" {
		t.Fatalf("tools[0].name = %v, want lookup_weather", tools[0]["name"])
	}
	if tools[0]["description"] != "Lookup weather" {
		t.Fatalf("tools[0].description = %v, want Lookup weather", tools[0]["description"])
	}
	parameters, ok := tools[0]["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("tools[0].parameters = %T, want map[string]any", tools[0]["parameters"])
	}
	if parameters["type"] != "object" {
		t.Fatalf("tools[0].parameters.type = %v, want object", parameters["type"])
	}

	var toolChoice map[string]any
	mustUnmarshal(t, got.ToolChoice, &toolChoice)
	if toolChoice["type"] != "function" {
		t.Fatalf("toolChoice.type = %v, want function", toolChoice["type"])
	}
	if toolChoice["name"] != "lookup_weather" {
		t.Fatalf("toolChoice.name = %v, want lookup_weather", toolChoice["name"])
	}
}

func TestChatCompletionsRequestToResponsesRequestRejectsStop(t *testing.T) {
	_, err := ChatCompletionsRequestToResponsesRequest(&dto.GeneralOpenAIRequest{
		Model: "gpt-4.1",
		Messages: []dto.Message{
			{Role: "user", Content: "hello"},
		},
		Stop: "\n",
	})
	if err == nil {
		t.Fatal("ChatCompletionsRequestToResponsesRequest() error = nil, want stop error")
	}
	if !strings.Contains(err.Error(), "stop is not supported") {
		t.Fatalf("error = %q, want stop is not supported", err.Error())
	}
}

func TestChatCompletionsRequestToResponsesRequestMediaParts(t *testing.T) {
	req := &dto.GeneralOpenAIRequest{
		Model: "gpt-4.1",
		Messages: []dto.Message{
			{
				Role: "user",
				Content: []dto.MediaContent{
					{Type: dto.ContentTypeText, Text: "inspect these files"},
					{
						Type: dto.ContentTypeImageURL,
						ImageUrl: dto.MessageImageUrl{
							Url:    "https://example.com/image.png",
							Detail: "high",
						},
					},
					{
						Type: dto.ContentTypeFile,
						File: map[string]any{
							"file_id":   "file_123",
							"file_data": "data:application/pdf;base64,AAAA",
							"file_name": "brief.pdf",
							"file_url":  "https://example.com/brief.pdf",
						},
					},
				},
			},
		},
	}

	got, err := ChatCompletionsRequestToResponsesRequest(req)
	if err != nil {
		t.Fatalf("ChatCompletionsRequestToResponsesRequest() error = %v", err)
	}

	var input []map[string]any
	mustUnmarshal(t, got.Input, &input)
	if len(input) != 1 {
		t.Fatalf("len(input) = %d, want 1", len(input))
	}
	if input[0]["role"] != "user" {
		t.Fatalf("input[0].role = %v, want user", input[0]["role"])
	}

	content, ok := input[0]["content"].([]any)
	if !ok {
		t.Fatalf("input[0].content = %T, want []any", input[0]["content"])
	}
	if len(content) != 3 {
		t.Fatalf("len(input[0].content) = %d, want 3", len(content))
	}

	image := mapAt(t, content, 1)
	if image["type"] != "input_image" {
		t.Fatalf("image.type = %v, want input_image", image["type"])
	}
	if image["image_url"] != "https://example.com/image.png" {
		t.Fatalf("image.image_url = %v", image["image_url"])
	}
	if image["detail"] != "high" {
		t.Fatalf("image.detail = %v, want high", image["detail"])
	}

	file := mapAt(t, content, 2)
	if file["type"] != "input_file" {
		t.Fatalf("file.type = %v, want input_file", file["type"])
	}
	if file["file_id"] != "file_123" {
		t.Fatalf("file.file_id = %v, want file_123", file["file_id"])
	}
	if file["file_data"] != "data:application/pdf;base64,AAAA" {
		t.Fatalf("file.file_data = %v", file["file_data"])
	}
	if file["filename"] != "brief.pdf" {
		t.Fatalf("file.filename = %v, want brief.pdf", file["filename"])
	}
	if file["file_url"] != "https://example.com/brief.pdf" {
		t.Fatalf("file.file_url = %v", file["file_url"])
	}
}

func TestShouldChatCompletionsUseResponsesPolicy(t *testing.T) {
	tests := []struct {
		name          string
		policy        testResponsesPolicy
		originModel   string
		upstreamModel string
		want          bool
	}{
		{
			name: "model pattern match without exclude",
			policy: testResponsesPolicy{
				enabled:       true,
				modelPatterns: []string{`^gpt-4\.1$`},
			},
			originModel: "gpt-4.1",
			want:        true,
		},
		{
			name: "exclude pattern wins",
			policy: testResponsesPolicy{
				enabled:         true,
				modelPatterns:   []string{`^gpt-4`},
				excludePatterns: []string{`mini`},
			},
			originModel: "gpt-4.1-mini",
			want:        false,
		},
		{
			name: "upstream match target uses upstream model",
			policy: testResponsesPolicy{
				enabled:       true,
				modelPatterns: []string{`^o3$`},
				matchTarget:   "upstream",
			},
			originModel:   "gpt-4.1",
			upstreamModel: "o3",
			want:          true,
		},
		{
			name: "origin match target uses origin model",
			policy: testResponsesPolicy{
				enabled:       true,
				modelPatterns: []string{`^gpt-4\.1$`},
				matchTarget:   "origin",
			},
			originModel:   "gpt-4.1",
			upstreamModel: "o3",
			want:          true,
		},
		{
			name: "upstream match target falls back to origin when upstream is empty",
			policy: testResponsesPolicy{
				enabled:       true,
				modelPatterns: []string{`^gpt-4\.1$`},
				matchTarget:   "upstream",
			},
			originModel:   "gpt-4.1",
			upstreamModel: "",
			want:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShouldChatCompletionsUseResponsesPolicy(
				tt.policy,
				1,
				constant.ChannelTypeOpenAI,
				tt.originModel,
				tt.upstreamModel,
			)
			if got != tt.want {
				t.Fatalf("ShouldChatCompletionsUseResponsesPolicy() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSupportsResponsesConversion(t *testing.T) {
	tests := []struct {
		name        string
		channelType int
		want        bool
	}{
		{name: "openai", channelType: constant.ChannelTypeOpenAI, want: true},
		{name: "azure", channelType: constant.ChannelTypeAzure, want: true},
		{name: "ali", channelType: constant.ChannelTypeAli, want: true},
		{name: "cloudflare", channelType: constant.ChannelCloudflare, want: true},
		{name: "perplexity", channelType: constant.ChannelTypePerplexity, want: true},
		{name: "volcengine", channelType: constant.ChannelTypeVolcEngine, want: true},
		{name: "xai", channelType: constant.ChannelTypeXai, want: true},
		{name: "codex", channelType: constant.ChannelTypeCodex, want: true},
		{name: "anthropic", channelType: constant.ChannelTypeAnthropic, want: false},
		{name: "aws", channelType: constant.ChannelTypeAws, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SupportsResponsesConversion(tt.channelType); got != tt.want {
				t.Fatalf("SupportsResponsesConversion(%d) = %v, want %v", tt.channelType, got, tt.want)
			}
		})
	}
}

func TestValidateRegexPatterns(t *testing.T) {
	tests := []struct {
		name    string
		pattern []string
		wantErr bool
	}{
		{name: "valid patterns", pattern: []string{`^gpt-4`, `(?i)o3`}},
		{name: "invalid pattern", pattern: []string{`[`}, wantErr: true},
		{name: "empty patterns skipped", pattern: []string{"", `^ok$`}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRegexPatterns(tt.pattern)
			if tt.wantErr && err == nil {
				t.Fatal("ValidateRegexPatterns() error = nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("ValidateRegexPatterns() error = %v", err)
			}
		})
	}
}

func mustRawJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := common.Marshal(v)
	if err != nil {
		t.Fatalf("common.Marshal() error = %v", err)
	}
	return b
}

func mustUnmarshal(t *testing.T, data []byte, v any) {
	t.Helper()
	if err := common.Unmarshal(data, v); err != nil {
		t.Fatalf("common.Unmarshal() error = %v; data = %s", err, string(data))
	}
}

func mapAt(t *testing.T, items []any, index int) map[string]any {
	t.Helper()
	item, ok := items[index].(map[string]any)
	if !ok {
		t.Fatalf("items[%d] = %T, want map[string]any", index, items[index])
	}
	return item
}
