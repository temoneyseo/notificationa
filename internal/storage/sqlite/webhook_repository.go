package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/user/notification-hub/internal/domain"
)

type WebhookRepository struct {
	db *DB
}

func NewWebhookRepository(db *DB) *WebhookRepository {
	return &WebhookRepository{db: db}
}

func (r *WebhookRepository) Create(ctx context.Context, hook *domain.WebhookConfig) error {
	hook.Normalize()
	events, err := toJSON(hook.Events)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO webhook_configs (
		id, url, events, secret, is_active, last_triggered_at, last_error, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		hook.ID, hook.URL, events, hook.Secret, boolToInt(hook.IsActive),
		nullableTime(hook.LastTriggeredAt), hook.LastError, formatTime(hook.CreatedAt), formatTime(hook.UpdatedAt))
	return err
}

func (r *WebhookRepository) Get(ctx context.Context, id string) (*domain.WebhookConfig, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, url, events, secret, is_active, last_triggered_at,
		last_error, created_at, updated_at FROM webhook_configs WHERE id = ?`, id)
	return scanWebhook(row)
}

func (r *WebhookRepository) List(ctx context.Context) ([]domain.WebhookConfig, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, url, events, secret, is_active, last_triggered_at,
		last_error, created_at, updated_at FROM webhook_configs ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanWebhooks(rows)
}

func (r *WebhookRepository) ListActive(ctx context.Context) ([]domain.WebhookConfig, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, url, events, secret, is_active, last_triggered_at,
		last_error, created_at, updated_at FROM webhook_configs WHERE is_active = 1 ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanWebhooks(rows)
}

func (r *WebhookRepository) Update(ctx context.Context, hook *domain.WebhookConfig) error {
	hook.Normalize()
	hook.UpdatedAt = time.Now().UTC()
	events, err := toJSON(hook.Events)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `UPDATE webhook_configs SET url = ?, events = ?, secret = ?,
		is_active = ?, last_triggered_at = ?, last_error = ?, updated_at = ? WHERE id = ?`,
		hook.URL, events, hook.Secret, boolToInt(hook.IsActive), nullableTime(hook.LastTriggeredAt),
		hook.LastError, formatTime(hook.UpdatedAt), hook.ID)
	return err
}

func (r *WebhookRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM webhook_configs WHERE id = ?`, id)
	return err
}

func (r *WebhookRepository) MarkTriggered(ctx context.Context, id string) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `UPDATE webhook_configs SET last_triggered_at = ?, last_error = '', updated_at = ? WHERE id = ?`,
		formatTime(now), formatTime(now), id)
	return err
}

func (r *WebhookRepository) MarkFailed(ctx context.Context, id, message string) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `UPDATE webhook_configs SET last_error = ?, updated_at = ? WHERE id = ?`,
		message, formatTime(now), id)
	return err
}

func scanWebhook(row scanner) (*domain.WebhookConfig, error) {
	var hook domain.WebhookConfig
	var events, createdAt, updatedAt string
	var active int
	var lastTriggered sql.NullString
	if err := row.Scan(&hook.ID, &hook.URL, &events, &hook.Secret, &active, &lastTriggered,
		&hook.LastError, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	if err := fromJSON(events, &hook.Events); err != nil {
		return nil, err
	}
	hook.IsActive = intToBool(active)
	var err error
	hook.LastTriggeredAt, err = parseNullableTime(lastTriggered)
	if err != nil {
		return nil, err
	}
	hook.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	hook.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return nil, err
	}
	hook.Normalize()
	return &hook, nil
}

func scanWebhooks(rows *sql.Rows) ([]domain.WebhookConfig, error) {
	items := []domain.WebhookConfig{}
	for rows.Next() {
		hook, err := scanWebhook(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *hook)
	}
	return items, rows.Err()
}
