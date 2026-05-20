# Notification Hub

Notification Hub is a Go + Gin + SQLite service for routing notifications to Telegram and Discord, collecting inbound messages into one inbox, and optionally applying OpenAI processing before messages are sent.

## Features

- REST API for outbound messages: `POST /api/v1/messages`
- Telegram Bot API outbound plus inbound long polling support
- Discord REST outbound plus Gateway websocket inbound support
- SQLite single-file storage for messages, channels, and webhook configs
- Rule-based routing with priority: request channels, then rules, then default channels
- OpenAI processor abstraction with `none`, `summarize`, `translate`, and `custom`
- AI failures fall back to the original message content
- AES-GCM encrypted storage for sensitive channel config and webhook secrets
- HMAC-SHA256 signed webhooks using `X-Notification-Signature`
- Unified inbox API: `GET /api/v1/messages/inbox`

## Configuration

Environment variables:

```env
HTTP_ADDR=127.0.0.1:18080
DATABASE_PATH=./data/notification-hub.db
ENCRYPTION_KEY=change-me-32-bytes-minimum-secret
OPENAI_API_KEY=
OPENAI_MODEL=gpt-4o-mini
OPENAI_BASE_URL=https://api.openai.com/v1
OPENAI_TIMEOUT=30s
ACP_ENDPOINT_URL=
ACP_AUTH_TOKEN=
```

Set `log_inbound_messages: true` in `config.yaml` to print incoming Telegram/Discord messages in the terminal where the service is running.

`ENCRYPTION_KEY` must be 16, 24, or 32 bytes, or base64 for one of those lengths.

The example `config.yaml` binds to `127.0.0.1:18080` so it works on machines without Tailscale. To expose the service over Tailscale, set `http_addr` or `HTTP_ADDR` to your `100.x.y.z:18080` address. Use `:18080` only when you want to listen on all interfaces.

## Run Locally

```bash
go mod download
ENCRYPTION_KEY=0123456789abcdef0123456789abcdef go run ./cmd/notification-hub
```

Health check:

```bash
curl http://127.0.0.1:18080/health
```

## Release Binaries

Tagged releases publish prebuilt archives for common platforms:

- macOS Apple Silicon: `notification-hub-<version>-darwin-arm64.tar.gz`
- macOS Intel: `notification-hub-<version>-darwin-amd64.tar.gz`
- Linux AMD64: `notification-hub-<version>-linux-amd64.tar.gz`
- Linux ARM64: `notification-hub-<version>-linux-arm64.tar.gz`
- Windows AMD64: `notification-hub-<version>-windows-amd64.zip`

Download the archive for your platform, extract it, copy `notification-hub` or `notification-hub.exe` onto your `PATH`, and configure the required environment variables.

## Log File Output

By default, Notification Hub writes process logs to the terminal. Use `--log` to append logs to a file:

```bash
ENCRYPTION_KEY=0123456789abcdef0123456789abcdef \
  notification-hub --log ./notification-hub.log
```

The log file must be in an existing directory. When `log_inbound_messages: true` is enabled in `config.yaml`, inbound Telegram and Discord message logs are written to the same target.

## Service Deployment

The `deploy/` directory contains templates for running Notification Hub as a host service. Service deployments need the same environment variables as local runs: `HTTP_ADDR`, `DATABASE_PATH`, `ENCRYPTION_KEY`, bot tokens, OpenAI settings, and optional ACP settings.

### Linux systemd

1. Copy the release binary to `/usr/local/bin/notification-hub`.
2. Install the service files:

```bash
sudo sh deploy/systemd/install.sh
```

3. Edit `/etc/notification-hub/notification-hub.env` and set real secrets.
4. Start the service:

```bash
sudo systemctl start notification-hub
sudo systemctl status notification-hub
```

The default database path is `/var/lib/notification-hub/notification-hub.db`. The default application log path is `/var/log/notification-hub/notification-hub.log`.

### macOS launchd

1. Copy the release binary to `/usr/local/bin/notification-hub`.
2. Install the launchd files:

```bash
sudo sh deploy/launchd/install.sh
```

3. Edit `/usr/local/etc/notification-hub/notification-hub.env` and set real secrets.
4. Restart the service:

```bash
sudo launchctl kickstart -k system/com.notification-hub
sudo launchctl print system/com.notification-hub
```

The default database path is `/usr/local/var/notification-hub/notification-hub.db`. The default application log path is `/usr/local/var/log/notification-hub.log`.

### Windows service

Use the WinSW example in `deploy/windows/`. The Windows service runs:

```powershell
notification-hub.exe --log logs\notification-hub.log
```

See `deploy/windows/README.md` for installation and uninstall commands.

## Docker

```bash
export ENCRYPTION_KEY=0123456789abcdef0123456789abcdef
docker compose up --build
```

SQLite data is stored in the `notification-data` volume.

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
POST /api/v1/channels       -> 501 channels are managed by config
PUT /api/v1/channels/:id    -> 501 channels are managed by config
DELETE /api/v1/channels/:id -> 501 channels are managed by config
```

You can still inspect loaded channels. Secrets are masked:

```bash
curl http://127.0.0.1:18080/api/v1/channels
```

## Quick Send

Use the shortcut endpoint when you just want to send a notification fast:

```bash
curl -X POST http://127.0.0.1:18080/api/v1/notify \
  -H 'Content-Type: application/json' \
  -d '{
    "text": "GSC traffic dropped 30%"
  }'
```

By default, `/api/v1/notify` sends to Discord. Choose a target with `to`:

```bash
curl -X POST http://127.0.0.1:18080/api/v1/notify \
  -H 'Content-Type: application/json' \
  -d '{
    "text": "Indexing alert: sitemap fetch failed",
    "to": "telegram"
  }'
```

Send to every active configured channel:

```bash
curl -X POST http://127.0.0.1:18080/api/v1/notify \
  -H 'Content-Type: application/json' \
  -d '{
    "text": "Daily notification smoke test",
    "to": "all"
  }'
```

Send to specific channels:

```bash
curl -X POST http://127.0.0.1:18080/api/v1/notify \
  -H 'Content-Type: application/json' \
  -d '{
    "text": "Urgent SEO alert",
    "to": ["telegram", "discord"]
  }'
```

If the service is running on another host, replace `127.0.0.1:18080` with your configured `HTTP_ADDR`, for example:

```bash
curl -X POST http://100.89.0.100:18080/api/v1/notify \
  -H 'Content-Type: application/json' \
  -d '{"text":"hello from API","to":"all"}'
```

Use `/api/v1/messages` when you need priority, metadata, or AI processing.

## Send Messages

```bash
curl -X POST http://127.0.0.1:18080/api/v1/messages \
  -H 'Content-Type: application/json' \
  -d '{
    "content": "GSC traffic dropped 30%",
    "channels": ["telegram", "discord"],
    "priority": "high",
    "ai_processing": "summarize",
    "metadata": {
      "source": "seoagent",
      "alert_type": "gsc_traffic_drop"
    }
  }'
```

If `channels` is empty, the service applies rules and then falls back to channels marked `is_default`.

## ACP Forwarding

ACP forwarding can analyze inbound Telegram/Discord messages into a stable internal event and POST approved events to a generic HTTP JSON endpoint. It is disabled by default.

```yaml
acp:
  enabled: false
  endpoint_url: ${ACP_ENDPOINT_URL}
  auth_token: ${ACP_AUTH_TOKEN}
  default_project: notification
  default_agent: triage
  min_confidence: 0.8
  allowed_intents: ["docs_request", "incident", "support_request"]
```

Environment variables `ACP_ENDPOINT_URL`, `ACP_AUTH_TOKEN`, `ACP_ENABLED`, `ACP_DEFAULT_PROJECT`, `ACP_DEFAULT_AGENT`, `ACP_MIN_CONFIDENCE`, and `ACP_ALLOWED_INTENTS` override YAML values. `ACP_ALLOWED_INTENTS` is a comma-separated list; an empty list means no intent restriction.

Forwarded event JSON:

```json
{
  "version": "2026-05-17",
  "event_type": "channel.inbound.analyzed",
  "message_id": "msg_...",
  "source": {
    "platform": "telegram",
    "channel_id": "-100123",
    "author_id": "42",
    "author_name": "alice"
  },
  "routing": {
    "should_forward": true,
    "project": "notification",
    "agent": "triage",
    "priority": "normal",
    "confidence": 0.86
  },
  "analysis": {
    "intent": "docs_request",
    "summary": "README lacks quick API send examples.",
    "action": "Add curl examples to the README.",
    "entities": ["README", "API", "curl"],
    "language": "en"
  },
  "content": {
    "original": "README is missing quick API examples",
    "normalized": "README is missing quick API message sending examples."
  }
}
```

Inbound handling remains: save message, dispatch existing webhooks, attempt ACP forwarding, then auto-reply. Existing webhook failure semantics are unchanged. ACP analysis, validation, HTTP, or endpoint failures are recorded in the local `acp_outbox` table and do not block auto-reply.

Events are dispatched only when ACP is enabled, `endpoint_url` is configured, LLM JSON validates, `routing.should_forward` is true, confidence is at least `min_confidence`, and the intent is allowed when `allowed_intents` is configured. Rule misses are stored as `skipped`; malformed LLM output and HTTP failures are stored as `failed`.

Security boundary: the LLM only receives whitelisted inbound context: content, platform, channel id, author id, author name, and message id. The HTTP request body contains only the internal ACP event. Bot tokens, decrypted channel config, OpenAI keys, webhook secrets, encryption keys, and ACP auth tokens are not included in the event body. The ACP auth token is sent only as `Authorization: Bearer <token>`.

## Inbox

```bash
curl 'http://127.0.0.1:18080/api/v1/messages/inbox?channel=telegram&limit=20&offset=0'
```

## Webhooks

```bash
curl -X POST http://127.0.0.1:18080/api/v1/webhooks \
  -H 'Content-Type: application/json' \
  -d '{
    "url": "https://example.com/api/notifications",
    "events": ["inbound.telegram", "inbound.discord"],
    "secret": "whsec_test"
  }'
```

Webhook payloads include `event` and `message`. If a secret is configured, the request includes:

```text
X-Notification-Signature: sha256=<hex-hmac>
```

## Bot Setup Notes

Telegram:

1. Create a bot with BotFather.
2. Add the bot to the target chat or group.
3. Store `bot_token` and `chat_id` in a Telegram channel config.
4. Inbound support uses `getUpdates` long polling, not webhooks.

Discord:

1. Create a Discord application and bot.
2. Enable Message Content Intent for inbound message text.
3. Invite the bot to the server with send/read message permissions.
4. Store `bot_token` and `channel_id` in a Discord channel config.
5. Inbound support uses the Discord Gateway websocket.

## Development

```bash
go test ./...
go build ./cmd/notification-hub
```
