package relay

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func TestShouldResponsesUseChatBridgeForChatOnlyOpenAICompatibleChannel(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeResponses,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeOpenAI,
			ChannelId:      1,
			ChannelBaseUrl: "http://proxy.example",
		},
	}

	if !shouldResponsesUseChatCompletionsBridge(info, openAIResponsesRequest("chat-only-model")) {
		t.Fatal("expected chat-only OpenAI-compatible channel to use responses->chat bridge")
	}
}

func TestShouldResponsesPreferNativeForOfficialOpenAI(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeResponses,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeOpenAI,
			ChannelId:      1,
			ChannelBaseUrl: "https://api.openai.com",
		},
	}

	if shouldResponsesUseChatCompletionsBridge(info, openAIResponsesRequest("gpt-5")) {
		t.Fatal("expected official OpenAI channel to keep native responses")
	}
}

func TestShouldResponsesPreferNativeForNativeResponseChannelType(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeResponses,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeXai,
			ChannelId:      1,
			ChannelBaseUrl: "https://api.x.ai",
		},
	}

	if shouldResponsesUseChatCompletionsBridge(info, openAIResponsesRequest("grok-4")) {
		t.Fatal("expected native responses channel type to keep native responses")
	}
}

func openAIResponsesRequest(model string) *dto.OpenAIResponsesRequest {
	return &dto.OpenAIResponsesRequest{Model: model}
}

func TestResponsesViaChatCompletionsConvertsRequestAndResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	stream := false
	request := &dto.OpenAIResponsesRequest{
		Model:        "chat-only-model",
		Input:        json.RawMessage(`[{"role":"user","content":[{"type":"input_text","text":"hello bridge"}]}]`),
		Instructions: json.RawMessage(`"be concise"`),
		Stream:       &stream,
	}
	info := &relaycommon.RelayInfo{
		RelayMode:      relayconstant.RelayModeResponses,
		RequestURLPath: "/v1/responses",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeOpenAI,
			ChannelBaseUrl:    "http://proxy.example",
			UpstreamModelName: "chat-only-model",
		},
	}
	adaptor := &responsesBridgeMockAdaptor{}

	usage, apiErr := responsesViaChatCompletions(ctx, info, adaptor, request)
	if apiErr != nil {
		t.Fatalf("unexpected bridge error: %v", apiErr)
	}
	if usage == nil || usage.InputTokens != 2 || usage.OutputTokens != 3 || usage.TotalTokens != 5 {
		t.Fatalf("usage = %+v", usage)
	}

	if adaptor.convertedMode != relayconstant.RelayModeChatCompletions || adaptor.doMode != relayconstant.RelayModeChatCompletions {
		t.Fatalf("bridge did not switch to chat mode: convert=%d do=%d", adaptor.convertedMode, adaptor.doMode)
	}
	if adaptor.convertedPath != "/v1/chat/completions" || adaptor.doPath != "/v1/chat/completions" {
		t.Fatalf("bridge path mismatch: convert=%q do=%q", adaptor.convertedPath, adaptor.doPath)
	}
	if adaptor.convertedRequest == nil {
		t.Fatal("converted request was not captured")
	}
	if len(adaptor.convertedRequest.Messages) != 2 {
		t.Fatalf("messages = %+v", adaptor.convertedRequest.Messages)
	}
	if adaptor.convertedRequest.Messages[0].Role != "system" || adaptor.convertedRequest.Messages[0].StringContent() != "be concise" {
		t.Fatalf("system message = %+v", adaptor.convertedRequest.Messages[0])
	}
	if adaptor.convertedRequest.Messages[1].Role != "user" || adaptor.convertedRequest.Messages[1].StringContent() != "hello bridge" {
		t.Fatalf("user message = %+v", adaptor.convertedRequest.Messages[1])
	}
	if adaptor.sentBody["model"] != "chat-only-model" {
		t.Fatalf("sent body model = %v", adaptor.sentBody["model"])
	}

	if info.RelayMode != relayconstant.RelayModeResponses || info.RequestURLPath != "/v1/responses" {
		t.Fatalf("relay info was not restored: mode=%d path=%q", info.RelayMode, info.RequestURLPath)
	}
	if info.UpstreamRequestURLPath != "/v1/chat/completions" {
		t.Fatalf("upstream path = %q", info.UpstreamRequestURLPath)
	}

	var response dto.OpenAIResponsesResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("response body is not responses JSON: %v\n%s", err, recorder.Body.String())
	}
	if response.Object != "response" || response.Model != "upstream-chat-model" || response.OutputText != "pong" {
		t.Fatalf("responses body = %+v", response)
	}
	if len(response.Output) != 1 || len(response.Output[0].Content) != 1 || response.Output[0].Content[0].Text != "pong" {
		t.Fatalf("responses output = %+v", response.Output)
	}
}

type responsesBridgeMockAdaptor struct {
	convertedRequest *dto.GeneralOpenAIRequest
	convertedMode    int
	convertedPath    string
	doMode           int
	doPath           string
	sentBody         map[string]any
}

func (a *responsesBridgeMockAdaptor) Init(info *relaycommon.RelayInfo) {}

func (a *responsesBridgeMockAdaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return "http://mock.invalid/v1/chat/completions", nil
}

func (a *responsesBridgeMockAdaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	return nil
}

func (a *responsesBridgeMockAdaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	a.convertedRequest = request
	a.convertedMode = info.RelayMode
	a.convertedPath = info.RequestURLPath
	return request, nil
}

func (a *responsesBridgeMockAdaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

func (a *responsesBridgeMockAdaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return nil, nil
}

func (a *responsesBridgeMockAdaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, nil
}

func (a *responsesBridgeMockAdaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	return nil, nil
}

func (a *responsesBridgeMockAdaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return nil, nil
}

func (a *responsesBridgeMockAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	a.doMode = info.RelayMode
	a.doPath = info.RequestURLPath
	body, err := io.ReadAll(requestBody)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(body, &a.sentBody); err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{
			"id":"chatcmpl_bridge",
			"object":"chat.completion",
			"created":123,
			"model":"upstream-chat-model",
			"choices":[{"index":0,"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}
		}`)),
	}, nil
}

func (a *responsesBridgeMockAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (any, *types.NewAPIError) {
	return nil, nil
}

func (a *responsesBridgeMockAdaptor) GetModelList() []string {
	return nil
}

func (a *responsesBridgeMockAdaptor) GetChannelName() string {
	return "mock"
}

func (a *responsesBridgeMockAdaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	return nil, nil
}

func (a *responsesBridgeMockAdaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	return nil, nil
}
