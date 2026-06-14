package service

import (
	"github.com/QuantumNous/new-api/service/openaicompat"
	"github.com/QuantumNous/new-api/setting/model_setting"
)

func ShouldChatCompletionsUseResponsesPolicy(policy model_setting.ChatCompletionsToResponsesPolicy, channelID int, channelType int, originModel string, upstreamModel ...string) bool {
	return openaicompat.ShouldChatCompletionsUseResponsesPolicy(policy, channelID, channelType, originModel, upstreamModel...)
}

func ShouldChatCompletionsUseResponsesGlobal(channelID int, channelType int, originModel string, upstreamModel ...string) bool {
	return openaicompat.ShouldChatCompletionsUseResponsesGlobal(channelID, channelType, originModel, upstreamModel...)
}

func ShouldResponsesUseChatCompletionsGlobal(channelID int, channelType int, originModel string, upstreamModel ...string) bool {
	return openaicompat.ShouldResponsesUseChatCompletionsGlobal(channelID, channelType, originModel, upstreamModel...)
}
