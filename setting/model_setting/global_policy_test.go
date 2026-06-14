package model_setting

import "testing"

func TestChatCompletionsToResponsesPolicyExcludePatterns(t *testing.T) {
	policy := ChatCompletionsToResponsesPolicy{
		Enabled:         true,
		ModelPatterns:   []string{`^gpt-5`},
		ExcludePatterns: []string{`mini$`},
	}

	if !policy.IsModelEnabled("gpt-5") {
		t.Fatal("expected gpt-5 to match")
	}
	if policy.IsModelEnabled("gpt-5-mini") {
		t.Fatal("expected exclude_patterns to block gpt-5-mini")
	}
}

func TestChatCompletionsToResponsesPolicyMatchTarget(t *testing.T) {
	policy := ChatCompletionsToResponsesPolicy{
		Enabled:       true,
		ModelPatterns: []string{`^upstream-model$`},
		MatchTarget:   PolicyMatchTargetUpstream,
	}

	if !policy.IsModelEnabledForTarget("origin-model", "upstream-model") {
		t.Fatal("expected upstream model to match")
	}
	if policy.IsModelEnabledForTarget("upstream-model", "mapped-model") {
		t.Fatal("expected origin model to be ignored when upstream target is set")
	}
	if !policy.IsModelEnabledForTarget("upstream-model", "") {
		t.Fatal("expected empty upstream model to fall back to origin model")
	}
}

func TestChatCompletionsToResponsesPolicyValidate(t *testing.T) {
	policy := ChatCompletionsToResponsesPolicy{
		ModelPatterns: []string{"["},
	}
	if err := policy.Validate(); err == nil {
		t.Fatal("expected invalid model_patterns regex to be rejected")
	}

	policy = ChatCompletionsToResponsesPolicy{
		ExcludePatterns: []string{"["},
	}
	if err := policy.Validate(); err == nil {
		t.Fatal("expected invalid exclude_patterns regex to be rejected")
	}

	policy = ChatCompletionsToResponsesPolicy{
		MatchTarget: "invalid",
	}
	if err := policy.Validate(); err == nil {
		t.Fatal("expected invalid match_target to be rejected")
	}
}
