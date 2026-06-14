package openaicompat

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/samber/lo"
)

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

	textRaw, _ := common.Marshal(map[string]any{
		"format": format,
	})
	return textRaw
}

func rawJSONIsSet(data json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(data))
	return trimmed != "" && trimmed != "null"
}

func responsesInputImagePart(part dto.MediaContent) map[string]any {
	item := map[string]any{
		"type":      "input_image",
		"image_url": normalizeChatImageURLToString(part.ImageUrl),
	}
	if detail := chatImageDetail(part.ImageUrl); detail != "" {
		item["detail"] = detail
	}
	return item
}

func chatImageDetail(v any) string {
	switch vv := v.(type) {
	case dto.MessageImageUrl:
		return strings.TrimSpace(vv.Detail)
	case *dto.MessageImageUrl:
		if vv == nil {
			return ""
		}
		return strings.TrimSpace(vv.Detail)
	case map[string]any:
		return strings.TrimSpace(common.Interface2String(vv["detail"]))
	default:
		return ""
	}
}

func responsesInputFilePart(part dto.MediaContent) map[string]any {
	item := map[string]any{
		"type": "input_file",
	}
	addFileFields(item, part.File)
	return item
}

func addFileFields(item map[string]any, file any) {
	switch v := file.(type) {
	case dto.MessageFile:
		addMessageFileFields(item, &v)
	case *dto.MessageFile:
		addMessageFileFields(item, v)
	case map[string]any:
		addFileMapFields(item, v)
	default:
		if file == nil {
			return
		}
		var m map[string]any
		if b, err := common.Marshal(file); err == nil {
			_ = common.Unmarshal(b, &m)
		}
		addFileMapFields(item, m)
	}
}

func addMessageFileFields(item map[string]any, file *dto.MessageFile) {
	if file == nil {
		return
	}
	addStringField(item, "file_id", file.FileId)
	addStringField(item, "file_data", file.FileData)
	addStringField(item, "filename", file.FileName)
}

func addFileMapFields(item map[string]any, file map[string]any) {
	if len(file) == 0 {
		return
	}
	addStringField(item, "file_id", common.Interface2String(file["file_id"]))
	addStringField(item, "file_data", common.Interface2String(file["file_data"]))
	filename := common.Interface2String(file["filename"])
	if filename == "" {
		filename = common.Interface2String(file["file_name"])
	}
	addStringField(item, "filename", filename)
	addStringField(item, "file_url", common.Interface2String(file["file_url"]))
}

func addStringField(item map[string]any, key string, value string) {
	if value != "" {
		item[key] = value
	}
}

func appendLegacyFunctions(tools []map[string]any, raw json.RawMessage) ([]map[string]any, error) {
	if !rawJSONIsSet(raw) {
		return tools, nil
	}
	if common.GetJsonType(raw) != "array" {
		return nil, fmt.Errorf("functions must be an array in responses compatibility mode")
	}
	var functions []dto.FunctionRequest
	if err := common.Unmarshal(raw, &functions); err != nil {
		return nil, fmt.Errorf("invalid functions: %w", err)
	}
	for _, function := range functions {
		name := strings.TrimSpace(function.Name)
		if name == "" {
			return nil, fmt.Errorf("functions entries must include name in responses compatibility mode")
		}
		tools = append(tools, map[string]any{
			"type":        "function",
			"name":        name,
			"description": function.Description,
			"parameters":  function.Parameters,
		})
	}
	return tools, nil
}

func convertChatToolChoiceToResponsesToolChoice(toolChoice any) json.RawMessage {
	if toolChoice == nil {
		return nil
	}
	var toolChoiceRaw json.RawMessage
	switch v := toolChoice.(type) {
	case string:
		toolChoiceRaw, _ = common.Marshal(v)
	default:
		var m map[string]any
		if b, err := common.Marshal(v); err == nil {
			_ = common.Unmarshal(b, &m)
		}
		if m == nil {
			toolChoiceRaw, _ = common.Marshal(v)
		} else if t, _ := m["type"].(string); t == "function" {
			if name, ok := m["name"].(string); ok && name != "" {
				toolChoiceRaw, _ = common.Marshal(map[string]any{
					"type": "function",
					"name": name,
				})
			} else if fn, ok := m["function"].(map[string]any); ok {
				if name, ok := fn["name"].(string); ok && name != "" {
					toolChoiceRaw, _ = common.Marshal(map[string]any{
						"type": "function",
						"name": name,
					})
				} else {
					toolChoiceRaw, _ = common.Marshal(v)
				}
			} else {
				toolChoiceRaw, _ = common.Marshal(v)
			}
		} else {
			toolChoiceRaw, _ = common.Marshal(v)
		}
	}
	return toolChoiceRaw
}

func convertLegacyFunctionCallToResponsesToolChoice(raw json.RawMessage) (json.RawMessage, error) {
	if !rawJSONIsSet(raw) {
		return nil, nil
	}
	switch common.GetJsonType(raw) {
	case "string":
		var choice string
		if err := common.Unmarshal(raw, &choice); err != nil {
			return nil, fmt.Errorf("invalid function_call: %w", err)
		}
		choice = strings.TrimSpace(choice)
		switch choice {
		case "auto", "none", "required":
			out, _ := common.Marshal(choice)
			return out, nil
		default:
			return nil, fmt.Errorf("unsupported function_call value %q in responses compatibility mode", choice)
		}
	case "object":
		var m map[string]any
		if err := common.Unmarshal(raw, &m); err != nil {
			return nil, fmt.Errorf("invalid function_call: %w", err)
		}
		name := strings.TrimSpace(common.Interface2String(m["name"]))
		if name == "" {
			if fn, ok := m["function"].(map[string]any); ok {
				name = strings.TrimSpace(common.Interface2String(fn["name"]))
			}
		}
		if name == "" {
			return nil, fmt.Errorf("function_call object must include name in responses compatibility mode")
		}
		out, _ := common.Marshal(map[string]any{
			"type": "function",
			"name": name,
		})
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported function_call type %s in responses compatibility mode", common.GetJsonType(raw))
	}
}

func parseChatContentPartsForResponses(msg *dto.Message) []dto.MediaContent {
	if msg == nil {
		return nil
	}
	if parts := rawChatContentPartsForResponses(msg.Content); len(parts) > 0 {
		return parts
	}
	return msg.ParseContent()
}

func rawChatContentPartsForResponses(content any) []dto.MediaContent {
	switch v := content.(type) {
	case []dto.MediaContent:
		return v
	case []any:
		parts := make([]dto.MediaContent, 0, len(v))
		for _, item := range v {
			switch vv := item.(type) {
			case dto.MediaContent:
				parts = append(parts, vv)
			case map[string]any:
				if part, ok := mediaContentFromMapForResponses(vv); ok {
					parts = append(parts, part)
				}
			}
		}
		return parts
	case []map[string]any:
		parts := make([]dto.MediaContent, 0, len(v))
		for _, item := range v {
			if part, ok := mediaContentFromMapForResponses(item); ok {
				parts = append(parts, part)
			}
		}
		return parts
	default:
		return nil
	}
}

func mediaContentFromMapForResponses(item map[string]any) (dto.MediaContent, bool) {
	contentType := common.Interface2String(item["type"])
	if contentType == "" {
		return dto.MediaContent{}, false
	}
	switch contentType {
	case dto.ContentTypeText:
		return dto.MediaContent{
			Type: dto.ContentTypeText,
			Text: common.Interface2String(item["text"]),
		}, true
	case dto.ContentTypeImageURL:
		return dto.MediaContent{
			Type:     dto.ContentTypeImageURL,
			ImageUrl: item["image_url"],
		}, true
	case dto.ContentTypeInputAudio:
		return dto.MediaContent{
			Type:       dto.ContentTypeInputAudio,
			InputAudio: item["input_audio"],
		}, true
	case dto.ContentTypeFile:
		return dto.MediaContent{
			Type: dto.ContentTypeFile,
			File: item["file"],
		}, true
	case dto.ContentTypeVideoUrl:
		return dto.MediaContent{
			Type:     dto.ContentTypeVideoUrl,
			VideoUrl: item["video_url"],
		}, true
	default:
		return dto.MediaContent{Type: contentType}, true
	}
}

func ChatCompletionsRequestToResponsesRequest(req *dto.GeneralOpenAIRequest) (*dto.OpenAIResponsesRequest, error) {
	if req == nil {
		return nil, errors.New("request is nil")
	}
	if req.Model == "" {
		return nil, errors.New("model is required")
	}
	if lo.FromPtrOr(req.N, 1) > 1 {
		return nil, fmt.Errorf("n>1 is not supported in responses compatibility mode")
	}
	if req.Stop != nil {
		return nil, fmt.Errorf("stop is not supported in responses compatibility mode")
	}

	var instructionsParts []string
	inputItems := make([]map[string]any, 0, len(req.Messages))

	for _, msg := range req.Messages {
		role := strings.TrimSpace(msg.Role)
		if role == "" {
			continue
		}

		if role == "tool" || role == "function" {
			callID := strings.TrimSpace(msg.ToolCallId)

			var output any
			if msg.Content == nil {
				output = ""
			} else if msg.IsStringContent() {
				output = msg.StringContent()
			} else {
				if b, err := common.Marshal(msg.Content); err == nil {
					output = string(b)
				} else {
					output = fmt.Sprintf("%v", msg.Content)
				}
			}

			if callID == "" {
				inputItems = append(inputItems, map[string]any{
					"role":    "user",
					"content": fmt.Sprintf("[tool_output_missing_call_id] %v", output),
				})
				continue
			}

			inputItems = append(inputItems, map[string]any{
				"type":    "function_call_output",
				"call_id": callID,
				"output":  output,
			})
			continue
		}

		// Prefer mapping system/developer messages into `instructions`.
		if role == "system" || role == "developer" {
			if msg.Content == nil {
				continue
			}
			if msg.IsStringContent() {
				if s := strings.TrimSpace(msg.StringContent()); s != "" {
					instructionsParts = append(instructionsParts, s)
				}
				continue
			}
			parts := parseChatContentPartsForResponses(&msg)
			var sb strings.Builder
			for _, part := range parts {
				if part.Type == dto.ContentTypeText && strings.TrimSpace(part.Text) != "" {
					if sb.Len() > 0 {
						sb.WriteString("\n")
					}
					sb.WriteString(part.Text)
				}
			}
			if s := strings.TrimSpace(sb.String()); s != "" {
				instructionsParts = append(instructionsParts, s)
			}
			continue
		}

		item := map[string]any{
			"role": role,
		}

		if msg.Content == nil {
			item["content"] = ""
			inputItems = append(inputItems, item)

			if role == "assistant" {
				for _, tc := range msg.ParseToolCalls() {
					if strings.TrimSpace(tc.ID) == "" {
						continue
					}
					if tc.Type != "" && tc.Type != "function" {
						continue
					}
					name := strings.TrimSpace(tc.Function.Name)
					if name == "" {
						continue
					}
					inputItems = append(inputItems, map[string]any{
						"type":      "function_call",
						"call_id":   tc.ID,
						"name":      name,
						"arguments": tc.Function.Arguments,
					})
				}
			}
			continue
		}

		if msg.IsStringContent() {
			item["content"] = msg.StringContent()
			inputItems = append(inputItems, item)

			if role == "assistant" {
				for _, tc := range msg.ParseToolCalls() {
					if strings.TrimSpace(tc.ID) == "" {
						continue
					}
					if tc.Type != "" && tc.Type != "function" {
						continue
					}
					name := strings.TrimSpace(tc.Function.Name)
					if name == "" {
						continue
					}
					inputItems = append(inputItems, map[string]any{
						"type":      "function_call",
						"call_id":   tc.ID,
						"name":      name,
						"arguments": tc.Function.Arguments,
					})
				}
			}
			continue
		}

		parts := parseChatContentPartsForResponses(&msg)
		contentParts := make([]map[string]any, 0, len(parts))
		for _, part := range parts {
			switch part.Type {
			case dto.ContentTypeText:
				textType := "input_text"
				if role == "assistant" {
					textType = "output_text"
				}
				contentParts = append(contentParts, map[string]any{
					"type": textType,
					"text": part.Text,
				})
			case dto.ContentTypeImageURL:
				contentParts = append(contentParts, responsesInputImagePart(part))
			case dto.ContentTypeInputAudio:
				contentParts = append(contentParts, map[string]any{
					"type":        "input_audio",
					"input_audio": part.InputAudio,
				})
			case dto.ContentTypeFile:
				contentParts = append(contentParts, responsesInputFilePart(part))
			case dto.ContentTypeVideoUrl:
				contentParts = append(contentParts, map[string]any{
					"type":      "input_video",
					"video_url": part.VideoUrl,
				})
			default:
				contentParts = append(contentParts, map[string]any{
					"type": part.Type,
				})
			}
		}
		item["content"] = contentParts
		inputItems = append(inputItems, item)

		if role == "assistant" {
			for _, tc := range msg.ParseToolCalls() {
				if strings.TrimSpace(tc.ID) == "" {
					continue
				}
				if tc.Type != "" && tc.Type != "function" {
					continue
				}
				name := strings.TrimSpace(tc.Function.Name)
				if name == "" {
					continue
				}
				inputItems = append(inputItems, map[string]any{
					"type":      "function_call",
					"call_id":   tc.ID,
					"name":      name,
					"arguments": tc.Function.Arguments,
				})
			}
		}
	}

	inputRaw, err := common.Marshal(inputItems)
	if err != nil {
		return nil, err
	}

	var instructionsRaw json.RawMessage
	if len(instructionsParts) > 0 {
		instructions := strings.Join(instructionsParts, "\n\n")
		instructionsRaw, _ = common.Marshal(instructions)
	}

	var toolsRaw json.RawMessage
	if req.Tools != nil || rawJSONIsSet(req.Functions) {
		tools := make([]map[string]any, 0, len(req.Tools))
		for _, tool := range req.Tools {
			switch tool.Type {
			case "function":
				tools = append(tools, map[string]any{
					"type":        "function",
					"name":        tool.Function.Name,
					"description": tool.Function.Description,
					"parameters":  tool.Function.Parameters,
				})
			default:
				// Best-effort: keep original tool shape for unknown types.
				var m map[string]any
				if b, err := common.Marshal(tool); err == nil {
					_ = common.Unmarshal(b, &m)
				}
				if len(m) == 0 {
					m = map[string]any{"type": tool.Type}
				}
				tools = append(tools, m)
			}
		}
		tools, err = appendLegacyFunctions(tools, req.Functions)
		if err != nil {
			return nil, err
		}
		toolsRaw, _ = common.Marshal(tools)
	}

	toolChoiceRaw := convertChatToolChoiceToResponsesToolChoice(req.ToolChoice)
	if rawJSONIsSet(req.FunctionCall) {
		legacyToolChoiceRaw, err := convertLegacyFunctionCallToResponsesToolChoice(req.FunctionCall)
		if err != nil {
			return nil, err
		}
		if toolChoiceRaw != nil && string(toolChoiceRaw) != string(legacyToolChoiceRaw) {
			return nil, fmt.Errorf("function_call cannot be combined with tool_choice in responses compatibility mode")
		}
		toolChoiceRaw = legacyToolChoiceRaw
	}

	var parallelToolCallsRaw json.RawMessage
	if req.ParallelTooCalls != nil {
		parallelToolCallsRaw, _ = common.Marshal(*req.ParallelTooCalls)
	}

	textRaw := convertChatResponseFormatToResponsesText(req.ResponseFormat)

	maxOutputTokens := lo.FromPtrOr(req.MaxTokens, uint(0))
	maxCompletionTokens := lo.FromPtrOr(req.MaxCompletionTokens, uint(0))
	if maxCompletionTokens > maxOutputTokens {
		maxOutputTokens = maxCompletionTokens
	}
	// OpenAI Responses API rejects max_output_tokens < 16 when explicitly provided.
	//if maxOutputTokens > 0 && maxOutputTokens < 16 {
	//	maxOutputTokens = 16
	//}

	var topP *float64
	if req.TopP != nil {
		topP = common.GetPointer(lo.FromPtr(req.TopP))
	}

	out := &dto.OpenAIResponsesRequest{
		Model:             req.Model,
		Input:             inputRaw,
		Instructions:      instructionsRaw,
		Stream:            req.Stream,
		Temperature:       req.Temperature,
		Text:              textRaw,
		ToolChoice:        toolChoiceRaw,
		Tools:             toolsRaw,
		TopP:              topP,
		User:              req.User,
		ParallelToolCalls: parallelToolCallsRaw,
		Store:             req.Store,
		Metadata:          req.Metadata,
	}
	if req.MaxTokens != nil || req.MaxCompletionTokens != nil {
		out.MaxOutputTokens = lo.ToPtr(maxOutputTokens)
	}

	if req.ReasoningEffort != "" {
		out.Reasoning = &dto.Reasoning{
			Effort:  req.ReasoningEffort,
			Summary: "detailed",
		}
	}

	return out, nil
}
