package email

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gatherFamily(t *testing.T, reg *prometheus.Registry, name string) *dto.MetricFamily {
	t.Helper()
	families, err := reg.Gather()
	require.NoError(t, err)
	for _, f := range families {
		if f.GetName() == name {
			return f
		}
	}
	return nil
}

func labelVal(m *dto.Metric, name string) string {
	for _, l := range m.GetLabel() {
		if l.GetName() == name {
			return l.GetValue()
		}
	}
	return ""
}

func TestOutboxMetrics_RecordsOutcomesByEventTypeAndOutcome(t *testing.T) {
	m := NewOutboxMetrics()

	m.recordOutcome("verification", "sent", 0.02)
	m.recordOutcome("verification", "sent", 0.03)
	m.recordOutcome("password_reset", "retry", 0.5)
	m.recordOutcome("password_reset", "dead", 1.0)

	fam := gatherFamily(t, m.registry, "outbox_dispatch_events_total")
	require.NotNil(t, fam, "outbox_dispatch_events_total must be registered")

	counts := map[string]float64{}
	for _, metric := range fam.GetMetric() {
		key := labelVal(metric, "event_type") + "/" + labelVal(metric, "outcome")
		counts[key] = metric.GetCounter().GetValue()
		// Labels are bounded: no recipient/token/error text.
		assert.Equal(t, "", labelVal(metric, "recipient"))
		assert.Equal(t, "", labelVal(metric, "error"))
	}
	assert.Equal(t, float64(2), counts["verification/sent"])
	assert.Equal(t, float64(1), counts["password_reset/retry"])
	assert.Equal(t, float64(1), counts["password_reset/dead"])

	dur := gatherFamily(t, m.registry, "outbox_dispatch_duration_seconds")
	require.NotNil(t, dur, "outbox_dispatch_duration_seconds must be registered")
}

func TestOutboxMetrics_PendingGauges(t *testing.T) {
	m := NewOutboxMetrics()

	m.recordPending(7, 42.5)

	count := gatherFamily(t, m.registry, "outbox_pending_events")
	require.NotNil(t, count)
	require.Len(t, count.GetMetric(), 1)
	assert.Equal(t, float64(7), count.GetMetric()[0].GetGauge().GetValue())

	age := gatherFamily(t, m.registry, "outbox_oldest_pending_age_seconds")
	require.NotNil(t, age)
	require.Len(t, age.GetMetric(), 1)
	assert.Equal(t, float64(42.5), age.GetMetric()[0].GetGauge().GetValue())
}

func TestOutboxMetrics_NilIsSafe(t *testing.T) {
	// A nil *OutboxMetrics must be a no-op so the dispatcher can run without
	// metrics wired (e.g. in unit tests) without panicking.
	var m *OutboxMetrics
	assert.NotPanics(t, func() {
		m.recordOutcome("verification", "sent", 0.01)
		m.recordPending(1, 1.0)
	})
}
