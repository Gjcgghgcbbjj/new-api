package openaicompat

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/model_setting"
)

func TestShouldChatCompletionsUseResponsesPolicyUsesUpstreamTarget(t *testing.T) {
	policy := model_setting.ChatCompletionsToResponsesPolicy{
		Enabled:       true,
		AllChannels:   true,
		ModelPatterns: []string{`^upstream-model$`},
		MatchTarget:   model_setting.PolicyMatchTargetUpstream,
	}

	if !ShouldChatCompletionsUseResponsesPolicy(policy, 1, 1, "origin-model", "upstream-model") {
		t.Fatal("expected upstream model to match")
	}
	if ShouldChatCompletionsUseResponsesPolicy(policy, 1, 1, "origin-model", "other-upstream-model") {
		t.Fatal("expected non-matching upstream model to be rejected")
	}
}

func TestShouldChatCompletionsUseResponsesPolicyAppliesExcludePatterns(t *testing.T) {
	policy := model_setting.ChatCompletionsToResponsesPolicy{
		Enabled:         true,
		AllChannels:     true,
		ModelPatterns:   []string{`^gpt-5`},
		ExcludePatterns: []string{`mini$`},
	}

	if !ShouldChatCompletionsUseResponsesPolicy(policy, 1, 1, "gpt-5") {
		t.Fatal("expected model to match")
	}
	if ShouldChatCompletionsUseResponsesPolicy(policy, 1, 1, "gpt-5-mini") {
		t.Fatal("expected exclude pattern to block model")
	}
}
