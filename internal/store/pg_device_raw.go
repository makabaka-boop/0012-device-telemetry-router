// Package store provides the PostgreSQL-backed implementation of the
// persistence ports.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"device-telemetry-router/internal/domain"
)

// pgStore implements Storer on top of a *sql.DB.
type PGStore struct {
	db *sql.DB
}

// New returns a PostgreSQL Storer and TxRunner implementation.
func New(db *sql.DB) *PGStore {
	return &PGStore{db: db}
}

func (s *PGStore) InTx(ctx context.Context, fn TxFunc) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := fn(ctx, &txStore{tx: tx}); err != nil {
		return err
	}
	return tx.Commit()
}

// txStore is a Storer bound to a transaction.
type txStore struct {
	tx *sql.Tx
}

type queryer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func (s *PGStore) CreateDevice(ctx context.Context, d domain.Device) error {
	return insertDevice(ctx, s.db, d)
}
func (t *txStore) CreateDevice(ctx context.Context, d domain.Device) error {
	return insertDevice(ctx, t.tx, d)
}

func insertDevice(ctx context.Context, q queryer, d domain.Device) error {
	meta, _ := json.Marshal(d.Metadata)
	_, err := q.ExecContext(ctx, `
		INSERT INTO device (device_id, name, protocol_version, status, metadata, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7)`,
		d.DeviceID, d.Name, d.ProtocolVersion, string(d.Status), meta, d.CreatedAt, d.UpdatedAt)
	return err
}

func (s *PGStore) GetDevice(ctx context.Context, id string) (*domain.Device, error) {
	return getDevice(ctx, s.db, id)
}
func (t *txStore) GetDevice(ctx context.Context, id string) (*domain.Device, error) {
	return getDevice(ctx, t.tx, id)
}

func getDevice(ctx context.Context, q queryer, id string) (*domain.Device, error) {
	row := q.QueryRowContext(ctx, `
		SELECT device_id, name, protocol_version, status, metadata, last_seen_at, created_at, updated_at
		FROM device WHERE device_id = $1`, id)
	d := &domain.Device{}
	var meta []byte
	var lastSeen sql.NullTime
	if err := row.Scan(&d.DeviceID, &d.Name, &d.ProtocolVersion, &d.Status,
		&meta, &lastSeen, &d.CreatedAt, &d.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	d.Metadata = map[string]any{}
	_ = json.Unmarshal(meta, &d.Metadata)
	if lastSeen.Valid {
		t := lastSeen.Time
		d.LastSeenAt = &t
	}
	return d, nil
}

func (s *PGStore) ListDevices(ctx context.Context) ([]domain.Device, error) {
	return listDevices(ctx, s.db)
}
func (t *txStore) ListDevices(ctx context.Context) ([]domain.Device, error) {
	return listDevices(ctx, t.tx)
}

func listDevices(ctx context.Context, q queryer) ([]domain.Device, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT device_id, name, protocol_version, status, metadata, last_seen_at, created_at, updated_at
		FROM device ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.Device
	for rows.Next() {
		d := domain.Device{}
		var meta []byte
		var lastSeen sql.NullTime
		if err := rows.Scan(&d.DeviceID, &d.Name, &d.ProtocolVersion, &d.Status,
			&meta, &lastSeen, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		d.Metadata = map[string]any{}
		_ = json.Unmarshal(meta, &d.Metadata)
		if lastSeen.Valid {
			t := lastSeen.Time
			d.LastSeenAt = &t
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *PGStore) SetStatus(ctx context.Context, id string, status domain.DeviceStatus) error {
	return setStatus(ctx, s.db, id, status)
}
func (t *txStore) SetStatus(ctx context.Context, id string, status domain.DeviceStatus) error {
	return setStatus(ctx, t.tx, id, status)
}

func setStatus(ctx context.Context, q queryer, id string, status domain.DeviceStatus) error {
	_, err := q.ExecContext(ctx,
		`UPDATE device SET status = $1, updated_at = $2 WHERE device_id = $3`,
		string(status), time.Now().UTC(), id)
	return err
}

func (s *PGStore) TouchSeen(ctx context.Context, id string, at time.Time) error {
	return touchSeen(ctx, s.db, id, at)
}
func (t *txStore) TouchSeen(ctx context.Context, id string, at time.Time) error {
	return touchSeen(ctx, t.tx, id, at)
}

func touchSeen(ctx context.Context, q queryer, id string, at time.Time) error {
	_, err := q.ExecContext(ctx,
		`UPDATE device SET last_seen_at = $1, updated_at = $1 WHERE device_id = $2`, at, id)
	return err
}

func (s *PGStore) InsertRaw(ctx context.Context, m domain.RawMessage) (int64, error) {
	return insertRaw(ctx, s.db, m)
}
func (t *txStore) InsertRaw(ctx context.Context, m domain.RawMessage) (int64, error) {
	return insertRaw(ctx, t.tx, m)
}

func insertRaw(ctx context.Context, q queryer, m domain.RawMessage) (int64, error) {
	var id int64
	var eventID any
	if m.EventID != nil {
		eventID = *m.EventID
	}
	err := q.QueryRowContext(ctx, `
		INSERT INTO raw_message (device_id, raw_text, checksum_received, checksum_expected, received_at, dedup_key, status, parse_error, event_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`,
		m.DeviceID, m.RawText, m.ChecksumReceived, m.ChecksumExpected, m.ReceivedAt,
		m.DedupKey, string(m.Status), m.ParseError, eventID).Scan(&id)
	return id, err
}

func (s *PGStore) FindByDedupKey(ctx context.Context, key string) (*domain.RawMessage, error) {
	return findRawByDedup(ctx, s.db, key)
}
func (t *txStore) FindByDedupKey(ctx context.Context, key string) (*domain.RawMessage, error) {
	return findRawByDedup(ctx, t.tx, key)
}

func findRawByDedup(ctx context.Context, q queryer, key string) (*domain.RawMessage, error) {
	row := q.QueryRowContext(ctx, `
		SELECT id, device_id, raw_text, checksum_received, checksum_expected, received_at, dedup_key, status, parse_error, event_id
		FROM raw_message WHERE dedup_key = $1`, key)
	m := &domain.RawMessage{}
	var eventID sql.NullString
	err := row.Scan(&m.ID, &m.DeviceID, &m.RawText, &m.ChecksumReceived,
		&m.ChecksumExpected, &m.ReceivedAt, &m.DedupKey, &m.Status, &m.ParseError, &eventID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if eventID.Valid {
		m.EventID = &eventID.String
	}
	return m, nil
}
