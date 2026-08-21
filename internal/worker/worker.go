// Package worker implements the background delivery retry and dead-letter
// loop, plus the pluggable downstream topic sink used by dispatch.
package worker

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"device-telemetry-router/internal/domain"
)

// Sink is the downstream transport that actually delivers a routed event to
// a topic. Implementations are responsible for idempotency of their own
// transport; the worker drives retries based on returned errors.
type Sink interface {
	Dispatch(ctx context.Context, topic string, payload []byte) error
}

// DispatchPayload is the JSON payload handed to a topic sink.
type DispatchPayload struct {
	EventID  string            `json:"event_id"`
	DeviceID string            `json:"device_id"`
	Metric   string            `json:"metric"`
	Value    float64           `json:"value"`
	Unit     string            `json:"unit"`
	TS       time.Time         `json:"ts"`
	RuleID   string            `json:"rule_id"`
	Topic    string            `json:"topic"`
	Status   EventPayloadState `json:"status"`
}

// EventPayloadState is the serialized event lifecycle state.
type EventPayloadState string

const (
	StateRouted     EventPayloadState = "routed"
	StateDispatched EventPayloadState = "dispatched"
	StateDead       EventPayloadState = "dead_letter"
)

// Store is the persistence surface the worker needs from the repository
// layer. Deliberately narrower than store.Storer to keep the worker
// decoupled from unrelated query paths.
type Store interface {
	ListDueDeliveries(ctx context.Context, now time.Time, limit int) ([]domain.DeliveryRecord, error)
	MarkDelivered(ctx context.Context, id int64, at time.Time) error
	MarkFailed(ctx context.Context, id int64, attempt int, lastErr string, nextRetry time.Time) error
	MarkDead(ctx context.Context, id int64, lastErr string, at time.Time) error
	UpdateEventStatus(ctx context.Context, eventID string, status domain.EventStatus) error
	GetEvent(ctx context.Context, id string) (*domain.Event, error)
}

// Options configures the retry worker.
type Options struct {
	Interval   time.Duration // polling interval
	BatchSize  int           // max deliveries per pass
	MaxAttempt int           // attempts before dead-letter
	BaseDelay  time.Duration // initial retry backoff
}

// DefaultOptions returns conservative production-safe defaults.
func DefaultOptions() Options {
	return Options{
		Interval:   2 * time.Second,
		BatchSize:  100,
		MaxAttempt: 5,
		BaseDelay:  time.Second,
	}
}

// Worker periodically dispatches due delivery records, applies exponential
// backoff on failure and dead-letters records that exhaust their retries.
type Worker struct {
	store Store
	sink  Sink
	opts  Options
	now   func() time.Time
}

// New builds a Worker.
func New(st Store, sink Sink, opts Options) *Worker {
	if opts.Interval <= 0 {
		opts.Interval = 2 * time.Second
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = 100
	}
	if opts.MaxAttempt <= 0 {
		opts.MaxAttempt = 5
	}
	if opts.BaseDelay <= 0 {
		opts.BaseDelay = time.Second
	}
	return &Worker{store: st, sink: sink, opts: opts, now: time.Now}
}

// Run blocks until ctx is cancelled, processing due deliveries each interval.
func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.opts.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.processOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("worker: process pass failed: %v", err)
			}
		}
	}
}

// ProcessDue performs a single pass over due deliveries (used by Run and by
// tests for deterministic control).
func (w *Worker) ProcessDue(ctx context.Context) error {
	return w.processOnce(ctx)
}

func (w *Worker) processOnce(ctx context.Context) error {
	now := w.now().UTC()
	due, err := w.store.ListDueDeliveries(ctx, now, w.opts.BatchSize)
	if err != nil {
		return err
	}
	for _, d := range due {
		if err := w.dispatchOne(ctx, d, now); err != nil {
			return err
		}
	}
	return nil
}

func (w *Worker) dispatchOne(ctx context.Context, d domain.DeliveryRecord, now time.Time) error {
	ev, err := w.store.GetEvent(ctx, d.EventID)
	if err != nil {
		return err
	}
	if ev == nil {
		// Event no longer exists (should not happen given FK), but do not
		// retry forever on a dangling delivery.
		return w.store.MarkDead(ctx, d.ID, "event not found", now)
	}

	payload, err := json.Marshal(DispatchPayload{
		EventID:  ev.EventID,
		DeviceID: ev.DeviceID,
		Metric:   ev.Metric,
		Value:    ev.Value,
		Unit:     ev.Unit,
		TS:       ev.TS,
		RuleID:   d.RuleID,
		Topic:    d.Topic,
		Status:   StateRouted,
	})
	if err != nil {
		return err
	}

	if err := w.sink.Dispatch(ctx, d.Topic, payload); err != nil {
		return w.handleFailure(ctx, d, ev, now, err)
	}

	if err := w.store.MarkDelivered(ctx, d.ID, now); err != nil {
		return err
	}
	return w.store.UpdateEventStatus(ctx, ev.EventID, domain.EventDispatched)
}

func (w *Worker) handleFailure(ctx context.Context, d domain.DeliveryRecord, ev *domain.Event, now time.Time, dispatchErr error) error {
	attempt := d.Attempts + 1
	if attempt >= w.opts.MaxAttempt {
		if err := w.store.MarkDead(ctx, d.ID, dispatchErr.Error(), now); err != nil {
			return err
		}
		return w.store.UpdateEventStatus(ctx, ev.EventID, domain.EventDeadLetter)
	}
	delay := w.backoff(attempt)
	nextRetry := now.Add(delay)
	return w.store.MarkFailed(ctx, d.ID, attempt, dispatchErr.Error(), nextRetry)
}

func (w *Worker) backoff(attempt int) time.Duration {
	// Exponential backoff capped at a one-minute ceiling.
	d := w.opts.BaseDelay
	for i := 1; i < attempt; i++ {
		d *= 2
		if d > time.Minute {
			return time.Minute
		}
	}
	return d
}

// LogSink is a Sink implementation that logs the dispatch payload. It is the
// default sink in deployments without a real downstream broker.
type LogSink struct{}

// Dispatch logs the topic and payload at info level and always succeeds.
func (LogSink) Dispatch(_ context.Context, topic string, payload []byte) error {
	log.Printf("dispatch topic=%s payload=%s", topic, string(payload))
	return nil
}
