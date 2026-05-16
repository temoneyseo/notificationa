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
HTTP_ADDR=:8080
DATABASE_PATH=./data/notification-hub.db
ENCRYPTION_KEY=change-me-32-bytes-minimum-secret
OPENAI_API_KEY=
OPENAI_MODEL=gpt-4o-mini
OPENAI_BASE_URL=https://api.openai.com/v1
OPENAI_TIMEOUT=30s
```

Set `log_inbound_messages: true` in `config.yaml` to print incoming Telegram/Discord messages in the terminal where the service is running.

`ENCRYPTION_KEY` must be 16, 24, or 32 bytes, or base64 for one of those lengths.

## Run Locally

```bash
go mod download
ENCRYPTION_KEY=0123456789abcdef0123456789abcdef go run ./cmd/notification-hub
```

Health check:

```bash
curl http://127.0.0.1:8080/health
```

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
curl http://127.0.0.1:8080/api/v1/channels
```

## Send Messages

```bash
curl -X POST http://127.0.0.1:8080/api/v1/messages \
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

## Inbox

```bash
curl 'http://127.0.0.1:8080/api/v1/messages/inbox?channel=telegram&limit=20&offset=0'
```

## Webhooks

```bash
curl -X POST http://127.0.0.1:8080/api/v1/webhooks \
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
