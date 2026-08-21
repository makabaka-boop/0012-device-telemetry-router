package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"device-telemetry-router/internal/domain"
)

type fakeWorkerStore struct {
	deliveries map[int64]domain.DeliveryRecord
	events     map[string]*domain.Event
	nextID     int64
	now        time.Time
}

func newFakeWorkerStore() *fakeWorkerStore {
	return &fakeWorkerStore{
		deliveries: map[int64]domain.DeliveryRecord{},
		events:     map[string]*domain.Event{},
		nextID:     1,
	}
}

func (f *fakeWorkerStore) addEvent(ev domain.Event) {
	f.events[ev.EventID] = &ev
}

func (f *fakeWorkerStore) addDelivery(d domain.DeliveryRecord) int64 {
	d.ID = f.nextID
	f.nextID++
	f.deliveries[d.ID] = d
	return d.ID
}

func (f *fakeWorkerStore) ListDueDeliveries(ctx context.Context, now time.Time, limit int) ([]domain.DeliveryRecord, error) {
	out := []domain.DeliveryRecord{}
	for _, d := range f.deliveries {
		if d.Status == domain.DeliveryPending || d.Status == domain.DeliveryFailed {
			if d.NextRetryAt == nil || !d.NextRetryAt.After(now) {
				out = append(out, d)
			}
		}
	}
	return out, nil
}
func (f *fakeWorkerStore) MarkDelivered(ctx context.Context, id int64, at time.Time) error {
	d := f.deliveries[id]
	d.Status = domain.DeliveryDispatched
	d.Attempts++
	d.DispatchedAt = &at
	f.deliveries[id] = d
	return nil
}
func (f *fakeWorkerStore) MarkFailed(ctx context.Context, id int64, attempt int, lastErr string, nextRetry time.Time) error {
	d := f.deliveries[id]
	d.Status = domain.DeliveryFailed
	d.Attempts = attempt
	d.LastError = lastErr
	d.NextRetryAt = &nextRetry
	f.deliveries[id] = d
	return nil
}
func (f *fakeWorkerStore) MarkDead(ctx context.Context, id int64, lastErr string, at time.Time) error {
	d := f.deliveries[id]
	d.Status = domain.DeliveryDead
	d.LastError = lastErr
	f.deliveries[id] = d
	return nil
}
func (f *fakeWorkerStore) UpdateEventStatus(ctx context.Context, eventID string, status domain.EventStatus) error {
	if ev, ok := f.events[eventID]; ok {
		ev.Status = status
	}
	return nil
}
func (f *fakeWorkerStore) GetEvent(ctx context.Context, id string) (*domain.Event, error) {
	if ev, ok := f.events[id]; ok {
		cp := *ev
		return &cp, nil
	}
	return nil, nil
}

type failingSink struct{}

func (failingSink) Dispatch(ctx context.Context, topic string, payload []byte) error {
	return errors.New("downstream unavailable")
}

type okSink struct{}

func (okSink) Dispatch(ctx context.Context, topic string, payload []byte) error { return nil }

func TestWorkerDispatchesSuccessfully(t *testing.T) {
	st := newFakeWorkerStore()
	st.addEvent(domain.Event{EventID: "ev1", DeviceID: "D", Metric: "temperature", Value: 1, Unit: "C", Status: domain.EventCreated})
	now := time.Now().UTC()
	st.addDelivery(domain.DeliveryRecord{EventID: "ev1", RuleID: "r1", Topic: "t/1", Status: domain.DeliveryPending, NextRetryAt: &now})

	w := New(st, okSink{}, Options{BatchSize: 10, MaxAttempt: 3})
	_ = w.ProcessDue(context.Background())

	if st.deliveries[1].Status != domain.DeliveryDispatched {
		t.Fatalf("expected dispatched, got %s", st.deliveries[1].Status)
	}
	if st.events["ev1"].Status != domain.EventDispatched {
		t.Fatalf("expected event dispatched, got %s", st.events["ev1"].Status)
	}
}

func TestWorkerDeadLettersAfterMaxAttempts(t *testing.T) {
	st := newFakeWorkerStore()
	st.addEvent(domain.Event{EventID: "ev1", DeviceID: "D", Metric: "temperature", Value: 1, Unit: "C", Status: domain.EventCreated})
	now := time.Now().UTC()
	st.addDelivery(domain.DeliveryRecord{EventID: "ev1", RuleID: "r1", Topic: "t/1", Status: domain.DeliveryPending, NextRetryAt: &now})

	w := New(st, failingSink{}, Options{BatchSize: 10, MaxAttempt: 2, BaseDelay: time.Minute})
	_ = w.ProcessDue(context.Background()) // attempt 1 -> failed, retry in future
	// Force the retry to be due immediately for deterministic testing.
	past := time.Now().UTC().Add(-time.Second)
	for id, d := range st.deliveries {
		d.NextRetryAt = &past
		d.Status = domain.DeliveryFailed
		st.deliveries[id] = d
	}
	_ = w.ProcessDue(context.Background()) // attempt 2 -> dead letter

	if st.deliveries[1].Status != domain.DeliveryDead {
		t.Fatalf("expected dead_letter, got %s", st.deliveries[1].Status)
	}
	if st.events["ev1"].Status != domain.EventDeadLetter {
		t.Fatalf("expected event dead_letter, got %s", st.events["ev1"].Status)
	}
}

func TestWorkerBackoff(t *testing.T) {
	w := New(nil, okSink{}, Options{BaseDelay: time.Second})
	if w.backoff(1) != time.Second {
		t.Fatal("first retry should be base delay")
	}
	if w.backoff(2) != 2*time.Second {
		t.Fatal("second retry should double")
	}
	if w.backoff(3) != 4*time.Second {
		t.Fatal("third retry should be 4x")
	}
}
