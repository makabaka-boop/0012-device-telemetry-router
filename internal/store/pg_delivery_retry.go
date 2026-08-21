package store

import (
	"context"
	"database/sql"
	"time"

	"device-telemetry-router/internal/domain"
)

func (s *PGStore) ListDueDeliveries(ctx context.Context, now time.Time, limit int) ([]domain.DeliveryRecord, error) {
	return listDueDeliveries(ctx, s.db, now, limit)
}
func (t *txStore) ListDueDeliveries(ctx context.Context, now time.Time, limit int) ([]domain.DeliveryRecord, error) {
	return listDueDeliveries(ctx, t.tx, now, limit)
}

func listDueDeliveries(ctx context.Context, q queryer, now time.Time, limit int) ([]domain.DeliveryRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := q.QueryContext(ctx, `
		SELECT id, event_id, rule_id, topic, status, attempts, last_error, next_retry_at, dispatched_at
		FROM delivery_record
		WHERE status IN ('pending','failed') AND next_retry_at <= $1
		ORDER BY next_retry_at ASC, id ASC
		LIMIT $2`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDeliveries(rows)
}

func (s *PGStore) ListDeliveriesByEvent(ctx context.Context, eventID string) ([]domain.DeliveryRecord, error) {
	return listDeliveriesByEvent(ctx, s.db, eventID)
}
func (t *txStore) ListDeliveriesByEvent(ctx context.Context, eventID string) ([]domain.DeliveryRecord, error) {
	return listDeliveriesByEvent(ctx, t.tx, eventID)
}

func listDeliveriesByEvent(ctx context.Context, q queryer, eventID string) ([]domain.DeliveryRecord, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT id, event_id, rule_id, topic, status, attempts, last_error, next_retry_at, dispatched_at
		FROM delivery_record WHERE event_id = $1 ORDER BY id ASC`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDeliveries(rows)
}

func scanDeliveries(rows *sql.Rows) ([]domain.DeliveryRecord, error) {
	out := []domain.DeliveryRecord{}
	for rows.Next() {
		d := domain.DeliveryRecord{}
		var nextRetry, dispatched sql.NullTime
		if err := rows.Scan(&d.ID, &d.EventID, &d.RuleID, &d.Topic, &d.Status,
			&d.Attempts, &d.LastError, &nextRetry, &dispatched); err != nil {
			return nil, err
		}
		if nextRetry.Valid {
			t := nextRetry.Time
			d.NextRetryAt = &t
		}
		if dispatched.Valid {
			t := dispatched.Time
			d.DispatchedAt = &t
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *PGStore) MarkDelivered(ctx context.Context, id int64, at time.Time) error {
	return markDelivered(ctx, s.db, id, at)
}
func (t *txStore) MarkDelivered(ctx context.Context, id int64, at time.Time) error {
	return markDelivered(ctx, t.tx, id, at)
}

func markDelivered(ctx context.Context, q queryer, id int64, at time.Time) error {
	_, err := q.ExecContext(ctx, `
		UPDATE delivery_record SET status='dispatched', attempts=attempts+1, last_error='',
			next_retry_at=NULL, dispatched_at=$2 WHERE id=$1`, id, at)
	return err
}

func (s *PGStore) MarkFailed(ctx context.Context, id int64, attempt int, lastErr string, nextRetry time.Time) error {
	return markFailed(ctx, s.db, id, attempt, lastErr, nextRetry)
}
func (t *txStore) MarkFailed(ctx context.Context, id int64, attempt int, lastErr string, nextRetry time.Time) error {
	return markFailed(ctx, t.tx, id, attempt, lastErr, nextRetry)
}

func markFailed(ctx context.Context, q queryer, id int64, attempt int, lastErr string, nextRetry time.Time) error {
	_, err := q.ExecContext(ctx, `
		UPDATE delivery_record SET status='failed', attempts=$2, last_error=$3, next_retry_at=$4
		WHERE id=$1`, id, attempt, lastErr, nextRetry)
	return err
}

func (s *PGStore) MarkDead(ctx context.Context, id int64, lastErr string, at time.Time) error {
	return markDead(ctx, s.db, id, lastErr, at)
}
func (t *txStore) MarkDead(ctx context.Context, id int64, lastErr string, at time.Time) error {
	return markDead(ctx, t.tx, id, lastErr, at)
}

func markDead(ctx context.Context, q queryer, id int64, lastErr string, at time.Time) error {
	_, err := q.ExecContext(ctx, `
		UPDATE delivery_record SET status='dead_letter', last_error=$2, next_retry_at=NULL, dispatched_at=$3
		WHERE id=$1`, id, lastErr, at)
	return err
}
