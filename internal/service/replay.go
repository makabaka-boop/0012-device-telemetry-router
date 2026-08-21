package service

import (
	"context"

	"device-telemetry-router/internal/domain"
	"device-telemetry-router/internal/router"
)

// ReplayEvent re-dispatches an existing event through currently-enabled
// rules, producing fresh pending delivery records. It is idempotent with
// respect to the event (new records are only created for rules that match and
// for which no undispatched record is already pending).
func (s *Service) ReplayEvent(ctx context.Context, eventID string) (*ReplayResult, error) {
	ev, err := s.GetEvent(ctx, eventID)
	if err != nil {
		return nil, err
	}

	now := s.now().UTC()
	rules, err := s.store.ListRules(ctx, false)
	if err != nil {
		return nil, err
	}
	matched := s.router.Match(*ev, rules)
	plan := router.BuildPlan(matched)

	// Load existing delivery records to avoid duplicating pending work.
	existing, err := s.store.ListDeliveriesByEvent(ctx, eventID)
	if err != nil {
		return nil, err
	}
	pending := map[string]bool{}
	for _, d := range existing {
		if d.Status == domain.DeliveryPending || d.Status == domain.DeliveryFailed {
			pending[d.RuleID] = true
		}
	}

	created := make([]string, 0, plan.Len())
	for i := 0; i < plan.Len(); i++ {
		target, ok := plan.Target(i)
		if !ok {
			continue
		}
		if pending[target.RuleID] {
			continue
		}
		if err := s.store.CreateDelivery(ctx, domain.DeliveryRecord{
			EventID:     eventID,
			RuleID:      target.RuleID,
			Topic:       target.Topic,
			Status:      domain.DeliveryPending,
			Attempts:    0,
			NextRetryAt: &now,
		}); err != nil {
			return nil, err
		}
		created = append(created, target.RuleID)
	}
	return &ReplayResult{
		EventID:       eventID,
		MatchedRules:  plan.RuleIDs(),
		CreatedRules:  created,
		ReplayedCount: len(created),
	}, nil
}

// ReplayResult reports the outcome of an event replay.
type ReplayResult struct {
	EventID       string   `json:"event_id"`
	MatchedRules  []string `json:"matched_rules"`
	CreatedRules  []string `json:"created_rules"`
	ReplayedCount int      `json:"replayed_count"`
}

// ListDeliveries returns all delivery records for an event.
func (s *Service) ListDeliveries(ctx context.Context, eventID string) ([]domain.DeliveryRecord, error) {
	if _, err := s.GetEvent(ctx, eventID); err != nil {
		return nil, err
	}
	return s.store.ListDeliveriesByEvent(ctx, eventID)
}
