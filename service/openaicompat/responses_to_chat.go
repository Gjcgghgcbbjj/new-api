package openaicompat

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

func ResponsesResponseToChatCompletionsResponse(resp *dto.OpenAIResponsesResponse, id string) (*dto.OpenAITextResponse, *dto.Usage, error) {
	if resp == nil {
		return nil, nil, errors.New("response is nil")
	}

	text := ExtractOutputTextFromResponses(resp)

	usage := &dto.Usage{}
	if resp.Usage != nil {
		if resp.Usage.InputTokens != 0 {
			usage.PromptTokens = resp.Usage.InputTokens
			usage.InputTokens = resp.Usage.InputTokens
		}
		if resp.Usage.OutputTokens != 0 {
			usage.CompletionTokens = resp.Usage.OutputTokens
			usage.OutputTokens = resp.Usage.OutputTokens
		}
		if resp.Usage.TotalTokens != 0 {
			usage.TotalTokens = resp.Usage.TotalTokens
		} else {
			usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
		}
		if resp.Usage.InputTokensDetails != nil {
			usage.PromptTokensDetails.CachedTokens = resp.Usage.InputTokensDetails.CachedTokens
			usage.PromptTokensDetails.ImageTokens = resp.Usage.InputTokensDetails.ImageTokens
			usage.PromptTokensDetails.AudioTokens = resp.Usage.InputTokensDetails.AudioTokens
		}
		if resp.Usage.CompletionTokenDetails.ReasoningTokens != 0 {
			usage.CompletionTokenDetails.ReasoningTokens = resp.Usage.CompletionTokenDetails.ReasoningTokens
		}
	}

	created := resp.CreatedAt

	var toolCalls []dto.ToolCallResponse
	for _, out := range resp.Output {
		if out.Type != "function_call" {
			continue
		}
		name := strings.TrimSpace(out.Name)
		if name == "" {
			continue
		}
		callId := strings.TrimSpace(out.CallId)
		if callId == "" {
			callId = strings.TrimSpace(out.ID)
		}
		toolCalls = append(toolCalls, dto.ToolCallResponse{
			ID:   callId,
			Type: "function",
			Function: dto.FunctionResponse{
				Name:      name,
				Arguments: out.ArgumentsString(),
			},
		})
	}

	finishReason := ResponsesFinishReason(resp, len(toolCalls) > 0)

	msg := dto.Message{
		Role:    "assistant",
		Content: text,
	}
	if len(toolCalls) > 0 {
		msg.SetToolCalls(toolCalls)
	}

	out := &dto.OpenAITextResponse{
		Id:      id,
		Object:  "chat.completion",
		Created: created,
		Model:   resp.Model,
		Choices: []dto.OpenAITextResponseChoice{
			{
				Index:        0,
				Message:      msg,
				FinishReason: finishReason,
			},
		},
		Usage: *usage,
	}

	return out, usage, nil
}

func ResponsesFinishReason(resp *dto.OpenAIResponsesResponse, hasToolCalls bool) string {
	status := responsesStatus(resp)
	incompleteReason := ResponsesIncompleteReason(resp)
	if status == "incomplete" || incompleteReason != "" {
		switch incompleteReason {
		case "max_output_tokens":
			return "length"
		case "content_filter":
			return "content_filter"
		}
	}
	if hasToolCalls {
		return "tool_calls"
	}
	return "stop"
}

func ResponsesIncompleteReason(resp *dto.OpenAIResponsesResponse) string {
	if resp == nil || resp.IncompleteDetails == nil {
		return ""
	}
	return strings.TrimSpace(resp.IncompleteDetails.Reasoning)
}

func ApplyResponsesIncompleteReasonFromJSON(resp *dto.OpenAIResponsesResponse, data []byte) {
	if resp == nil {
		return
	}
	reason := extractResponsesIncompleteReasonFromJSON(data)
	if reason == "" {
		return
	}
	if resp.IncompleteDetails == nil {
		resp.IncompleteDetails = &dto.IncompleteDetails{}
	}
	resp.IncompleteDetails.Reasoning = reason
}

func responsesStatus(resp *dto.OpenAIResponsesResponse) string {
	if resp == nil || len(resp.Status) == 0 {
		return ""
	}
	switch common.GetJsonType(resp.Status) {
	case "string":
		var status string
		if err := common.Unmarshal(resp.Status, &status); err == nil {
			return strings.TrimSpace(status)
		}
	case "object":
		var statusObj map[string]any
		if err := common.Unmarshal(resp.Status, &statusObj); err == nil {
			return strings.TrimSpace(common.Interface2String(statusObj["status"]))
		}
	}
	return strings.Trim(strings.TrimSpace(string(resp.Status)), `"`)
}

func extractResponsesIncompleteReasonFromJSON(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	var root map[string]json.RawMessage
	if err := common.Unmarshal(data, &root); err != nil {
		return ""
	}
	if reason := extractIncompleteReasonRaw(root["incomplete_details"]); reason != "" {
		return reason
	}
	var response map[string]json.RawMessage
	if err := common.Unmarshal(root["response"], &response); err != nil {
		return ""
	}
	return extractIncompleteReasonRaw(response["incomplete_details"])
}

func extractIncompleteReasonRaw(raw json.RawMessage) string {
	if len(raw) == 0 || common.GetJsonType(raw) == "null" {
		return ""
	}
	var details map[string]any
	if err := common.Unmarshal(raw, &details); err != nil {
		return ""
	}
	return strings.TrimSpace(common.Interface2String(details["reason"]))
}

func ExtractOutputTextFromResponses(resp *dto.OpenAIResponsesResponse) string {
	if resp == nil || len(resp.Output) == 0 {
		return ""
	}

	var sb strings.Builder

	// Prefer assistant message outputs.
	for _, out := range resp.Output {
		if out.Type != "message" {
			continue
		}
		if out.Role != "" && out.Role != "assistant" {
			continue
		}
		for _, c := range out.Content {
			if c.Type == "output_text" && c.Text != "" {
				sb.WriteString(c.Text)
			}
		}
	}
	if sb.Len() > 0 {
		return sb.String()
	}
	for _, out := range resp.Output {
		for _, c := range out.Content {
			if c.Text != "" {
				sb.WriteString(c.Text)
			}
		}
	}
	return sb.String()
}
