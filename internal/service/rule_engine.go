package service

import (
	"regexp"
	"strings"

	"github.com/user/notification-hub/internal/domain"
)

type RuleEngine struct {
	channels []domain.Channel
}

func NewRuleEngine(channels []domain.Channel) *RuleEngine {
	return &RuleEngine{channels: channels}
}

func (e *RuleEngine) ResolveChannels(msg *domain.Message) []string {
	if len(msg.Channels) > 0 {
		return uniqueStrings(msg.Channels)
	}
	matched := []string{}
	for _, channel := range e.channels {
		if !channel.IsActive {
			continue
		}
		for _, rule := range channel.Rules {
			if ruleMatches(rule, msg) {
				matched = append(matched, channelsForRule(rule, channel.Platform)...)
			}
		}
	}
	if len(matched) > 0 {
		return uniqueStrings(matched)
	}
	defaults := []string{}
	for _, channel := range e.channels {
		if channel.IsActive && channel.IsDefault {
			defaults = append(defaults, string(channel.Platform))
		}
	}
	return uniqueStrings(defaults)
}

func ruleMatches(rule domain.Rule, msg *domain.Message) bool {
	switch rule.Type {
	case domain.RuleTypeKeyword:
		return matchPattern(rule.Pattern, msg.ContentOriginal) || matchPattern(rule.Pattern, msg.ContentProcessed)
	case domain.RuleTypeSource:
		if rule.Source != "" {
			return strings.EqualFold(rule.Source, msg.Source)
		}
		return matchPattern(rule.Pattern, msg.Source)
	case domain.RuleTypePriority:
		if rule.Priority != "" {
			return rule.Priority == msg.Priority
		}
		return strings.EqualFold(rule.Pattern, string(msg.Priority))
	default:
		return false
	}
}

func matchPattern(pattern, value string) bool {
	if pattern == "" {
		return false
	}
	re, err := regexp.Compile("(?i)" + pattern)
	if err == nil {
		return re.MatchString(value)
	}
	return strings.Contains(strings.ToLower(value), strings.ToLower(pattern))
}

func channelsForRule(rule domain.Rule, fallback domain.Platform) []string {
	if len(rule.Channels) > 0 {
		return rule.Channels
	}
	if len(rule.Action.Channels) > 0 {
		return rule.Action.Channels
	}
	return []string{string(fallback)}
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
