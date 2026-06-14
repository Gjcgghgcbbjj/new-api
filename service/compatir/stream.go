package compatir

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/dto"
)

type ResponsesStreamEvent struct {
	Type    string
	Payload map[string]any
}

type ChatToResponsesStreamOptions struct {
	ResponseID string
	MessageID  string
	Model      string
	CreatedAt  int
}

type ChatToResponsesStreamConverter struct {
	responseID string
	messageID  string
	model      string
	createdAt  int
	started    bool

	outputText strings.Builder
	toolCalls  map[int]*dto.ToolCallResponse
	usage      *dto.Usage
}

func NewChatToResponsesStreamConverter(options ChatToResponsesStreamOptions) *ChatToResponsesStreamConverter {
	createdAt := options.CreatedAt
	if createdAt == 0 {
		createdAt = int(time.Now().Unix())
	}
	responseID := strings.TrimSpace(options.ResponseID)
	if responseID == "" {
		responseID = fmt.Sprintf("resp_%d", time.Now().UnixNano())
	}
	messageID := strings.TrimSpace(options.MessageID)
	if messageID == "" {
		messageID = fmt.Sprintf("msg_%d", time.Now().UnixNano())
	}
	return &ChatToResponsesStreamConverter{
		responseID: responseID,
		messageID:  messageID,
		model:      options.Model,
		createdAt:  createdAt,
		toolCalls:  make(map[int]*dto.ToolCallResponse),
	}
}

func (c *ChatToResponsesStreamConverter) StartEvents() []ResponsesStreamEvent {
	if c == nil || c.started {
		return nil
	}
	c.started = true
	return []ResponsesStreamEvent{
		c.event("response.created", map[string]any{
			"response": map[string]any{
				"id":         c.responseID,
				"object":     "response",
				"created_at": c.createdAt,
				"status":     "in_progress",
				"model":      c.model,
				"output":     []any{},
			},
		}),
		c.event("response.output_item.added", map[string]any{
			"output_index": 0,
			"item": map[string]any{
				"id":      c.messageID,
				"type":    "message",
				"status":  "in_progress",
				"role":    "assistant",
				"content": []any{},
			},
		}),
		c.event("response.content_part.added", map[string]any{
			"item_id":       c.messageID,
			"output_index":  0,
			"content_index": 0,
			"part": map[string]any{
				"type":        "output_text",
				"text":        "",
				"annotations": []any{},
			},
		}),
	}
}

func (c *ChatToResponsesStreamConverter) EventsFromChatChunk(chunk *dto.ChatCompletionsStreamResponse) []ResponsesStreamEvent {
	if c == nil || chunk == nil {
		return nil
	}
	if chunk.Model != "" {
		c.model = chunk.Model
	}
	if chunk.Created != 0 {
		c.createdAt = int(chunk.Created)
	}
	if chunk.Usage != nil {
		c.usage = chatUsageToIRUsage(chunk.Usage)
	}
	if len(chunk.Choices) == 0 {
		return nil
	}

	delta := chunk.Choices[0].Delta
	c.collectToolCalls(delta.ToolCalls)

	content := delta.GetContentString()
	if content == "" {
		return nil
	}
	c.outputText.WriteString(content)
	events := c.StartEvents()
	events = append(events, c.event("response.output_text.delta", map[string]any{
		"item_id":       c.messageID,
		"output_index":  0,
		"content_index": 0,
		"delta":         content,
	}))
	return events
}

func (c *ChatToResponsesStreamConverter) FinalEvents() ([]ResponsesStreamEvent, *dto.Usage) {
	if c == nil {
		return nil, nil
	}
	events := c.StartEvents()
	text := c.outputText.String()
	events = append(events,
		c.event("response.output_text.done", map[string]any{
			"item_id":       c.messageID,
			"output_index":  0,
			"content_index": 0,
			"text":          text,
		}),
		c.event("response.content_part.done", map[string]any{
			"item_id":       c.messageID,
			"output_index":  0,
			"content_index": 0,
			"part": map[string]any{
				"type":        "output_text",
				"text":        text,
				"annotations": []any{},
			},
		}),
		c.event("response.output_item.done", map[string]any{
			"output_index": 0,
			"item": map[string]any{
				"id":     c.messageID,
				"type":   "message",
				"status": "completed",
				"role":   "assistant",
				"content": []any{map[string]any{
					"type":        "output_text",
					"text":        text,
					"annotations": []any{},
				}},
			},
		}),
	)

	outputIndex := 1
	for _, toolCall := range c.stableToolCalls() {
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
		events = append(events,
			c.event("response.output_item.added", map[string]any{
				"output_index": outputIndex,
				"item":         item,
			}),
			c.event("response.output_item.done", map[string]any{
				"output_index": outputIndex,
				"item":         item,
			}),
		)
		outputIndex++
	}

	events = append(events, c.event("response.completed", map[string]any{
		"response": map[string]any{
			"id":          c.responseID,
			"object":      "response",
			"created_at":  c.createdAt,
			"status":      "completed",
			"model":       c.model,
			"output":      []any{},
			"output_text": text,
			"usage":       c.usage,
		},
	}))
	return events, cloneUsage(c.usage)
}

func (c *ChatToResponsesStreamConverter) OutputText() string {
	if c == nil {
		return ""
	}
	return c.outputText.String()
}

func (c *ChatToResponsesStreamConverter) Usage() *dto.Usage {
	if c == nil {
		return nil
	}
	return cloneUsage(c.usage)
}

func (c *ChatToResponsesStreamConverter) SetUsage(usage *dto.Usage) {
	if c == nil {
		return
	}
	c.usage = cloneUsage(usage)
}

func (c *ChatToResponsesStreamConverter) event(eventType string, payload map[string]any) ResponsesStreamEvent {
	if payload == nil {
		payload = map[string]any{}
	}
	payload["type"] = eventType
	return ResponsesStreamEvent{Type: eventType, Payload: payload}
}

func (c *ChatToResponsesStreamConverter) collectToolCalls(deltas []dto.ToolCallResponse) {
	for _, delta := range deltas {
		idx := 0
		if delta.Index != nil {
			idx = *delta.Index
		} else {
			idx = len(c.toolCalls)
		}
		current := c.toolCalls[idx]
		if current == nil {
			current = &dto.ToolCallResponse{Type: ToolTypeFunction}
			c.toolCalls[idx] = current
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

func (c *ChatToResponsesStreamConverter) stableToolCalls() []*dto.ToolCallResponse {
	keys := make([]int, 0, len(c.toolCalls))
	for idx := range c.toolCalls {
		keys = append(keys, idx)
	}
	sort.Ints(keys)
	out := make([]*dto.ToolCallResponse, 0, len(keys))
	for _, idx := range keys {
		out = append(out, c.toolCalls[idx])
	}
	return out
}

type ResponsesToChatStreamOptions struct {
	ResponseID string
	Model      string
	CreatedAt  int64
}

type ResponsesToChatStreamConverter struct {
	responseID string
	model      string
	createdAt  int64

	usage     *dto.Usage
	usageText strings.Builder

	sentStart bool
	sentStop  bool

	toolCallIndexByID           map[string]int
	toolCallNameByID            map[string]string
	toolCallArgsByID            map[string]string
	toolCallNameSent            map[string]bool
	toolCallCanonicalIDByItemID map[string]string
	sawToolCall                 bool

	hasSentReasoningSummary      bool
	needsReasoningSummarySpacing bool
}

func NewResponsesToChatStreamConverter(options ResponsesToChatStreamOptions) *ResponsesToChatStreamConverter {
	createdAt := options.CreatedAt
	if createdAt == 0 {
		createdAt = time.Now().Unix()
	}
	responseID := strings.TrimSpace(options.ResponseID)
	if responseID == "" {
		responseID = fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
	}
	return &ResponsesToChatStreamConverter{
		responseID:                   responseID,
		model:                        options.Model,
		createdAt:                    createdAt,
		usage:                        &dto.Usage{},
		toolCallIndexByID:            make(map[string]int),
		toolCallNameByID:             make(map[string]string),
		toolCallArgsByID:             make(map[string]string),
		toolCallNameSent:             make(map[string]bool),
		toolCallCanonicalIDByItemID:  make(map[string]string),
		hasSentReasoningSummary:      false,
		needsReasoningSummarySpacing: false,
	}
}

func (c *ResponsesToChatStreamConverter) ChunksFromResponsesEvent(event *dto.ResponsesStreamResponse) []*dto.ChatCompletionsStreamResponse {
	if c == nil || event == nil {
		return nil
	}
	switch event.Type {
	case "response.created":
		c.updateFromResponsesResponse(event.Response)
		return nil
	case "response.reasoning_summary_text.delta":
		return c.reasoningSummaryDeltaChunks(event.Delta)
	case "response.reasoning_summary_text.done":
		if c.hasSentReasoningSummary {
			c.needsReasoningSummarySpacing = true
		}
		return nil
	case "response.output_text.delta":
		return c.outputTextDeltaChunks(event.Delta)
	case "response.output_item.added", "response.output_item.done":
		return c.outputItemChunks(event.Item)
	case "response.function_call_arguments.delta":
		return c.functionCallArgumentsDeltaChunks(event.ItemID, event.Delta)
	case "response.function_call_arguments.done":
		return nil
	case "response.completed":
		c.updateFromResponsesResponse(event.Response)
		return c.StopChunks()
	default:
		return nil
	}
}

func (c *ResponsesToChatStreamConverter) StartChunks() []*dto.ChatCompletionsStreamResponse {
	if c == nil || c.sentStart {
		return nil
	}
	c.sentStart = true
	empty := ""
	return []*dto.ChatCompletionsStreamResponse{
		{
			Id:      c.responseID,
			Object:  "chat.completion.chunk",
			Created: c.createdAt,
			Model:   c.model,
			Choices: []dto.ChatCompletionsStreamResponseChoice{
				{
					Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
						Role:    "assistant",
						Content: &empty,
					},
				},
			},
		},
	}
}

func (c *ResponsesToChatStreamConverter) StopChunks() []*dto.ChatCompletionsStreamResponse {
	if c == nil || c.sentStop {
		return nil
	}
	chunks := c.StartChunks()
	finishReason := "stop"
	if c.sawToolCall {
		finishReason = "tool_calls"
	}
	chunks = append(chunks, &dto.ChatCompletionsStreamResponse{
		Id:      c.responseID,
		Object:  "chat.completion.chunk",
		Created: c.createdAt,
		Model:   c.model,
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{
				FinishReason: &finishReason,
			},
		},
	})
	c.sentStop = true
	return chunks
}

func (c *ResponsesToChatStreamConverter) Usage() *dto.Usage {
	if c == nil {
		return nil
	}
	return cloneUsage(c.usage)
}

func (c *ResponsesToChatStreamConverter) SetUsage(usage *dto.Usage) {
	if c == nil {
		return
	}
	c.usage = cloneUsage(usage)
}

func (c *ResponsesToChatStreamConverter) UsageText() string {
	if c == nil {
		return ""
	}
	return c.usageText.String()
}

func (c *ResponsesToChatStreamConverter) ResponseID() string {
	if c == nil {
		return ""
	}
	return c.responseID
}

func (c *ResponsesToChatStreamConverter) CreatedAt() int64 {
	if c == nil {
		return 0
	}
	return c.createdAt
}

func (c *ResponsesToChatStreamConverter) Model() string {
	if c == nil {
		return ""
	}
	return c.model
}

func (c *ResponsesToChatStreamConverter) updateFromResponsesResponse(resp *dto.OpenAIResponsesResponse) {
	if resp == nil {
		return
	}
	if resp.Model != "" {
		c.model = resp.Model
	}
	if resp.CreatedAt != 0 {
		c.createdAt = int64(resp.CreatedAt)
	}
	if resp.Usage != nil {
		c.usage = responsesStreamUsageToChatUsage(resp.Usage)
	}
}

func (c *ResponsesToChatStreamConverter) reasoningSummaryDeltaChunks(delta string) []*dto.ChatCompletionsStreamResponse {
	if delta == "" {
		return nil
	}
	if c.needsReasoningSummarySpacing {
		if strings.HasPrefix(delta, "\n\n") {
			c.needsReasoningSummarySpacing = false
		} else if strings.HasPrefix(delta, "\n") {
			delta = "\n" + delta
			c.needsReasoningSummarySpacing = false
		} else {
			delta = "\n\n" + delta
			c.needsReasoningSummarySpacing = false
		}
	}
	c.usageText.WriteString(delta)
	chunks := c.StartChunks()
	reasoningDelta := delta
	chunks = append(chunks, &dto.ChatCompletionsStreamResponse{
		Id:      c.responseID,
		Object:  "chat.completion.chunk",
		Created: c.createdAt,
		Model:   c.model,
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{
				Index: 0,
				Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
					ReasoningContent: &reasoningDelta,
				},
			},
		},
	})
	c.hasSentReasoningSummary = true
	return chunks
}

func (c *ResponsesToChatStreamConverter) outputTextDeltaChunks(delta string) []*dto.ChatCompletionsStreamResponse {
	chunks := c.StartChunks()
	if delta == "" {
		return chunks
	}
	c.usageText.WriteString(delta)
	contentDelta := delta
	chunks = append(chunks, &dto.ChatCompletionsStreamResponse{
		Id:      c.responseID,
		Object:  "chat.completion.chunk",
		Created: c.createdAt,
		Model:   c.model,
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{
				Index: 0,
				Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
					Content: &contentDelta,
				},
			},
		},
	})
	return chunks
}

func (c *ResponsesToChatStreamConverter) outputItemChunks(item *dto.ResponsesOutput) []*dto.ChatCompletionsStreamResponse {
	if item == nil || item.Type != OutputTypeFunctionCall {
		return nil
	}

	itemID := strings.TrimSpace(item.ID)
	callID := strings.TrimSpace(item.CallId)
	if callID == "" {
		callID = itemID
	}
	if itemID != "" && callID != "" {
		c.toolCallCanonicalIDByItemID[itemID] = callID
	}
	name := strings.TrimSpace(item.Name)
	if name != "" {
		c.toolCallNameByID[callID] = name
	}

	newArgs := item.ArgumentsString()
	prevArgs := c.toolCallArgsByID[callID]
	argsDelta := ""
	if newArgs != "" {
		if strings.HasPrefix(newArgs, prevArgs) {
			argsDelta = newArgs[len(prevArgs):]
		} else {
			argsDelta = newArgs
		}
		c.toolCallArgsByID[callID] = newArgs
	}
	return c.toolCallDeltaChunks(callID, name, argsDelta)
}

func (c *ResponsesToChatStreamConverter) functionCallArgumentsDeltaChunks(itemID string, delta string) []*dto.ChatCompletionsStreamResponse {
	itemID = strings.TrimSpace(itemID)
	callID := c.toolCallCanonicalIDByItemID[itemID]
	if callID == "" {
		callID = itemID
	}
	if callID == "" {
		return nil
	}
	c.toolCallArgsByID[callID] += delta
	return c.toolCallDeltaChunks(callID, "", delta)
}

func (c *ResponsesToChatStreamConverter) toolCallDeltaChunks(callID string, name string, argsDelta string) []*dto.ChatCompletionsStreamResponse {
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return nil
	}

	idx, ok := c.toolCallIndexByID[callID]
	if !ok {
		idx = len(c.toolCallIndexByID)
		c.toolCallIndexByID[callID] = idx
	}
	if name != "" {
		c.toolCallNameByID[callID] = name
	}
	if c.toolCallNameByID[callID] != "" {
		name = c.toolCallNameByID[callID]
	}

	tool := dto.ToolCallResponse{
		ID:   callID,
		Type: ToolTypeFunction,
		Function: dto.FunctionResponse{
			Arguments: argsDelta,
		},
	}
	tool.SetIndex(idx)
	if name != "" && !c.toolCallNameSent[callID] {
		tool.Function.Name = name
		c.toolCallNameSent[callID] = true
	}

	chunks := c.StartChunks()
	chunks = append(chunks, &dto.ChatCompletionsStreamResponse{
		Id:      c.responseID,
		Object:  "chat.completion.chunk",
		Created: c.createdAt,
		Model:   c.model,
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{
				Index: 0,
				Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
					ToolCalls: []dto.ToolCallResponse{tool},
				},
			},
		},
	})
	c.sawToolCall = true
	if tool.Function.Name != "" {
		c.usageText.WriteString(tool.Function.Name)
	}
	if argsDelta != "" {
		c.usageText.WriteString(argsDelta)
	}
	return chunks
}

func responsesStreamUsageToChatUsage(usage *dto.Usage) *dto.Usage {
	out := responsesUsageToIRUsage(usage)
	if out == nil {
		return nil
	}
	if usage.InputTokensDetails != nil {
		out.PromptTokensDetails.CachedTokens = usage.InputTokensDetails.CachedTokens
		out.PromptTokensDetails.ImageTokens = usage.InputTokensDetails.ImageTokens
		out.PromptTokensDetails.AudioTokens = usage.InputTokensDetails.AudioTokens
	}
	if usage.CompletionTokenDetails.ReasoningTokens != 0 {
		out.CompletionTokenDetails.ReasoningTokens = usage.CompletionTokenDetails.ReasoningTokens
	}
	return out
}
