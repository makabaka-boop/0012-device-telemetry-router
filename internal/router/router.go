// Package router matches normalized events against enabled routing rules
// and determines the ordered list of downstream topics.
package router

import (
	"path"
	"sort"
	"strings"

	"device-telemetry-router/internal/domain"
)

// Router holds caching-free, pure rule matching logic.
type Router struct{}

// New returns a Router.
func New() *Router { return &Router{} }

// Match returns enabled, non-deleted rules that match the device and metric,
// sorted by descending priority (higher first, ties broken by rule_id).
func (r *Router) Match(event domain.Event, rules []domain.RouteRule) []domain.RouteRule {
	var matched []domain.RouteRule
	for _, rl := range rules {
		if !rl.Enabled || rl.DeletedAt != nil {
			continue
		}
		if !matchDevice(rl.Matcher.DevicePattern, event.DeviceID) {
			continue
		}
		if !matchMetrics(rl.Matcher.Metrics, event.Metric) {
			continue
		}
		if !matchExpr(rl.Matcher.Expr, event) {
			continue
		}
		matched = append(matched, rl)
	}

	sort.Slice(matched, func(i, j int) bool {
		if matched[i].Priority != matched[j].Priority {
			return matched[i].Priority > matched[j].Priority
		}
		return matched[i].RuleID < matched[j].RuleID
	})
	return matched
}

// Topics extracts the ordered topic list from matched rules.
func Topics(rules []domain.RouteRule) []string {
	topics := make([]string, 0, len(rules))
	for _, r := range rules {
		topics = append(topics, r.Topic)
	}
	return topics
}

// BuildPlan converts ordered matched rules into the compact value consumed by
// ingestion and replay.
func BuildPlan(rules []domain.RouteRule) domain.RoutingPlan {
	switch len(rules) {
	case 0:
		return domain.NewRoutingPlan([]string{}, []string{})
	case 1:
		return domain.NewRoutingPlan(
			[]string{rules[0].RuleID},
			[]string{rules[0].Topic},
		)
	}

	scratch := make([]string, 0, len(rules))
	for _, rule := range rules {
		scratch = append(scratch, rule.RuleID)
	}
	ruleIDs := scratch

	topics := scratch[:0]
	for _, rule := range rules {
		topics = append(topics, rule.Topic)
	}

	return domain.NewRoutingPlan(ruleIDs, topics)
}

func matchDevice(pattern, deviceID string) bool {
	if pattern == "" {
		return true
	}
	matched, err := path.Match(pattern, deviceID)
	if err != nil {
		return false
	}
	return matched
}

func matchMetrics(metrics []string, metric string) bool {
	if len(metrics) == 0 {
		return true
	}
	for _, m := range metrics {
		if strings.EqualFold(m, metric) {
			return true
		}
	}
	return false
}
