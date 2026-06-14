package model_setting

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/setting/config"
)

type ChatCompletionsToResponsesPolicy struct {
	Enabled         bool     `json:"enabled"`
	AllChannels     bool     `json:"all_channels"`
	ChannelIDs      []int    `json:"channel_ids,omitempty"`
	ChannelTypes    []int    `json:"channel_types,omitempty"`
	ModelPatterns   []string `json:"model_patterns,omitempty"`
	ExcludePatterns []string `json:"exclude_patterns,omitempty"`
	MatchTarget     string   `json:"match_target,omitempty"`
}

var policyRegexCache sync.Map // map[string]*regexp.Regexp

const (
	PolicyMatchTargetOrigin   = "origin"
	PolicyMatchTargetUpstream = "upstream"
)

func (p ChatCompletionsToResponsesPolicy) IsChannelEnabled(channelID int, channelType int) bool {
	if !p.Enabled {
		return false
	}
	if p.AllChannels {
		return true
	}

	if channelID > 0 && len(p.ChannelIDs) > 0 && slices.Contains(p.ChannelIDs, channelID) {
		return true
	}
	if channelType > 0 && len(p.ChannelTypes) > 0 && slices.Contains(p.ChannelTypes, channelType) {
		return true
	}
	return false
}

func (p ChatCompletionsToResponsesPolicy) IsModelEnabled(model string) bool {
	return p.IsModelEnabledForTarget(model, "")
}

func (p ChatCompletionsToResponsesPolicy) IsModelEnabledForTarget(originModel string, upstreamModel string) bool {
	model := p.MatchModelName(originModel, upstreamModel)
	if !p.Enabled || strings.TrimSpace(model) == "" {
		return false
	}
	if !matchPolicyPatterns(p.ModelPatterns, model) {
		return false
	}
	if matchPolicyPatterns(p.ExcludePatterns, model) {
		return false
	}
	return true
}

func (p ChatCompletionsToResponsesPolicy) MatchModelName(originModel string, upstreamModel string) string {
	if strings.EqualFold(strings.TrimSpace(p.MatchTarget), PolicyMatchTargetUpstream) {
		if strings.TrimSpace(upstreamModel) != "" {
			return upstreamModel
		}
	}
	return originModel
}

func (p ChatCompletionsToResponsesPolicy) Validate() error {
	if err := validatePolicyPatterns("model_patterns", p.ModelPatterns); err != nil {
		return err
	}
	if err := validatePolicyPatterns("exclude_patterns", p.ExcludePatterns); err != nil {
		return err
	}
	target := strings.TrimSpace(p.MatchTarget)
	if target != "" && target != PolicyMatchTargetOrigin && target != PolicyMatchTargetUpstream {
		return fmt.Errorf("match_target must be %q or %q", PolicyMatchTargetOrigin, PolicyMatchTargetUpstream)
	}
	return nil
}

func validatePolicyPatterns(field string, patterns []string) error {
	for idx, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if _, err := regexp.Compile(pattern); err != nil {
			return fmt.Errorf("%s[%d] invalid regex %q: %w", field, idx, pattern, err)
		}
	}
	return nil
}

func matchPolicyPatterns(patterns []string, model string) bool {
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		re, ok := policyRegexCache.Load(pattern)
		if !ok {
			compiled, err := regexp.Compile(pattern)
			if err != nil {
				continue
			}
			re = compiled
			policyRegexCache.Store(pattern, re)
		}
		if re.(*regexp.Regexp).MatchString(model) {
			return true
		}
	}
	return false
}

type GlobalSettings struct {
	PassThroughRequestEnabled        bool                             `json:"pass_through_request_enabled"`
	ThinkingModelBlacklist           []string                         `json:"thinking_model_blacklist"`
	ChatCompletionsToResponsesPolicy ChatCompletionsToResponsesPolicy `json:"chat_completions_to_responses_policy"`
	ResponsesToChatCompletionsPolicy ChatCompletionsToResponsesPolicy `json:"responses_to_chat_completions_policy"`
}

// 默认配置
var defaultOpenaiSettings = GlobalSettings{
	PassThroughRequestEnabled: false,
	ThinkingModelBlacklist: []string{
		"moonshotai/kimi-k2-thinking",
		"kimi-k2-thinking",
	},
	ChatCompletionsToResponsesPolicy: ChatCompletionsToResponsesPolicy{
		Enabled:     false,
		AllChannels: true,
	},
	ResponsesToChatCompletionsPolicy: ChatCompletionsToResponsesPolicy{
		Enabled:     false,
		AllChannels: true,
	},
}

// 全局实例
var globalSettings = defaultOpenaiSettings

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("global", &globalSettings)
}

func GetGlobalSettings() *GlobalSettings {
	return &globalSettings
}

// ShouldPreserveThinkingSuffix 判断模型是否配置为保留 thinking/-nothinking/-low/-high/-medium 后缀
func ShouldPreserveThinkingSuffix(modelName string) bool {
	target := strings.TrimSpace(modelName)
	if target == "" {
		return false
	}

	for _, entry := range globalSettings.ThinkingModelBlacklist {
		if strings.TrimSpace(entry) == target {
			return true
		}
	}
	return false
}
