#!/usr/bin/env bash
# scripts/ci/validate-ci-workflow.sh
# Validates .github/workflows/ci.yml structure and content.
# Covers task IDs: 11-01-01, 11-01-02 (requirements CI-01)
#
# Usage: ./scripts/ci/validate-ci-workflow.sh
# Exit: 0 on success, 1 on failure

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
CI_FILE="${PROJECT_ROOT}/.github/workflows/ci.yml"

PASSED=0
FAILED=0

pass() {
  echo "  PASS: $1"
  PASSED=$((PASSED + 1))
}

fail() {
  echo "  FAIL: $1"
  FAILED=$((FAILED + 1))
}

echo "========================================"
echo "CI Workflow Validation"
echo "File: ${CI_FILE}"
echo "========================================"

# ── 11-01-01: File exists and YAML parses ─────────────────────

echo ""
echo "--- Gap 11-01-01: Structure validation ---"

# Test: file exists
if test -f "${CI_FILE}"; then
  pass "ci.yml file exists"
else
  fail "ci.yml file does not exist"
fi

# Test: YAML parses without error
if python3 -c "import yaml; yaml.safe_load(open('${CI_FILE}'))" 2>/dev/null; then
  pass "ci.yml is valid YAML"
else
  fail "ci.yml has invalid YAML syntax"
fi

# Test: workflow name is "CI"
WORKFLOW_NAME=$(python3 -c "
import yaml
data = yaml.safe_load(open('${CI_FILE}'))
print(data.get('name', ''))
" 2>/dev/null)
if test "${WORKFLOW_NAME}" = "CI"; then
  pass "workflow name is 'CI'"
else
  fail "workflow name is '${WORKFLOW_NAME}', expected 'CI'"
fi

# Test: triggers on push to dev
# Note: PyYAML parses YAML 'on' as Python boolean True, so we use data[True]
TRIGGER_DEV=$(python3 -c "
import yaml
data = yaml.safe_load(open('${CI_FILE}'))
trigger = data.get(True, data.get('on', {}))
branches = trigger.get('push', {}).get('branches', [])
print('dev' in branches)
" 2>/dev/null)
if test "${TRIGGER_DEV}" = "True"; then
  pass "triggers on push to 'dev' branch"
else
  fail "does not trigger on push to 'dev' branch"
fi

# Test: triggers on pull_request to main
# Note: PyYAML parses YAML 'on' as Python boolean True, so we use data[True]
TRIGGER_MAIN=$(python3 -c "
import yaml
data = yaml.safe_load(open('${CI_FILE}'))
trigger = data.get(True, data.get('on', {}))
branches = trigger.get('pull_request', {}).get('branches', [])
print('main' in branches)
" 2>/dev/null)
if test "${TRIGGER_MAIN}" = "True"; then
  pass "triggers on pull_request to 'main' branch"
else
  fail "does not trigger on pull_request to 'main' branch"
fi

# Test: exactly 6 jobs
JOBS=$(python3 -c "
import yaml
data = yaml.safe_load(open('${CI_FILE}'))
jobs = list(data.get('jobs', {}).keys())
print(len(jobs))
" 2>/dev/null)
if test "${JOBS}" = "6"; then
  pass "workflow has exactly 6 jobs"
else
  fail "workflow has ${JOBS} jobs, expected 6"
fi

# Test: correct job names
EXPECTED_JOBS="build fmt-check lint swagger-check test-integration test-unit"
ACTUAL_JOBS=$(python3 -c "
import yaml
data = yaml.safe_load(open('${CI_FILE}'))
jobs = sorted(data.get('jobs', {}).keys())
print(' '.join(jobs))
" 2>/dev/null)
if test "${ACTUAL_JOBS}" = "${EXPECTED_JOBS}"; then
  pass "job names are: lint, fmt-check, test-unit, test-integration, swagger-check, build"
else
  fail "job names are '${ACTUAL_JOBS}', expected '${EXPECTED_JOBS}'"
fi

# Test: build needs all 5 other jobs
BUILD_NEEDS=$(python3 -c "
import yaml
data = yaml.safe_load(open('${CI_FILE}'))
needs = sorted(data['jobs']['build'].get('needs', []))
print(' '.join(needs))
" 2>/dev/null)
EXPECTED_NEEDS="fmt-check lint swagger-check test-integration test-unit"
if test "${BUILD_NEEDS}" = "${EXPECTED_NEEDS}"; then
  pass "build job depends on all 5 other jobs"
else
  fail "build job needs are '${BUILD_NEEDS}', expected '${EXPECTED_NEEDS}'"
fi

# ── 11-01-02: Content validation (action versions, permissions, go-version-file) ──

echo ""
echo "--- Gap 11-01-02: Content validation ---"

# Test: permissions are read-only
PERMISSIONS=$(python3 -c "
import yaml
data = yaml.safe_load(open('${CI_FILE}'))
perm = data.get('permissions', {})
print(perm.get('contents', ''))
" 2>/dev/null)
if test "${PERMISSIONS}" = "read"; then
  pass "permissions.contents is 'read' (least privilege)"
else
  fail "permissions.contents is '${PERMISSIONS}', expected 'read'"
fi

# Test: golangci-lint-action@v7 with version v2.11
LINT_ACTION=$(python3 -c "
import yaml
data = yaml.safe_load(open('${CI_FILE}'))
steps = data['jobs']['lint']['steps']
for step in steps:
    uses = step.get('uses', '')
    if 'golangci-lint' in uses:
        print(uses)
        break
" 2>/dev/null)
if test "${LINT_ACTION}" = "golangci/golangci-lint-action@v7"; then
  pass "lint job uses golangci/golangci-lint-action@v7"
else
  fail "lint job uses '${LINT_ACTION}', expected 'golangci/golangci-lint-action@v7'"
fi

LINT_VERSION=$(python3 -c "
import yaml
data = yaml.safe_load(open('${CI_FILE}'))
steps = data['jobs']['lint']['steps']
for step in steps:
    if 'golangci-lint' in step.get('uses', ''):
        print(step.get('with', {}).get('version', ''))
        break
" 2>/dev/null)
if test "${LINT_VERSION}" = "v2.11"; then
  pass "lint job uses golangci-lint version v2.11"
else
  fail "lint job golangci-lint version is '${LINT_VERSION}', expected 'v2.11'"
fi

# Test: all jobs use go-version-file: 'go.mod'
GO_VERSION_FILE_JOBS=$(python3 -c "
import yaml
data = yaml.safe_load(open('${CI_FILE}'))
expected_jobs = ['lint', 'fmt-check', 'test-unit', 'test-integration', 'swagger-check', 'build']
missing = []
for job_name in expected_jobs:
    steps = data['jobs'][job_name].get('steps', [])
    found = False
    for step in steps:
        gv = step.get('with', {}).get('go-version-file', '')
        if gv == 'go.mod':
            found = True
            break
    if not found:
        missing.append(job_name)
if missing:
    print('MISSING:' + ','.join(missing))
else:
    print('ALL_OK')
" 2>/dev/null)
if test "${GO_VERSION_FILE_JOBS}" = "ALL_OK"; then
  pass "all 6 jobs use go-version-file: 'go.mod'"
else
  fail "jobs missing go-version-file: ${GO_VERSION_FILE_JOBS}"
fi

# Test: parallel jobs have no needs (lint, fmt-check, test-unit, test-integration, swagger-check)
PARALLEL_OK=$(python3 -c "
import yaml
data = yaml.safe_load(open('${CI_FILE}'))
parallel_jobs = ['lint', 'fmt-check', 'test-unit', 'test-integration', 'swagger-check']
has_needs = []
for job_name in parallel_jobs:
    needs = data['jobs'][job_name].get('needs')
    if needs is not None:
        has_needs.append(job_name)
if has_needs:
    print('HAS_NEEDS:' + ','.join(has_needs))
else:
    print('ALL_PARALLEL')
" 2>/dev/null)
if test "${PARALLEL_OK}" = "ALL_PARALLEL"; then
  pass "lint, fmt-check, test-unit, test-integration, swagger-check have no inter-dependencies"
else
  fail "parallel jobs unexpectedly have needs: ${PARALLEL_OK}"
fi

# Test: actions/checkout@v6 in all jobs
CHECKOUT_VERSION=$(python3 -c "
import yaml
data = yaml.safe_load(open('${CI_FILE}'))
expected_jobs = ['lint', 'fmt-check', 'test-unit', 'test-integration', 'swagger-check', 'build']
wrong = []
for job_name in expected_jobs:
    steps = data['jobs'][job_name].get('steps', [])
    found = False
    for step in steps:
        if step.get('uses', '') == 'actions/checkout@v6':
            found = True
            break
    if not found:
        wrong.append(job_name)
if wrong:
    print('WRONG:' + ','.join(wrong))
else:
    print('ALL_OK')
" 2>/dev/null)
if test "${CHECKOUT_VERSION}" = "ALL_OK"; then
  pass "all 6 jobs use actions/checkout@v6"
else
  fail "jobs not using actions/checkout@v6: ${CHECKOUT_VERSION}"
fi

# Test: actions/setup-go@v5 in all jobs
SETUP_GO_VERSION=$(python3 -c "
import yaml
data = yaml.safe_load(open('${CI_FILE}'))
expected_jobs = ['lint', 'fmt-check', 'test-unit', 'test-integration', 'swagger-check', 'build']
wrong = []
for job_name in expected_jobs:
    steps = data['jobs'][job_name].get('steps', [])
    found = False
    for step in steps:
        if step.get('uses', '') == 'actions/setup-go@v5':
            found = True
            break
    if not found:
        wrong.append(job_name)
if wrong:
    print('WRONG:' + ','.join(wrong))
else:
    print('ALL_OK')
" 2>/dev/null)
if test "${SETUP_GO_VERSION}" = "ALL_OK"; then
  pass "all 6 jobs use actions/setup-go@v5"
else
  fail "jobs not using actions/setup-go@v5: ${SETUP_GO_VERSION}"
fi

# ── Summary ────────────────────────────────────────────────────

echo ""
echo "========================================"
TOTAL=$((PASSED + FAILED))
echo "Results: ${PASSED}/${TOTAL} passed, ${FAILED}/${TOTAL} failed"
echo "========================================"

if test "${FAILED}" -gt 0; then
  exit 1
fi

exit 0
