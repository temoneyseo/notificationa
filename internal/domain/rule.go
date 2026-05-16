package domain

type RuleType string

const (
	RuleTypeKeyword  RuleType = "keyword"
	RuleTypeSource   RuleType = "source"
	RuleTypePriority RuleType = "priority"
)

type RuleAction struct {
	Channels          []string     `json:"channels,omitempty"`
	Priority          Priority     `json:"priority,omitempty"`
	NotifyAllChannels bool         `json:"notify_all_channels,omitempty"`
	AIProcessing      AIProcessing `json:"ai_processing,omitempty"`
}

type Rule struct {
	Type     RuleType   `json:"type"`
	Pattern  string     `json:"pattern,omitempty"`
	Source   string     `json:"source,omitempty"`
	Priority Priority   `json:"priority,omitempty"`
	Channels []string   `json:"channels,omitempty"`
	Action   RuleAction `json:"action,omitempty"`
}
