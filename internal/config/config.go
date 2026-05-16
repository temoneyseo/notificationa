package config

import (
	"bufio"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/user/notification-hub/internal/domain"
)

type OpenAIConfig struct {
	APIKey  string
	Model   string
	BaseURL string
	Timeout time.Duration
}

type ChannelConfig struct {
	Platform  string
	Name      string
	Config    map[string]any
	Rules     []RuleConfig
	AIEnabled bool
	AIPrompt  string
	IsActive  bool
	IsDefault bool
}

type RuleConfig struct {
	Type     string
	Pattern  string
	Source   string
	Priority string
	Channels []string
}

type Config struct {
	HTTPAddr           string
	DatabasePath       string
	EncryptionKey      string
	ConfigPath         string
	ShutdownTimeout    time.Duration
	OpenAI             OpenAIConfig
	Channels           []ChannelConfig
	LogInboundMessages bool
}

func Load() (Config, error) {
	dotenv, err := loadDotEnv(".env")
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		HTTPAddr:           ":8080",
		DatabasePath:       "./data/notification-hub.db",
		ConfigPath:         defaultConfigPath(getenv("APP_CONFIG", dotenv, "")),
		ShutdownTimeout:    10 * time.Second,
		LogInboundMessages: true,
		OpenAI: OpenAIConfig{
			Model:   "gpt-4o-mini",
			BaseURL: "https://api.openai.com/v1",
			Timeout: 30 * time.Second,
		},
	}

	if cfg.ConfigPath != "" {
		if err := loadYAML(cfg.ConfigPath, &cfg); err != nil {
			return Config{}, err
		}
		expandChannelConfigValues(cfg.Channels, dotenv)
	}

	cfg.HTTPAddr = getenv("HTTP_ADDR", dotenv, cfg.HTTPAddr)
	cfg.DatabasePath = getenv("DATABASE_PATH", dotenv, cfg.DatabasePath)
	cfg.EncryptionKey = getenv("ENCRYPTION_KEY", dotenv, cfg.EncryptionKey)
	cfg.OpenAI.APIKey = getenv("OPENAI_API_KEY", dotenv, cfg.OpenAI.APIKey)
	cfg.OpenAI.Model = getenv("OPENAI_MODEL", dotenv, cfg.OpenAI.Model)
	cfg.OpenAI.BaseURL = getenv("OPENAI_BASE_URL", dotenv, cfg.OpenAI.BaseURL)

	if v := getenv("OPENAI_TIMEOUT", dotenv, ""); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, err
		}
		cfg.OpenAI.Timeout = d
	}
	return cfg, nil
}

func (c Config) DomainChannels() []domain.Channel {
	channels := make([]domain.Channel, 0, len(c.Channels))
	for _, item := range c.Channels {
		ch := domain.NewChannel(domain.Platform(item.Platform), item.Name)
		ch.ID = item.Name
		ch.Config = copyConfigMap(item.Config)
		ch.AIEnabled = item.AIEnabled
		ch.AIPrompt = item.AIPrompt
		ch.IsActive = item.IsActive
		ch.IsDefault = item.IsDefault
		ch.Rules = make([]domain.Rule, 0, len(item.Rules))
		for _, rule := range item.Rules {
			ch.Rules = append(ch.Rules, domain.Rule{
				Type:     domain.RuleType(rule.Type),
				Pattern:  rule.Pattern,
				Source:   rule.Source,
				Priority: domain.Priority(rule.Priority),
				Channels: append([]string{}, rule.Channels...),
			})
		}
		ch.Normalize()
		channels = append(channels, *ch)
	}
	return channels
}

func copyConfigMap(input map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range input {
		out[key] = value
	}
	return out
}

func defaultConfigPath(configPath string) string {
	if configPath != "" {
		return configPath
	}
	if _, err := os.Stat("config.yaml"); err == nil {
		return "config.yaml"
	}
	if _, err := os.Stat("config.yml"); err == nil {
		return "config.yml"
	}
	return "config.yaml"
}

func loadYAML(path string, cfg *Config) error {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer file.Close()

	lines := []string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if line == "channels:" {
			channels, err := parseChannelConfigs(lines[i+1:])
			if err != nil {
				return err
			}
			cfg.Channels = channels
			break
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := trimConfigValue(parts[1])
		switch key {
		case "http_addr":
			cfg.HTTPAddr = value
		case "database_path":
			cfg.DatabasePath = value
		case "encryption_key":
			cfg.EncryptionKey = value
		case "openai_api_key":
			cfg.OpenAI.APIKey = value
		case "openai_model":
			cfg.OpenAI.Model = value
		case "log_inbound_messages":
			cfg.LogInboundMessages = parseBool(value)
		case "openai_timeout":
			d, err := time.ParseDuration(value)
			if err != nil {
				return err
			}
			cfg.OpenAI.Timeout = d
		}
	}
	return nil
}

func parseChannelConfigs(lines []string) ([]ChannelConfig, error) {
	channels := []ChannelConfig{}
	var current *ChannelConfig
	var section string
	var currentRule *RuleConfig
	for _, raw := range lines {
		if strings.TrimSpace(raw) == "" || strings.HasPrefix(strings.TrimSpace(raw), "#") {
			continue
		}
		indent := leadingSpaces(raw)
		line := strings.TrimSpace(raw)
		if indent == 0 && !strings.HasPrefix(line, "-") {
			break
		}
		if indent == 2 && strings.HasPrefix(line, "- ") {
			if current != nil {
				channels = append(channels, *current)
			}
			current = &ChannelConfig{Config: map[string]any{}, Rules: []RuleConfig{}, IsActive: true}
			section = ""
			currentRule = nil
			key, value, ok := splitConfigPair(strings.TrimPrefix(line, "- "))
			if ok {
				setChannelField(current, key, value)
			}
			continue
		}
		if current == nil {
			continue
		}
		if indent == 4 && strings.HasSuffix(line, ":") {
			section = strings.TrimSuffix(line, ":")
			currentRule = nil
			continue
		}
		if section == "config" && indent == 6 {
			key, value, ok := splitConfigPair(line)
			if ok {
				current.Config[key] = value
			}
			continue
		}
		if section == "rules" && indent == 6 && strings.HasPrefix(line, "- ") {
			current.Rules = append(current.Rules, RuleConfig{})
			currentRule = &current.Rules[len(current.Rules)-1]
			key, value, ok := splitConfigPair(strings.TrimPrefix(line, "- "))
			if ok {
				setRuleField(currentRule, key, value)
			}
			continue
		}
		if section == "rules" && indent == 8 && currentRule != nil {
			key, value, ok := splitConfigPair(line)
			if ok {
				setRuleField(currentRule, key, value)
			}
			continue
		}
		if indent == 4 {
			key, value, ok := splitConfigPair(line)
			if ok {
				setChannelField(current, key, value)
			}
		}
	}
	if current != nil {
		channels = append(channels, *current)
	}
	return channels, nil
}

func leadingSpaces(value string) int {
	return len(value) - len(strings.TrimLeft(value, " "))
}

func splitConfigPair(line string) (string, string, bool) {
	key, value, ok := strings.Cut(line, ":")
	if !ok {
		return "", "", false
	}
	return strings.TrimSpace(key), trimConfigValue(value), true
}

func trimConfigValue(value string) string {
	return strings.Trim(strings.TrimSpace(value), `"'`)
}

func setChannelField(channel *ChannelConfig, key string, value string) {
	switch key {
	case "platform":
		channel.Platform = value
	case "name":
		channel.Name = value
	case "default", "is_default":
		channel.IsDefault = parseBool(value)
	case "active", "is_active":
		channel.IsActive = parseBool(value)
	case "ai_enabled":
		channel.AIEnabled = parseBool(value)
	case "ai_prompt":
		channel.AIPrompt = value
	}
}

func setRuleField(rule *RuleConfig, key string, value string) {
	switch key {
	case "type":
		rule.Type = value
	case "pattern":
		rule.Pattern = value
	case "source":
		rule.Source = value
	case "priority":
		rule.Priority = value
	case "channels":
		rule.Channels = parseStringList(value)
	}
}

func parseBool(value string) bool {
	return value == "true" || value == "yes" || value == "1"
}

func parseStringList(value string) []string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "[")
	value = strings.TrimSuffix(value, "]")
	if strings.TrimSpace(value) == "" {
		return []string{}
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.Trim(strings.TrimSpace(part), `"'`)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func expandChannelConfigValues(channels []ChannelConfig, dotenv map[string]string) {
	for i := range channels {
		for key, value := range channels[i].Config {
			text, ok := value.(string)
			if !ok {
				continue
			}
			channels[i].Config[key] = expandEnvValue(text, dotenv)
		}
	}
}

func expandEnvValue(value string, dotenv map[string]string) string {
	if !strings.HasPrefix(value, "${") || !strings.HasSuffix(value, "}") {
		return value
	}
	key := strings.TrimSuffix(strings.TrimPrefix(value, "${"), "}")
	if env := os.Getenv(key); env != "" {
		return env
	}
	if env := dotenv[key]; env != "" {
		return env
	}
	return value
}

func loadDotEnv(path string) (map[string]string, error) {
	values := map[string]string{}
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return values, nil
		}
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if key != "" {
			values[key] = value
		}
	}
	return values, scanner.Err()
}

func getenv(key string, dotenv map[string]string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	if value := dotenv[key]; value != "" {
		return value
	}
	return fallback
}
