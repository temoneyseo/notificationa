package sqlite

import (
	"context"
	"database/sql"
)

func Migrate(ctx context.Context, db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS messages (
			id TEXT PRIMARY KEY,
			direction TEXT NOT NULL,
			content_original TEXT NOT NULL,
			content_processed TEXT NOT NULL,
			source TEXT NOT NULL,
			channels TEXT NOT NULL,
			status TEXT NOT NULL,
			priority TEXT NOT NULL,
			ai_processing TEXT NOT NULL,
			metadata TEXT NOT NULL,
			platform_message_ids TEXT NOT NULL,
			error_message TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			sent_at TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_direction_created_at ON messages(direction, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_source_created_at ON messages(source, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS channels (
			id TEXT PRIMARY KEY,
			platform TEXT NOT NULL,
			name TEXT NOT NULL,
			config TEXT NOT NULL,
			rules TEXT NOT NULL,
			ai_enabled INTEGER NOT NULL DEFAULT 0,
			ai_prompt TEXT NOT NULL DEFAULT '',
			is_active INTEGER NOT NULL DEFAULT 1,
			is_default INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_channels_platform_active ON channels(platform, is_active)`,
		`CREATE TABLE IF NOT EXISTS webhook_configs (
			id TEXT PRIMARY KEY,
			url TEXT NOT NULL,
			events TEXT NOT NULL,
			secret TEXT NOT NULL DEFAULT '',
			is_active INTEGER NOT NULL DEFAULT 1,
			last_triggered_at TEXT,
			last_error TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_webhook_configs_active ON webhook_configs(is_active)`,
	}
	for _, stmt := range statements {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}
