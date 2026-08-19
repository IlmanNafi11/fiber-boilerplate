# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed (BREAKING)

- **Error response contract.** General (non-validation) errors now return the machine
  `code` and human `message` at the **top level** of the response envelope. The `errors[]`
  array is reserved exclusively for field-level validation detail (`{ "field", "message" }`)
  and is absent on general and 5xx errors.

  Before:

  ```json
  { "success": false, "errors": [ { "code": "NOT_FOUND", "message": "user not found" } ] }
  ```

  After:

  ```json
  { "success": false, "message": "user not found", "code": "NOT_FOUND" }
  ```

  All 5xx responses collapse to an opaque `{ "code": "INTERNAL_ERROR", "message": "Internal
  Server Error" }` body while preserving the source HTTP status. See the "Response Envelope"
  section in `README.md` for the full contract and consumer migration steps.

  **Migration impact:** this is a breaking change to a public HTTP contract. If external
  consumers already depend on the legacy `errors[0].code` / `errors[0].message` shape, this
  change **must** be released under a **major version bump**. If no consumers depend on the
  legacy shape yet, it may ship in the initial release without a major bump.

### Added

- Dependency vulnerability gate: `make vulncheck` (pinned `govulncheck`) and a CI job that
  blocks the build on reachable Critical/High findings.
- Fail-closed production configuration: production rejects empty/wildcard `ALLOWED_ORIGINS`,
  malformed booleans, and an enabled Swagger UI before the server listens.
