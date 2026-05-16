package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/user/notification-hub/internal/domain"
)

type MessageRepository struct {
	db *DB
}

type MessageListOptions struct {
	Channel string
	Source  string
	Limit   int
	Offset  int
}

func NewMessageRepository(db *DB) *MessageRepository {
	return &MessageRepository{db: db}
}

func (r *MessageRepository) Create(ctx context.Context, msg *domain.Message) error {
	msg.Normalize()
	channels, err := toJSON(msg.Channels)
	if err != nil {
		return err
	}
	metadata, err := toJSON(msg.Metadata)
	if err != nil {
		return err
	}
	platformIDs, err := toJSON(msg.PlatformMessageIDs)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO messages (
		id, direction, content_original, content_processed, source, channels, status,
		priority, ai_processing, metadata, platform_message_ids, error_message, created_at, sent_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		msg.ID, msg.Direction, msg.ContentOriginal, msg.ContentProcessed, msg.Source,
		channels, msg.Status, msg.Priority, msg.AIProcessing, metadata, platformIDs,
		msg.ErrorMessage, formatTime(msg.CreatedAt), nullableTime(msg.SentAt))
	return err
}

func (r *MessageRepository) Get(ctx context.Context, id string) (*domain.Message, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, direction, content_original, content_processed,
		source, channels, status, priority, ai_processing, metadata, platform_message_ids,
		error_message, created_at, sent_at FROM messages WHERE id = ?`, id)
	return scanMessage(row)
}

func (r *MessageRepository) Update(ctx context.Context, msg *domain.Message) error {
	msg.Normalize()
	channels, err := toJSON(msg.Channels)
	if err != nil {
		return err
	}
	metadata, err := toJSON(msg.Metadata)
	if err != nil {
		return err
	}
	platformIDs, err := toJSON(msg.PlatformMessageIDs)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `UPDATE messages SET
		direction = ?, content_original = ?, content_processed = ?, source = ?, channels = ?,
		status = ?, priority = ?, ai_processing = ?, metadata = ?, platform_message_ids = ?,
		error_message = ?, sent_at = ? WHERE id = ?`,
		msg.Direction, msg.ContentOriginal, msg.ContentProcessed, msg.Source, channels,
		msg.Status, msg.Priority, msg.AIProcessing, metadata, platformIDs,
		msg.ErrorMessage, nullableTime(msg.SentAt), msg.ID)
	return err
}

func (r *MessageRepository) UpdateStatus(ctx context.Context, id string, status domain.Status, errMsg string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE messages SET status = ?, error_message = ? WHERE id = ?`, status, errMsg, id)
	return err
}

func (r *MessageRepository) MarkSent(ctx context.Context, msg *domain.Message) error {
	now := time.Now().UTC()
	msg.Status = domain.StatusSent
	msg.SentAt = &now
	return r.Update(ctx, msg)
}

func (r *MessageRepository) ListInbox(ctx context.Context, opts MessageListOptions) ([]domain.Message, error) {
	opts = normalizeMessageListOptions(opts)
	args := []any{domain.DirectionInbound}
	where := []string{"direction = ?"}
	if opts.Channel != "" {
		where = append(where, "(source = ? OR channels LIKE ?)")
		args = append(args, opts.Channel, `%`+quoteJSONFragment(opts.Channel)+`%`)
	}
	if opts.Source != "" {
		where = append(where, "source = ?")
		args = append(args, opts.Source)
	}
	args = append(args, opts.Limit, opts.Offset)
	query := fmt.Sprintf(`SELECT id, direction, content_original, content_processed,
		source, channels, status, priority, ai_processing, metadata, platform_message_ids,
		error_message, created_at, sent_at FROM messages WHERE %s ORDER BY created_at DESC LIMIT ? OFFSET ?`, strings.Join(where, " AND "))
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMessages(rows)
}

func (r *MessageRepository) List(ctx context.Context, opts MessageListOptions) ([]domain.Message, error) {
	opts = normalizeMessageListOptions(opts)
	args := []any{}
	where := []string{"1=1"}
	if opts.Channel != "" {
		where = append(where, "channels LIKE ?")
		args = append(args, `%`+quoteJSONFragment(opts.Channel)+`%`)
	}
	if opts.Source != "" {
		where = append(where, "source = ?")
		args = append(args, opts.Source)
	}
	args = append(args, opts.Limit, opts.Offset)
	query := fmt.Sprintf(`SELECT id, direction, content_original, content_processed,
		source, channels, status, priority, ai_processing, metadata, platform_message_ids,
		error_message, created_at, sent_at FROM messages WHERE %s ORDER BY created_at DESC LIMIT ? OFFSET ?`, strings.Join(where, " AND "))
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMessages(rows)
}

func normalizeMessageListOptions(opts MessageListOptions) MessageListOptions {
	if opts.Limit <= 0 || opts.Limit > 100 {
		opts.Limit = 20
	}
	if opts.Offset < 0 {
		opts.Offset = 0
	}
	return opts
}

type scanner interface {
	Scan(dest ...any) error
}

func scanMessage(row scanner) (*domain.Message, error) {
	var msg domain.Message
	var channels, metadata, platformIDs, createdAt string
	var sentAt sql.NullString
	if err := row.Scan(&msg.ID, &msg.Direction, &msg.ContentOriginal, &msg.ContentProcessed,
		&msg.Source, &channels, &msg.Status, &msg.Priority, &msg.AIProcessing, &metadata,
		&platformIDs, &msg.ErrorMessage, &createdAt, &sentAt); err != nil {
		return nil, err
	}
	if err := decodeMessageJSON(&msg, channels, metadata, platformIDs); err != nil {
		return nil, err
	}
	t, err := parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	msg.CreatedAt = t
	msg.SentAt, err = parseNullableTime(sentAt)
	if err != nil {
		return nil, err
	}
	msg.Normalize()
	return &msg, nil
}

func scanMessages(rows *sql.Rows) ([]domain.Message, error) {
	items := []domain.Message{}
	for rows.Next() {
		msg, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *msg)
	}
	return items, rows.Err()
}

func decodeMessageJSON(msg *domain.Message, channels, metadata, platformIDs string) error {
	if err := fromJSON(channels, &msg.Channels); err != nil {
		return err
	}
	if err := fromJSON(metadata, &msg.Metadata); err != nil {
		return err
	}
	return fromJSON(platformIDs, &msg.PlatformMessageIDs)
}

func quoteJSONFragment(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}
