package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadParsesLogInboundMessages(t *testing.T) {
	dir := t.TempDir()
	body := []byte("log_inbound_messages: false\n")
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldwd) })
	t.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.LogInboundMessages {
		t.Fatal("LogInboundMessages should be false")
	}
}

func TestLoadExpandsScalarConfigEnvValues(t *testing.T) {
	dir := t.TempDir()
	body := []byte("encryption_key: ${ENCRYPTION_KEY}\nopenai_api_key: ${OPENAI_API_KEY}\n")
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	dotenv := []byte("ENCRYPTION_KEY=0123456789abcdef0123456789abcdef\nOPENAI_API_KEY=key-from-dotenv\n")
	if err := os.WriteFile(filepath.Join(dir, ".env"), dotenv, 0o600); err != nil {
		t.Fatal(err)
	}
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldwd) })

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.EncryptionKey != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("EncryptionKey = %q", cfg.EncryptionKey)
	}
	if cfg.OpenAI.APIKey != "key-from-dotenv" {
		t.Fatalf("OpenAI API key = %q", cfg.OpenAI.APIKey)
	}
}

func TestConfigChannelsToDomainChannels(t *testing.T) {
	cfg := Config{Channels: []ChannelConfig{
		{
			Platform:  "telegram",
			Name:      "telegram-main",
			Config:    map[string]any{"bot_token": "token", "chat_id": "-100"},
			IsActive:  true,
			IsDefault: true,
			Rules: []RuleConfig{{
				Type:     "keyword",
				Pattern:  "urgent",
				Channels: []string{"telegram"},
			}},
		},
	}}

	channels := cfg.DomainChannels()
	if len(channels) != 1 {
		t.Fatalf("channels length = %d", len(channels))
	}
	ch := channels[0]
	if ch.ID != "telegram-main" || ch.Platform != "telegram" || !ch.IsDefault || !ch.IsActive {
		t.Fatalf("unexpected channel: %+v", ch)
	}
	if ch.Config["bot_token"] != "token" || ch.Config["chat_id"] != "-100" {
		t.Fatalf("unexpected config: %+v", ch.Config)
	}
	if len(ch.Rules) != 1 || ch.Rules[0].Pattern != "urgent" || ch.Rules[0].Channels[0] != "telegram" {
		t.Fatalf("unexpected rules: %+v", ch.Rules)
	}
}

func TestLoadExpandsChannelConfigEnvValues(t *testing.T) {
	dir := t.TempDir()
	body := []byte(`channels:
  - platform: telegram
    name: telegram-main
    config:
      bot_token: ${TELEGRAM_BOT_TOKEN}
      chat_id: "${TELEGRAM_CHAT_ID}"
`)
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	dotenv := []byte("TELEGRAM_BOT_TOKEN=token-from-dotenv\nTELEGRAM_CHAT_ID=-100987654321\n")
	if err := os.WriteFile(filepath.Join(dir, ".env"), dotenv, 0o600); err != nil {
		t.Fatal(err)
	}
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldwd) })
	t.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if got := cfg.Channels[0].Config["bot_token"]; got != "token-from-dotenv" {
		t.Fatalf("bot_token = %q", got)
	}
	if got := cfg.Channels[0].Config["chat_id"]; got != "-100987654321" {
		t.Fatalf("chat_id = %q", got)
	}
}

func TestLoadParsesChannelsFromConfigYML(t *testing.T) {
	dir := t.TempDir()
	body := []byte(`http_addr: :18083
channels:
  - platform: telegram
    name: telegram-main
    default: true
    active: true
    ai_enabled: false
    ai_prompt: ""
    config:
      bot_token: plain-telegram-token
      chat_id: "-1001234567890"
  - platform: discord
    name: discord-main
    default: false
    active: true
    config:
      bot_token: plain-discord-token
      channel_id: "123456789012345678"
    rules:
      - type: keyword
        pattern: "urgent|紧急"
        channels: ["discord"]
`)
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldwd) })
	t.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if len(cfg.Channels) != 2 {
		t.Fatalf("channels length = %d", len(cfg.Channels))
	}
	telegram := cfg.Channels[0]
	if telegram.Platform != "telegram" || telegram.Name != "telegram-main" || !telegram.IsDefault || !telegram.IsActive {
		t.Fatalf("unexpected telegram channel: %+v", telegram)
	}
	if telegram.Config["bot_token"] != "plain-telegram-token" || telegram.Config["chat_id"] != "-1001234567890" {
		t.Fatalf("unexpected telegram config: %+v", telegram.Config)
	}
	discord := cfg.Channels[1]
	if discord.Platform != "discord" || discord.Name != "discord-main" || discord.IsDefault || !discord.IsActive {
		t.Fatalf("unexpected discord channel: %+v", discord)
	}
	if len(discord.Rules) != 1 || discord.Rules[0].Type != "keyword" || discord.Rules[0].Pattern != "urgent|紧急" {
		t.Fatalf("unexpected discord rules: %+v", discord.Rules)
	}
}

func TestLoadReadsDotEnvFile(t *testing.T) {
	dir := t.TempDir()
	dotenv := []byte("HTTP_ADDR=:18081\nDATABASE_PATH=./data/from-dotenv.db\nENCRYPTION_KEY=0123456789abcdef0123456789abcdef\nOPENAI_MODEL=from-dotenv\n")
	if err := os.WriteFile(filepath.Join(dir, ".env"), dotenv, 0o600); err != nil {
		t.Fatal(err)
	}
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldwd) })
	t.Setenv("APP_CONFIG", filepath.Join(dir, "missing.yaml"))

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.HTTPAddr != ":18081" {
		t.Fatalf("HTTPAddr = %q", cfg.HTTPAddr)
	}
	if cfg.DatabasePath != "./data/from-dotenv.db" {
		t.Fatalf("DatabasePath = %q", cfg.DatabasePath)
	}
	if cfg.EncryptionKey != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("EncryptionKey was not loaded from .env")
	}
	if cfg.OpenAI.Model != "from-dotenv" {
		t.Fatalf("OpenAI model = %q", cfg.OpenAI.Model)
	}
}

func TestLoadFallsBackToConfigYML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	body := []byte("http_addr: :18082\ndatabase_path: ./data/from-yml.db\nopenai_model: gpt-yml\n")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldwd) })
	t.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.HTTPAddr != ":18082" {
		t.Fatalf("HTTPAddr = %q", cfg.HTTPAddr)
	}
	if cfg.DatabasePath != "./data/from-yml.db" {
		t.Fatalf("DatabasePath = %q", cfg.DatabasePath)
	}
	if cfg.OpenAI.Model != "gpt-yml" {
		t.Fatalf("OpenAI model = %q", cfg.OpenAI.Model)
	}
}

func TestLoadUsesEnvAndYAMLConfig(t *testing.T) {
	t.Setenv("APP_CONFIG", filepath.Join(t.TempDir(), "missing.yaml"))
	t.Setenv("HTTP_ADDR", ":19090")
	t.Setenv("DATABASE_PATH", filepath.Join(t.TempDir(), "hub.db"))
	t.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("OPENAI_MODEL", "gpt-test")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.HTTPAddr != ":19090" {
		t.Fatalf("HTTPAddr = %q", cfg.HTTPAddr)
	}
	if cfg.DatabasePath == "" {
		t.Fatal("DatabasePath should be populated")
	}
	if cfg.OpenAI.Model != "gpt-test" {
		t.Fatalf("OpenAI model = %q", cfg.OpenAI.Model)
	}
}

func TestLoadParsesSimpleYAMLWhenPresent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := []byte("http_addr: :18080\ndatabase_path: ./data/test.db\nopenai_model: gpt-yaml\n")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APP_CONFIG", path)
	t.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.HTTPAddr != ":18080" {
		t.Fatalf("HTTPAddr = %q", cfg.HTTPAddr)
	}
	if cfg.DatabasePath != "./data/test.db" {
		t.Fatalf("DatabasePath = %q", cfg.DatabasePath)
	}
	if cfg.OpenAI.Model != "gpt-yaml" {
		t.Fatalf("OpenAI model = %q", cfg.OpenAI.Model)
	}
}

func TestLoadParsesACPConfigAfterChannelsAndEnvOverrides(t *testing.T) {
	dir := t.TempDir()
	body := []byte(`channels:
  - platform: telegram
    name: telegram-main
    config:
      bot_token: ${TELEGRAM_BOT_TOKEN}
      chat_id: "-100"

acp:
  enabled: true
  endpoint_url: https://yaml.example/acp
  auth_token: yaml-token
  default_project: notification
  default_agent: triage
  min_confidence: 0.75
  allowed_intents: ["docs_request", "incident"]
`)
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	dotenv := []byte("TELEGRAM_BOT_TOKEN=telegram-token\nACP_AUTH_TOKEN=dotenv-token\n")
	if err := os.WriteFile(filepath.Join(dir, ".env"), dotenv, 0o600); err != nil {
		t.Fatal(err)
	}
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldwd) })
	t.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("ACP_ENDPOINT_URL", "https://env.example/acp")
	t.Setenv("ACP_MIN_CONFIDENCE", "0.91")
	t.Setenv("ACP_ALLOWED_INTENTS", "support_request,docs_request")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if len(cfg.Channels) != 1 || cfg.Channels[0].Config["bot_token"] != "telegram-token" {
		t.Fatalf("channels not parsed or env-expanded: %+v", cfg.Channels)
	}
	if !cfg.ACP.Enabled {
		t.Fatal("ACP enabled should be true")
	}
	if cfg.ACP.EndpointURL != "https://env.example/acp" {
		t.Fatalf("ACP endpoint = %q", cfg.ACP.EndpointURL)
	}
	if cfg.ACP.AuthToken != "dotenv-token" {
		t.Fatalf("ACP auth token = %q", cfg.ACP.AuthToken)
	}
	if cfg.ACP.DefaultProject != "notification" || cfg.ACP.DefaultAgent != "triage" {
		t.Fatalf("ACP defaults = %+v", cfg.ACP)
	}
	if cfg.ACP.MinConfidence != 0.91 {
		t.Fatalf("ACP min confidence = %v", cfg.ACP.MinConfidence)
	}
	want := []string{"support_request", "docs_request"}
	if len(cfg.ACP.AllowedIntents) != len(want) {
		t.Fatalf("allowed intents = %+v", cfg.ACP.AllowedIntents)
	}
	for i := range want {
		if cfg.ACP.AllowedIntents[i] != want[i] {
			t.Fatalf("allowed intents = %+v", cfg.ACP.AllowedIntents)
		}
	}
}

func TestLoadTreatsUnresolvedACPPlaceholdersAsEmpty(t *testing.T) {
	dir := t.TempDir()
	body := []byte(`acp:
  enabled: true
  endpoint_url: ${ACP_ENDPOINT_URL}
  auth_token: ${ACP_AUTH_TOKEN}
`)
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldwd) })
	t.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.ACP.EndpointURL != "" {
		t.Fatalf("ACP endpoint should be empty for unresolved placeholder, got %q", cfg.ACP.EndpointURL)
	}
	if cfg.ACP.AuthToken != "" {
		t.Fatalf("ACP auth token should be empty for unresolved placeholder, got %q", cfg.ACP.AuthToken)
	}
}
