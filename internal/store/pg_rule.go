package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"device-telemetry-router/internal/domain"
)

func (s *PGStore) CreateRule(ctx context.Context, r domain.RouteRule) (int64, error) {
	return createRule(ctx, s.db, r)
}
func (t *txStore) CreateRule(ctx context.Context, r domain.RouteRule) (int64, error) {
	return createRule(ctx, t.tx, r)
}

func createRule(ctx context.Context, q queryer, r domain.RouteRule) (int64, error) {
	matcher, _ := json.Marshal(r.Matcher)
	var id int64
	err := q.QueryRowContext(ctx, `
		INSERT INTO route_rule (rule_id, name, matcher, topic, priority, enabled, deleted_at, created_at, updated_at)
		VALUES ($1,$2,$3::jsonb,$4,$5,$6,NULL,$7,$8) RETURNING id`,
		r.RuleID, r.Name, matcher, r.Topic, r.Priority, r.Enabled, r.CreatedAt, r.UpdatedAt).Scan(&id)
	return id, err
}

func (s *PGStore) GetRule(ctx context.Context, ruleID string) (*domain.RouteRule, error) {
	return getRule(ctx, s.db, ruleID)
}
func (t *txStore) GetRule(ctx context.Context, ruleID string) (*domain.RouteRule, error) {
	return getRule(ctx, t.tx, ruleID)
}

func getRule(ctx context.Context, q queryer, ruleID string) (*domain.RouteRule, error) {
	row := q.QueryRowContext(ctx, `
		SELECT id, rule_id, name, matcher, topic, priority, enabled, deleted_at, created_at, updated_at
		FROM route_rule WHERE rule_id = $1`, ruleID)
	r := &domain.RouteRule{}
	var matcher []byte
	var deletedAt sql.NullTime
	err := row.Scan(&r.ID, &r.RuleID, &r.Name, &matcher, &r.Topic,
		&r.Priority, &r.Enabled, &deletedAt, &r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(matcher, &r.Matcher)
	if deletedAt.Valid {
		t := deletedAt.Time
		r.DeletedAt = &t
	}
	return r, nil
}

func (s *PGStore) UpdateRule(ctx context.Context, r domain.RouteRule) error {
	return updateRule(ctx, s.db, r)
}
func (t *txStore) UpdateRule(ctx context.Context, r domain.RouteRule) error {
	return updateRule(ctx, t.tx, r)
}

func updateRule(ctx context.Context, q queryer, r domain.RouteRule) error {
	matcher, _ := json.Marshal(r.Matcher)
	_, err := q.ExecContext(ctx, `
		UPDATE route_rule SET name=$1, matcher=$2::jsonb, topic=$3, priority=$4, enabled=$5, updated_at=$6
		WHERE rule_id=$7`,
		r.Name, matcher, r.Topic, r.Priority, r.Enabled, r.UpdatedAt, r.RuleID)
	return err
}

func (s *PGStore) SoftDelete(ctx context.Context, ruleID string, at time.Time) error {
	return softDelete(ctx, s.db, ruleID, at)
}
func (t *txStore) SoftDelete(ctx context.Context, ruleID string, at time.Time) error {
	return softDelete(ctx, t.tx, ruleID, at)
}

func softDelete(ctx context.Context, q queryer, ruleID string, at time.Time) error {
	_, err := q.ExecContext(ctx, `
		UPDATE route_rule SET deleted_at=$1, enabled=FALSE, updated_at=$1 WHERE rule_id=$2`, at, ruleID)
	return err
}

func (s *PGStore) ListRules(ctx context.Context, includeDeleted bool) ([]domain.RouteRule, error) {
	return listRules(ctx, s.db, includeDeleted)
}
func (t *txStore) ListRules(ctx context.Context, includeDeleted bool) ([]domain.RouteRule, error) {
	return listRules(ctx, t.tx, includeDeleted)
}

func listRules(ctx context.Context, q queryer, includeDeleted bool) ([]domain.RouteRule, error) {
	qs := `SELECT id, rule_id, name, matcher, topic, priority, enabled, deleted_at, created_at, updated_at
		FROM route_rule`
	if !includeDeleted {
		qs += ` WHERE deleted_at IS NULL`
	}
	qs += ` ORDER BY priority DESC, rule_id`
	rows, err := q.QueryContext(ctx, qs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []domain.RouteRule{}
	for rows.Next() {
		r := domain.RouteRule{}
		var matcher []byte
		var deletedAt sql.NullTime
		if err := rows.Scan(&r.ID, &r.RuleID, &r.Name, &matcher, &r.Topic,
			&r.Priority, &r.Enabled, &deletedAt, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(matcher, &r.Matcher)
		if deletedAt.Valid {
			t := deletedAt.Time
			r.DeletedAt = &t
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *PGStore) ChangeLog(ctx context.Context, ruleID string) ([]domain.RouteRuleChangeLog, error) {
	return changeLog(ctx, s.db, ruleID)
}
func (t *txStore) ChangeLog(ctx context.Context, ruleID string) ([]domain.RouteRuleChangeLog, error) {
	return changeLog(ctx, t.tx, ruleID)
}

func changeLog(ctx context.Context, q queryer, ruleID string) ([]domain.RouteRuleChangeLog, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT id, rule_id, action, before_json, after_json, operator, changed_at
		FROM route_rule_change_log WHERE rule_id=$1 ORDER BY changed_at DESC`, ruleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []domain.RouteRuleChangeLog{}
	for rows.Next() {
		c := domain.RouteRuleChangeLog{}
		var before, after []byte
		if err := rows.Scan(&c.ID, &c.RuleID, &c.Action, &before, &after,
			&c.Operator, &c.ChangedAt); err != nil {
			return nil, err
		}
		c.BeforeJSON = before
		c.AfterJSON = after
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *PGStore) AppendChange(ctx context.Context, e domain.RouteRuleChangeLog) error {
	return appendChange(ctx, s.db, e)
}
func (t *txStore) AppendChange(ctx context.Context, e domain.RouteRuleChangeLog) error {
	return appendChange(ctx, t.tx, e)
}

func appendChange(ctx context.Context, q queryer, e domain.RouteRuleChangeLog) error {
	_, err := q.ExecContext(ctx, `
		INSERT INTO route_rule_change_log (rule_id, action, before_json, after_json, operator, changed_at)
		VALUES ($1,$2,$3::jsonb,$4::jsonb,$5,$6)`,
		e.RuleID, string(e.Action), e.BeforeJSON, e.AfterJSON, e.Operator, e.ChangedAt)
	return err
}
