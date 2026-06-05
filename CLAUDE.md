# SupportSentinel — Project Memory

Autopilot support-ticket agent. Hackathon Track 4 (QwenCloud / Alibaba Cloud).
Full design: `docs/superpowers/specs/2026-06-05-supportsentinel-design.md`.
Plans: `docs/superpowers/plans/`.

## Stack
- Backend: Go 1.22+ (net/http ServeMux, jackc/pgx v5, google/uuid, testify).
- AI core: Qwen via Alibaba Cloud DashScope — IMPLEMENTED in `internal/qwen/client.go`
  (the primary "Proof of Alibaba Cloud" artifact). OpenAI-compatible endpoint,
  JSON-mode classification with validation + one re-prompt, free-text drafts,
  bounded retry. `cmd/server` uses it when `DASHSCOPE_API_KEY` is set, else the
  fake classifier. Live smoke test: `go test -tags live ./internal/qwen/`.
- DB: PostgreSQL (local for dev/test; Alibaba Cloud RDS in prod).
- Frontend: React, built and embedded via `//go:embed` (Plan 4).
- Deploy: NO Docker. Single Go binary + systemd + nginx on Alibaba Cloud ECS (Plan 7).

## Fixed taxonomies (hard contract — see internal/domain)
- Urgency: low | normal | high | critical
- Type: billing | technical | account | feature_request | general
- Department: billing | engineering | accounts | product | support_tier1

## State machine
NEW → CLASSIFYING → {ROUTED | AWAITING_CLASSIFICATION_REVIEW} → DRAFTING →
AWAITING_REPLY_APPROVAL → RESOLVED (FAILED on unrecoverable error).
- Checkpoint 1: confidence < threshold OR urgency == critical → park for human review.
- Checkpoint 2: every reply is human-approved before send.

## Core principles
- FAIL TOWARD A HUMAN — never silently drop a ticket. Errors route to review.
- Every state change goes through Store.Apply, which writes the audit_log row in
  the same transaction. Do not mutate ticket.state outside Apply.
- classifications / replies / audit_log are APPEND-ONLY (full replay).

## Commands
- `make dev`   run server
- `make test`  run all tests (DB tests skip unless TEST_DATABASE_URL is set)
- `make build` cross-compile linux/amd64 binary
- `make test-db` create local test database
