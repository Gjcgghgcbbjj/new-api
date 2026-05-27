package model

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/model_setting"
)

func TestAppendResponsesViaChatEndpoints(t *testing.T) {
	settings := model_setting.GetGlobalSettings()
	original := *settings
	defer func() {
		*settings = original
	}()

	settings.ResponsesToChatCompletionsPolicy = model_setting.ChatCompletionsToResponsesPolicy{
		Enabled:       true,
		ChannelIDs:    []int{10},
		ModelPatterns: []string{"^deepseek-chat$"},
	}

	endpoints := map[string][]string{
		"deepseek-chat": {string(constant.EndpointTypeOpenAI)},
		"other-model":   {string(constant.EndpointTypeOpenAI)},
	}
	appendResponsesViaChatEndpoints(endpoints, []AbilityWithChannel{
		{
			Ability: Ability{
				Model:     "deepseek-chat",
				ChannelId: 10,
			},
			ChannelType: constant.ChannelTypeOpenAI,
		},
		{
			Ability: Ability{
				Model:     "other-model",
				ChannelId: 10,
			},
			ChannelType: constant.ChannelTypeOpenAI,
		},
		{
			Ability: Ability{
				Model:     "deepseek-chat",
				ChannelId: 11,
			},
			ChannelType: constant.ChannelTypeOpenAI,
		},
	})

	got := endpoints["deepseek-chat"]
	want := string(constant.EndpointTypeOpenAIResponseViaChat)
	if len(got) != 2 || got[1] != want {
		t.Fatalf("expected bridge endpoint appended once, got %#v", got)
	}
	if len(endpoints["other-model"]) != 1 {
		t.Fatalf("expected non-matching model unchanged, got %#v", endpoints["other-model"])
	}
}

func TestAppendResponsesViaChatEndpointsIgnoresUnsupportedChannelType(t *testing.T) {
	settings := model_setting.GetGlobalSettings()
	original := *settings
	defer func() {
		*settings = original
	}()

	settings.ResponsesToChatCompletionsPolicy = model_setting.ChatCompletionsToResponsesPolicy{
		Enabled:       true,
		AllChannels:   true,
		ModelPatterns: []string{".*"},
	}

	endpoints := map[string][]string{
		"claude-sonnet": {string(constant.EndpointTypeAnthropic), string(constant.EndpointTypeOpenAI)},
	}
	appendResponsesViaChatEndpoints(endpoints, []AbilityWithChannel{
		{
			Ability: Ability{
				Model:     "claude-sonnet",
				ChannelId: 20,
			},
			ChannelType: constant.ChannelTypeAnthropic,
		},
	})

	if len(endpoints["claude-sonnet"]) != 2 {
		t.Fatalf("expected unsupported channel type unchanged, got %#v", endpoints["claude-sonnet"])
	}
}
