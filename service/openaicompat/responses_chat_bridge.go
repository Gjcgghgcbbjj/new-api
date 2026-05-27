package openaicompat

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/samber/lo"
)

func ResponsesRequestToChatCompletionsRequest(req *dto.OpenAIResponsesRequest) (*dto.GeneralOpenAIRequest, error) {
	if req == nil {
		return nil, errors.New("request is nil")
	}
	if req.Model == "" {
		return nil, errors.New("model is required")
	}

	messages, err := responsesInputToChatMessages(req.Input)
	if err != nil {
		return nil, err
	}

	if len(req.Instructions) > 0 && string(req.Instructions) != "null" {
		instructions := strings.TrimSpace(common.JsonRawMessageToString(req.Instructions))
		if instructions != "" {
			messages = append([]dto.Message{{
				Role:    "system",
				Content: instructions,
			}}, messages...)
		}
	}

	stream := lo.FromPtrOr(req.Stream, false)
	out := &dto.GeneralOpenAIRequest{
		Model:                req.Model,
		Messages:             messages,
		Stream:               &stream,
		StreamOptions:        req.StreamOptions,
		MaxTokens:            req.MaxOutputTokens,
		Temperature:          req.Temperature,
		TopP:                 req.TopP,
		User:                 req.User,
		Metadata:             req.Metadata,
		Store:                req.Store,
		PromptCacheKey:       common.JsonRawMessageToString(req.PromptCacheKey),
		Reasoning:            responsesReasoningToChatReasoning(req.Reasoning),
		ReasoningEffort:      responsesReasoningEffort(req.Reasoning),
		ParallelTooCalls:     rawBoolPtr(req.ParallelToolCalls),
		PromptCacheRetention: req.PromptCacheRetention,
	}

	if len(req.Tools) > 0 {
		out.Tools = responsesToolsToChatTools(req.Tools)
	}
	if len(req.ToolChoice) > 0 && string(req.ToolChoice) != "null" {
		out.ToolChoice = responsesToolChoiceToChatToolChoice(req.ToolChoice)
	}
	return out, nil
}

func ChatCompletionsResponseToResponsesResponse(resp *dto.OpenAITextResponse, original *dto.OpenAIResponsesRequest, id string) (*dto.OpenAIResponsesResponse, *dto.Usage, error) {
	if resp == nil {
		return nil, nil, errors.New("response is nil")
	}
	if oaiError := resp.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, nil, errors.New(oaiError.Message)
	}

	model := resp.Model
	if model == "" && original != nil {
		model = original.Model
	}
	createdAt := normalizeCreatedAt(resp.Created)
	if createdAt == 0 {
		createdAt = int(time.Now().Unix())
	}

	output, outputText := chatMessageToResponsesOutput(resp)
	if id == "" {
		id = resp.Id
	}
	if id == "" {
		id = fmt.Sprintf("resp_%d", time.Now().UnixNano())
	}

	usage := chatUsageToResponsesUsage(&resp.Usage)
	out := &dto.OpenAIResponsesResponse{
		ID:         id,
		Object:     "response",
		CreatedAt:  createdAt,
		Status:     json.RawMessage(`"completed"`),
		Model:      model,
		Output:     output,
		Usage:      usage,
		OutputText: outputText,
	}
	if original != nil {
		out.Instructions = original.Instructions
		out.ToolChoice = original.ToolChoice
		out.User = original.User
		out.Metadata = original.Metadata
		out.Reasoning = original.Reasoning
		out.Tools = original.GetToolsMap()
		if original.MaxOutputTokens != nil {
			out.MaxOutputTokens = int(*original.MaxOutputTokens)
		}
	}
	return out, usage, nil
}

func responsesInputToChatMessages(input json.RawMessage) ([]dto.Message, error) {
	if len(input) == 0 || string(input) == "null" {
		return []dto.Message{{Role: "user", Content: ""}}, nil
	}
	switch common.GetJsonType(input) {
	case "string":
		var text string
		if err := common.Unmarshal(input, &text); err != nil {
			return nil, err
		}
		return []dto.Message{{Role: "user", Content: text}}, nil
	case "array":
		var items []map[string]any
		if err := common.Unmarshal(input, &items); err != nil {
			return nil, err
		}
		messages := make([]dto.Message, 0, len(items))
		for _, item := range items {
			messages = append(messages, responseInputItemToChatMessages(item)...)
		}
		if len(messages) == 0 {
			messages = append(messages, dto.Message{Role: "user", Content: ""})
		}
		return messages, nil
	default:
		return []dto.Message{{Role: "user", Content: common.JsonRawMessageToString(input)}}, nil
	}
}

func responseInputItemToChatMessages(item map[string]any) []dto.Message {
	itemType := common.Interface2String(item["type"])
	switch itemType {
	case "function_call_output":
		return []dto.Message{{
			Role:       "tool",
			ToolCallId: common.Interface2String(item["call_id"]),
			Content:    interfaceToText(item["output"]),
		}}
	case "function_call":
		msg := dto.Message{
			Role:    "assistant",
			Content: "",
		}
		msg.SetToolCalls([]dto.ToolCallRequest{{
			ID:   common.Interface2String(firstNonEmpty(item["call_id"], item["id"])),
			Type: "function",
			Function: dto.FunctionRequest{
				Name:      common.Interface2String(item["name"]),
				Arguments: interfaceToText(item["arguments"]),
			},
		}})
		return []dto.Message{msg}
	}

	role := common.Interface2String(item["role"])
	if role == "" {
		role = "user"
	}
	if itemType == "message" || item["content"] != nil {
		return []dto.Message{{
			Role:    role,
			Content: responsesContentToChatContent(role, item["content"]),
		}}
	}
	return []dto.Message{{Role: role, Content: interfaceToText(item)}}
}

func responsesContentToChatContent(role string, content any) any {
	switch v := content.(type) {
	case nil:
		return ""
	case string:
		return v
	case []any:
		parts := make([]dto.MediaContent, 0, len(v))
		var textOnly strings.Builder
		allText := true
		for _, itemAny := range v {
			item, ok := itemAny.(map[string]any)
			if !ok {
				continue
			}
			switch common.Interface2String(item["type"]) {
			case "input_text", "output_text", "text":
				text := common.Interface2String(item["text"])
				textOnly.WriteString(text)
				parts = append(parts, dto.MediaContent{Type: dto.ContentTypeText, Text: text})
			case "input_image":
				allText = false
				parts = append(parts, dto.MediaContent{Type: dto.ContentTypeImageURL, ImageUrl: item["image_url"]})
			case "input_audio":
				allText = false
				parts = append(parts, dto.MediaContent{Type: dto.ContentTypeInputAudio, InputAudio: item["input_audio"]})
			case "input_file":
				allText = false
				parts = append(parts, dto.MediaContent{Type: dto.ContentTypeFile, File: item["file"]})
			}
		}
		if allText {
			return textOnly.String()
		}
		return parts
	default:
		return interfaceToText(v)
	}
}

func responsesToolsToChatTools(raw json.RawMessage) []dto.ToolCallRequest {
	var tools []map[string]any
	if err := common.Unmarshal(raw, &tools); err != nil {
		return nil
	}
	out := make([]dto.ToolCallRequest, 0, len(tools))
	for _, tool := range tools {
		if common.Interface2String(tool["type"]) != "function" {
			continue
		}
		out = append(out, dto.ToolCallRequest{
			Type: "function",
			Function: dto.FunctionRequest{
				Name:        common.Interface2String(firstNonEmpty(tool["name"], nestedValue(tool, "function", "name"))),
				Description: common.Interface2String(firstNonEmpty(tool["description"], nestedValue(tool, "function", "description"))),
				Parameters:  firstNonNil(tool["parameters"], nestedValue(tool, "function", "parameters")),
			},
		})
	}
	return out
}

func responsesToolChoiceToChatToolChoice(raw json.RawMessage) any {
	if common.GetJsonType(raw) == "string" {
		var choice string
		_ = common.Unmarshal(raw, &choice)
		return choice
	}
	var choice map[string]any
	if err := common.Unmarshal(raw, &choice); err != nil {
		return raw
	}
	if common.Interface2String(choice["type"]) == "function" && common.Interface2String(choice["name"]) != "" {
		return map[string]any{
			"type": "function",
			"function": map[string]any{
				"name": common.Interface2String(choice["name"]),
			},
		}
	}
	return choice
}

func chatMessageToResponsesOutput(resp *dto.OpenAITextResponse) ([]dto.ResponsesOutput, string) {
	if len(resp.Choices) == 0 {
		return nil, ""
	}
	msg := resp.Choices[0].Message
	output := make([]dto.ResponsesOutput, 0, 1)
	text := msg.StringContent()
	if text != "" {
		output = append(output, dto.ResponsesOutput{
			Type:   "message",
			ID:     fmt.Sprintf("msg_%d", time.Now().UnixNano()),
			Status: "completed",
			Role:   "assistant",
			Content: []dto.ResponsesOutputContent{{
				Type:        "output_text",
				Text:        text,
				Annotations: []interface{}{},
			}},
		})
	}
	for _, toolCall := range msg.ParseToolCalls() {
		if toolCall.Type != "" && toolCall.Type != "function" {
			continue
		}
		callID := strings.TrimSpace(toolCall.ID)
		if callID == "" {
			callID = fmt.Sprintf("call_%d", time.Now().UnixNano())
		}
		argsRaw, _ := common.Marshal(toolCall.Function.Arguments)
		output = append(output, dto.ResponsesOutput{
			Type:      "function_call",
			ID:        callID,
			Status:    "completed",
			CallId:    callID,
			Name:      toolCall.Function.Name,
			Arguments: argsRaw,
		})
	}
	return output, text
}

func chatUsageToResponsesUsage(usage *dto.Usage) *dto.Usage {
	if usage == nil {
		return nil
	}
	out := *usage
	if out.InputTokens == 0 {
		out.InputTokens = usage.PromptTokens
	}
	if out.OutputTokens == 0 {
		out.OutputTokens = usage.CompletionTokens
	}
	if out.TotalTokens == 0 {
		out.TotalTokens = out.InputTokens + out.OutputTokens
	}
	if out.PromptTokens == 0 {
		out.PromptTokens = out.InputTokens
	}
	if out.CompletionTokens == 0 {
		out.CompletionTokens = out.OutputTokens
	}
	return &out
}

func responsesReasoningEffort(reasoning *dto.Reasoning) string {
	if reasoning == nil {
		return ""
	}
	return reasoning.Effort
}

func responsesReasoningToChatReasoning(reasoning *dto.Reasoning) json.RawMessage {
	if reasoning == nil {
		return nil
	}
	raw, _ := common.Marshal(reasoning)
	return raw
}

func rawBoolPtr(raw json.RawMessage) *bool {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var value bool
	if err := common.Unmarshal(raw, &value); err != nil {
		return nil
	}
	return &value
}

func normalizeCreatedAt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return 0
	}
}

func interfaceToText(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, err := common.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

func firstNonEmpty(values ...any) any {
	for _, value := range values {
		if common.Interface2String(value) != "" {
			return value
		}
	}
	return nil
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func nestedValue(m map[string]any, key string, nested string) any {
	child, ok := m[key].(map[string]any)
	if !ok {
		return nil
	}
	return child[nested]
}
