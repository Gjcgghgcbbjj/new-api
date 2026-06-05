package openai

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func TestOaiChatToResponsesStreamHandlerSendsStartBeforeUpstreamChunk(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstreamReader, upstreamWriter := io.Pipe()
	upstreamResp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       upstreamReader,
	}
	handlerDone := make(chan error, 1)

	router := gin.New()
	router.GET("/stream", func(c *gin.Context) {
		c.Set(common.RequestIdKey, "start-before-upstream")
		_, apiErr := OaiChatToResponsesStreamHandler(
			c,
			&relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "upstream-chat-model"},
			},
			upstreamResp,
			&dto.OpenAIResponsesRequest{Model: "codex-model"},
		)
		if apiErr != nil {
			handlerDone <- fmt.Errorf("%v", apiErr)
			return
		}
		handlerDone <- nil
	})

	server := httptest.NewServer(router)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/stream", nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("stream did not start before upstream produced data: %v", err)
	}
	defer resp.Body.Close()

	line, err := readSSEEventLine(resp.Body, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("did not receive initial Responses event before upstream data: %v", err)
	}
	if line != "event: response.created" {
		t.Fatalf("first event = %q, want response.created", line)
	}

	if err := upstreamWriter.Close(); err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)

	select {
	case err := <-handlerDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("handler did not finish after upstream closed")
	}
}

func readSSEEventLine(r io.Reader, timeout time.Duration) (string, error) {
	type result struct {
		line string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		reader := bufio.NewReader(r)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				done <- result{err: err}
				return
			}
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "event: ") {
				done <- result{line: line}
				return
			}
		}
	}()

	select {
	case res := <-done:
		return res.line, res.err
	case <-time.After(timeout):
		return "", fmt.Errorf("timeout waiting for SSE event")
	}
}

func TestOaiResponsesToChatStreamHandlerPreservesTextAndToolCallDeltas(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set(common.RequestIdKey, "responses-to-chat-mixed")
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	stream := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","object":"response","created_at":123,"status":"in_progress","model":"responses-model","output":[]}}`,
		`data: {"type":"response.output_text.delta","delta":"Let me check."}`,
		`data: {"type":"response.output_item.done","item":{"id":"fc_1","type":"function_call","status":"completed","call_id":"call_1","name":"get_weather","arguments":{"city":"Shanghai"}}}`,
		`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","created_at":123,"status":"completed","model":"responses-model","output":[],"usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}}`,
		`data: [DONE]`,
		``,
	}, "\n\n")

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(stream)),
	}

	usage, apiErr := OaiResponsesToChatStreamHandler(ctx, &relaycommon.RelayInfo{
		RelayFormat:        types.RelayFormatOpenAI,
		ShouldIncludeUsage: true,
		ChannelMeta:        &relaycommon.ChannelMeta{UpstreamModelName: "responses-model"},
	}, resp)
	if apiErr != nil {
		t.Fatalf("unexpected error: %v", apiErr)
	}
	if usage == nil || usage.InputTokens != 2 || usage.OutputTokens != 3 || usage.TotalTokens != 5 {
		t.Fatalf("usage = %+v", usage)
	}

	chunks := parseChatStreamChunks(t, recorder.Body.String())
	var sawText bool
	var sawToolCall bool
	var finishReason string
	for _, chunk := range chunks {
		if len(chunk.Choices) == 0 {
			continue
		}
		choice := chunk.Choices[0]
		if choice.Delta.GetContentString() == "Let me check." {
			sawText = true
		}
		if len(choice.Delta.ToolCalls) > 0 {
			sawToolCall = true
			tool := choice.Delta.ToolCalls[0]
			if tool.ID != "call_1" || tool.Function.Name != "get_weather" || tool.Function.Arguments != `{"city":"Shanghai"}` {
				t.Fatalf("tool call delta = %+v", tool)
			}
		}
		if choice.FinishReason != nil {
			finishReason = *choice.FinishReason
		}
	}
	if !sawText {
		t.Fatalf("expected text delta in stream, got body:\n%s", recorder.Body.String())
	}
	if !sawToolCall {
		t.Fatalf("expected tool call delta in stream, got body:\n%s", recorder.Body.String())
	}
	if finishReason != "tool_calls" {
		t.Fatalf("finish reason = %q", finishReason)
	}
}

func parseChatStreamChunks(t *testing.T, body string) []dto.ChatCompletionsStreamResponse {
	t.Helper()
	var chunks []dto.ChatCompletionsStreamResponse
	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
		if data == "" || data == "[DONE]" {
			continue
		}
		var chunk dto.ChatCompletionsStreamResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			t.Fatalf("unmarshal stream chunk %q: %v", data, err)
		}
		chunks = append(chunks, chunk)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return chunks
}
