// Package service orchestrates business use cases: device registration,
// telemetry ingestion (parse + dedup + route), rule CRUD, event queries and
// statistics.
package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"regexp"
	"time"

	"device-telemetry-router/internal/dedup"
	"device-telemetry-router/internal/domain"
	"device-telemetry-router/internal/parser"
	"device-telemetry-router/internal/router"
	"device-telemetry-router/internal/store"
)

var deviceIDRe = regexp.MustCompile(`^[A-Z0-9-]{8,32}$`)

// Service is the application service holding all use cases.
type Service struct {
	store  store.Storer
	tx     store.TxRunner
	router *router.Router
	dedup  time.Duration
	now    func() time.Time
}

// New builds a Service with its dependencies.
func New(s store.Storer, tx store.TxRunner, r *router.Router, dedupWindow time.Duration) *Service {
	return &Service{store: s, tx: tx, router: r, dedup: dedupWindow, now: time.Now}
}

// RegisterDevice validates and creates a device archive.
func (s *Service) RegisterDevice(ctx context.Context, deviceID, name, protocolVersion string, metadata map[string]any) (*domain.Device, error) {
	if !deviceIDRe.MatchString(deviceID) {
		return nil, domain.ErrInvalidField.WithDetails(map[string]any{"field": "device_id"})
	}
	if name == "" {
		return nil, domain.ErrInvalidField.WithDetails(map[string]any{"field": "name"})
	}
	if protocolVersion == "" {
		protocolVersion = "v1"
	}

	now := s.now().UTC()
	d := domain.Device{
		DeviceID:        deviceID,
		Name:            name,
		ProtocolVersion: protocolVersion,
		Status:          domain.DeviceActive,
		Metadata:        metadata,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	err := s.store.CreateDevice(ctx, d)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, domain.ErrDeviceConflict
		}
		return nil, err
	}
	return &d, nil
}

// GetDevice fetches a device archive.
func (s *Service) GetDevice(ctx context.Context, deviceID string) (*domain.Device, error) {
	d, err := s.store.GetDevice(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	if d == nil {
		return nil, domain.ErrDeviceNotFound
	}
	return d, nil
}

// ListDevices returns all devices.
func (s *Service) ListDevices(ctx context.Context) ([]domain.Device, error) {
	return s.store.ListDevices(ctx)
}

// SetDeviceStatus transitions a device status.
func (s *Service) SetDeviceStatus(ctx context.Context, deviceID string, status domain.DeviceStatus) error {
	if _, err := s.GetDevice(ctx, deviceID); err != nil {
		return err
	}
	switch status {
	case domain.DeviceActive, domain.DeviceSuspended, domain.DeviceDeleted:
	default:
		return domain.ErrInvalidField.WithDetails(map[string]any{"field": "status"})
	}
	return s.store.SetStatus(ctx, deviceID, status)
}

// TelemetryResult is the outcome of a telemetry ingestion.
type TelemetryResult struct {
	EventID      string   `json:"event_id"`
	Duplicate    bool     `json:"duplicate"`
	MatchedRules []string `json:"matched_rules"`
	Topics       []string `json:"topics"`
}

// IngestTelemetry parses, validates, dedups, stores and routes a raw message.
func (s *Service) IngestTelemetry(ctx context.Context, rawText string) (*TelemetryResult, error) {
	now := s.now().UTC()

	tel, err := parser.ParseMessage(rawText, now)
	if err != nil {
		return nil, domain.ErrInvalidField.WithDetails(map[string]any{"cause": err.Error()})
	}

	device, err := s.store.GetDevice(ctx, tel.DeviceID)
	if err != nil {
		return nil, err
	}
	if device == nil {
		return nil, domain.ErrDeviceNotFound
	}
	if device.Status != domain.DeviceActive {
		return nil, domain.ErrDeviceInactive
	}

	key := dedup.Key(tel.DeviceID, tel.TS, tel.Metric, tel.Value, tel.Unit)

	// Dedup check outside transaction for fast path.
	if existing, err := s.store.FindByDedupKey(ctx, key); err != nil {
		return nil, err
	} else if existing != nil && existing.Status == domain.RawParsed && dedup.WithinWindow(existing.ReceivedAt, now, s.dedup) {
		return s.duplicateResult(ctx, existing)
	}

	eventID := newEventID()
	event := domain.Event{
		EventID:  eventID,
		DeviceID: tel.DeviceID,
		TS:       tel.TS,
		Metric:   tel.Metric,
		Value:    tel.Value,
		Unit:     tel.Unit,
		DedupKey: key,
		RouteKey: tel.Metric,
		Status:   domain.EventCreated,
		ParsedAt: now,
	}

	var result *TelemetryResult
	err = s.tx.InTx(ctx, func(ctx context.Context, txs store.Storer) error {
		if existing, err := txs.FindByDedupKey(ctx, key); err != nil {
			return err
		} else if existing != nil && existing.Status == domain.RawParsed {
			return errDuplicate
		}

		if _, err := txs.CreateEvent(ctx, event); err != nil {
			return err
		}
		if _, err := txs.InsertRaw(ctx, domain.RawMessage{
			DeviceID:         tel.DeviceID,
			RawText:          rawText,
			ChecksumReceived: tel.Checksum,
			ChecksumExpected: tel.Checksum,
			ReceivedAt:       now,
			DedupKey:         key,
			Status:           domain.RawParsed,
			EventID:          &eventID,
		}); err != nil {
			return err
		}
		if err := txs.TouchSeen(ctx, tel.DeviceID, now); err != nil {
			return err
		}

		rules, err := s.listActiveRules(ctx, txs)
		if err != nil {
			return err
		}
		matched := s.router.Match(event, rules)
		topics := router.Topics(matched)
		matchedRuleIDs := make([]string, 0, len(matched))
		for i, r := range matched {
			matchedRuleIDs = append(matchedRuleIDs, r.RuleID)
			if err := txs.CreateDelivery(ctx, domain.DeliveryRecord{
				EventID:     eventID,
				RuleID:      r.RuleID,
				Topic:       topics[i],
				Status:      domain.DeliveryPending,
				Attempts:    0,
				NextRetryAt: &now,
			}); err != nil {
				return err
			}
		}
		result = &TelemetryResult{
			EventID:      eventID,
			Duplicate:    false,
			MatchedRules: matchedRuleIDs,
			Topics:       topics,
		}
		return nil
	})
	if err == errDuplicate {
		if existing, e2 := s.store.FindByDedupKey(ctx, key); e2 == nil && existing != nil {
			return s.duplicateResult(ctx, existing)
		}
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) listActiveRules(ctx context.Context, txs store.Storer) ([]domain.RouteRule, error) {
	return txs.ListRules(ctx, false)
}

func (s *Service) duplicateResult(ctx context.Context, existing *domain.RawMessage) (*TelemetryResult, error) {
	res := &TelemetryResult{Duplicate: true, Topics: []string{}, MatchedRules: []string{}}
	if existing.EventID != nil {
		res.EventID = *existing.EventID
		if ev, err := s.store.GetEvent(ctx, *existing.EventID); err == nil && ev != nil {
			rules, err := s.store.ListRules(ctx, false)
			if err == nil {
				matched := s.router.Match(*ev, rules)
				res.Topics = router.Topics(matched)
				for _, r := range matched {
					res.MatchedRules = append(res.MatchedRules, r.RuleID)
				}
			}
		}
	}
	return res, nil
}

var errDuplicate = domain.NewError(200, "DUPLICATE_MESSAGE", "duplicate message")

type operatorCtxKey struct{}

// WithOperator stores the authenticated operator identity in the context so
// that auditable write operations can attribute changes.
func WithOperator(ctx context.Context, operator string) context.Context {
	return context.WithValue(ctx, operatorCtxKey{}, operator)
}

// OperatorFrom extracts the authenticated operator, defaulting to "system".
func OperatorFrom(ctx context.Context) string {
	if v, ok := ctx.Value(operatorCtxKey{}).(string); ok && v != "" {
		return v
	}
	return "system"
}

func newEventID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	return containsAny(err.Error(), "duplicate key", "unique constraint", "SQLSTATE 23505")
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if len(s) >= len(sub) {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}
