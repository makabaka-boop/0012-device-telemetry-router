// Package auth implements API Key authentication and audit for mutating
// endpoints. Keys are loaded from configuration as plaintext pairs of
// "operator:key"; only the SHA-256 of the key is retained in memory.
package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"device-telemetry-router/internal/domain"
)

// KeyEntry is a provisioned API key looked up by its SHA-256 hash.
type KeyEntry struct {
	Operator string
	Enabled  bool
}

// Auth validates incoming API keys and records authentication attempts into
// the audit store. It is safe for concurrent use once built via New.
type Auth struct {
	keys   map[string]KeyEntry
	audit  Auditor
	now    func() time.Time
	header string
}

// Auditor persists API key authentication attempts for auditability.
type Auditor interface {
	RecordAudit(ctx context.Context, keyHash, action, operator, remote, path string, at time.Time) error
}

// New builds an Auth from a map of API keys (opaque key identifier -> entry).
func New(keys map[string]KeyEntry, audit Auditor) *Auth {
	if audit == nil {
		audit = discardAuditor{}
	}
	m := make(map[string]KeyEntry, len(keys))
	for id, e := range keys {
		// Canonical lookup key is the SHA-256 of the presented secret, but
		// the configuration supplies a stable identifier; normalize by hashing
		// the identifier itself so presentation keys are never stored raw.
		h := hash(id)
		m[h] = e
	}
	return &Auth{keys: m, audit: audit, now: time.Now, header: "X-API-Key"}
}

// Authenticate validates the presented API key and returns the operator
// identity on success. A nil return with a false flag indicates rejection.
func (a *Auth) Authenticate(ctx context.Context, presented, remote, path string) (string, bool) {
	h := hash(presented)
	e, ok := a.keys[h]
	at := a.now().UTC()
	if !ok {
		_ = a.audit.RecordAudit(ctx, h, string(domain.AuditRejected), "", remote, path, at)
		return "", false
	}
	if !e.Enabled {
		_ = a.audit.RecordAudit(ctx, h, string(domain.AuditRejected), e.Operator, remote, path, at)
		return "", false
	}
	_ = a.audit.RecordAudit(ctx, h, string(domain.AuditAuthenticated), e.Operator, remote, path, at)
	return e.Operator, true
}

func hash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// ParseKeyPairs parses an "operator:secret,operator2:secret2" configuration
// string into an API key map keyed by secret.
func ParseKeyPairs(spec string) (map[string]KeyEntry, error) {
	out := map[string]KeyEntry{}
	if strings.TrimSpace(spec) == "" {
		return out, nil
	}
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		idx := strings.Index(part, ":")
		if idx <= 0 || idx == len(part)-1 {
			return nil, fmt.Errorf("invalid api key pair %q (want operator:secret)", part)
		}
		operator := strings.TrimSpace(part[:idx])
		secret := strings.TrimSpace(part[idx+1:])
		if _, dup := out[secret]; dup {
			return nil, fmt.Errorf("duplicate api key for operator %q", operator)
		}
		out[secret] = KeyEntry{Operator: operator, Enabled: true}
	}
	return out, nil
}

type discardAuditor struct{}

func (discardAuditor) RecordAudit(context.Context, string, string, string, string, string, time.Time) error {
	return nil
}

// Middleware wraps an http.Handler and enforces API key authentication.
func Middleware(a *Auth) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			presented := r.Header.Get("X-API-Key")
			operator, ok := a.Authenticate(r.Context(), presented, r.RemoteAddr, r.URL.Path)
			if !ok {
				writeUnauthorized(w)
				return
			}
			ctx := context.WithValue(r.Context(), operatorKey{}, operator)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

type operatorKey struct{}

// OperatorFromContext extracts the authenticated operator, if any.
func OperatorFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(operatorKey{}).(string)
	return v, ok
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":{"code":"UNAUTHORIZED","message":"invalid or missing api key"}}`))
}
