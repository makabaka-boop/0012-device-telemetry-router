// Package migrate applies database migrations at startup.
package migrate

import (
	"database/sql"
	"fmt"
)

// Statements is the ordered list of migration SQL statements.
var Statements = []string{
	`CREATE TABLE IF NOT EXISTS device (
		device_id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		protocol_version TEXT NOT NULL,
		status TEXT NOT NULL,
		metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
		last_seen_at TIMESTAMPTZ,
		created_at TIMESTAMPTZ NOT NULL,
		updated_at TIMESTAMPTZ NOT NULL
	)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS device_device_id_idx ON device (device_id)`,
	`CREATE TABLE IF NOT EXISTS raw_message (
		id BIGSERIAL PRIMARY KEY,
		device_id TEXT NOT NULL REFERENCES device(device_id),
		raw_text TEXT NOT NULL,
		checksum_received TEXT NOT NULL,
		checksum_expected TEXT NOT NULL,
		received_at TIMESTAMPTZ NOT NULL,
		dedup_key TEXT NOT NULL,
		status TEXT NOT NULL,
		parse_error TEXT NOT NULL DEFAULT '',
		event_id TEXT
	)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS raw_message_dedup_key_idx ON raw_message (dedup_key)`,
	`CREATE INDEX IF NOT EXISTS raw_message_received_at_idx ON raw_message (received_at)`,
	`CREATE TABLE IF NOT EXISTS event (
		id BIGSERIAL PRIMARY KEY,
		event_id TEXT UNIQUE NOT NULL,
		device_id TEXT NOT NULL REFERENCES device(device_id),
		ts TIMESTAMPTZ NOT NULL,
		metric TEXT NOT NULL,
		value DOUBLE PRECISION NOT NULL,
		unit TEXT NOT NULL,
		dedup_key TEXT NOT NULL,
		route_key TEXT NOT NULL,
		status TEXT NOT NULL,
		parsed_at TIMESTAMPTZ NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS event_device_ts_idx ON event (device_id, ts)`,
	`CREATE INDEX IF NOT EXISTS event_metric_ts_idx ON event (metric, ts)`,
	`CREATE TABLE IF NOT EXISTS route_rule (
		id BIGSERIAL PRIMARY KEY,
		rule_id TEXT UNIQUE NOT NULL,
		name TEXT NOT NULL,
		matcher JSONB NOT NULL DEFAULT '{}'::jsonb,
		topic TEXT NOT NULL,
		priority INT NOT NULL DEFAULT 0,
		enabled BOOLEAN NOT NULL DEFAULT TRUE,
		deleted_at TIMESTAMPTZ,
		created_at TIMESTAMPTZ NOT NULL,
		updated_at TIMESTAMPTZ NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS route_rule_enabled_idx ON route_rule (enabled)`,
	`CREATE TABLE IF NOT EXISTS route_rule_change_log (
		id BIGSERIAL PRIMARY KEY,
		rule_id TEXT NOT NULL REFERENCES route_rule(rule_id),
		action TEXT NOT NULL,
		before_json JSONB,
		after_json JSONB,
		operator TEXT NOT NULL,
		changed_at TIMESTAMPTZ NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS change_log_rule_id_idx ON route_rule_change_log (rule_id)`,
	`CREATE TABLE IF NOT EXISTS delivery_record (
		id BIGSERIAL PRIMARY KEY,
		event_id TEXT NOT NULL REFERENCES event(event_id),
		rule_id TEXT NOT NULL REFERENCES route_rule(rule_id),
		topic TEXT NOT NULL,
		status TEXT NOT NULL,
		attempts INT NOT NULL DEFAULT 0,
		last_error TEXT NOT NULL DEFAULT '',
		next_retry_at TIMESTAMPTZ,
		dispatched_at TIMESTAMPTZ
	)`,
	`CREATE INDEX IF NOT EXISTS delivery_record_next_retry_idx ON delivery_record (next_retry_at)`,
	`CREATE TABLE IF NOT EXISTS api_key_audit (
		id BIGSERIAL PRIMARY KEY,
		key_hash TEXT NOT NULL,
		action TEXT NOT NULL,
		operator TEXT NOT NULL DEFAULT '',
		remote_addr TEXT NOT NULL DEFAULT '',
		path TEXT NOT NULL DEFAULT '',
		at TIMESTAMPTZ NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS api_key_audit_at_idx ON api_key_audit (at)`,
	`CREATE INDEX IF NOT EXISTS delivery_record_status_retry_idx ON delivery_record (status, next_retry_at)`,
}

// Apply executes all migration statements in order.
func Apply(db *sql.DB) error {
	for i, stmt := range Statements {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("migration %d failed: %w", i, err)
		}
	}
	return nil
}
