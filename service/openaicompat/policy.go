package openaicompat

import "github.com/QuantumNous/new-api/setting/model_setting"

func ShouldChatCompletionsUseResponsesPolicy(policy model_setting.ChatCompletionsToResponsesPolicy, channelID int, channelType int, originModel string, upstreamModel ...string) bool {
	if !policy.IsChannelEnabled(channelID, channelType) {
		return false
	}
	targetUpstream := ""
	if len(upstreamModel) > 0 {
		targetUpstream = upstreamModel[0]
	}
	return policy.IsModelEnabledForTarget(originModel, targetUpstream)
}

func ShouldChatCompletionsUseResponsesGlobal(channelID int, channelType int, originModel string, upstreamModel ...string) bool {
	return ShouldChatCompletionsUseResponsesPolicy(
		model_setting.GetGlobalSettings().ChatCompletionsToResponsesPolicy,
		channelID,
		channelType,
		originModel,
		upstreamModel...,
	)
}

func ShouldResponsesUseChatCompletionsGlobal(channelID int, channelType int, originModel string, upstreamModel ...string) bool {
	return ShouldChatCompletionsUseResponsesPolicy(
		model_setting.GetGlobalSettings().ResponsesToChatCompletionsPolicy,
		channelID,
		channelType,
		originModel,
		upstreamModel...,
	)
}
