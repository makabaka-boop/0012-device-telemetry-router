// Package metrics exposes a minimal Prometheus text-format metrics surface
// with no external dependencies, instrumenting request counts, telemetry
// ingestion, routing matches and delivery dispatch outcomes.
package metrics

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"sync"
)

// Counter is a monotonically increasing integer metric.
type Counter struct {
	mu    sync.Mutex
	value float64
	help  string
	label []string
}

// Gauge is an integer metric that can go up and down.
type Gauge struct {
	mu    sync.Mutex
	value float64
	help  string
	label []string
}

// Histogram tracks request duration distributions via a fixed bucket set.
type Histogram struct {
	mu     sync.Mutex
	help   string
	label  []string
	bounds []float64
	counts []float64
	sum    float64
	count  float64
}

// Registry holds all instruments and renders the Prometheus text format.
type Registry struct {
	mu         sync.Mutex
	counters   map[string]*Counter
	gauges     map[string]*Gauge
	histograms map[string]*Histogram
}

// NewRegistry returns an empty metrics registry.
func NewRegistry() *Registry {
	return &Registry{
		counters:   map[string]*Counter{},
		gauges:     map[string]*Gauge{},
		histograms: map[string]*Histogram{},
	}
}

// NewCounter registers a counter with the given help text and label names.
func (r *Registry) NewCounter(name, help string, labels ...string) *Counter {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := &Counter{help: help, label: labels}
	r.counters[name] = c
	return c
}

// NewGauge registers a gauge with the given help text and label names.
func (r *Registry) NewGauge(name, help string, labels ...string) *Gauge {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := &Gauge{help: help, label: labels}
	r.gauges[name] = g
	return g
}

// NewHistogram registers a histogram with the given bounds (seconds).
func (r *Registry) NewHistogram(name, help string, bounds []float64, labels ...string) *Histogram {
	r.mu.Lock()
	defer r.mu.Unlock()
	h := &Histogram{help: help, label: labels, bounds: bounds, counts: make([]float64, len(bounds)+1)}
	r.histograms[name] = h
	return h
}

// Inc increments a counter by one.
func (c *Counter) Inc() { c.Add(1) }

// Add increments a counter by n.
func (c *Counter) Add(n float64) {
	c.mu.Lock()
	c.value += n
	c.mu.Unlock()
}

// Set sets a gauge to the given value.
func (g *Gauge) Set(n float64) {
	g.mu.Lock()
	g.value = n
	g.mu.Unlock()
}

// Inc increments a gauge by one.
func (g *Gauge) Inc() { g.Add(1) }

// Add increments a gauge by n.
func (g *Gauge) Add(n float64) {
	g.mu.Lock()
	g.value += n
	g.mu.Unlock()
}

// Observe records a duration observation in seconds.
func (h *Histogram) Observe(seconds float64) {
	h.mu.Lock()
	h.sum += seconds
	h.count++
	for i, b := range h.bounds {
		if seconds <= b {
			h.counts[i]++
			break
		}
	}
	if seconds > h.bounds[len(h.bounds)-1] {
		h.counts[len(h.counts)-1]++
	}
	h.mu.Unlock()
}

// Handler renders all registered metrics in Prometheus text exposition format.
func (r *Registry) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		var b []byte
		names := make([]string, 0, len(r.counters)+len(r.gauges)+len(r.histograms))
		r.mu.Lock()
		for n := range r.counters {
			names = append(names, n)
		}
		for n := range r.gauges {
			names = append(names, n)
		}
		for n := range r.histograms {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			if c, ok := r.counters[n]; ok {
				b = append(b, renderCounter(n, c)...)
			}
			if g, ok := r.gauges[n]; ok {
				b = append(b, renderGauge(n, g)...)
			}
			if h, ok := r.histograms[n]; ok {
				b = append(b, renderHistogram(n, h)...)
			}
		}
		r.mu.Unlock()
		_, _ = w.Write(b)
	})
}

func renderCounter(name string, c *Counter) []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return []byte(fmt.Sprintf("# HELP %s %s\n# TYPE %s counter\n%s %s\n",
		name, c.help, name, name, strconv.FormatFloat(c.value, 'f', -1, 64)))
}

func renderGauge(name string, g *Gauge) []byte {
	g.mu.Lock()
	defer g.mu.Unlock()
	return []byte(fmt.Sprintf("# HELP %s %s\n# TYPE %s gauge\n%s %s\n",
		name, g.help, name, name, strconv.FormatFloat(g.value, 'f', -1, 64)))
}

func renderHistogram(name string, h *Histogram) []byte {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := fmt.Sprintf("# HELP %s %s\n# TYPE %s histogram\n", name, h.help, name)
	cum := 0.0
	for i, b := range h.bounds {
		cum += h.counts[i]
		out += fmt.Sprintf("%s_bucket{le=%q} %s\n", name, strconv.FormatFloat(b, 'f', -1, 64), strconv.FormatFloat(cum, 'f', -1, 64))
	}
	cum += h.counts[len(h.counts)-1]
	out += fmt.Sprintf("%s_bucket{le=\"+Inf\"} %s\n", name, strconv.FormatFloat(cum, 'f', -1, 64))
	out += fmt.Sprintf("%s_sum %s\n", name, strconv.FormatFloat(h.sum, 'f', -1, 64))
	out += fmt.Sprintf("%s_count %s\n", name, strconv.FormatFloat(h.count, 'f', -1, 64))
	return []byte(out)
}

// defaultBounds are the latency bucket boundaries in seconds.
func defaultBounds() []float64 {
	return []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}
}

// NewDefaultHistogram builds a histogram with the standard latency buckets.
func (r *Registry) NewDefaultHistogram(name, help string) *Histogram {
	return r.NewHistogram(name, help, defaultBounds())
}
