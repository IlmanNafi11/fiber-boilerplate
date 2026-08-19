package docs

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// runbookPath resolves docs/operations/runbook.md relative to the repo root,
// derived from this test file's location so it is independent of the working
// directory the test runner uses.
func runbookPath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve caller location")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	return filepath.Join(repoRoot, "docs", "operations", "runbook.md")
}

func readRunbook(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(runbookPath(t))
	if err != nil {
		t.Fatalf("runbook not found (Task 9 acceptance): %v", err)
	}
	return string(data)
}

// TestRunbookExists asserts the operations runbook is present. Task 9 requires a
// production deploy/monitoring/rollback runbook to exist as a tracked artifact.
func TestRunbookExists(t *testing.T) {
	readRunbook(t)
}

// TestRunbookHasRequiredSections guards the runbook's structure so it keeps the
// operational content Task 9 mandates: exact deploy/rollback commands,
// monitoring/alerts, migrations, and a measured RTO.
func TestRunbookHasRequiredSections(t *testing.T) {
	content := readRunbook(t)
	required := []string{
		"Required environment",
		"Deploy",
		"Smoke",
		"Migration",
		"Monitoring",
		"Alert",
		"Rollback",
		"RTO",
	}
	for _, section := range required {
		if !strings.Contains(content, section) {
			t.Errorf("runbook missing required section keyword %q", section)
		}
	}
}

// TestRunbookReferencesImplementedMetrics ties the runbook's monitoring section
// to the metric names actually emitted by the code. If a metric is renamed in
// code without updating the runbook, this fails — preventing silent drift
// between telemetry and alert definitions.
func TestRunbookReferencesImplementedMetrics(t *testing.T) {
	content := readRunbook(t)
	metrics := []string{
		"http_requests_total",
		"http_request_duration_seconds",
		"outbox_dispatch_events_total",
		"outbox_dispatch_duration_seconds",
		"outbox_pending_events",
		"outbox_oldest_pending_age_seconds",
	}
	for _, m := range metrics {
		if !strings.Contains(content, m) {
			t.Errorf("runbook does not reference implemented metric %q", m)
		}
	}
}

// TestRunbookDocumentsRequiredAlerts checks each alert Task 9 enumerates is
// present, so the runbook's alert catalogue stays complete.
func TestRunbookDocumentsRequiredAlerts(t *testing.T) {
	content := strings.ToLower(readRunbook(t))
	alerts := map[string]string{
		"error rate":  "error rate",
		"P95 latency": "p95",
		"auth spike":  "auth failure",
		"oldest age":  "oldest pending",
		"dead-letter": "dead-letter",
	}
	for name, needle := range alerts {
		if !strings.Contains(content, needle) {
			t.Errorf("runbook missing alert coverage for %s (looked for %q)", name, needle)
		}
	}
}

// TestRunbookRecordsMeasuredRTO asserts the rollback dry-run left a concrete,
// measured recovery time rather than a placeholder.
func TestRunbookRecordsMeasuredRTO(t *testing.T) {
	content := readRunbook(t)
	if !strings.Contains(content, "measured") && !strings.Contains(content, "Measured") {
		t.Error("runbook must record a measured rollback time (RTO), not just a target")
	}
	for _, placeholder := range []string{"TODO", "TBD", "FIXME", "XXX"} {
		if strings.Contains(content, placeholder) {
			t.Errorf("runbook contains unresolved placeholder %q", placeholder)
		}
	}
}
