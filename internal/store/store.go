// Package store defines persistence interfaces and the PostgreSQL
// implementation. Method names are kept distinct across entity types to
// allow a single concrete store to satisfy all interfaces.
package store

import (
	"context"
	"time"

	"device-telemetry-router/internal/domain"
)

// DeviceStorer is the device persistence port.
type DeviceStorer interface {
	CreateDevice(ctx context.Context, d domain.Device) error
	GetDevice(ctx context.Context, id string) (*domain.Device, error)
	ListDevices(ctx context.Context) ([]domain.Device, error)
	SetStatus(ctx context.Context, id string, status domain.DeviceStatus) error
	TouchSeen(ctx context.Context, id string, at time.Time) error
}

// RawStorer is the raw message persistence port.
type RawStorer interface {
	InsertRaw(ctx context.Context, m domain.RawMessage) (int64, error)
	FindByDedupKey(ctx context.Context, key string) (*domain.RawMessage, error)
}

// EventStorer is the event persistence port.
type EventStorer interface {
	CreateEvent(ctx context.Context, e domain.Event) (int64, error)
	GetEvent(ctx context.Context, id string) (*domain.Event, error)
	QueryEvents(ctx context.Context, f EventFilter) ([]domain.Event, int64, error)
	UpdateEventStatus(ctx context.Context, eventID string, status domain.EventStatus) error
}

// RuleStorer is the routing-rule persistence port.
type RuleStorer interface {
	CreateRule(ctx context.Context, r domain.RouteRule) (int64, error)
	GetRule(ctx context.Context, ruleID string) (*domain.RouteRule, error)
	UpdateRule(ctx context.Context, r domain.RouteRule) error
	SoftDelete(ctx context.Context, ruleID string, at time.Time) error
	ListRules(ctx context.Context, includeDeleted bool) ([]domain.RouteRule, error)
	ChangeLog(ctx context.Context, ruleID string) ([]domain.RouteRuleChangeLog, error)
	AppendChange(ctx context.Context, e domain.RouteRuleChangeLog) error
}

// DeliveryStorer is the delivery-record persistence port.
type DeliveryStorer interface {
	CreateDelivery(ctx context.Context, d domain.DeliveryRecord) error
	PendingCount(ctx context.Context) (int64, error)
	ListDueDeliveries(ctx context.Context, now time.Time, limit int) ([]domain.DeliveryRecord, error)
	ListDeliveriesByEvent(ctx context.Context, eventID string) ([]domain.DeliveryRecord, error)
	MarkDelivered(ctx context.Context, id int64, at time.Time) error
	MarkFailed(ctx context.Context, id int64, attempt int, lastErr string, nextRetry time.Time) error
	MarkDead(ctx context.Context, id int64, lastErr string, at time.Time) error
}

// AuditStorer persists API key authentication audit entries.
type AuditStorer interface {
	RecordAudit(ctx context.Context, keyHash, action, operator, remote, path string, at time.Time) error
	ListAudit(ctx context.Context, limit int) ([]domain.APIKeyAudit, error)
}

// DeviceCountStorer reports aggregate counts efficiently.
type DeviceCountStorer interface {
	DeviceCount(ctx context.Context) (int64, error)
	EventCount(ctx context.Context) (int64, error)
}

// EventFilter is the query filter for event listing.
type EventFilter struct {
	DeviceID string
	Metric   string
	From     *time.Time
	To       *time.Time
	Status   string
	Page     int
	Size     int
}

// Storer aggregates all persistence ports.
type Storer interface {
	DeviceStorer
	RawStorer
	EventStorer
	RuleStorer
	DeliveryStorer
	AuditStorer
	DeviceCountStorer
}

// TxFunc runs fn inside a transaction.
type TxFunc func(context.Context, Storer) error

// TxRunner runs a transactional unit of work.
type TxRunner interface {
	InTx(context.Context, TxFunc) error
}
