package store

import (
	"context"
	"database/sql"
	"time"

	"device-telemetry-router/internal/domain"
)

func (s *PGStore) RecordAudit(ctx context.Context, keyHash, action, operator, remote, path string, at time.Time) error {
	return recordAudit(ctx, s.db, keyHash, action, operator, remote, path, at)
}
func (t *txStore) RecordAudit(ctx context.Context, keyHash, action, operator, remote, path string, at time.Time) error {
	return recordAudit(ctx, t.tx, keyHash, action, operator, remote, path, at)
}

func recordAudit(ctx context.Context, q queryer, keyHash, action, operator, remote, path string, at time.Time) error {
	_, err := q.ExecContext(ctx, `
		INSERT INTO api_key_audit (key_hash, action, operator, remote_addr, path, at)
		VALUES ($1,$2,$3,$4,$5,$6)`,
		keyHash, action, operator, remote, path, at)
	return err
}

func (s *PGStore) ListAudit(ctx context.Context, limit int) ([]domain.APIKeyAudit, error) {
	return listAudit(ctx, s.db, limit)
}
func (t *txStore) ListAudit(ctx context.Context, limit int) ([]domain.APIKeyAudit, error) {
	return listAudit(ctx, t.tx, limit)
}

func listAudit(ctx context.Context, q queryer, limit int) ([]domain.APIKeyAudit, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := q.QueryContext(ctx, `
		SELECT id, key_hash, action, operator, remote_addr, path, at
		FROM api_key_audit ORDER BY at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []domain.APIKeyAudit{}
	for rows.Next() {
		a := domain.APIKeyAudit{}
		var operator sql.NullString
		if err := rows.Scan(&a.ID, &a.KeyHash, &a.Action, &operator, &a.Remote, &a.Path, &a.At); err != nil {
			return nil, err
		}
		if operator.Valid {
			a.Operator = operator.String
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *PGStore) DeviceCount(ctx context.Context) (int64, error) {
	return countRows(ctx, s.db, "device")
}
func (t *txStore) DeviceCount(ctx context.Context) (int64, error) {
	return countRows(ctx, t.tx, "device")
}

func (s *PGStore) EventCount(ctx context.Context) (int64, error) {
	return countRows(ctx, s.db, "event")
}
func (t *txStore) EventCount(ctx context.Context) (int64, error) {
	return countRows(ctx, t.tx, "event")
}

func countRows(ctx context.Context, q queryer, table string) (int64, error) {
	var n int64
	err := q.QueryRowContext(ctx, `SELECT count(*) FROM `+table).Scan(&n)
	return n, err
}
