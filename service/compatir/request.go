package compatir

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

type ConversionError struct {
	Field   string
	Message string
}

func (e *ConversionError) Error() string {
	if e == nil {
		return ""
	}
	if e.Field == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

func FromChatRequest(req *dto.GeneralOpenAIRequest) (*Request, error) {
	if req == nil {
		return nil, errors.New("request is nil")
	}
	if req.Model == "" {
		return nil, errors.New("model is required")
	}
	if req.N != nil && *req.N > 1 {
		return nil, fmt.Errorf("n>1 is not supported in responses compatibility mode")
	}

	out := &Request{
		Model:                req.Model,
		Stream:               req.Stream,
		StreamOptions:        req.StreamOptions,
		MaxOutputTokens:      maxOutputTokens(req),
		Temperature:          req.Temperature,
		TopP:                 copyFloat64Ptr(req.TopP),
		User:                 req.User,
		Metadata:             req.Metadata,
		Store:                req.Store,
		Text:                 convertChatResponseFormatToResponsesText(req.ResponseFormat),
		PromptCacheKey:       req.PromptCacheKey,
		PromptCacheRetention: req.PromptCacheRetention,
		ParallelToolCalls:    req.ParallelTooCalls,
	}
	if req.ReasoningEffort != "" {
		out.Reasoning = &dto.Reasoning{
			Effort:  req.ReasoningEffort,
			Summary: "detailed",
		}
	}
	if len(req.Tools) > 0 {
		out.Tools = make([]ToolDefinition, 0, len(req.Tools))
		for _, tool := range req.Tools {
			out.Tools = append(out.Tools, ToolDefinition{
				Type:        firstNonEmptyString(tool.Type, ToolTypeFunction),
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				Parameters:  tool.Function.Parameters,
			})
		}
	}
	if req.ToolChoice != nil {
		out.ToolChoice = chatToolChoiceToIR(req.ToolChoice)
	}

	var instructions []string
	for _, msg := range req.Messages {
		role := strings.TrimSpace(msg.Role)
		if role == "" {
			continue
		}
		switch role {
		case "system", "developer":
			text := messageText(msg)
			if strings.TrimSpace(text) != "" {
				instructions = append(instructions, strings.TrimSpace(text))
			}
			continue
		case "tool", "function":
			out.Messages = append(out.Messages, Message{
				Role:       "tool",
				ToolCallID: strings.TrimSpace(msg.ToolCallId),
				Content:    []ContentPart{{Type: ContentTypeText, Text: messageText(msg)}},
			})
			continue
		}

		irMsg := Message{
			Role:    role,
			Content: chatContentToIR(msg),
		}
		for idx, tool := range msg.ParseToolCalls() {
			if tool.Type != "" && tool.Type != ToolTypeFunction {
				out.AddWarning(WarningIgnoredField, "messages.tool_calls.type", "only function tool calls are represented by Responses")
				continue
			}
			irMsg.ToolCalls = append(irMsg.ToolCalls, ToolCall{
				Index:     idx,
				ID:        strings.TrimSpace(tool.ID),
				Type:      ToolTypeFunction,
				Name:      tool.Function.Name,
				Arguments: tool.Function.Arguments,
			})
		}
		out.Messages = append(out.Messages, irMsg)
	}
	if len(instructions) > 0 {
		out.Instructions = strings.Join(instructions, "\n\n")
	}
	return out, nil
}

func ToResponsesRequest(req *Request) (*dto.OpenAIResponsesRequest, error) {
	if req == nil {
		return nil, errors.New("request is nil")
	}
	if req.Model == "" {
		return nil, errors.New("model is required")
	}

	inputItems := make([]map[string]any, 0, len(req.Messages))
	for _, msg := range req.Messages {
		if msg.Role == "tool" {
			if strings.TrimSpace(msg.ToolCallID) == "" {
				inputItems = append(inputItems, map[string]any{
					"role":    "user",
					"content": "[tool_output_missing_call_id] " + contentPartsText(msg.Content),
				})
				continue
			}
			inputItems = append(inputItems, map[string]any{
				"type":    "function_call_output",
				"call_id": msg.ToolCallID,
				"output":  contentPartsText(msg.Content),
			})
			continue
		}

		item := map[string]any{
			"role": msg.Role,
		}
		item["content"] = irContentToResponsesContent(msg.Role, msg.Content)
		inputItems = append(inputItems, item)

		if msg.Role == "assistant" {
			for _, call := range StableToolCalls(msg.ToolCalls) {
				if strings.TrimSpace(call.ID) == "" || strings.TrimSpace(call.Name) == "" {
					continue
				}
				inputItems = append(inputItems, map[string]any{
					"type":      "function_call",
					"call_id":   call.ID,
					"name":      call.Name,
					"arguments": call.Arguments,
				})
			}
		}
	}

	inputRaw, err := common.Marshal(inputItems)
	if err != nil {
		return nil, err
	}
	out := &dto.OpenAIResponsesRequest{
		Model:                req.Model,
		Input:                inputRaw,
		Stream:               req.Stream,
		StreamOptions:        req.StreamOptions,
		MaxOutputTokens:      req.MaxOutputTokens,
		Temperature:          req.Temperature,
		TopP:                 req.TopP,
		User:                 req.User,
		Metadata:             req.Metadata,
		Store:                req.Store,
		Text:                 req.Text,
		PromptCacheRetention: req.PromptCacheRetention,
		Reasoning:            req.Reasoning,
	}
	if strings.TrimSpace(req.Instructions) != "" {
		out.Instructions, _ = common.Marshal(req.Instructions)
	}
	if req.PromptCacheKey != "" {
		out.PromptCacheKey, _ = common.Marshal(req.PromptCacheKey)
	}
	if req.ParallelToolCalls != nil {
		out.ParallelToolCalls, _ = common.Marshal(*req.ParallelToolCalls)
	}
	if len(req.Tools) > 0 {
		tools := make([]map[string]any, 0, len(req.Tools))
		for _, tool := range req.Tools {
			if tool.Type != "" && tool.Type != ToolTypeFunction {
				tools = append(tools, map[string]any{"type": tool.Type})
				continue
			}
			tools = append(tools, map[string]any{
				"type":        ToolTypeFunction,
				"name":        tool.Name,
				"description": tool.Description,
				"parameters":  tool.Parameters,
			})
		}
		out.Tools, _ = common.Marshal(tools)
	}
	if req.ToolChoice != nil {
		out.ToolChoice, _ = common.Marshal(irToolChoiceToResponses(req.ToolChoice))
	}
	return out, nil
}

func FromResponsesRequest(req *dto.OpenAIResponsesRequest) (*Request, error) {
	if req == nil {
		return nil, errors.New("request is nil")
	}
	if req.Model == "" {
		return nil, errors.New("model is required")
	}
	if strings.TrimSpace(req.PreviousResponseID) != "" {
		return nil, &ConversionError{Field: "previous_response_id", Message: "cannot be represented by chat completions"}
	}
	if rawJSONSet(req.Conversation) {
		return nil, &ConversionError{Field: "conversation", Message: "cannot be represented by chat completions"}
	}
	if rawJSONSet(req.ContextManagement) {
		return nil, &ConversionError{Field: "context_management", Message: "cannot be represented by chat completions"}
	}
	if rawJSONSet(req.Include) {
		return nil, &ConversionError{Field: "include", Message: "cannot be represented by chat completions"}
	}

	messages, err := responsesInputToIRMessages(req.Input)
	if err != nil {
		return nil, err
	}
	out := &Request{
		Model:                req.Model,
		Instructions:         strings.TrimSpace(common.JsonRawMessageToString(req.Instructions)),
		Messages:             messages,
		Stream:               req.Stream,
		StreamOptions:        req.StreamOptions,
		MaxOutputTokens:      req.MaxOutputTokens,
		Temperature:          req.Temperature,
		TopP:                 copyFloat64Ptr(req.TopP),
		User:                 req.User,
		Metadata:             req.Metadata,
		Store:                req.Store,
		PromptCacheKey:       common.JsonRawMessageToString(req.PromptCacheKey),
		PromptCacheRetention: req.PromptCacheRetention,
		Reasoning:            req.Reasoning,
		ParallelToolCalls:    rawBoolPtr(req.ParallelToolCalls),
	}
	if rawJSONSet(req.Tools) {
		tools, err := responsesToolsToIR(req.Tools)
		if err != nil {
			return nil, err
		}
		out.Tools = tools
	}
	if rawJSONSet(req.ToolChoice) {
		out.ToolChoice = responsesToolChoiceToIR(req.ToolChoice)
	}
	return out, nil
}

func ToChatRequest(req *Request) (*dto.GeneralOpenAIRequest, error) {
	if req == nil {
		return nil, errors.New("request is nil")
	}
	if req.Model == "" {
		return nil, errors.New("model is required")
	}

	messages := make([]dto.Message, 0, len(req.Messages)+1)
	if strings.TrimSpace(req.Instructions) != "" {
		messages = append(messages, dto.Message{
			Role:    "system",
			Content: req.Instructions,
		})
	}
	for _, msg := range req.Messages {
		if msg.Role == "tool" {
			messages = append(messages, dto.Message{
				Role:       "tool",
				ToolCallId: msg.ToolCallID,
				Content:    contentPartsText(msg.Content),
			})
			continue
		}
		chatMsg := dto.Message{
			Role:    firstNonEmptyString(msg.Role, "user"),
			Content: irContentToChatContent(msg.Content),
		}
		if len(msg.ToolCalls) > 0 {
			toolCalls := make([]dto.ToolCallRequest, 0, len(msg.ToolCalls))
			for _, call := range StableToolCalls(msg.ToolCalls) {
				if strings.TrimSpace(call.Name) == "" {
					continue
				}
				toolCalls = append(toolCalls, dto.ToolCallRequest{
					ID:   call.ID,
					Type: ToolTypeFunction,
					Function: dto.FunctionRequest{
						Name:      call.Name,
						Arguments: call.Arguments,
					},
				})
			}
			chatMsg.SetToolCalls(toolCalls)
		}
		messages = append(messages, chatMsg)
	}

	out := &dto.GeneralOpenAIRequest{
		Model:                req.Model,
		Messages:             messages,
		Stream:               req.Stream,
		StreamOptions:        req.StreamOptions,
		MaxTokens:            req.MaxOutputTokens,
		Temperature:          req.Temperature,
		TopP:                 copyFloat64Ptr(req.TopP),
		User:                 req.User,
		Metadata:             req.Metadata,
		Store:                req.Store,
		PromptCacheKey:       req.PromptCacheKey,
		PromptCacheRetention: req.PromptCacheRetention,
		Reasoning:            reasoningRaw(req.Reasoning),
		ReasoningEffort:      reasoningEffort(req.Reasoning),
		ParallelTooCalls:     req.ParallelToolCalls,
	}
	if len(req.Tools) > 0 {
		out.Tools = make([]dto.ToolCallRequest, 0, len(req.Tools))
		for _, tool := range req.Tools {
			out.Tools = append(out.Tools, dto.ToolCallRequest{
				Type: firstNonEmptyString(tool.Type, ToolTypeFunction),
				Function: dto.FunctionRequest{
					Name:        tool.Name,
					Description: tool.Description,
					Parameters:  tool.Parameters,
				},
			})
		}
	}
	if req.ToolChoice != nil {
		out.ToolChoice = irToolChoiceToChat(req.ToolChoice)
	}
	return out, nil
}

func maxOutputTokens(req *dto.GeneralOpenAIRequest) *uint {
	if req == nil {
		return nil
	}
	maxOutputTokens := uint(0)
	if req.MaxTokens != nil {
		maxOutputTokens = *req.MaxTokens
	}
	if req.MaxCompletionTokens != nil && *req.MaxCompletionTokens > maxOutputTokens {
		maxOutputTokens = *req.MaxCompletionTokens
	}
	if maxOutputTokens == 0 && req.MaxTokens == nil && req.MaxCompletionTokens == nil {
		return nil
	}
	return &maxOutputTokens
}

func convertChatResponseFormatToResponsesText(reqFormat *dto.ResponseFormat) json.RawMessage {
	if reqFormat == nil || strings.TrimSpace(reqFormat.Type) == "" {
		return nil
	}

	format := map[string]any{
		"type": reqFormat.Type,
	}
	if reqFormat.Type == "json_schema" && len(reqFormat.JsonSchema) > 0 {
		var chatSchema map[string]any
		if err := common.Unmarshal(reqFormat.JsonSchema, &chatSchema); err == nil {
			for key, value := range chatSchema {
				if key == "type" {
					continue
				}
				format[key] = value
			}
			if nested, ok := format["json_schema"].(map[string]any); ok {
				for key, value := range nested {
					if _, exists := format[key]; !exists {
						format[key] = value
					}
				}
				delete(format, "json_schema")
			}
		} else {
			format["json_schema"] = reqFormat.JsonSchema
		}
	}

	raw, _ := common.Marshal(map[string]any{
		"format": format,
	})
	return raw
}

func messageText(msg dto.Message) string {
	if msg.Content == nil {
		return ""
	}
	if msg.IsStringContent() {
		return msg.StringContent()
	}
	return contentPartsText(chatContentToIR(msg))
}

func chatContentToIR(msg dto.Message) []ContentPart {
	if msg.Content == nil {
		return nil
	}
	if msg.IsStringContent() {
		return []ContentPart{{Type: ContentTypeText, Text: msg.StringContent()}}
	}
	parts := msg.ParseContent()
	out := make([]ContentPart, 0, len(parts))
	for _, part := range parts {
		out = append(out, ContentPart{
			Type:       part.Type,
			Text:       part.Text,
			ImageURL:   part.ImageUrl,
			InputAudio: part.InputAudio,
			File:       part.File,
			VideoURL:   part.VideoUrl,
		})
	}
	return out
}

func contentPartsText(parts []ContentPart) string {
	var sb strings.Builder
	for _, part := range parts {
		if part.Type == ContentTypeText || part.Type == "" {
			sb.WriteString(part.Text)
			continue
		}
		raw, err := common.Marshal(partPayload(part))
		if err == nil {
			sb.Write(raw)
		}
	}
	return sb.String()
}

func partPayload(part ContentPart) any {
	switch part.Type {
	case ContentTypeText, "":
		return part.Text
	case ContentTypeImageURL:
		return part.ImageURL
	case ContentTypeInputAudio:
		return part.InputAudio
	case ContentTypeFile:
		return part.File
	case ContentTypeVideoURL:
		return part.VideoURL
	default:
		return part.Text
	}
}

func irContentToResponsesContent(role string, parts []ContentPart) any {
	if len(parts) == 0 {
		return ""
	}
	textOnly := true
	var text strings.Builder
	contentParts := make([]map[string]any, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case ContentTypeText, "":
			partType := "input_text"
			if role == "assistant" {
				partType = "output_text"
			}
			text.WriteString(part.Text)
			contentParts = append(contentParts, map[string]any{"type": partType, "text": part.Text})
		case ContentTypeImageURL:
			textOnly = false
			contentParts = append(contentParts, map[string]any{"type": "input_image", "image_url": normalizeChatImageURLToString(part.ImageURL)})
		case ContentTypeInputAudio:
			textOnly = false
			contentParts = append(contentParts, map[string]any{"type": "input_audio", "input_audio": part.InputAudio})
		case ContentTypeFile:
			textOnly = false
			contentParts = append(contentParts, map[string]any{"type": "input_file", "file": part.File})
		case ContentTypeVideoURL:
			textOnly = false
			contentParts = append(contentParts, map[string]any{"type": "input_video", "video_url": part.VideoURL})
		}
	}
	if textOnly {
		return text.String()
	}
	return contentParts
}

func irContentToChatContent(parts []ContentPart) any {
	if len(parts) == 0 {
		return ""
	}
	textOnly := true
	var text strings.Builder
	media := make([]dto.MediaContent, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case ContentTypeText, "":
			text.WriteString(part.Text)
			media = append(media, dto.MediaContent{Type: dto.ContentTypeText, Text: part.Text})
		case ContentTypeImageURL:
			textOnly = false
			media = append(media, dto.MediaContent{Type: dto.ContentTypeImageURL, ImageUrl: part.ImageURL})
		case ContentTypeInputAudio:
			textOnly = false
			media = append(media, dto.MediaContent{Type: dto.ContentTypeInputAudio, InputAudio: part.InputAudio})
		case ContentTypeFile:
			textOnly = false
			media = append(media, dto.MediaContent{Type: dto.ContentTypeFile, File: part.File})
		case ContentTypeVideoURL:
			textOnly = false
			media = append(media, dto.MediaContent{Type: dto.ContentTypeVideoUrl, VideoUrl: part.VideoURL})
		}
	}
	if textOnly {
		return text.String()
	}
	return media
}

func responsesInputToIRMessages(input json.RawMessage) ([]Message, error) {
	if !rawJSONSet(input) {
		return []Message{{Role: "user", Content: []ContentPart{{Type: ContentTypeText}}}}, nil
	}
	switch common.GetJsonType(input) {
	case "string":
		var text string
		if err := common.Unmarshal(input, &text); err != nil {
			return nil, err
		}
		return []Message{{Role: "user", Content: []ContentPart{{Type: ContentTypeText, Text: text}}}}, nil
	case "array":
		var items []map[string]any
		if err := common.Unmarshal(input, &items); err != nil {
			return nil, err
		}
		messages := make([]Message, 0, len(items))
		for _, item := range items {
			messages = append(messages, responsesInputItemToIRMessages(item)...)
		}
		if len(messages) == 0 {
			messages = append(messages, Message{Role: "user", Content: []ContentPart{{Type: ContentTypeText}}})
		}
		return messages, nil
	default:
		return []Message{{Role: "user", Content: []ContentPart{{Type: ContentTypeText, Text: common.JsonRawMessageToString(input)}}}}, nil
	}
}

func responsesInputItemToIRMessages(item map[string]any) []Message {
	itemType := common.Interface2String(item["type"])
	switch itemType {
	case "function_call_output":
		return []Message{{
			Role:       "tool",
			ToolCallID: common.Interface2String(item["call_id"]),
			Content:    []ContentPart{{Type: ContentTypeText, Text: interfaceToText(item["output"])}},
		}}
	case "function_call":
		return []Message{{
			Role: "assistant",
			ToolCalls: []ToolCall{{
				Index:     0,
				ID:        common.Interface2String(firstNonEmptyAny(item["call_id"], item["id"])),
				Type:      ToolTypeFunction,
				Name:      common.Interface2String(item["name"]),
				Arguments: interfaceToText(item["arguments"]),
			}},
		}}
	}

	role := common.Interface2String(item["role"])
	if role == "" {
		role = "user"
	}
	if itemType == "message" || item["content"] != nil {
		return []Message{{
			Role:    role,
			Content: responsesContentToIRContent(item["content"]),
		}}
	}
	return []Message{{Role: role, Content: []ContentPart{{Type: ContentTypeText, Text: interfaceToText(item)}}}}
}

func responsesContentToIRContent(content any) []ContentPart {
	switch v := content.(type) {
	case nil:
		return nil
	case string:
		return []ContentPart{{Type: ContentTypeText, Text: v}}
	case []any:
		parts := make([]ContentPart, 0, len(v))
		for _, itemAny := range v {
			item, ok := itemAny.(map[string]any)
			if !ok {
				continue
			}
			switch common.Interface2String(item["type"]) {
			case "input_text", "output_text", "text":
				parts = append(parts, ContentPart{Type: ContentTypeText, Text: common.Interface2String(item["text"])})
			case "input_image":
				parts = append(parts, ContentPart{Type: ContentTypeImageURL, ImageURL: item["image_url"]})
			case "input_audio":
				parts = append(parts, ContentPart{Type: ContentTypeInputAudio, InputAudio: item["input_audio"]})
			case "input_file":
				parts = append(parts, ContentPart{Type: ContentTypeFile, File: item["file"]})
			case "input_video":
				parts = append(parts, ContentPart{Type: ContentTypeVideoURL, VideoURL: item["video_url"]})
			}
		}
		return parts
	default:
		return []ContentPart{{Type: ContentTypeText, Text: interfaceToText(v)}}
	}
}

func responsesToolsToIR(raw json.RawMessage) ([]ToolDefinition, error) {
	var tools []map[string]any
	if err := common.Unmarshal(raw, &tools); err != nil {
		return nil, err
	}
	out := make([]ToolDefinition, 0, len(tools))
	for idx, tool := range tools {
		toolType := common.Interface2String(tool["type"])
		if toolType != "" && toolType != ToolTypeFunction {
			return nil, &ConversionError{
				Field:   fmt.Sprintf("tools[%d].type", idx),
				Message: fmt.Sprintf("Responses built-in tool %q cannot be represented by chat completions", toolType),
			}
		}
		out = append(out, ToolDefinition{
			Type:        ToolTypeFunction,
			Name:        common.Interface2String(firstNonEmptyAny(tool["name"], nestedValue(tool, "function", "name"))),
			Description: common.Interface2String(firstNonEmptyAny(tool["description"], nestedValue(tool, "function", "description"))),
			Parameters:  firstNonNilAny(tool["parameters"], nestedValue(tool, "function", "parameters")),
		})
	}
	return out, nil
}

func chatToolChoiceToIR(raw any) *ToolChoice {
	switch v := raw.(type) {
	case string:
		return &ToolChoice{Mode: v}
	default:
		var m map[string]any
		if b, err := common.Marshal(v); err == nil {
			_ = common.Unmarshal(b, &m)
		}
		if m == nil {
			return &ToolChoice{Raw: v}
		}
		if common.Interface2String(m["type"]) == ToolTypeFunction {
			name := common.Interface2String(m["name"])
			if name == "" {
				name = common.Interface2String(nestedValue(m, "function", "name"))
			}
			return &ToolChoice{Mode: ToolTypeFunction, FunctionName: name, Raw: v}
		}
		return &ToolChoice{Mode: common.Interface2String(m["type"]), Raw: v}
	}
}

func responsesToolChoiceToIR(raw json.RawMessage) *ToolChoice {
	if common.GetJsonType(raw) == "string" {
		var choice string
		_ = common.Unmarshal(raw, &choice)
		return &ToolChoice{Mode: choice}
	}
	var choice map[string]any
	if err := common.Unmarshal(raw, &choice); err != nil {
		return &ToolChoice{Raw: raw}
	}
	if common.Interface2String(choice["type"]) == ToolTypeFunction {
		return &ToolChoice{Mode: ToolTypeFunction, FunctionName: common.Interface2String(choice["name"]), Raw: choice}
	}
	return &ToolChoice{Mode: common.Interface2String(choice["type"]), Raw: choice}
}

func irToolChoiceToResponses(choice *ToolChoice) any {
	if choice == nil {
		return nil
	}
	if choice.Mode == ToolTypeFunction && choice.FunctionName != "" {
		return map[string]any{"type": ToolTypeFunction, "name": choice.FunctionName}
	}
	if choice.Mode != "" && choice.Raw == nil {
		return choice.Mode
	}
	return choice.Raw
}

func irToolChoiceToChat(choice *ToolChoice) any {
	if choice == nil {
		return nil
	}
	if choice.Mode == ToolTypeFunction && choice.FunctionName != "" {
		return map[string]any{"type": ToolTypeFunction, "function": map[string]any{"name": choice.FunctionName}}
	}
	if choice.Mode != "" && choice.Raw == nil {
		return choice.Mode
	}
	return choice.Raw
}

func normalizeChatImageURLToString(v any) any {
	switch vv := v.(type) {
	case string:
		return vv
	case map[string]any:
		if url := common.Interface2String(vv["url"]); url != "" {
			return url
		}
		return v
	case dto.MessageImageUrl:
		if vv.Url != "" {
			return vv.Url
		}
		return v
	case *dto.MessageImageUrl:
		if vv != nil && vv.Url != "" {
			return vv.Url
		}
		return v
	default:
		return v
	}
}

func rawBoolPtr(raw json.RawMessage) *bool {
	if !rawJSONSet(raw) {
		return nil
	}
	var value bool
	if err := common.Unmarshal(raw, &value); err != nil {
		return nil
	}
	return &value
}

func rawJSONSet(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed != "" && trimmed != "null"
}

func reasoningRaw(reasoning *dto.Reasoning) json.RawMessage {
	if reasoning == nil {
		return nil
	}
	raw, _ := common.Marshal(reasoning)
	return raw
}

func reasoningEffort(reasoning *dto.Reasoning) string {
	if reasoning == nil {
		return ""
	}
	return reasoning.Effort
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

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func firstNonEmptyAny(values ...any) any {
	for _, value := range values {
		if common.Interface2String(value) != "" {
			return value
		}
	}
	return nil
}

func firstNonNilAny(values ...any) any {
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

func copyFloat64Ptr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
