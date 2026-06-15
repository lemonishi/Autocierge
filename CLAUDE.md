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
- Tool layer: `internal/tools` (DashScope function-calling) — `lookup_customer` and
  `lookup_similar_tickets`, store-backed, attached via `qwen.Client.WithTools`. The
  classifier invokes them during Classify; invocations are recorded in
  `classifications.tools_used`. Demo customers seeded at server startup.
- Ingestion: two sources feed the same idempotent `orchestrator.Ingest` (dedupe on
  Message-ID). (1) HTTP `POST /api/emails` (`internal/httpapi`). (2) IMAP poller
  (`internal/ingest/imap`) — a background goroutine watching a mailbox, started by
  `cmd/server` only when `IMAPEnabled()` (i.e. `IMAP_HOST` set); polls UNSEEN, parses
  via `ingest.ParseRFC822`, ingests source `"imap"`, marks `\Seen`. Best-effort per
  message; off by default. (Live IMAP validation is a manual step — point `IMAP_*`
  at a real inbox.) Uses `github.com/emersion/go-imap/v2` (beta).
- Alerting: `internal/alert` — best-effort `Multi` fan-out (`alert.FromConfig`) of Log +
  Slack webhook + SMTP email, activated by `SLACK_WEBHOOK_URL` / `SMTP_*` config.
  Failures are logged and never block the pipeline.
- Evaluation: `internal/eval` + `cmd/eval` — gold dataset (`eval/gold.jsonl`, ~30
  labeled support emails) run through the classifier to produce a quality report:
  overall accuracy, per-class precision/recall/F1, confusion matrices (urgency &
  type), and a confidence-threshold calibration sweep that recommends
  `CONFIDENCE_THRESHOLD` (calibrated, not guessed). `make eval` replays a committed
  cache (`eval/recorded.json`, bootstrapped from the fake classifier — free,
  deterministic, no API key); `make eval-live` refreshes it via real Qwen (spends
  quota). The report recommends the threshold; a human sets it in `app.env`. Pure
  metrics in `internal/eval` are unit-tested; the report is the source of demo metrics.
- DB: PostgreSQL (local for dev/test; Alibaba Cloud RDS in prod).
- Dashboard: `frontend/` (Vite + React + TS + Tailwind v4), two-pane reviewer console
  (queue + detail with reasoning/confidence/tools-used + both checkpoint controls +
  audit timeline). Built into `internal/webui/dist` and embedded via `//go:embed`;
  served at `/` by the Go binary (`internal/webui`). Read endpoints: `GET /api/tickets`,
  `/api/tickets/{id}/detail`, `/api/tickets/{id}/audit`. Dev: `cd frontend && npm run dev`
  (proxies /api → :8080). Build: `make frontend` then `go build`.
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
