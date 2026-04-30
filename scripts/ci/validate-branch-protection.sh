#!/usr/bin/env bash
# scripts/ci/validate-branch-protection.sh
# Validates scripts/setup-branch-protection.sh syntax and content.
# Covers task IDs: 11-02-01, 11-02-02 (requirements CI-03, CI-04)
#
# Usage: ./scripts/ci/validate-branch-protection.sh
# Exit: 0 on success, 1 on failure

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
BP_SCRIPT="${PROJECT_ROOT}/scripts/setup-branch-protection.sh"

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
echo "Branch Protection Script Validation"
echo "File: ${BP_SCRIPT}"
echo "========================================"

# ── 11-02-01: File exists and is executable ────────────────────

echo ""
echo "--- Gap 11-02-01: File and executable validation ---"

# Test: file exists
if test -f "${BP_SCRIPT}"; then
  pass "setup-branch-protection.sh file exists"
else
  fail "setup-branch-protection.sh file does not exist"
fi

# Test: file is executable
if test -x "${BP_SCRIPT}"; then
  pass "setup-branch-protection.sh is executable"
else
  fail "setup-branch-protection.sh is not executable"
fi

# Test: bash syntax is valid
if bash -n "${BP_SCRIPT}" 2>/dev/null; then
  pass "bash syntax is valid (bash -n)"
else
  fail "bash syntax check failed (bash -n)"
fi

# ── 11-02-02: Content validation ───────────────────────────────

echo ""
echo "--- Gap 11-02-02: Content validation ---"

# Test: set -euo pipefail
if grep -qE '^\s*set\s+-euo\s+pipefail' "${BP_SCRIPT}"; then
  pass "script uses 'set -euo pipefail'"
else
  fail "script does not use 'set -euo pipefail'"
fi

# Test: gh auth check before API calls
if grep -qE 'gh\s+auth\s+(status|check)' "${BP_SCRIPT}"; then
  pass "script checks 'gh auth status' before API calls"
else
  fail "script does not check 'gh auth status' before API calls"
fi

# Test: all 6 status check context names present
STATUS_CHECKS="lint fmt-check test-unit test-integration swagger-check build"
ALL_CHECKS_OK=true
for check in ${STATUS_CHECKS}; do
  if ! grep -q "\"context\":\s*\"${check}\"" "${BP_SCRIPT}"; then
    fail "status check '${check}' not found in script"
    ALL_CHECKS_OK=false
  fi
done
if test "${ALL_CHECKS_OK}" = "true"; then
  pass "all 6 status check context names present (lint, fmt-check, test-unit, test-integration, swagger-check, build)"
fi

# Test: enforce_admins true
if grep -qE '"enforce_admins":\s*true' "${BP_SCRIPT}"; then
  pass "enforce_admins is set to true"
else
  fail "enforce_admins is not set to true"
fi

# Test: allow_force_pushes false
if grep -qE '"allow_force_pushes":\s*false' "${BP_SCRIPT}"; then
  pass "allow_force_pushes is set to false"
else
  fail "allow_force_pushes is not set to false"
fi

# Test: required_approving_review_count 1
if grep -qE '"required_approving_review_count":\s*1' "${BP_SCRIPT}"; then
  pass "required_approving_review_count is 1"
else
  fail "required_approving_review_count is not 1"
fi

# Test: dismiss_stale_reviews true
if grep -qE '"dismiss_stale_reviews":\s*true' "${BP_SCRIPT}"; then
  pass "dismiss_stale_reviews is set to true"
else
  fail "dismiss_stale_reviews is not set to true"
fi

# Test: 422 fallback (retry on failure)
if grep -qE '(Fallback|fallback|retry|Retry|FULL_PAYLOAD|FALLBACK_PAYLOAD)' "${BP_SCRIPT}"; then
  pass "script has 422 fallback/retry mechanism for personal repos"
else
  fail "script does not have 422 fallback/retry mechanism"
fi

# Test: targets main branch
if grep -qE 'branches/main/protection' "${BP_SCRIPT}"; then
  pass "script targets 'main' branch for protection"
else
  fail "script does not target 'main' branch"
fi

# Test: uses PUT method
if grep -qE '\-\-method\s+PUT' "${BP_SCRIPT}"; then
  pass "script uses PUT method for branch protection API"
else
  fail "script does not use PUT method"
fi

# Test: gh CLI dependency check
if grep -qE 'command\s+-v\s+gh' "${BP_SCRIPT}"; then
  pass "script validates gh CLI availability before API calls"
else
  fail "script does not validate gh CLI availability"
fi

# Test: required_conversation_resolution true
if grep -qE '"required_conversation_resolution":\s*true' "${BP_SCRIPT}"; then
  pass "required_conversation_resolution is set to true"
else
  fail "required_conversation_resolution is not set to true"
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
