package service

import (
	"context"
	"testing"
	"time"

	"device-telemetry-router/internal/domain"
	"device-telemetry-router/internal/parser"
	"device-telemetry-router/internal/router"
	"device-telemetry-router/internal/store"
)

// fakeStore is an in-memory Storer used to exercise service use cases
// without a real PostgreSQL connection.
type fakeStore struct {
	devices    map[string]*domain.Device
	raws       map[string]*domain.RawMessage
	events     map[string]*domain.Event
	rules      map[string]*domain.RouteRule
	changes    map[string][]domain.RouteRuleChangeLog
	deliveries []domain.DeliveryRecord
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		devices: map[string]*domain.Device{},
		raws:    map[string]*domain.RawMessage{},
		events:  map[string]*domain.Event{},
		rules:   map[string]*domain.RouteRule{},
		changes: map[string][]domain.RouteRuleChangeLog{},
	}
}

func (f *fakeStore) CreateDevice(ctx context.Context, d domain.Device) error {
	if _, ok := f.devices[d.DeviceID]; ok {
		return errDup
	}
	cp := d
	f.devices[d.DeviceID] = &cp
	return nil
}
func (f *fakeStore) GetDevice(ctx context.Context, id string) (*domain.Device, error) {
	if d, ok := f.devices[id]; ok {
		cp := *d
		return &cp, nil
	}
	return nil, nil
}
func (f *fakeStore) ListDevices(ctx context.Context) ([]domain.Device, error) {
	out := []domain.Device{}
	for _, d := range f.devices {
		out = append(out, *d)
	}
	return out, nil
}
func (f *fakeStore) SetStatus(ctx context.Context, id string, s domain.DeviceStatus) error {
	if d, ok := f.devices[id]; ok {
		d.Status = s
	}
	return nil
}
func (f *fakeStore) TouchSeen(ctx context.Context, id string, at time.Time) error {
	if d, ok := f.devices[id]; ok {
		d.LastSeenAt = &at
	}
	return nil
}
func (f *fakeStore) InsertRaw(ctx context.Context, m domain.RawMessage) (int64, error) {
	if _, ok := f.raws[m.DedupKey]; ok {
		return 0, errDup
	}
	cp := m
	f.raws[m.DedupKey] = &cp
	return int64(len(f.raws)), nil
}
func (f *fakeStore) FindByDedupKey(ctx context.Context, key string) (*domain.RawMessage, error) {
	if m, ok := f.raws[key]; ok {
		cp := *m
		return &cp, nil
	}
	return nil, nil
}
func (f *fakeStore) CreateEvent(ctx context.Context, e domain.Event) (int64, error) {
	cp := e
	f.events[e.EventID] = &cp
	return int64(len(f.events)), nil
}
func (f *fakeStore) GetEvent(ctx context.Context, id string) (*domain.Event, error) {
	if e, ok := f.events[id]; ok {
		cp := *e
		return &cp, nil
	}
	return nil, nil
}
func (f *fakeStore) QueryEvents(ctx context.Context, fq store.EventFilter) ([]domain.Event, int64, error) {
	out := []domain.Event{}
	for _, e := range f.events {
		if fq.DeviceID != "" && e.DeviceID != fq.DeviceID {
			continue
		}
		if fq.Metric != "" && e.Metric != fq.Metric {
			continue
		}
		out = append(out, *e)
	}
	return out, int64(len(out)), nil
}
func (f *fakeStore) CreateRule(ctx context.Context, r domain.RouteRule) (int64, error) {
	if _, ok := f.rules[r.RuleID]; ok {
		return 0, errDup
	}
	cp := r
	f.rules[r.RuleID] = &cp
	return int64(len(f.rules)), nil
}
func (f *fakeStore) GetRule(ctx context.Context, id string) (*domain.RouteRule, error) {
	if r, ok := f.rules[id]; ok {
		cp := *r
		return &cp, nil
	}
	return nil, nil
}
func (f *fakeStore) UpdateRule(ctx context.Context, r domain.RouteRule) error {
	if _, ok := f.rules[r.RuleID]; ok {
		cp := r
		f.rules[r.RuleID] = &cp
	}
	return nil
}
func (f *fakeStore) SoftDelete(ctx context.Context, id string, at time.Time) error {
	if r, ok := f.rules[id]; ok {
		r.DeletedAt = &at
		r.Enabled = false
	}
	return nil
}
func (f *fakeStore) ListRules(ctx context.Context, includeDeleted bool) ([]domain.RouteRule, error) {
	out := []domain.RouteRule{}
	for _, r := range f.rules {
		if !includeDeleted && r.DeletedAt != nil {
			continue
		}
		out = append(out, *r)
	}
	return out, nil
}
func (f *fakeStore) ChangeLog(ctx context.Context, id string) ([]domain.RouteRuleChangeLog, error) {
	return f.changes[id], nil
}
func (f *fakeStore) AppendChange(ctx context.Context, c domain.RouteRuleChangeLog) error {
	f.changes[c.RuleID] = append(f.changes[c.RuleID], c)
	return nil
}
func (f *fakeStore) CreateDelivery(ctx context.Context, d domain.DeliveryRecord) error {
	f.deliveries = append(f.deliveries, d)
	return nil
}
func (f *fakeStore) PendingCount(ctx context.Context) (int64, error) {
	var n int64
	for _, d := range f.deliveries {
		if d.Status == domain.DeliveryPending {
			n++
		}
	}
	return n, nil
}
func (f *fakeStore) ListDueDeliveries(ctx context.Context, now time.Time, limit int) ([]domain.DeliveryRecord, error) {
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
func (f *fakeStore) ListDeliveriesByEvent(ctx context.Context, eventID string) ([]domain.DeliveryRecord, error) {
	out := []domain.DeliveryRecord{}
	for _, d := range f.deliveries {
		if d.EventID == eventID {
			out = append(out, d)
		}
	}
	return out, nil
}
func (f *fakeStore) MarkDelivered(ctx context.Context, id int64, at time.Time) error {
	for i := range f.deliveries {
		if f.deliveries[i].ID == id {
			f.deliveries[i].Status = domain.DeliveryDispatched
			f.deliveries[i].Attempts++
			f.deliveries[i].DispatchedAt = &at
		}
	}
	return nil
}
func (f *fakeStore) MarkFailed(ctx context.Context, id int64, attempt int, lastErr string, nextRetry time.Time) error {
	for i := range f.deliveries {
		if f.deliveries[i].ID == id {
			f.deliveries[i].Status = domain.DeliveryFailed
			f.deliveries[i].Attempts = attempt
			f.deliveries[i].LastError = lastErr
			f.deliveries[i].NextRetryAt = &nextRetry
		}
	}
	return nil
}
func (f *fakeStore) MarkDead(ctx context.Context, id int64, lastErr string, at time.Time) error {
	for i := range f.deliveries {
		if f.deliveries[i].ID == id {
			f.deliveries[i].Status = domain.DeliveryDead
			f.deliveries[i].LastError = lastErr
		}
	}
	return nil
}
func (f *fakeStore) RecordAudit(ctx context.Context, keyHash, action, operator, remote, path string, at time.Time) error {
	return nil
}
func (f *fakeStore) ListAudit(ctx context.Context, limit int) ([]domain.APIKeyAudit, error) {
	return nil, nil
}
func (f *fakeStore) DeviceCount(ctx context.Context) (int64, error) {
	return int64(len(f.devices)), nil
}
func (f *fakeStore) EventCount(ctx context.Context) (int64, error) {
	return int64(len(f.events)), nil
}
func (f *fakeStore) UpdateEventStatus(ctx context.Context, eventID string, status domain.EventStatus) error {
	for _, e := range f.events {
		if e.EventID == eventID {
			e.Status = status
		}
	}
	return nil
}
func (f *fakeStore) InTx(ctx context.Context, fn store.TxFunc) error {
	return fn(ctx, f)
}

var errDup = domain.NewError(409, "CONFLICT", "duplicate")

func newTestService(f *fakeStore) *Service {
	return New(f, f, router.New(), 24*time.Hour)
}

func TestIngestAndDedup(t *testing.T) {
	f := newFakeStore()
	svc := newTestService(f)
	ctx := context.Background()

	dev, err := svc.RegisterDevice(ctx, "DEVICE01", "test", "v1", nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if dev.Status != domain.DeviceActive {
		t.Fatalf("expected active device")
	}

	// Create a matching rule.
	if _, err := svc.CreateRule(ctx, RuleInput{
		RuleID: "r1", Name: "temp rule", Topic: "sensors/temp",
		Matcher: domain.Matcher{Metrics: []string{"temperature"}}, Enabled: true,
	}); err != nil {
		t.Fatalf("create rule: %v", err)
	}

	ts := time.Now().UTC()
	raw := parser.BuildMessage("DEVICE01", ts, "temperature", 25.5, "C")

	res, err := svc.IngestTelemetry(ctx, raw)
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if res.Duplicate {
		t.Fatal("first ingest should not be duplicate")
	}
	if res.EventID == "" {
		t.Fatal("expected event_id")
	}
	if len(res.Topics) != 1 || res.Topics[0] != "sensors/temp" {
		t.Fatalf("expected routed topic, got %v", res.Topics)
	}

	// Duplicate ingest.
	res2, err := svc.IngestTelemetry(ctx, raw)
	if err != nil {
		t.Fatalf("duplicate ingest: %v", err)
	}
	if !res2.Duplicate {
		t.Fatal("expected duplicate flag")
	}
	if res2.EventID != res.EventID {
		t.Fatalf("expected same event id, got %s vs %s", res2.EventID, res.EventID)
	}

	// Verify only one event recorded.
	events, total, _ := f.QueryEvents(ctx, store.EventFilter{})
	if total != 1 || len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", total)
	}
}

func TestIngestRejectsInactiveDevice(t *testing.T) {
	f := newFakeStore()
	svc := newTestService(f)
	ctx := context.Background()

	if _, err := svc.RegisterDevice(ctx, "DEVICE01", "test", "v1", nil); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetDeviceStatus(ctx, "DEVICE01", domain.DeviceSuspended); err != nil {
		t.Fatal(err)
	}

	ts := time.Now().UTC()
	raw := parser.BuildMessage("DEVICE01", ts, "temperature", 25.5, "C")
	if _, err := svc.IngestTelemetry(ctx, raw); err == nil {
		t.Fatal("expected error for inactive device")
	}
}

func TestIngestUnknownDevice(t *testing.T) {
	f := newFakeStore()
	svc := newTestService(f)
	ctx := context.Background()

	ts := time.Now().UTC()
	raw := parser.BuildMessage("DEVICE99", ts, "temperature", 25.5, "C")
	if _, err := svc.IngestTelemetry(ctx, raw); err == nil {
		t.Fatal("expected error for unknown device")
	}
}

func TestRuleChangeLog(t *testing.T) {
	f := newFakeStore()
	svc := newTestService(f)
	ctx := context.Background()

	if _, err := svc.CreateRule(ctx, RuleInput{RuleID: "r1", Name: "n", Topic: "t", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateRule(ctx, "r1", RuleInput{Name: "n2", Topic: "t", Enabled: false}); err != nil {
		t.Fatal(err)
	}
	changes, err := svc.RuleChanges(ctx, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 {
		t.Fatalf("expected 2 changes, got %d", len(changes))
	}
}

func TestReplayEvent(t *testing.T) {
	f := newFakeStore()
	svc := newTestService(f)
	ctx := context.Background()

	if _, err := svc.CreateRule(ctx, RuleInput{
		RuleID: "r1", Name: "temp", Topic: "sensors/temp",
		Matcher: domain.Matcher{Metrics: []string{"temperature"}}, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}

	// Manually seed an event via ingest.
	if _, err := svc.RegisterDevice(ctx, "DEVICE01", "d", "v1", nil); err != nil {
		t.Fatal(err)
	}
	ts := time.Now().UTC()
	raw := parser.BuildMessage("DEVICE01", ts, "temperature", 25.5, "C")
	res, err := svc.IngestTelemetry(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}

	// Replay should not create duplicate pending delivery for existing rule.
	replay, err := svc.ReplayEvent(ctx, res.EventID)
	if err != nil {
		t.Fatal(err)
	}
	if replay.ReplayedCount != 0 {
		t.Fatalf("expected 0 new delivery (already pending), got %d", replay.ReplayedCount)
	}

	deliveries, err := svc.ListDeliveries(ctx, res.EventID)
	if err != nil {
		t.Fatal(err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery record, got %d", len(deliveries))
	}
}

func TestStats(t *testing.T) {
	f := newFakeStore()
	svc := newTestService(f)
	ctx := context.Background()

	if _, err := svc.RegisterDevice(ctx, "DEVICE01", "d", "v1", nil); err != nil {
		t.Fatal(err)
	}
	stats, err := svc.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Devices != 1 {
		t.Fatalf("expected 1 device, got %d", stats.Devices)
	}
}
