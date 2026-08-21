package service

import (
	"context"
	"time"

	"device-telemetry-router/internal/domain"
	"device-telemetry-router/internal/store"
)

// EventQuery is the filter for listing events.
type EventQuery struct {
	DeviceID string
	Metric   string
	From     *time.Time
	To       *time.Time
	Status   string
	Page     int
	Size     int
}

// EventPage is a paginated event listing.
type EventPage struct {
	Items []domain.Event `json:"items"`
	Total int64          `json:"total"`
	Page  int            `json:"page"`
	Size  int            `json:"size"`
}

// QueryEvents lists events matching the filter.
func (s *Service) QueryEvents(ctx context.Context, q EventQuery) (*EventPage, error) {
	f := store.EventFilter{
		DeviceID: q.DeviceID,
		Metric:   q.Metric,
		From:     q.From,
		To:       q.To,
		Status:   q.Status,
		Page:     q.Page,
		Size:     q.Size,
	}
	items, total, err := s.store.QueryEvents(ctx, f)
	if err != nil {
		return nil, err
	}
	return &EventPage{Items: items, Total: total, Page: q.Page, Size: q.Size}, nil
}

// GetEvent fetches a single event.
func (s *Service) GetEvent(ctx context.Context, eventID string) (*domain.Event, error) {
	e, err := s.store.GetEvent(ctx, eventID)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, domain.ErrEventNotFound
	}
	return e, nil
}

// Stats aggregates dashboard statistics.
type Stats struct {
	Devices int64 `json:"devices"`
	Events  int64 `json:"events"`
	Pending int64 `json:"pending_deliveries"`
}

// Stats returns device, event and pending delivery counts.
func (s *Service) Stats(ctx context.Context) (*Stats, error) {
	devices, err := s.store.DeviceCount(ctx)
	if err != nil {
		return nil, err
	}
	events, err := s.store.EventCount(ctx)
	if err != nil {
		return nil, err
	}
	pending, err := s.store.PendingCount(ctx)
	if err != nil {
		return nil, err
	}
	return &Stats{Devices: devices, Events: events, Pending: pending}, nil
}

// ListAudit returns recent API key authentication audit entries.
func (s *Service) ListAudit(ctx context.Context, limit int) ([]domain.APIKeyAudit, error) {
	return s.store.ListAudit(ctx, limit)
}
