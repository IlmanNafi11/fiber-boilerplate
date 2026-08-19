package email

import (
	"github.com/prometheus/client_golang/prometheus"
)

// OutboxMetrics holds operational metrics for the email outbox dispatcher.
//
// Cardinality is bounded: outcome counters are labelled only by event_type
// (three values) and outcome ("sent"/"retry"/"dead"). Recipients, tokens,
// payloads, and error text are never used as labels. Gauges are unlabelled.
//
// A nil *OutboxMetrics is a valid no-op, so callers may run without metrics.
type OutboxMetrics struct {
	registry      *prometheus.Registry
	events        *prometheus.CounterVec
	duration      *prometheus.HistogramVec
	pending       prometheus.Gauge
	oldestPending prometheus.Gauge
}

// NewOutboxMetrics builds and registers the outbox collectors.
func NewOutboxMetrics() *OutboxMetrics {
	reg := prometheus.NewRegistry()
	m := &OutboxMetrics{
		registry: reg,
		events: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "outbox_dispatch_events_total",
			Help: "Email outbox delivery outcomes by event type and outcome (sent/retry/dead).",
		}, []string{"event_type", "outcome"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "outbox_dispatch_duration_seconds",
			Help:    "Email outbox per-event send duration in seconds by event type.",
			Buckets: prometheus.DefBuckets,
		}, []string{"event_type"}),
		pending: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "outbox_pending_events",
			Help: "Number of email outbox events currently pending delivery.",
		}),
		oldestPending: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "outbox_oldest_pending_age_seconds",
			Help: "Age in seconds of the oldest pending email outbox event (0 when none).",
		}),
	}
	reg.MustRegister(m.events, m.duration, m.pending, m.oldestPending)
	return m
}

// Registry exposes the collectors for scraping. Returns nil for a nil receiver.
func (m *OutboxMetrics) Registry() *prometheus.Registry {
	if m == nil {
		return nil
	}
	return m.registry
}

// recordOutcome records a single delivery outcome and its send duration.
func (m *OutboxMetrics) recordOutcome(eventType, outcome string, seconds float64) {
	if m == nil {
		return
	}
	m.events.WithLabelValues(eventType, outcome).Inc()
	m.duration.WithLabelValues(eventType).Observe(seconds)
}

// recordPending updates the pending-backlog gauges. count is the number of
// pending events; oldestAgeSeconds is the age of the oldest one (0 when none).
func (m *OutboxMetrics) recordPending(count int, oldestAgeSeconds float64) {
	if m == nil {
		return
	}
	m.pending.Set(float64(count))
	m.oldestPending.Set(oldestAgeSeconds)
}
