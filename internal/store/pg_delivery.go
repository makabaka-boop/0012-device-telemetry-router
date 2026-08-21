package store

import (
	"context"
	"database/sql"

	"device-telemetry-router/internal/domain"
)

func (s *PGStore) CreateDelivery(ctx context.Context, d domain.DeliveryRecord) error {
	return createDelivery(ctx, s.db, d)
}
func (t *txStore) CreateDelivery(ctx context.Context, d domain.DeliveryRecord) error {
	return createDelivery(ctx, t.tx, d)
}

func createDelivery(ctx context.Context, q queryer, d domain.DeliveryRecord) error {
	_, err := q.ExecContext(ctx, `
		INSERT INTO delivery_record (event_id, rule_id, topic, status, attempts, last_error, next_retry_at, dispatched_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		d.EventID, d.RuleID, d.Topic, string(d.Status), d.Attempts,
		d.LastError, d.NextRetryAt, d.DispatchedAt)
	return err
}

func (s *PGStore) PendingCount(ctx context.Context) (int64, error) {
	return pendingCount(ctx, s.db)
}
func (t *txStore) PendingCount(ctx context.Context) (int64, error) {
	return pendingCount(ctx, t.tx)
}

func pendingCount(ctx context.Context, q queryer) (int64, error) {
	row := q.QueryRowContext(ctx,
		`SELECT count(*) FROM delivery_record WHERE status = 'pending'`)
	var n int64
	if err := row.Scan(&n); err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	return n, nil
}
