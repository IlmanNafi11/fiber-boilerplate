package http

import (
	"bytes"
	"errors"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v3"
	"github.com/ilmannafi/fiber-boilerplate/pkg/errx"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
)

// Metrics holds the application's HTTP RED (Rate, Errors, Duration) metrics.
//
// Cardinality is bounded by construction: the only labels are method, route
// template, and status class ("2xx".."5xx"). Raw URLs, path parameters, user
// IDs, tokens, and error text are never used as label values, so the series
// count stays proportional to routes × methods × 5, not to traffic.
//
// Each Metrics owns its own registry so servers and tests are isolated and can
// be constructed repeatedly without duplicate-registration panics.
type Metrics struct {
	registry  *prometheus.Registry
	gatherers prometheus.Gatherers
	requests  *prometheus.CounterVec
	duration  *prometheus.HistogramVec
}

// NewMetrics builds and registers the HTTP metric collectors.
func NewMetrics() *Metrics {
	reg := prometheus.NewRegistry()
	m := &Metrics{
		registry: reg,
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total HTTP requests by method, route template, and status class.",
		}, []string{"method", "route", "status_class"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration in seconds by method, route template, and status class.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "route", "status_class"}),
	}
	reg.MustRegister(m.requests, m.duration)
	m.gatherers = prometheus.Gatherers{reg}
	return m
}

// AddGatherer includes an additional metrics registry (e.g. the email
// dispatcher's) in the /metrics scrape output. Call before serving.
func (m *Metrics) AddGatherer(g prometheus.Gatherer) {
	if g == nil {
		return
	}
	m.gatherers = append(m.gatherers, g)
}

// observeHTTP records a single completed request. status is the numeric HTTP
// status; it is reduced to a bounded class label before being recorded.
func (m *Metrics) observeHTTP(method, route string, status int, seconds float64) {
	class := statusClass(status)
	m.requests.WithLabelValues(method, route, class).Inc()
	m.duration.WithLabelValues(method, route, class).Observe(seconds)
}

// Middleware records RED metrics for every request, keyed by the matched route
// template (not the raw URL) so cardinality stays bounded.
//
// Fiber's top-level ErrorHandler runs after the middleware chain unwinds, so a
// handler that returns an error (or a recovered panic, surfaced as an error)
// has not yet had its final status written when this middleware regains
// control. We therefore derive the status from the returned error rather than
// reading c.Response().StatusCode(), which would still show 200.
func (m *Metrics) Middleware() fiber.Handler {
	return func(c fiber.Ctx) error {
		start := time.Now()
		err := c.Next()

		route := c.Route().Path
		if route == "" {
			route = "unmatched"
		}
		status := c.Response().StatusCode()
		if err != nil {
			status = statusFromError(err)
		}
		m.observeHTTP(c.Method(), route, status, time.Since(start).Seconds())
		return err
	}
}

// statusFromError maps a handler error to the HTTP status the ErrorHandler will
// ultimately send, so 4xx client errors are not miscounted as 5xx. It mirrors
// the cases in response.ErrorHandler.
func statusFromError(err error) int {
	var appErr *errx.AppError
	if errors.As(err, &appErr) {
		return appErr.HTTPStatus
	}
	var valErrs validator.ValidationErrors
	if errors.As(err, &valErrs) {
		return 422
	}
	var fe *fiber.Error
	if errors.As(err, &fe) {
		return fe.Code
	}
	return 500
}

// Handler serves the metrics registry in Prometheus text exposition format.
func (m *Metrics) Handler() fiber.Handler {
	format := expfmt.NewFormat(expfmt.TypeTextPlain)
	return func(c fiber.Ctx) error {
		families, err := m.gatherers.Gather()
		if err != nil {
			return err
		}
		var buf bytes.Buffer
		enc := expfmt.NewEncoder(&buf, format)
		for _, mf := range families {
			if err := enc.Encode(mf); err != nil {
				return err
			}
		}
		c.Set(fiber.HeaderContentType, string(format))
		return c.Send(buf.Bytes())
	}
}

// statusClass reduces a numeric HTTP status to its class label.
func statusClass(status int) string {
	switch {
	case status >= 500:
		return "5xx"
	case status >= 400:
		return "4xx"
	case status >= 300:
		return "3xx"
	case status >= 200:
		return "2xx"
	default:
		return "1xx"
	}
}
