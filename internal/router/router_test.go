package router

import (
	"reflect"
	"testing"

	"device-telemetry-router/internal/domain"
)

func rule(id, pattern, metric, topic string, priority int, enabled bool) domain.RouteRule {
	m := domain.Matcher{DevicePattern: pattern}
	if metric != "" {
		m.Metrics = []string{metric}
	}
	return domain.RouteRule{RuleID: id, Matcher: m, Topic: topic, Priority: priority, Enabled: enabled}
}

func event(device, metric string) domain.Event {
	return domain.Event{DeviceID: device, Metric: metric}
}

func TestSingleMatch(t *testing.T) {
	r := New()
	rules := []domain.RouteRule{rule("r1", "", "temperature", "sensors/temp", 0, true)}
	got := r.Match(event("DEVICE01", "temperature"), rules)
	if len(got) != 1 || got[0].RuleID != "r1" {
		t.Fatalf("expected single match, got %+v", got)
	}
}

func TestMultipleMatchPriorityOrder(t *testing.T) {
	r := New()
	rules := []domain.RouteRule{
		rule("low", "", "temperature", "t/low", 1, true),
		rule("high", "", "temperature", "t/high", 10, true),
		rule("mid", "", "temperature", "t/mid", 5, true),
	}
	got := r.Match(event("DEVICE01", "temperature"), rules)
	if len(got) != 3 {
		t.Fatalf("expected 3 matches, got %d", len(got))
	}
	if got[0].RuleID != "high" || got[1].RuleID != "mid" || got[2].RuleID != "low" {
		t.Fatalf("wrong priority order: %v", Topics(got))
	}
}

func TestNoMatch(t *testing.T) {
	r := New()
	rules := []domain.RouteRule{rule("r1", "", "humidity", "sensors/hum", 0, true)}
	got := r.Match(event("DEVICE01", "temperature"), rules)
	if len(got) != 0 {
		t.Fatalf("expected no match, got %+v", got)
	}
}

func TestDisabledRuleNotMatched(t *testing.T) {
	r := New()
	rules := []domain.RouteRule{
		rule("r1", "", "temperature", "t/1", 0, false),
		rule("r2", "", "temperature", "t/2", 0, true),
	}
	got := r.Match(event("DEVICE01", "temperature"), rules)
	if len(got) != 1 || got[0].RuleID != "r2" {
		t.Fatalf("disabled rule should not match: %+v", got)
	}
}

func TestDevicePatternMatch(t *testing.T) {
	r := New()
	rules := []domain.RouteRule{rule("r1", "DEV*", "temperature", "t/1", 0, true)}
	if len(r.Match(event("DEV01", "temperature"), rules)) != 1 {
		t.Fatal("wildcard pattern should match")
	}
	if len(r.Match(event("ABC01", "temperature"), rules)) != 0 {
		t.Fatal("non-matching pattern should not match")
	}
}

func TestTopics(t *testing.T) {
	rules := []domain.RouteRule{
		rule("a", "", "", "t/a", 1, true),
		rule("b", "", "", "t/b", 0, true),
	}
	got := Topics(rules)
	if len(got) != 2 || got[0] != "t/a" || got[1] != "t/b" {
		t.Fatalf("unexpected topics: %v", got)
	}
}
