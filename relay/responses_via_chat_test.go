package relay

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
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
