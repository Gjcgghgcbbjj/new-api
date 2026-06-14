package openaicompat

import (
	"github.com/QuantumNous/new-api/constant"
)

type chatCompletionsToResponsesPolicy interface {
	IsChannelEnabled(channelID int, channelType int) bool
	GetModelPatterns() []string
	GetExcludePatterns() []string
	GetMatchTarget() string
}

func SupportsResponsesConversion(channelType int) bool {
	switch channelType {
	case constant.ChannelTypeOpenAI,
		constant.ChannelTypeAzure,
		constant.ChannelTypeAli,
		constant.ChannelCloudflare,
		constant.ChannelTypePerplexity,
		constant.ChannelTypeVolcEngine,
		constant.ChannelTypeXai,
		constant.ChannelTypeCodex:
		return true
	default:
		return false
	}
}

func ShouldChatCompletionsUseResponsesPolicy(policy chatCompletionsToResponsesPolicy, channelID int, channelType int, originModel string, upstreamModel ...string) bool {
	if !policy.IsChannelEnabled(channelID, channelType) {
		return false
	}
	model := policyMatchModel(policy.GetMatchTarget(), originModel, upstreamModel...)
	return matchAnyRegex(policy.GetModelPatterns(), model) && !matchAnyRegex(policy.GetExcludePatterns(), model)
}

func policyMatchModel(matchTarget string, originModel string, upstreamModel ...string) string {
	if matchTarget == "upstream" && len(upstreamModel) > 0 && upstreamModel[0] != "" {
		return upstreamModel[0]
	}
	return originModel
}
