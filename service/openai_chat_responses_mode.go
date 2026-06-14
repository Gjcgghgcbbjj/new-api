package service

import (
	"github.com/QuantumNous/new-api/service/openaicompat"
	"github.com/QuantumNous/new-api/setting/model_setting"
)

func SupportsResponsesConversion(channelType int) bool {
	return openaicompat.SupportsResponsesConversion(channelType)
}

func ShouldChatCompletionsUseResponsesPolicy(policy model_setting.ChatCompletionsToResponsesPolicy, channelID int, channelType int, originModel string, upstreamModel ...string) bool {
	return openaicompat.ShouldChatCompletionsUseResponsesPolicy(policy, channelID, channelType, originModel, upstreamModel...)
}

func ShouldChatCompletionsUseResponsesGlobal(channelID int, channelType int, originModel string, upstreamModel ...string) bool {
	return openaicompat.ShouldChatCompletionsUseResponsesPolicy(
		model_setting.GetGlobalSettings().ChatCompletionsToResponsesPolicy,
		channelID,
		channelType,
		originModel,
		upstreamModel...,
	)
}
