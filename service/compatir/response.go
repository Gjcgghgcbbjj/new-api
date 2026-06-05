package compatir

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

func FromChatResponse(resp *dto.OpenAITextResponse) (*Response, error) {
	if resp == nil {
		return nil, errors.New("response is nil")
	}
	if oaiError := resp.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, errors.New(oaiError.Message)
	}

	createdAt := normalizeCreatedAt(resp.Created)
	if createdAt == 0 {
		createdAt = int(time.Now().Unix())
	}
	out := &Response{
		ID:        resp.Id,
		CreatedAt: createdAt,
		Model:     resp.Model,
		Status:    "completed",
		Usage:     chatUsageToIRUsage(&resp.Usage),
	}
	if len(resp.Choices) == 0 {
		return out, nil
	}

	msg := resp.Choices[0].Message
	text := msg.StringContent()
	if text != "" {
		out.Output = append(out.Output, OutputItem{
			Type:    OutputTypeMessage,
			ID:      fmt.Sprintf("msg_%d", time.Now().UnixNano()),
			Status:  "completed",
			Role:    "assistant",
			Content: []ContentPart{{Type: ContentTypeText, Text: text}},
		})
	}
	for idx, toolCall := range msg.ParseToolCalls() {
		if toolCall.Type != "" && toolCall.Type != ToolTypeFunction {
			out.AddWarning(WarningIgnoredField, "choices[0].message.tool_calls.type", "only function tool calls can be represented by Responses")
			continue
		}
		callID := strings.TrimSpace(toolCall.ID)
		if callID == "" {
			callID = fmt.Sprintf("call_%d", time.Now().UnixNano())
		}
		out.Output = append(out.Output, OutputItem{
			Type:   OutputTypeFunctionCall,
			ID:     callID,
			Status: "completed",
			ToolCall: ToolCall{
				Index:     idx,
				ID:        callID,
				Type:      ToolTypeFunction,
				Name:      toolCall.Function.Name,
				Arguments: toolCall.Function.Arguments,
			},
		})
	}
	return out, nil
}

func ToResponsesResponse(resp *Response, original *dto.OpenAIResponsesRequest, id string) (*dto.OpenAIResponsesResponse, *dto.Usage, error) {
	if resp == nil {
		return nil, nil, errors.New("response is nil")
	}
	model := resp.Model
	if model == "" && original != nil {
		model = original.Model
	}
	createdAt := resp.CreatedAt
	if createdAt == 0 {
		createdAt = int(time.Now().Unix())
	}
	if id == "" {
		id = resp.ID
	}
	if id == "" {
		id = fmt.Sprintf("resp_%d", time.Now().UnixNano())
	}

	output := make([]dto.ResponsesOutput, 0, len(resp.Output))
	var outputText strings.Builder
	for _, item := range resp.Output {
		switch item.Type {
		case OutputTypeMessage:
			text := contentPartsText(item.Content)
			if text == "" {
				continue
			}
			outputText.WriteString(text)
			output = append(output, dto.ResponsesOutput{
				Type:   OutputTypeMessage,
				ID:     firstNonEmptyString(item.ID, fmt.Sprintf("msg_%d", time.Now().UnixNano())),
				Status: firstNonEmptyString(item.Status, "completed"),
				Role:   firstNonEmptyString(item.Role, "assistant"),
				Content: []dto.ResponsesOutputContent{{
					Type:        "output_text",
					Text:        text,
					Annotations: []interface{}{},
				}},
			})
		case OutputTypeFunctionCall:
			call := item.ToolCall
			callID := strings.TrimSpace(call.ID)
			if callID == "" {
				callID = fmt.Sprintf("call_%d", time.Now().UnixNano())
			}
			argsRaw, _ := common.Marshal(call.Arguments)
			output = append(output, dto.ResponsesOutput{
				Type:      OutputTypeFunctionCall,
				ID:        callID,
				Status:    firstNonEmptyString(item.Status, "completed"),
				CallId:    callID,
				Name:      call.Name,
				Arguments: argsRaw,
			})
		}
	}

	usage := cloneUsage(resp.Usage)
	out := &dto.OpenAIResponsesResponse{
		ID:         id,
		Object:     "response",
		CreatedAt:  createdAt,
		Status:     json.RawMessage(`"completed"`),
		Model:      model,
		Output:     output,
		Usage:      usage,
		OutputText: outputText.String(),
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

func FromResponsesResponse(resp *dto.OpenAIResponsesResponse) (*Response, error) {
	if resp == nil {
		return nil, errors.New("response is nil")
	}
	out := &Response{
		ID:        resp.ID,
		CreatedAt: resp.CreatedAt,
		Model:     resp.Model,
		Status:    common.JsonRawMessageToString(resp.Status),
		Usage:     responsesUsageToIRUsage(resp.Usage),
	}
	for _, item := range resp.Output {
		switch item.Type {
		case OutputTypeMessage:
			out.Output = append(out.Output, OutputItem{
				Type:    OutputTypeMessage,
				ID:      item.ID,
				Status:  item.Status,
				Role:    firstNonEmptyString(item.Role, "assistant"),
				Content: responsesOutputContentToIR(item.Content),
			})
		case OutputTypeFunctionCall:
			callID := strings.TrimSpace(item.CallId)
			if callID == "" {
				callID = strings.TrimSpace(item.ID)
			}
			out.Output = append(out.Output, OutputItem{
				Type:   OutputTypeFunctionCall,
				ID:     item.ID,
				Status: item.Status,
				ToolCall: ToolCall{
					ID:        callID,
					Type:      ToolTypeFunction,
					Name:      item.Name,
					Arguments: item.ArgumentsString(),
				},
			})
		}
	}
	return out, nil
}

func ToChatResponse(resp *Response, id string) (*dto.OpenAITextResponse, *dto.Usage, error) {
	if resp == nil {
		return nil, nil, errors.New("response is nil")
	}
	if id == "" {
		id = resp.ID
	}

	var text strings.Builder
	var toolCalls []dto.ToolCallResponse
	for _, item := range resp.Output {
		switch item.Type {
		case OutputTypeMessage:
			text.WriteString(contentPartsText(item.Content))
		case OutputTypeFunctionCall:
			call := item.ToolCall
			if strings.TrimSpace(call.Name) == "" {
				continue
			}
			callID := strings.TrimSpace(call.ID)
			if callID == "" {
				callID = strings.TrimSpace(item.ID)
			}
			toolCalls = append(toolCalls, dto.ToolCallResponse{
				ID:   callID,
				Type: ToolTypeFunction,
				Function: dto.FunctionResponse{
					Name:      call.Name,
					Arguments: call.Arguments,
				},
			})
		}
	}

	finishReason := "stop"
	if len(toolCalls) > 0 {
		finishReason = "tool_calls"
	}
	msg := dto.Message{
		Role:    "assistant",
		Content: text.String(),
	}
	if len(toolCalls) > 0 {
		msg.SetToolCalls(toolCalls)
	}

	usage := cloneUsage(resp.Usage)
	if usage == nil {
		usage = &dto.Usage{}
	}
	out := &dto.OpenAITextResponse{
		Id:      id,
		Object:  "chat.completion",
		Created: resp.CreatedAt,
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

func ExtractOutputTextFromResponses(resp *dto.OpenAIResponsesResponse) string {
	ir, err := FromResponsesResponse(resp)
	if err != nil {
		return ""
	}
	var sb strings.Builder
	for _, item := range ir.Output {
		if item.Type == OutputTypeMessage && (item.Role == "" || item.Role == "assistant") {
			sb.WriteString(contentPartsText(item.Content))
		}
	}
	if sb.Len() > 0 {
		return sb.String()
	}
	for _, item := range ir.Output {
		if item.Type == OutputTypeMessage {
			sb.WriteString(contentPartsText(item.Content))
		}
	}
	return sb.String()
}

func responsesOutputContentToIR(content []dto.ResponsesOutputContent) []ContentPart {
	parts := make([]ContentPart, 0, len(content))
	for _, part := range content {
		if part.Text == "" {
			continue
		}
		parts = append(parts, ContentPart{Type: ContentTypeText, Text: part.Text})
	}
	return parts
}

func chatUsageToIRUsage(usage *dto.Usage) *dto.Usage {
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

func responsesUsageToIRUsage(usage *dto.Usage) *dto.Usage {
	if usage == nil {
		return nil
	}
	out := *usage
	if out.PromptTokens == 0 {
		out.PromptTokens = usage.InputTokens
	}
	if out.CompletionTokens == 0 {
		out.CompletionTokens = usage.OutputTokens
	}
	if out.TotalTokens == 0 {
		out.TotalTokens = out.PromptTokens + out.CompletionTokens
	}
	if out.InputTokens == 0 {
		out.InputTokens = out.PromptTokens
	}
	if out.OutputTokens == 0 {
		out.OutputTokens = out.CompletionTokens
	}
	return &out
}

func cloneUsage(usage *dto.Usage) *dto.Usage {
	if usage == nil {
		return nil
	}
	out := *usage
	return &out
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
