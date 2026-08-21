package metrics

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCounterRender(t *testing.T) {
	r := NewRegistry()
	c := r.NewCounter("requests_total", "Total requests.")
	c.Inc()
	c.Inc()

	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	body := rec.Body.String()
	if !strings.Contains(body, "requests_total 2") {
		t.Fatalf("expected counter value, got:\n%s", body)
	}
	if !strings.Contains(body, "# TYPE requests_total counter") {
		t.Fatal("expected counter type line")
	}
}

func TestGaugeRender(t *testing.T) {
	r := NewRegistry()
	g := r.NewGauge("in_flight", "In-flight.")
	g.Set(3)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	if !strings.Contains(rec.Body.String(), "in_flight 3") {
		t.Fatalf("expected gauge value:\n%s", rec.Body.String())
	}
}

func TestHistogramRender(t *testing.T) {
	r := NewRegistry()
	h := r.NewHistogram("latency", "Latency.", []float64{0.1, 1})
	h.Observe(0.05)
	h.Observe(0.5)
	h.Observe(2.0)

	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	body := rec.Body.String()
	if !strings.Contains(body, "latency_count 3") {
		t.Fatalf("expected histogram count:\n%s", body)
	}
	if !strings.Contains(body, "latency_sum") {
		t.Fatal("expected histogram sum")
	}
	if !strings.Contains(body, `le="+Inf"`) {
		t.Fatal("expected +Inf bucket")
	}
}
