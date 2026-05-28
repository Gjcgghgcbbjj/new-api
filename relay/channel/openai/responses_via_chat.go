package openai

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func OaiChatToResponsesHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response, original *dto.OpenAIResponsesRequest) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	defer service.CloseResponseBodyGracefully(resp)

	var chatResp dto.OpenAITextResponse
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	if err := common.Unmarshal(body, &chatResp); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if oaiError := chatResp.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}

	responsesResp, usage, err := service.ChatCompletionsResponseToResponsesResponse(&chatResp, original, helper.GetResponseID(c))
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if usage == nil || usage.TotalTokens == 0 {
		usage = service.ResponseText2Usage(c, responsesResp.OutputText, info.UpstreamModelName, info.GetEstimatePromptTokens())
		responsesResp.Usage = usage
	}

	responseBody, err := common.Marshal(responsesResp)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
	}
	service.IOCopyBytesGracefully(c, resp, responseBody)
	return usage, nil
}

func OaiChatToResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response, original *dto.OpenAIResponsesRequest) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	defer service.CloseResponseBodyGracefully(resp)

	responseID := helper.GetResponseID(c)
	messageID := fmt.Sprintf("msg_%d", time.Now().UnixNano())
	createdAt := int(time.Now().Unix())
	model := info.UpstreamModelName
	if original != nil && original.Model != "" {
		model = original.Model
	}

	usage := &dto.Usage{}
	var outputText strings.Builder
	streamErr := (*types.NewAPIError)(nil)
	started := false
	done := false
	toolCalls := make(map[int]*dto.ToolCallResponse)

	send := func(eventType string, payload map[string]any) bool {
		if payload == nil {
			payload = map[string]any{}
		}
		payload["type"] = eventType
		data, err := common.Marshal(payload)
		if err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
			return false
		}
		helper.ResponseChunkData(c, dto.ResponsesStreamResponse{Type: eventType}, string(data))
		return true
	}

	sendStart := func() bool {
		if started {
			return true
		}
		started = true
		return send("response.created", map[string]any{
			"response": map[string]any{
				"id":         responseID,
				"object":     "response",
				"created_at": createdAt,
				"status":     "in_progress",
				"model":      model,
				"output":     []any{},
			},
		}) &&
			send("response.output_item.added", map[string]any{
				"output_index": 0,
				"item": map[string]any{
					"id":      messageID,
					"type":    "message",
					"status":  "in_progress",
					"role":    "assistant",
					"content": []any{},
				},
			}) &&
			send("response.content_part.added", map[string]any{
				"item_id":       messageID,
				"output_index":  0,
				"content_index": 0,
				"part": map[string]any{
					"type":        "output_text",
					"text":        "",
					"annotations": []any{},
				},
			})
	}

	helper.SetEventStreamHeaders(c)
	if !sendStart() {
		return nil, streamErr
	}

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		if streamErr != nil {
			sr.Stop(streamErr)
			return
		}
		var chunk dto.ChatCompletionsStreamResponse
		if err := common.UnmarshalJsonStr(data, &chunk); err != nil {
			sr.Error(err)
			return
		}
		if chunk.Model != "" {
			model = chunk.Model
		}
		if chunk.Created != 0 {
			createdAt = int(chunk.Created)
		}
		if chunk.Usage != nil {
			usage = chatUsageForResponses(chunk.Usage)
		}
		if len(chunk.Choices) == 0 {
			return
		}

		delta := chunk.Choices[0].Delta
		if content := delta.GetContentString(); content != "" {
			if !sendStart() {
				sr.Stop(streamErr)
				return
			}
			outputText.WriteString(content)
			if !send("response.output_text.delta", map[string]any{
				"item_id":       messageID,
				"output_index":  0,
				"content_index": 0,
				"delta":         content,
			}) {
				sr.Stop(streamErr)
				return
			}
		}
		collectResponsesBridgeToolCalls(toolCalls, delta.ToolCalls)
	})

	if streamErr != nil {
		return nil, streamErr
	}
	if !sendStart() {
		return nil, streamErr
	}

	text := outputText.String()
	if !send("response.output_text.done", map[string]any{
		"item_id":       messageID,
		"output_index":  0,
		"content_index": 0,
		"text":          text,
	}) ||
		!send("response.content_part.done", map[string]any{
			"item_id":       messageID,
			"output_index":  0,
			"content_index": 0,
			"part": map[string]any{
				"type":        "output_text",
				"text":        text,
				"annotations": []any{},
			},
		}) ||
		!send("response.output_item.done", map[string]any{
			"output_index": 0,
			"item": map[string]any{
				"id":     messageID,
				"type":   "message",
				"status": "completed",
				"role":   "assistant",
				"content": []any{map[string]any{
					"type":        "output_text",
					"text":        text,
					"annotations": []any{},
				}},
			},
		}) {
		return nil, streamErr
	}

	outputIndex := 1
	for _, toolCall := range toolCalls {
		if toolCall == nil || strings.TrimSpace(toolCall.Function.Name) == "" {
			continue
		}
		callID := strings.TrimSpace(toolCall.ID)
		if callID == "" {
			callID = fmt.Sprintf("call_%d", time.Now().UnixNano())
		}
		item := map[string]any{
			"id":        callID,
			"type":      "function_call",
			"status":    "completed",
			"call_id":   callID,
			"name":      toolCall.Function.Name,
			"arguments": toolCall.Function.Arguments,
		}
		if !send("response.output_item.added", map[string]any{
			"output_index": outputIndex,
			"item":         item,
		}) ||
			!send("response.output_item.done", map[string]any{
				"output_index": outputIndex,
				"item":         item,
			}) {
			return nil, streamErr
		}
		outputIndex++
	}

	if usage == nil || usage.TotalTokens == 0 {
		usage = service.ResponseText2Usage(c, text, info.UpstreamModelName, info.GetEstimatePromptTokens())
	}
	if !send("response.completed", map[string]any{
		"response": map[string]any{
			"id":          responseID,
			"object":      "response",
			"created_at":  createdAt,
			"status":      "completed",
			"model":       model,
			"output":      []any{},
			"output_text": text,
			"usage":       usage,
		},
	}) {
		return nil, streamErr
	}
	done = true
	if done {
		helper.Done(c)
	}
	return usage, nil
}

func collectResponsesBridgeToolCalls(toolCalls map[int]*dto.ToolCallResponse, deltas []dto.ToolCallResponse) {
	for _, delta := range deltas {
		idx := 0
		if delta.Index != nil {
			idx = *delta.Index
		} else {
			idx = len(toolCalls)
		}
		current := toolCalls[idx]
		if current == nil {
			current = &dto.ToolCallResponse{Type: "function"}
			toolCalls[idx] = current
		}
		if delta.ID != "" {
			current.ID = delta.ID
		}
		if delta.Type != nil {
			current.Type = delta.Type
		}
		if delta.Function.Name != "" {
			current.Function.Name += delta.Function.Name
		}
		if delta.Function.Arguments != "" {
			current.Function.Arguments += delta.Function.Arguments
		}
	}
}

func chatUsageForResponses(usage *dto.Usage) *dto.Usage {
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
