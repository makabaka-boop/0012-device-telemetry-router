package auth

import (
	"context"
	"testing"
	"time"

	"device-telemetry-router/internal/domain"
)

type memAudit struct {
	entries []auditEntry
}

type auditEntry struct {
	keyHash, action, operator, remote, path string
	at                                      time.Time
}

func (m *memAudit) RecordAudit(ctx context.Context, keyHash, action, operator, remote, path string, at time.Time) error {
	m.entries = append(m.entries, auditEntry{keyHash, action, operator, remote, path, at})
	return nil
}

func TestParseKeyPairs(t *testing.T) {
	m, err := ParseKeyPairs("alice:secret1,bob:secret2")
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(m))
	}
	if m["secret1"].Operator != "alice" || m["secret2"].Operator != "bob" {
		t.Fatalf("unexpected operators: %+v", m)
	}
}

func TestParseKeyPairsInvalid(t *testing.T) {
	if _, err := ParseKeyPairs("nocolon"); err == nil {
		t.Fatal("expected error for missing colon")
	}
	if _, err := ParseKeyPairs(":secret"); err == nil {
		t.Fatal("expected error for empty operator")
	}
}

func TestAuthenticate(t *testing.T) {
	audit := &memAudit{}
	keys, _ := ParseKeyPairs("alice:secret1")
	a := New(keys, audit)

	op, ok := a.Authenticate(context.Background(), "secret1", "127.0.0.1", "/api/v1/rules")
	if !ok || op != "alice" {
		t.Fatalf("expected alice, got %q ok=%v", op, ok)
	}
	if len(audit.entries) != 1 || audit.entries[0].action != string(domain.AuditAuthenticated) {
		t.Fatalf("expected authenticated audit, got %+v", audit.entries)
	}
}

func TestAuthenticateRejects(t *testing.T) {
	audit := &memAudit{}
	keys, _ := ParseKeyPairs("alice:secret1")
	a := New(keys, audit)

	if _, ok := a.Authenticate(context.Background(), "wrong", "127.0.0.1", "/x"); ok {
		t.Fatal("expected rejection for wrong key")
	}
	if len(audit.entries) != 1 || audit.entries[0].action != string(domain.AuditRejected) {
		t.Fatalf("expected rejected audit, got %+v", audit.entries)
	}
}
