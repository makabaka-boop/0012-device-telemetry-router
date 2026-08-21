package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"device-telemetry-router/internal/domain"
)

func (s *PGStore) CreateEvent(ctx context.Context, e domain.Event) (int64, error) {
	return createEvent(ctx, s.db, e)
}
func (t *txStore) CreateEvent(ctx context.Context, e domain.Event) (int64, error) {
	return createEvent(ctx, t.tx, e)
}

func createEvent(ctx context.Context, q queryer, e domain.Event) (int64, error) {
	var id int64
	err := q.QueryRowContext(ctx, `
		INSERT INTO event (event_id, device_id, ts, metric, value, unit, dedup_key, route_key, status, parsed_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`,
		e.EventID, e.DeviceID, e.TS, e.Metric, e.Value, e.Unit, e.DedupKey,
		e.RouteKey, string(e.Status), e.ParsedAt).Scan(&id)
	return id, err
}

func (s *PGStore) GetEvent(ctx context.Context, id string) (*domain.Event, error) {
	return getEventByID(ctx, s.db, id)
}
func (t *txStore) GetEvent(ctx context.Context, id string) (*domain.Event, error) {
	return getEventByID(ctx, t.tx, id)
}

// UpdateEventStatus advances an event's lifecycle status.
func (s *PGStore) UpdateEventStatus(ctx context.Context, eventID string, status domain.EventStatus) error {
	return updateEventStatus(ctx, s.db, eventID, status)
}
func (t *txStore) UpdateEventStatus(ctx context.Context, eventID string, status domain.EventStatus) error {
	return updateEventStatus(ctx, t.tx, eventID, status)
}

func updateEventStatus(ctx context.Context, q queryer, eventID string, status domain.EventStatus) error {
	_, err := q.ExecContext(ctx, `UPDATE event SET status=$1 WHERE event_id=$2`, string(status), eventID)
	return err
}

func getEventByID(ctx context.Context, q queryer, id string) (*domain.Event, error) {
	row := q.QueryRowContext(ctx, `
		SELECT id, event_id, device_id, ts, metric, value, unit, dedup_key, route_key, status, parsed_at
		FROM event WHERE event_id = $1`, id)
	e := &domain.Event{}
	err := row.Scan(&e.ID, &e.EventID, &e.DeviceID, &e.TS, &e.Metric, &e.Value,
		&e.Unit, &e.DedupKey, &e.RouteKey, &e.Status, &e.ParsedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return e, err
}

func (s *PGStore) QueryEvents(ctx context.Context, f EventFilter) ([]domain.Event, int64, error) {
	return queryEvents(ctx, s.db, f)
}
func (t *txStore) QueryEvents(ctx context.Context, f EventFilter) ([]domain.Event, int64, error) {
	return queryEvents(ctx, t.tx, f)
}

func queryEvents(ctx context.Context, q queryer, f EventFilter) ([]domain.Event, int64, error) {
	where := []string{"1=1"}
	args := []any{}
	aidx := 1
	appendEq := func(cond string, val any) {
		where = append(where, fmt.Sprintf("%s = $%d", cond, aidx))
		args = append(args, val)
		aidx++
	}
	if f.DeviceID != "" {
		appendEq("device_id", f.DeviceID)
	}
	if f.Metric != "" {
		appendEq("metric", f.Metric)
	}
	if f.Status != "" {
		appendEq("status", f.Status)
	}
	if f.From != nil {
		where = append(where, fmt.Sprintf("ts >= $%d", aidx))
		args = append(args, *f.From)
		aidx++
	}
	if f.To != nil {
		where = append(where, fmt.Sprintf("ts <= $%d", aidx))
		args = append(args, *f.To)
		aidx++
	}
	cond := strings.Join(where, " AND ")

	var total int64
	if err := q.QueryRowContext(ctx,
		"SELECT count(*) FROM event WHERE "+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	if f.Page < 1 {
		f.Page = 1
	}
	if f.Size < 1 {
		f.Size = 20
	}
	offset := (f.Page - 1) * f.Size
	qsql := fmt.Sprintf(`
		SELECT id, event_id, device_id, ts, metric, value, unit, dedup_key, route_key, status, parsed_at
		FROM event WHERE %s ORDER BY ts DESC LIMIT $%d OFFSET $%d`, cond, aidx, aidx+1)
	args = append(args, f.Size, offset)

	rows, err := q.QueryContext(ctx, qsql, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := []domain.Event{}
	for rows.Next() {
		e := domain.Event{}
		if err := rows.Scan(&e.ID, &e.EventID, &e.DeviceID, &e.TS, &e.Metric, &e.Value,
			&e.Unit, &e.DedupKey, &e.RouteKey, &e.Status, &e.ParsedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, e)
	}
	return out, total, rows.Err()
}
