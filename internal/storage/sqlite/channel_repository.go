package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/user/notification-hub/internal/domain"
)

type ChannelRepository struct {
	db *DB
}

func NewChannelRepository(db *DB) *ChannelRepository {
	return &ChannelRepository{db: db}
}

func (r *ChannelRepository) Create(ctx context.Context, ch *domain.Channel) error {
	ch.Normalize()
	config, err := toJSON(ch.Config)
	if err != nil {
		return err
	}
	rules, err := toJSON(ch.Rules)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO channels (
		id, platform, name, config, rules, ai_enabled, ai_prompt, is_active, is_default, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ch.ID, ch.Platform, ch.Name, config, rules, boolToInt(ch.AIEnabled), ch.AIPrompt,
		boolToInt(ch.IsActive), boolToInt(ch.IsDefault), formatTime(ch.CreatedAt), formatTime(ch.UpdatedAt))
	return err
}

func (r *ChannelRepository) Get(ctx context.Context, id string) (*domain.Channel, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, platform, name, config, rules, ai_enabled,
		ai_prompt, is_active, is_default, created_at, updated_at FROM channels WHERE id = ?`, id)
	return scanChannel(row)
}

func (r *ChannelRepository) List(ctx context.Context) ([]domain.Channel, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, platform, name, config, rules, ai_enabled,
		ai_prompt, is_active, is_default, created_at, updated_at FROM channels ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanChannels(rows)
}

func (r *ChannelRepository) ListActive(ctx context.Context) ([]domain.Channel, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, platform, name, config, rules, ai_enabled,
		ai_prompt, is_active, is_default, created_at, updated_at FROM channels WHERE is_active = 1 ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanChannels(rows)
}

func (r *ChannelRepository) ListByPlatforms(ctx context.Context, platforms []string) ([]domain.Channel, error) {
	if len(platforms) == 0 {
		return []domain.Channel{}, nil
	}
	placeholders := make([]string, len(platforms))
	args := make([]any, 0, len(platforms)+1)
	for i, platform := range platforms {
		placeholders[i] = "?"
		args = append(args, platform)
	}
	query := fmt.Sprintf(`SELECT id, platform, name, config, rules, ai_enabled, ai_prompt,
		is_active, is_default, created_at, updated_at FROM channels WHERE is_active = 1 AND platform IN (%s)
		ORDER BY created_at ASC`, strings.Join(placeholders, ","))
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanChannels(rows)
}

func (r *ChannelRepository) ListDefault(ctx context.Context) ([]domain.Channel, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, platform, name, config, rules, ai_enabled,
		ai_prompt, is_active, is_default, created_at, updated_at FROM channels WHERE is_active = 1 AND is_default = 1
		ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanChannels(rows)
}

func (r *ChannelRepository) Update(ctx context.Context, ch *domain.Channel) error {
	ch.Normalize()
	ch.UpdatedAt = time.Now().UTC()
	config, err := toJSON(ch.Config)
	if err != nil {
		return err
	}
	rules, err := toJSON(ch.Rules)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `UPDATE channels SET platform = ?, name = ?, config = ?,
		rules = ?, ai_enabled = ?, ai_prompt = ?, is_active = ?, is_default = ?, updated_at = ? WHERE id = ?`,
		ch.Platform, ch.Name, config, rules, boolToInt(ch.AIEnabled), ch.AIPrompt,
		boolToInt(ch.IsActive), boolToInt(ch.IsDefault), formatTime(ch.UpdatedAt), ch.ID)
	return err
}

func (r *ChannelRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM channels WHERE id = ?`, id)
	return err
}

func scanChannel(row scanner) (*domain.Channel, error) {
	var ch domain.Channel
	var config, rules, createdAt, updatedAt string
	var aiEnabled, isActive, isDefault int
	if err := row.Scan(&ch.ID, &ch.Platform, &ch.Name, &config, &rules, &aiEnabled,
		&ch.AIPrompt, &isActive, &isDefault, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	if err := fromJSON(config, &ch.Config); err != nil {
		return nil, err
	}
	if err := fromJSON(rules, &ch.Rules); err != nil {
		return nil, err
	}
	ch.AIEnabled = intToBool(aiEnabled)
	ch.IsActive = intToBool(isActive)
	ch.IsDefault = intToBool(isDefault)
	var err error
	ch.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	ch.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return nil, err
	}
	ch.Normalize()
	return &ch, nil
}

func scanChannels(rows *sql.Rows) ([]domain.Channel, error) {
	items := []domain.Channel{}
	for rows.Next() {
		ch, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *ch)
	}
	return items, rows.Err()
}
