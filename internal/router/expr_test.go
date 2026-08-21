package router

import (
	"testing"

	"device-telemetry-router/internal/domain"
)

func exprRule(id, expr string) domain.RouteRule {
	return domain.RouteRule{RuleID: id, Matcher: domain.Matcher{Expr: expr}, Topic: "t/" + id, Enabled: true}
}

func TestMatchExprAndOr(t *testing.T) {
	r := New()
	rules := []domain.RouteRule{
		exprRule("a", `metric == "temperature" or metric == "humidity"`),
		exprRule("b", `metric == "temperature" and value >= 20`),
	}
	got := r.Match(event("DEVICE01", "temperature"), rules)
	// Event value defaults to 0; rule a matches on metric, rule b needs value>=20.
	if len(got) != 1 || got[0].RuleID != "a" {
		t.Fatalf("expected only rule a to match, got %v", Topics(got))
	}
}

func TestMatchExprNumericAndWildcard(t *testing.T) {
	r := New()
	rules := []domain.RouteRule{
		exprRule("hot", `metric == "temperature" and value > 80`),
		exprRule("dev", `device ~ "DEV*"`),
	}
	ev := event("DEVICE01", "temperature")
	ev.Value = 90
	got := r.Match(ev, rules)
	if len(got) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(got))
	}
}

func TestMatchExprParentheses(t *testing.T) {
	r := New()
	rules := []domain.RouteRule{
		exprRule("paren", `(metric == "temperature" or metric == "pressure") and device == "DEVICE01"`),
	}
	got := r.Match(event("DEVICE01", "temperature"), rules)
	if len(got) != 1 {
		t.Fatalf("expected 1 match, got %d", len(got))
	}
}

func TestMatchExprEmpty(t *testing.T) {
	r := New()
	rules := []domain.RouteRule{exprRule("any", "")}
	if len(r.Match(event("X", "temperature"), rules)) != 1 {
		t.Fatal("empty expression should match everything")
	}
}

func TestMatchExprInvalidFallsThrough(t *testing.T) {
	r := New()
	rules := []domain.RouteRule{exprRule("bad", `metric == `)}
	if len(r.Match(event("X", "temperature"), rules)) != 0 {
		t.Fatal("invalid expression should not match")
	}
}

func TestMatchExprUnitField(t *testing.T) {
	r := New()
	rules := []domain.RouteRule{exprRule("c", `unit == "C"`)}
	if len(r.Match(event("X", "temperature"), rules)) != 0 {
		t.Fatal("unit comparison on empty unit should not match")
	}
}
