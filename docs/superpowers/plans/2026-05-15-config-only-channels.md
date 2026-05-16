# Config-Only Channels Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Telegram and Discord channels come only from server-side config, so bot tokens never need to be sent through HTTP channel APIs or stored in SQLite.

**Architecture:** Add channel definitions to `config.Config`, parse them from `config.yaml`/`config.yml` and `.env` with `${ENV_VAR}` expansion, and expose them through a config-backed `service.ChannelStore`. The message pipeline, rule engine, default routing, channel listing API, and inbound listeners will use the config-backed store. Channel mutation APIs become read-only/disabled for v1.

**Tech Stack:** Go, Gin, SQLite, existing `internal/config`, `internal/domain`, `internal/service`, `internal/http/handlers`, Telegram/Discord adapters.

---

## File Structure

- Modify `internal/config/config.go`
  - Add `ChannelConfig` to `Config`.
  - Parse repeated channel blocks from config files.
  - Expand `${ENV_VAR}` values using real environment and `.env` values.
- Modify `internal/config/config_test.go`
  - Add config channel parsing tests.
  - Add `${ENV_VAR}` expansion test.
- Create `internal/service/config_channel_store.go`
  - Implement `service.ChannelStore` using in-memory `[]domain.Channel` loaded from config.
  - Return masked or full config depending on caller. The store itself keeps full config for pipeline/listeners.
  - Mutation methods return `ErrConfigOnlyChannels`.
- Modify `internal/service/ports.go`
  - Add exported `ErrConfigOnlyChannels` error if not already present.
- Create `internal/service/config_channel_store_test.go`
  - Test list, active list, default list, platform lookup, and disabled mutation behavior.
- Modify `internal/http/handlers/channels.go`
  - `GET /channels` and `GET /channels/:id` return masked config channels.
  - `POST/PUT/DELETE /channels` return HTTP 501 with clear message.
- Modify `internal/http/handlers/channels_test.go`
  - Replace create/update/delete success expectations with config-only 501 expectations.
  - Add list/get masked config tests.
- Modify `cmd/notification-hub/main.go`
  - Build `ConfigChannelStore` from `cfg.Channels` and pass it everywhere instead of SQLite channel repository.
  - Remove channel encryption/decryption for config channels because tokens are not stored in SQLite.
  - Start inbound listeners from config channel store.
- Modify `config.yaml.example`
  - Add Telegram and Discord channel examples using `${TELEGRAM_BOT_TOKEN}` and `${DISCORD_BOT_TOKEN}`.
- Modify `.env.example`
  - Add `TELEGRAM_BOT_TOKEN=` and `DISCORD_BOT_TOKEN=` placeholders.
- Modify `README.md`
  - Document config-only channels and remove token-through-API as the primary setup path.

---

### Task 1: Parse config-only channels from config files

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write failing test for `config.yml` channel parsing**

Add this test to `internal/config/config_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/config -run TestLoadParsesChannelsFromConfigYML -count=1
```

Expected: FAIL because `Config` has no `Channels` field or the parser ignores `channels:`.

- [ ] **Step 3: Add config channel structs**

Modify `internal/config/config.go` imports to include no new packages yet. Add these types after `OpenAIConfig`:

```go
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
```

Add this field to `Config`:

```go
Channels []ChannelConfig
```

- [ ] **Step 4: Implement minimal channel YAML parsing**

Modify `loadYAML` in `internal/config/config.go` so it reads all lines first and parses the existing flat keys plus `channels:`. Replace the current scanner loop with this shape:

```go
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
		case "openai_base_url":
			cfg.OpenAI.BaseURL = value
		case "openai_timeout":
			d, err := time.ParseDuration(value)
			if err != nil {
				return err
			}
			cfg.OpenAI.Timeout = d
		}
	}
	return nil
```

Add helpers below `loadYAML`:

```go
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
```

- [ ] **Step 5: Run test to verify it passes**

Run:

```bash
go test ./internal/config -run TestLoadParsesChannelsFromConfigYML -count=1
```

Expected: PASS.

---

### Task 2: Expand `${ENV_VAR}` values in channel config

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write failing env expansion test**

Add this test to `internal/config/config_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/config -run TestLoadExpandsChannelConfigEnvValues -count=1
```

Expected: FAIL with literal `${TELEGRAM_BOT_TOKEN}` returned.

- [ ] **Step 3: Implement expansion using `.env` and real env**

In `internal/config/config.go`, after `loadYAML(cfg.ConfigPath, &cfg)` succeeds, expand channels before applying scalar env overrides:

```go
	expandChannelConfigValues(cfg.Channels, dotenv)
```

Add helpers below `parseStringList`:

```go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run:

```bash
go test ./internal/config -run TestLoadExpandsChannelConfigEnvValues -count=1
```

Expected: PASS.

---

### Task 3: Add config-backed channel store

**Files:**
- Create: `internal/service/config_channel_store.go`
- Create: `internal/service/config_channel_store_test.go`
- Modify: `internal/service/ports.go`

- [ ] **Step 1: Inspect existing `ChannelStore` interface**

Read `internal/service/ports.go`. Confirm `ChannelStore` includes:

```go
Create(ctx context.Context, ch *domain.Channel) error
Get(ctx context.Context, id string) (*domain.Channel, error)
List(ctx context.Context) ([]domain.Channel, error)
ListActive(ctx context.Context) ([]domain.Channel, error)
ListByPlatforms(ctx context.Context, platforms []string) ([]domain.Channel, error)
ListDefault(ctx context.Context) ([]domain.Channel, error)
Update(ctx context.Context, ch *domain.Channel) error
Delete(ctx context.Context, id string) error
```

- [ ] **Step 2: Write failing store tests**

Create `internal/service/config_channel_store_test.go`:

```go
package service

import (
	"context"
	"errors"
	"testing"

	"github.com/user/notification-hub/internal/domain"
)

func TestConfigChannelStoreListsAndFindsChannels(t *testing.T) {
	store := NewConfigChannelStore([]domain.Channel{
		{
			ID:        "telegram-main",
			Platform:  domain.PlatformTelegram,
			Name:      "telegram-main",
			Config:    map[string]any{"bot_token": "telegram-token", "chat_id": "-100"},
			IsActive:  true,
			IsDefault: true,
		},
		{
			ID:       "discord-main",
			Platform: domain.PlatformDiscord,
			Name:     "discord-main",
			Config:   map[string]any{"bot_token": "discord-token", "channel_id": "123"},
			IsActive: true,
		},
	})

	all, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("List length = %d", len(all))
	}

	defaults, err := store.ListDefault(context.Background())
	if err != nil {
		t.Fatalf("ListDefault: %v", err)
	}
	if len(defaults) != 1 || defaults[0].Platform != domain.PlatformTelegram {
		t.Fatalf("unexpected defaults: %+v", defaults)
	}

	telegram, err := store.ListByPlatforms(context.Background(), []string{"telegram"})
	if err != nil {
		t.Fatalf("ListByPlatforms: %v", err)
	}
	if len(telegram) != 1 || telegram[0].Config["bot_token"] != "telegram-token" {
		t.Fatalf("unexpected telegram channels: %+v", telegram)
	}

	got, err := store.Get(context.Background(), "discord-main")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Platform != domain.PlatformDiscord {
		t.Fatalf("got platform = %s", got.Platform)
	}
}

func TestConfigChannelStoreRejectsMutations(t *testing.T) {
	store := NewConfigChannelStore(nil)
	if err := store.Create(context.Background(), domain.NewChannel(domain.PlatformTelegram, "x")); !errors.Is(err, ErrConfigOnlyChannels) {
		t.Fatalf("Create error = %v", err)
	}
	if err := store.Update(context.Background(), domain.NewChannel(domain.PlatformTelegram, "x")); !errors.Is(err, ErrConfigOnlyChannels) {
		t.Fatalf("Update error = %v", err)
	}
	if err := store.Delete(context.Background(), "x"); !errors.Is(err, ErrConfigOnlyChannels) {
		t.Fatalf("Delete error = %v", err)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run:

```bash
go test ./internal/service -run 'TestConfigChannelStore' -count=1
```

Expected: FAIL because `NewConfigChannelStore` and `ErrConfigOnlyChannels` are undefined.

- [ ] **Step 4: Implement store**

Add to `internal/service/ports.go` imports if needed:

```go
import "errors"
```

Add:

```go
var ErrConfigOnlyChannels = errors.New("channels are managed by config")
```

Create `internal/service/config_channel_store.go`:

```go
package service

import (
	"context"
	"database/sql"

	"github.com/user/notification-hub/internal/domain"
)

type ConfigChannelStore struct {
	channels []domain.Channel
}

func NewConfigChannelStore(channels []domain.Channel) *ConfigChannelStore {
	items := make([]domain.Channel, 0, len(channels))
	for _, ch := range channels {
		ch.Normalize()
		if ch.ID == "" {
			ch.ID = ch.Name
		}
		items = append(items, cloneChannel(ch))
	}
	return &ConfigChannelStore{channels: items}
}

func (s *ConfigChannelStore) Create(context.Context, *domain.Channel) error {
	return ErrConfigOnlyChannels
}

func (s *ConfigChannelStore) Get(_ context.Context, id string) (*domain.Channel, error) {
	for _, ch := range s.channels {
		if ch.ID == id || ch.Name == id {
			copy := cloneChannel(ch)
			return &copy, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (s *ConfigChannelStore) List(context.Context) ([]domain.Channel, error) {
	return cloneChannels(s.channels), nil
}

func (s *ConfigChannelStore) ListActive(context.Context) ([]domain.Channel, error) {
	out := []domain.Channel{}
	for _, ch := range s.channels {
		if ch.IsActive {
			out = append(out, cloneChannel(ch))
		}
	}
	return out, nil
}

func (s *ConfigChannelStore) ListByPlatforms(_ context.Context, platforms []string) ([]domain.Channel, error) {
	wanted := map[string]bool{}
	for _, platform := range platforms {
		wanted[platform] = true
	}
	out := []domain.Channel{}
	for _, ch := range s.channels {
		if ch.IsActive && wanted[string(ch.Platform)] {
			out = append(out, cloneChannel(ch))
		}
	}
	return out, nil
}

func (s *ConfigChannelStore) ListDefault(context.Context) ([]domain.Channel, error) {
	out := []domain.Channel{}
	for _, ch := range s.channels {
		if ch.IsActive && ch.IsDefault {
			out = append(out, cloneChannel(ch))
		}
	}
	return out, nil
}

func (s *ConfigChannelStore) Update(context.Context, *domain.Channel) error {
	return ErrConfigOnlyChannels
}

func (s *ConfigChannelStore) Delete(context.Context, string) error {
	return ErrConfigOnlyChannels
}

func cloneChannels(channels []domain.Channel) []domain.Channel {
	out := make([]domain.Channel, 0, len(channels))
	for _, ch := range channels {
		out = append(out, cloneChannel(ch))
	}
	return out
}

func cloneChannel(ch domain.Channel) domain.Channel {
	copy := ch
	copy.Config = map[string]any{}
	for key, value := range ch.Config {
		copy.Config[key] = value
	}
	copy.Rules = append([]domain.Rule{}, ch.Rules...)
	return copy
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run:

```bash
go test ./internal/service -run 'TestConfigChannelStore' -count=1
```

Expected: PASS.

---

### Task 4: Convert config channels to domain channels

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write failing conversion test**

Add this test to `internal/config/config_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/config -run TestConfigChannelsToDomainChannels -count=1
```

Expected: FAIL because `DomainChannels` is undefined.

- [ ] **Step 3: Implement conversion**

Add import for domain in `internal/config/config.go`:

```go
"github.com/user/notification-hub/internal/domain"
```

Add this method after `Load()`:

```go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run:

```bash
go test ./internal/config -run TestConfigChannelsToDomainChannels -count=1
```

Expected: PASS.

---

### Task 5: Wire config channel store into main

**Files:**
- Modify: `cmd/notification-hub/main.go`

- [ ] **Step 1: Write a compile-focused test command**

Run before implementation:

```bash
go test ./cmd/notification-hub ./internal/service ./internal/config
```

Expected: PASS before change. This is the baseline.

- [ ] **Step 2: Replace SQLite channel repository with config store**

In `cmd/notification-hub/main.go`, replace:

```go
	channels := sqlite.NewChannelRepository(db)
```

with:

```go
	channels := service.NewConfigChannelStore(cfg.DomainChannels())
```

Keep `messages := sqlite.NewMessageRepository(db)` and `webhooks := sqlite.NewWebhookRepository(db)` unchanged.

- [ ] **Step 3: Remove config channel decryption in listener startup**

Change function signature:

```go
func startInboundListeners(ctx context.Context, channels service.ChannelStore, inbound *service.InboundService) {
```

Change call site:

```go
	startInboundListeners(listenerCtx, channels, inbound)
```

Inside the loop, replace:

```go
		decrypted, err := cipher.DecryptConfig(channel.Config)
		if err != nil {
			log.Printf("decrypt %s channel %s: %v", channel.Platform, channel.ID, err)
			continue
		}
```

with:

```go
		decrypted := channel.Config
```

Do not pass `cipher` to `startInboundListeners`; config tokens are not encrypted in SQLite because they are not stored in SQLite.

- [ ] **Step 4: Run compile tests**

Run:

```bash
go test ./cmd/notification-hub ./internal/service ./internal/config
```

Expected: PASS.

---

### Task 6: Make channel API read-only and masked

**Files:**
- Modify: `internal/http/handlers/channels.go`
- Modify: `internal/http/handlers/channels_test.go`

- [ ] **Step 1: Write failing test for disabled channel mutations**

Add this test to `internal/http/handlers/channels_test.go`:

```go
func TestChannelMutationsReturnConfigOnly(t *testing.T) {
	router, _ := newTestRouter(t)

	for _, tc := range []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPost, path: "/api/v1/channels", body: `{"platform":"telegram","name":"x"}`},
		{method: http.MethodPut, path: "/api/v1/channels/x", body: `{"platform":"telegram","name":"x"}`},
		{method: http.MethodDelete, path: "/api/v1/channels/x", body: ``},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		if tc.body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != http.StatusNotImplemented {
			t.Fatalf("%s %s status = %d body=%s", tc.method, tc.path, res.Code, res.Body.String())
		}
	}
}
```

If `channels_test.go` does not import `strings`, add it.

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/http/handlers -run TestChannelMutationsReturnConfigOnly -count=1
```

Expected: FAIL because current POST/PUT/DELETE still mutate channels.

- [ ] **Step 3: Implement 501 responses**

In `internal/http/handlers/channels.go`, replace `Create`, `Update`, and `Delete` bodies with:

```go
func (h *ChannelHandler) Create(c *gin.Context) {
	respondError(c, http.StatusNotImplemented, "channels are managed by config")
}
```

```go
func (h *ChannelHandler) Update(c *gin.Context) {
	respondError(c, http.StatusNotImplemented, "channels are managed by config")
}
```

```go
func (h *ChannelHandler) Delete(c *gin.Context) {
	respondError(c, http.StatusNotImplemented, "channels are managed by config")
}
```

Leave `List`, `Get`, `maskChannel`, and `channelFromRequest` for now. `channelFromRequest` becomes unused; remove it after tests pass if Go reports no usage issue.

- [ ] **Step 4: Run channel handler tests**

Run:

```bash
go test ./internal/http/handlers -run 'TestChannel' -count=1
```

Expected: PASS or expose old tests that need update.

- [ ] **Step 5: Update old channel mutation tests**

If existing tests expect POST/PUT/DELETE success, update them to expect `http.StatusNotImplemented` and response body containing `channels are managed by config`.

- [ ] **Step 6: Run all handler tests**

Run:

```bash
go test ./internal/http/handlers
```

Expected: PASS.

---

### Task 7: Use config store in handler tests

**Files:**
- Modify: `internal/http/handlers/test_setup_test.go`
- Modify: `internal/http/handlers/channels_test.go`

- [ ] **Step 1: Inspect current test setup**

Read `internal/http/handlers/test_setup_test.go`. Identify where `sqlite.NewChannelRepository(db)` is used.

- [ ] **Step 2: Replace test channel store with config store**

In `newTestRouter`, replace the channel repository with:

```go
channels := service.NewConfigChannelStore([]domain.Channel{
	{
		ID:        "telegram-main",
		Platform:  domain.PlatformTelegram,
		Name:      "telegram-main",
		Config:    map[string]any{"bot_token": "telegram-token", "chat_id": "-100"},
		IsActive:  true,
		IsDefault: true,
	},
})
```

Add `domain` import if needed.

- [ ] **Step 3: Add list/get masking assertions**

Add this test to `internal/http/handlers/channels_test.go`:

```go
func TestChannelListMasksConfigChannels(t *testing.T) {
	router, _ := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/channels", nil)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", res.Code, res.Body.String())
	}
	if strings.Contains(res.Body.String(), "telegram-token") {
		t.Fatalf("response leaked token: %s", res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "***") {
		t.Fatalf("response did not contain masked config: %s", res.Body.String())
	}
}
```

- [ ] **Step 4: Run handler tests**

Run:

```bash
go test ./internal/http/handlers
```

Expected: PASS.

---

### Task 8: Update config templates and docs

**Files:**
- Modify: `config.yaml.example`
- Modify: `.env.example`
- Modify: `README.md`

- [ ] **Step 1: Update `config.yaml.example`**

Replace `config.yaml.example` content with:

```yaml
# Notification Hub configuration
# Copy this file to config.yaml or config.yml, then adjust values.
# Secrets should come from environment variables or .env, not from HTTP APIs.

http_addr: 100.89.0.100:18080
database_path: ./data/notification-hub.db
encryption_key: ${ENCRYPTION_KEY}
openai_api_key: ${OPENAI_API_KEY}
openai_model: google/gemma-4-26b-a4b-it:free
openai_base_url: https://openrouter.ai/api/v1
openai_timeout: 120s

channels:
  - platform: telegram
    name: telegram-main
    default: true
    active: true
    ai_enabled: false
    ai_prompt: ""
    config:
      bot_token: ${TELEGRAM_BOT_TOKEN}
      chat_id: "${TELEGRAM_CHAT_ID}"

  - platform: discord
    name: discord-main
    default: false
    active: true
    ai_enabled: false
    ai_prompt: ""
    config:
      bot_token: ${DISCORD_BOT_TOKEN}
      channel_id: "${DISCORD_CHANNEL_ID}"
    rules:
      - type: keyword
        pattern: "urgent|紧急"
        channels: ["discord"]
```

- [ ] **Step 2: Update `.env.example`**

Append these lines:

```env
TELEGRAM_BOT_TOKEN=
TELEGRAM_CHAT_ID=
DISCORD_BOT_TOKEN=
DISCORD_CHANNEL_ID=
```

- [ ] **Step 3: Update README channel setup section**

Replace the `## Configure Channels` section in `README.md` with:

```markdown
## Configure Channels

Channels are config-only in the first version. Do not send bot tokens through the HTTP API.

Copy the config template:

```bash
cp config.yaml.example config.yaml
```

Put secrets in `.env`:

```env
ENCRYPTION_KEY=0123456789abcdef0123456789abcdef
TELEGRAM_BOT_TOKEN=123456:telegram-token
TELEGRAM_CHAT_ID=-1001234567890
DISCORD_BOT_TOKEN=discord-token
DISCORD_CHANNEL_ID=123456789012345678
```

Reference those secrets from `config.yaml`:

```yaml
channels:
  - platform: telegram
    name: telegram-main
    default: true
    active: true
    config:
      bot_token: ${TELEGRAM_BOT_TOKEN}
      chat_id: "${TELEGRAM_CHAT_ID}"

  - platform: discord
    name: discord-main
    active: true
    config:
      bot_token: ${DISCORD_BOT_TOKEN}
      channel_id: "${DISCORD_CHANNEL_ID}"
```

Channel mutation endpoints are disabled in this version:

```text
POST /api/v1/channels    -> 501 channels are managed by config
PUT /api/v1/channels/:id -> 501 channels are managed by config
DELETE /api/v1/channels/:id -> 501 channels are managed by config
```

You can still inspect loaded channels. Secrets are masked:

```bash
curl http://127.0.0.1:8080/api/v1/channels
```
```

- [ ] **Step 4: Run doc-adjacent smoke check**

Run:

```bash
go test ./...
go build -o notification-hub ./cmd/notification-hub
```

Expected: PASS and successful build.

---

### Task 9: End-to-end local smoke test

**Files:**
- No source changes unless this test finds a bug.

- [ ] **Step 1: Create local config with fake tokens**

Use a temp directory so real user config is not overwritten:

```bash
TMPDIR=$(mktemp -d)
cp notification-hub "$TMPDIR/"
cat > "$TMPDIR/.env" <<'EOF'
ENCRYPTION_KEY=0123456789abcdef0123456789abcdef
TELEGRAM_BOT_TOKEN=fake-telegram-token
TELEGRAM_CHAT_ID=-100
DISCORD_BOT_TOKEN=fake-discord-token
DISCORD_CHANNEL_ID=123
EOF
cat > "$TMPDIR/config.yml" <<'EOF'
http_addr: 127.0.0.1:18089
database_path: ./notification-hub.db
channels:
  - platform: telegram
    name: telegram-main
    default: true
    active: true
    config:
      bot_token: ${TELEGRAM_BOT_TOKEN}
      chat_id: "${TELEGRAM_CHAT_ID}"
EOF
```

- [ ] **Step 2: Start service**

Run:

```bash
(cd "$TMPDIR" && ./notification-hub > server.log 2>&1 & echo $! > server.pid)
```

Expected: service starts and listens on `127.0.0.1:18089`.

- [ ] **Step 3: Verify channels list is masked**

Run:

```bash
curl -s http://127.0.0.1:18089/api/v1/channels
```

Expected: response includes `telegram-main`, includes masked secret values, and does not include `fake-telegram-token`.

- [ ] **Step 4: Verify channel mutation is disabled**

Run:

```bash
curl -s -o /tmp/channel-create.out -w '%{http_code}' -X POST http://127.0.0.1:18089/api/v1/channels \
  -H 'Content-Type: application/json' \
  -d '{"platform":"telegram","name":"x"}'
```

Expected: `501`.

- [ ] **Step 5: Stop service**

Run:

```bash
kill $(cat "$TMPDIR/server.pid")
```

Expected: process stops.

---

## Self-Review

**Spec coverage:**
- Config-only channels: Tasks 1, 3, 4, 5.
- No token through API: Task 6 disables mutation endpoints, Task 8 updates docs.
- No token stored in SQLite: Task 5 wires config store instead of SQLite channel repository.
- GET/list returns masked config channels: Tasks 6 and 7.
- Sending/routing/default channel selection use config store: Tasks 3 and 5.
- Inbound listeners use config channels: Task 5.
- `${ENV_VAR}` expansion: Task 2.
- Config template: Task 8.

**Placeholder scan:** No TBD/TODO placeholders. Every code step includes concrete code or exact replacement instructions.

**Type consistency:** `config.ChannelConfig` converts to `domain.Channel`; `service.ConfigChannelStore` implements existing `service.ChannelStore`; `cmd/notification-hub/main.go` passes `service.ChannelStore` to pipeline, handlers, and listeners.
