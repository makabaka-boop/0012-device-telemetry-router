// Package domain defines entities, value objects and domain errors shared
// across layers. It has no dependency on storage or transport.
package domain

import "time"

// DeviceStatus enumerates the lifecycle of a device.
type DeviceStatus string

const (
	DevicePending   DeviceStatus = "pending"
	DeviceActive    DeviceStatus = "active"
	DeviceSuspended DeviceStatus = "suspended"
	DeviceDeleted   DeviceStatus = "deleted"
)

// Device is the device registry record.
type Device struct {
	DeviceID        string         `json:"device_id"`
	Name            string         `json:"name"`
	ProtocolVersion string         `json:"protocol_version"`
	Status          DeviceStatus   `json:"status"`
	Metadata        map[string]any `json:"metadata,omitempty"`
	LastSeenAt      *time.Time     `json:"last_seen_at,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

// RawStatus enumerates the raw message lifecycle.
type RawStatus string

const (
	RawReceived  RawStatus = "received"
	RawParsed    RawStatus = "parsed"
	RawRejected  RawStatus = "rejected"
	RawDuplicate RawStatus = "duplicate"
)

// RawMessage is the original ingested telemetry text.
type RawMessage struct {
	ID               int64     `json:"id"`
	DeviceID         string    `json:"device_id"`
	RawText          string    `json:"raw_text"`
	ChecksumReceived string    `json:"checksum_received"`
	ChecksumExpected string    `json:"checksum_expected"`
	ReceivedAt       time.Time `json:"received_at"`
	DedupKey         string    `json:"dedup_key"`
	Status           RawStatus `json:"status"`
	ParseError       string    `json:"parse_error,omitempty"`
	EventID          *string   `json:"event_id,omitempty"`
}

// EventStatus enumerates the event lifecycle.
type EventStatus string

const (
	EventCreated    EventStatus = "created"
	EventRouted     EventStatus = "routed"
	EventDispatched EventStatus = "dispatched"
	EventDeadLetter EventStatus = "dead_letter"
)

// Event is the normalized telemetry event parsed from a raw message.
type Event struct {
	ID       int64       `json:"id"`
	EventID  string      `json:"event_id"`
	DeviceID string      `json:"device_id"`
	TS       time.Time   `json:"ts"`
	Metric   string      `json:"metric"`
	Value    float64     `json:"value"`
	Unit     string      `json:"unit"`
	DedupKey string      `json:"dedup_key"`
	RouteKey string      `json:"route_key"`
	Status   EventStatus `json:"status"`
	ParsedAt time.Time   `json:"parsed_at"`
}

// Matcher is the JSON-encoded route rule matcher definition. In addition to
// the field-based device pattern and metric set, an optional Expr expression
// (AND/OR over metric/device/value comparisons) further narrows matching.
type Matcher struct {
	DevicePattern string   `json:"device_pattern,omitempty"`
	Metrics       []string `json:"metrics,omitempty"`
	Expr          string   `json:"expr,omitempty"`
}

// RouteRule is a routing rule mapping matching events to a topic.
type RouteRule struct {
	ID        int64      `json:"id"`
	RuleID    string     `json:"rule_id"`
	Name      string     `json:"name"`
	Matcher   Matcher    `json:"matcher"`
	Topic     string     `json:"topic"`
	Priority  int        `json:"priority"`
	Enabled   bool       `json:"enabled"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// RouteTarget identifies one ordered destination selected for an event.
type RouteTarget struct {
	RuleID string
	Topic  string
}

// RoutingPlan is the ordered set of destinations produced by routing.
type RoutingPlan struct {
	ruleIDs []string
	topics  []string
}

// NewRoutingPlan constructs a plan from parallel ordered rule and topic lists.
func NewRoutingPlan(ruleIDs, topics []string) RoutingPlan {
	return RoutingPlan{ruleIDs: ruleIDs, topics: topics}
}

// Len returns the number of complete route targets in the plan.
func (p RoutingPlan) Len() int {
	if len(p.ruleIDs) < len(p.topics) {
		return len(p.ruleIDs)
	}
	return len(p.topics)
}

// Target returns the route target at index and reports whether it exists.
func (p RoutingPlan) Target(index int) (RouteTarget, bool) {
	if index < 0 || index >= p.Len() {
		return RouteTarget{}, false
	}
	return RouteTarget{RuleID: p.ruleIDs[index], Topic: p.topics[index]}, true
}

// RuleIDs returns the ordered rule identifiers in the plan.
func (p RoutingPlan) RuleIDs() []string {
	ids := make([]string, len(p.ruleIDs))
	copy(ids, p.ruleIDs)
	return ids
}

// Topics returns the ordered topics in the plan.
func (p RoutingPlan) Topics() []string {
	topics := make([]string, len(p.topics))
	copy(topics, p.topics)
	return topics
}

// ChangeAction enumerates the audit actions for rule changes.
type ChangeAction string

const (
	ActionCreate  ChangeAction = "create"
	ActionUpdate  ChangeAction = "update"
	ActionDisable ChangeAction = "disable"
	ActionEnable  ChangeAction = "enable"
	ActionDelete  ChangeAction = "delete"
)

// RouteRuleChangeLog is an immutable audit record of a rule change.
type RouteRuleChangeLog struct {
	ID         int64        `json:"id"`
	RuleID     string       `json:"rule_id"`
	Action     ChangeAction `json:"action"`
	BeforeJSON []byte       `json:"before_json,omitempty"`
	AfterJSON  []byte       `json:"after_json,omitempty"`
	Operator   string       `json:"operator"`
	ChangedAt  time.Time    `json:"changed_at"`
}

// DeliveryStatus enumerates delivery lifecycle states.
type DeliveryStatus string

const (
	DeliveryPending    DeliveryStatus = "pending"
	DeliveryDispatched DeliveryStatus = "dispatched"
	DeliveryFailed     DeliveryStatus = "failed"
	DeliveryDead       DeliveryStatus = "dead_letter"
)

// DeliveryRecord records a dispatch attempt for an event to a topic.
type DeliveryRecord struct {
	ID           int64          `json:"id"`
	EventID      string         `json:"event_id"`
	RuleID       string         `json:"rule_id"`
	Topic        string         `json:"topic"`
	Status       DeliveryStatus `json:"status"`
	Attempts     int            `json:"attempts"`
	LastError    string         `json:"last_error,omitempty"`
	NextRetryAt  *time.Time     `json:"next_retry_at,omitempty"`
	DispatchedAt *time.Time     `json:"dispatched_at,omitempty"`
}

// AuditAction enumerates the audited API key operations.
type AuditAction string

const (
	AuditAuthenticated AuditAction = "authenticated"
	AuditRejected      AuditAction = "rejected"
)

// APIKeyAudit is an immutable record of an authentication attempt.
type APIKeyAudit struct {
	ID       int64       `json:"id"`
	KeyHash  string      `json:"key_hash"`
	Action   AuditAction `json:"action"`
	Operator string      `json:"operator,omitempty"`
	Remote   string      `json:"remote_addr,omitempty"`
	Path     string      `json:"path,omitempty"`
	At       time.Time   `json:"at"`
}
