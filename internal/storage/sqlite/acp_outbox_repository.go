package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/user/notification-hub/internal/domain"
)

type ACPOutboxRepository struct {
	db *DB
}

func NewACPOutboxRepository(db *DB) *ACPOutboxRepository {
	return &ACPOutboxRepository{db: db}
}

func (r *ACPOutboxRepository) Create(ctx context.Context, item *domain.ACPOutboxItem) error {
	item.Normalize()
	eventJSON := sql.NullString{}
	if item.Event != nil {
		data, err := json.Marshal(item.Event)
		if err != nil {
			return err
		}
		item.EventJSON = string(data)
		eventJSON = sql.NullString{String: item.EventJSON, Valid: true}
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO acp_outbox (
		id, message_id, event_json, raw_llm_output, status, skip_reason, error_message,
		dispatch_attempts, last_status_code, last_attempted_at, dispatched_at, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.MessageID, eventJSON, item.RawLLMOutput, item.Status, item.SkipReason,
		item.ErrorMessage, item.DispatchAttempts, nullableInt(item.LastStatusCode),
		nullableTime(item.LastAttemptedAt), nullableTime(item.DispatchedAt),
		formatTime(item.CreatedAt), formatTime(item.UpdatedAt))
	return err
}

func (r *ACPOutboxRepository) Get(ctx context.Context, id string) (*domain.ACPOutboxItem, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, message_id, event_json, raw_llm_output, status,
		skip_reason, error_message, dispatch_attempts, last_status_code, last_attempted_at,
		dispatched_at, created_at, updated_at FROM acp_outbox WHERE id = ?`, id)
	return scanACPOutboxItem(row)
}

func (r *ACPOutboxRepository) MarkSkipped(ctx context.Context, id string, reason string) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `UPDATE acp_outbox SET
		status = ?, skip_reason = ?, updated_at = ? WHERE id = ?`,
		domain.ACPOutboxStatusSkipped, reason, formatTime(now), id)
	return err
}

func (r *ACPOutboxRepository) MarkFailed(ctx context.Context, id string, message string, statusCode *int) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `UPDATE acp_outbox SET
		status = ?, error_message = ?, dispatch_attempts = dispatch_attempts + 1,
		last_status_code = ?, last_attempted_at = ?, updated_at = ? WHERE id = ?`,
		domain.ACPOutboxStatusFailed, message, nullableInt(statusCode), formatTime(now), formatTime(now), id)
	return err
}

func (r *ACPOutboxRepository) MarkDispatched(ctx context.Context, id string, statusCode int) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `UPDATE acp_outbox SET
		status = ?, error_message = '', dispatch_attempts = dispatch_attempts + 1,
		last_status_code = ?, last_attempted_at = ?, dispatched_at = ?, updated_at = ? WHERE id = ?`,
		domain.ACPOutboxStatusDispatched, statusCode, formatTime(now), formatTime(now), formatTime(now), id)
	return err
}

func scanACPOutboxItem(row scanner) (*domain.ACPOutboxItem, error) {
	var item domain.ACPOutboxItem
	var eventJSON sql.NullString
	var statusCode sql.NullInt64
	var lastAttemptedAt, dispatchedAt sql.NullString
	var createdAt, updatedAt string
	if err := row.Scan(&item.ID, &item.MessageID, &eventJSON, &item.RawLLMOutput, &item.Status,
		&item.SkipReason, &item.ErrorMessage, &item.DispatchAttempts, &statusCode, &lastAttemptedAt,
		&dispatchedAt, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	if eventJSON.Valid && eventJSON.String != "" {
		item.EventJSON = eventJSON.String
		var event domain.ACPEvent
		if err := json.Unmarshal([]byte(eventJSON.String), &event); err != nil {
			return nil, err
		}
		item.Event = &event
	}
	if statusCode.Valid {
		v := int(statusCode.Int64)
		item.LastStatusCode = &v
	}
	var err error
	item.LastAttemptedAt, err = parseNullableTime(lastAttemptedAt)
	if err != nil {
		return nil, err
	}
	item.DispatchedAt, err = parseNullableTime(dispatchedAt)
	if err != nil {
		return nil, err
	}
	item.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	item.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return nil, err
	}
	item.Normalize()
	return &item, nil
}

func nullableInt(v *int) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*v), Valid: true}
}
