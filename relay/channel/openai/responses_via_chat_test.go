package openai

import (
	"bufio"
	"context"
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
