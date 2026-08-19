package http

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// findMetric returns the named metric family from the registry, or nil.
func findMetric(t *testing.T, reg *prometheus.Registry, name string) *dto.MetricFamily {
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

// labelValue returns the value of label from a metric, or "".
func labelValue(m *dto.Metric, label string) string {
	for _, l := range m.GetLabel() {
		if l.GetName() == label {
			return l.GetValue()
		}
	}
	return ""
}

func TestMetrics_HTTPRequestRecordsBoundedLabels(t *testing.T) {
	m := NewMetrics()

	m.observeHTTP("GET", "/api/v1/products/:id", 200, 0.012)

	fam := findMetric(t, m.registry, "http_requests_total")
	require.NotNil(t, fam, "http_requests_total must be registered")
	require.Len(t, fam.GetMetric(), 1)

	metric := fam.GetMetric()[0]
	// Only method, route, and status_class labels — no raw URL, id, or user.
	labelNames := map[string]string{}
	for _, l := range metric.GetLabel() {
		labelNames[l.GetName()] = l.GetValue()
	}
	assert.Equal(t, map[string]string{
		"method":       "GET",
		"route":        "/api/v1/products/:id",
		"status_class": "2xx",
	}, labelNames)
	assert.Equal(t, float64(1), metric.GetCounter().GetValue())

	dur := findMetric(t, m.registry, "http_request_duration_seconds")
	require.NotNil(t, dur, "http_request_duration_seconds must be registered")
	require.Len(t, dur.GetMetric(), 1)
	assert.Equal(t, uint64(1), dur.GetMetric()[0].GetHistogram().GetSampleCount())
}

func TestMetrics_StatusClassBuckets(t *testing.T) {
	m := NewMetrics()
	cases := map[int]string{200: "2xx", 302: "3xx", 404: "4xx", 500: "5xx"}
	for status := range cases {
		m.observeHTTP("GET", "/health", status, 0.001)
	}

	fam := findMetric(t, m.registry, "http_requests_total")
	require.NotNil(t, fam)
	got := map[string]bool{}
	for _, metric := range fam.GetMetric() {
		got[labelValue(metric, "status_class")] = true
	}
	for status, class := range cases {
		assert.True(t, got[class], "status %d should map to class %s", status, class)
	}
}

func TestMetrics_ErrorAndSuccessAreDistinguishable(t *testing.T) {
	m := NewMetrics()
	m.observeHTTP("POST", "/api/v1/auth/login", 200, 0.01)
	m.observeHTTP("POST", "/api/v1/auth/login", 500, 0.01)

	fam := findMetric(t, m.registry, "http_requests_total")
	require.NotNil(t, fam)

	var errors, success float64
	for _, metric := range fam.GetMetric() {
		switch labelValue(metric, "status_class") {
		case "5xx":
			errors += metric.GetCounter().GetValue()
		case "2xx":
			success += metric.GetCounter().GetValue()
		}
	}
	assert.Equal(t, float64(1), errors)
	assert.Equal(t, float64(1), success)
}

func TestMetrics_HandlerExposesRegisteredSeries(t *testing.T) {
	m := NewMetrics()
	m.observeHTTP("GET", "/health", 200, 0.001)

	app := fiber.New()
	app.Get("/metrics", m.Handler())

	req := httptest.NewRequest("GET", "/metrics", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	text := string(body)
	assert.True(t, strings.Contains(text, "http_requests_total"),
		"/metrics output must expose http_requests_total")
}

// TestServer_MetricsEndpointRecordsRequests asserts the wired server exposes
// /metrics and that traffic through the middleware is recorded with the route
// template rather than the raw path.
func TestServer_MetricsEndpointRecordsRequests(t *testing.T) {
	app := NewServer(testConfig("*"), zap.NewNop(), nil)

	// Drive a successful request through the middleware; "/" returns 200.
	_, err := app.Test(httptest.NewRequest("GET", "/", nil))
	require.NoError(t, err)

	resp, err := app.Test(httptest.NewRequest("GET", "/metrics", nil))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	text := string(body)
	assert.Contains(t, text, "http_requests_total")
	assert.Contains(t, text, `route="/"`)
}

// TestServer_MetricsRecordsRecoveredPanicAs5xx proves the metrics middleware
// wraps recover: a handler panic is recovered into a 500 and still counted.
func TestServer_MetricsRecordsRecoveredPanicAs5xx(t *testing.T) {
	app := NewServer(testConfig("*"), zap.NewNop(), nil)

	// "/panic" is registered outside production and panics; recover turns it
	// into a 500 response.
	resp, err := app.Test(httptest.NewRequest("GET", "/panic", nil))
	require.NoError(t, err)
	assert.Equal(t, 500, resp.StatusCode)

	metricsResp, err := app.Test(httptest.NewRequest("GET", "/metrics", nil))
	require.NoError(t, err)
	defer metricsResp.Body.Close()
	body, err := io.ReadAll(metricsResp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), `status_class="5xx"`)
}

// TestServer_ExposesExtraGathererMetrics proves an extra registry (e.g. the
// email dispatcher's) passed to NewServer is scraped alongside HTTP metrics.
func TestServer_ExposesExtraGathererMetrics(t *testing.T) {
	extra := prometheus.NewRegistry()
	g := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "outbox_pending_events",
		Help: "test gauge",
	})
	extra.MustRegister(g)
	g.Set(3)

	app := NewServer(testConfig("*"), zap.NewNop(), nil, extra)

	resp, err := app.Test(httptest.NewRequest("GET", "/metrics", nil))
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "outbox_pending_events")
}
