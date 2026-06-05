package openaicompat

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/service/compatir"
)

func ResponsesRequestToChatCompletionsRequest(req *dto.OpenAIResponsesRequest) (*dto.GeneralOpenAIRequest, error) {
	ir, err := compatir.FromResponsesRequest(req)
	if err != nil {
		return nil, err
	}
	return compatir.ToChatRequest(ir)
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
