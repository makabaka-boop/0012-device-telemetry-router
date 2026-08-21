package store

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"device-telemetry-router/internal/domain"
	"device-telemetry-router/internal/migrate"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping store integration test")
	}
	db, err := sql.Open("postgres", url)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}
	if err := migrate.Apply(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() {
		// Clean test tables in reverse dependency order.
		for _, stmt := range []string{
			`DELETE FROM delivery_record`,
			`DELETE FROM route_rule_change_log`,
			`DELETE FROM route_rule`,
			`DELETE FROM event`,
			`DELETE FROM raw_message`,
			`DELETE FROM device`,
		} {
			_, _ = db.Exec(stmt)
		}
		db.Close()
	})
	return db
}

func TestDeviceUniqueConstraint(t *testing.T) {
	db := testDB(t)
	s := New(db)
	ctx := context.Background()
	now := time.Now().UTC()

	d := domain.Device{DeviceID: "TESTDEV001", Name: "n", ProtocolVersion: "v1", Status: domain.DeviceActive, Metadata: map[string]any{}, CreatedAt: now, UpdatedAt: now}
	if err := s.CreateDevice(ctx, d); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := s.CreateDevice(ctx, d); err == nil {
		t.Fatal("expected unique violation on duplicate device_id")
	}
}

func TestTransactionRollback(t *testing.T) {
	db := testDB(t)
	s := New(db)
	ctx := context.Background()
	now := time.Now().UTC()

	// Register a device first (outside tx) so FK is satisfied.
	d := domain.Device{DeviceID: "TESTDEV002", Name: "n", ProtocolVersion: "v1", Status: domain.DeviceActive, Metadata: map[string]any{}, CreatedAt: now, UpdatedAt: now}
	if err := s.CreateDevice(ctx, d); err != nil {
		t.Fatalf("create device: %v", err)
	}

	// Force a rollback by returning an error from the tx fn.
	err := s.InTx(ctx, func(ctx context.Context, txs Storer) error {
		if _, err := txs.CreateEvent(ctx, domain.Event{EventID: "ev-rollback", DeviceID: "TESTDEV002", TS: now, Metric: "temperature", Value: 1, Unit: "C", DedupKey: "dk-1", RouteKey: "temperature", Status: domain.EventCreated, ParsedAt: now}); err != nil {
			return err
		}
		return domain.ErrRuleConflict
	})
	if err == nil {
		t.Fatal("expected error to force rollback")
	}
	ev, err := s.GetEvent(ctx, "ev-rollback")
	if err != nil {
		t.Fatalf("get event: %v", err)
	}
	if ev != nil {
		t.Fatal("event should have been rolled back")
	}
}
