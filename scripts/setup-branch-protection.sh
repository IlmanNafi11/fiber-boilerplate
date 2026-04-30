#!/usr/bin/env bash
# scripts/setup-branch-protection.sh
# Configures branch protection on the main branch for this repository.
# Requires: gh CLI (https://cli.github.com), authenticated with repo admin access.
#
# Usage: ./scripts/setup-branch-protection.sh <owner>/<repo>
# Example: ./scripts/setup-branch-protection.sh myorg/my-api

set -euo pipefail

# -- Validate arguments -----------------------------------------------
REPO="${1:?Usage: $0 <owner>/<repo>}"

# -- Check dependencies -----------------------------------------------
if ! command -v gh >/dev/null 2>&1; then
  echo "Error: gh CLI is required but not installed."
  echo "Install from https://cli.github.com or run: brew install gh"
  exit 1
fi

# -- Verify authentication --------------------------------------------
if ! gh auth status >/dev/null 2>&1; then
  echo "Error: Not authenticated with GitHub."
  echo "Run 'gh auth login' first."
  exit 1
fi

# -- Verify repository exists and is accessible -----------------------
if ! gh repo view "${REPO}" >/dev/null 2>&1; then
  echo "Error: Repository '${REPO}' not found or not accessible."
  echo "Check that the repository exists and you have admin access."
  exit 1
fi

echo "Setting up branch protection for '${REPO}' on branch 'main'..."

# -- Apply branch protection ------------------------------------------
# Status check names must exactly match the CI job names in .github/workflows/ci.yml

apply_protection() {
  local payload="$1"
  echo "${payload}" | gh api \
    --method PUT \
    -H "Accept: application/vnd.github+json" \
    "/repos/${REPO}/branches/main/protection" \
    --input -
}

# Full payload with restrictions: null (works for org repos and most personal repos)
FULL_PAYLOAD='{
  "required_status_checks": {
    "strict": false,
    "checks": [
      {"context": "lint"},
      {"context": "fmt-check"},
      {"context": "test-unit"},
      {"context": "test-integration"},
      {"context": "swagger-check"},
      {"context": "build"}
    ]
  },
  "enforce_admins": true,
  "required_pull_request_reviews": {
    "required_approving_review_count": 1,
    "dismiss_stale_reviews": true
  },
  "restrictions": null,
  "allow_force_pushes": false,
  "required_conversation_resolution": true
}'

# Fallback payload without restrictions field (for personal repos that reject restrictions: null)
# Uses jq to remove the restrictions key, or a fixed payload without it.
FALLBACK_PAYLOAD='{
  "required_status_checks": {
    "strict": false,
    "checks": [
      {"context": "lint"},
      {"context": "fmt-check"},
      {"context": "test-unit"},
      {"context": "test-integration"},
      {"context": "swagger-check"},
      {"context": "build"}
    ]
  },
  "enforce_admins": true,
  "required_pull_request_reviews": {
    "required_approving_review_count": 1,
    "dismiss_stale_reviews": true
  },
  "allow_force_pushes": false,
  "required_conversation_resolution": true
}'

if ! apply_protection "${FULL_PAYLOAD}" 2>/dev/null; then
  echo "Full payload failed (possible personal repo restriction). Retrying without restrictions field..."
  if ! apply_protection "${FALLBACK_PAYLOAD}"; then
    echo "Error: Failed to configure branch protection."
    echo "Ensure you have admin access to the repository and the main branch exists."
    exit 1
  fi
fi

echo ""
echo "Branch protection configured successfully for '${REPO}/main':"
echo "  - Requires pull request with 1 approval"
echo "  - All CI status checks must pass (lint, fmt-check, test-unit, test-integration, swagger-check, build)"
echo "  - Force push disabled"
echo "  - Admins are subject to the same rules (enforce_admins: true)"
echo "  - Stale reviews are dismissed on new commits"
echo "  - Conversations must be resolved before merge"
