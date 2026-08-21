package service

import (
	"context"
	"encoding/json"
	"reflect"

	"device-telemetry-router/internal/domain"
	"device-telemetry-router/internal/store"
)

// RuleInput is the payload for creating or updating a routing rule.
type RuleInput struct {
	RuleID   string         `json:"rule_id"`
	Name     string         `json:"name"`
	Matcher  domain.Matcher `json:"matcher"`
	Topic    string         `json:"topic"`
	Priority int            `json:"priority"`
	Enabled  bool           `json:"enabled"`
}

// CreateRule validates and creates a routing rule with change logging.
func (s *Service) CreateRule(ctx context.Context, in RuleInput) (*domain.RouteRule, error) {
	now := s.now().UTC()
	if in.Name == "" {
		return nil, domain.ErrInvalidField.WithDetails(map[string]any{"field": "name"})
	}
	if in.Topic == "" {
		return nil, domain.ErrInvalidField.WithDetails(map[string]any{"field": "topic"})
	}
	if in.RuleID == "" {
		in.RuleID = newEventID()
	}
	if existing, err := s.store.GetRule(ctx, in.RuleID); err != nil {
		return nil, err
	} else if existing != nil {
		return nil, domain.ErrRuleConflict
	}

	r := domain.RouteRule{
		RuleID:    in.RuleID,
		Name:      in.Name,
		Matcher:   in.Matcher,
		Topic:     in.Topic,
		Priority:  in.Priority,
		Enabled:   in.Enabled,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.tx.InTx(ctx, func(ctx context.Context, txs store.Storer) error {
		if _, err := txs.CreateRule(ctx, r); err != nil {
			return err
		}
		after, _ := json.Marshal(r)
		return txs.AppendChange(ctx, domain.RouteRuleChangeLog{
			RuleID:    r.RuleID,
			Action:    domain.ActionCreate,
			AfterJSON: after,
			Operator:  OperatorFrom(ctx),
			ChangedAt: now,
		})
	}); err != nil {
		if isUniqueViolation(err) {
			return nil, domain.ErrRuleConflict
		}
		return nil, err
	}
	return &r, nil
}

// UpdateRule updates an existing rule and logs the change.
func (s *Service) UpdateRule(ctx context.Context, ruleID string, in RuleInput) (*domain.RouteRule, error) {
	now := s.now().UTC()
	existing, err := s.store.GetRule(ctx, ruleID)
	if err != nil {
		return nil, err
	}
	if existing == nil || existing.DeletedAt != nil {
		return nil, domain.ErrRuleNotFound
	}

	before, _ := json.Marshal(existing)

	updated := *existing
	if in.Name != "" {
		updated.Name = in.Name
	}
	if in.Topic != "" {
		updated.Topic = in.Topic
	}
	if !reflect.DeepEqual(in.Matcher, domain.Matcher{}) {
		updated.Matcher = in.Matcher
	}
	updated.Priority = in.Priority
	updated.Enabled = in.Enabled
	updated.UpdatedAt = now

	action := domain.ActionUpdate
	if !existing.Enabled && updated.Enabled {
		action = domain.ActionEnable
	} else if existing.Enabled && !updated.Enabled {
		action = domain.ActionDisable
	}

	if err := s.tx.InTx(ctx, func(ctx context.Context, txs store.Storer) error {
		if err := txs.UpdateRule(ctx, updated); err != nil {
			return err
		}
		after, _ := json.Marshal(updated)
		return txs.AppendChange(ctx, domain.RouteRuleChangeLog{
			RuleID:     ruleID,
			Action:     action,
			BeforeJSON: before,
			AfterJSON:  after,
			Operator:   OperatorFrom(ctx),
			ChangedAt:  now,
		})
	}); err != nil {
		return nil, err
	}
	return &updated, nil
}

// DeleteRule soft-deletes a rule and logs the change.
func (s *Service) DeleteRule(ctx context.Context, ruleID string) error {
	now := s.now().UTC()
	existing, err := s.store.GetRule(ctx, ruleID)
	if err != nil {
		return err
	}
	if existing == nil || existing.DeletedAt != nil {
		return domain.ErrRuleNotFound
	}

	before, _ := json.Marshal(existing)
	after := *existing
	after.Enabled = false
	after.DeletedAt = &now
	after.UpdatedAt = now
	afterJSON, _ := json.Marshal(after)

	return s.tx.InTx(ctx, func(ctx context.Context, txs store.Storer) error {
		if err := txs.SoftDelete(ctx, ruleID, now); err != nil {
			return err
		}
		return txs.AppendChange(ctx, domain.RouteRuleChangeLog{
			RuleID:     ruleID,
			Action:     domain.ActionDelete,
			BeforeJSON: before,
			AfterJSON:  afterJSON,
			Operator:   OperatorFrom(ctx),
			ChangedAt:  now,
		})
	})
}

// ListRules returns enabled (non-deleted) rules.
func (s *Service) ListRules(ctx context.Context) ([]domain.RouteRule, error) {
	return s.store.ListRules(ctx, false)
}

// GetRule fetches a single rule by id.
func (s *Service) GetRule(ctx context.Context, ruleID string) (*domain.RouteRule, error) {
	r, err := s.store.GetRule(ctx, ruleID)
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, domain.ErrRuleNotFound
	}
	return r, nil
}

// RuleChanges returns the change log for a rule.
func (s *Service) RuleChanges(ctx context.Context, ruleID string) ([]domain.RouteRuleChangeLog, error) {
	return s.store.ChangeLog(ctx, ruleID)
}
