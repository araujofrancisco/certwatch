package repository

import (
	"context"

	"fmt"
	"time"

	"github.com/araujofrancisco/certwatch/internal/database"
)

// notificationDedupRepo persists notification dedup keys so alerts are not
// re-sent after a process restart within the dedup window.
type notificationDedupRepo struct {
	db *database.DB
}

func NewNotificationDedupRepository(db *database.DB) NotificationDedupRepository {
	return &notificationDedupRepo{db: db}
}

func (r *notificationDedupRepo) Seen(ctx context.Context, key string) (bool, error) {
	var count int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM notification_dedup WHERE key = ?`, key).Scan(&count); err != nil {
		return false, fmt.Errorf("dedup seen: %w", err)
	}
	return count > 0, nil
}

func (r *notificationDedupRepo) Mark(ctx context.Context, key string, at time.Time) error {
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO notification_dedup (key, notified_at) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET notified_at = excluded.notified_at`,
		key, at.UTC(),
	); err != nil {
		return fmt.Errorf("dedup mark: %w", err)
	}
	return nil
}

func (r *notificationDedupRepo) Cleanup(ctx context.Context, before time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM notification_dedup WHERE notified_at < ?`, before.UTC())
	if err != nil {
		return 0, fmt.Errorf("dedup cleanup: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("dedup cleanup rows: %w", err)
	}
	return n, nil
}
