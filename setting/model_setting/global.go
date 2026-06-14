package model_setting

import (
	"fmt"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service/openaicompat"
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

func (p ChatCompletionsToResponsesPolicy) GetModelPatterns() []string {
	return p.ModelPatterns
}

func (p ChatCompletionsToResponsesPolicy) GetExcludePatterns() []string {
	return p.ExcludePatterns
}

func (p ChatCompletionsToResponsesPolicy) GetMatchTarget() string {
	return p.MatchTarget
}

func ValidateChatCompletionsToResponsesPolicyJSON(value string) error {
	var policy ChatCompletionsToResponsesPolicy
	if err := common.UnmarshalJsonStr(value, &policy); err != nil {
		return err
	}
	return ValidateChatCompletionsToResponsesPolicy(policy)
}

func ValidateChatCompletionsToResponsesPolicy(policy ChatCompletionsToResponsesPolicy) error {
	if err := openaicompat.ValidateRegexPatterns(policy.ModelPatterns); err != nil {
		return fmt.Errorf("invalid model_patterns: %w", err)
	}
	if err := openaicompat.ValidateRegexPatterns(policy.ExcludePatterns); err != nil {
		return fmt.Errorf("invalid exclude_patterns: %w", err)
	}
	return nil
}

type GlobalSettings struct {
	PassThroughRequestEnabled        bool                             `json:"pass_through_request_enabled"`
	ThinkingModelBlacklist           []string                         `json:"thinking_model_blacklist"`
	ChatCompletionsToResponsesPolicy ChatCompletionsToResponsesPolicy `json:"chat_completions_to_responses_policy"`
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
