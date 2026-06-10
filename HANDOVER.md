# SupportSentinel — Handover & Onboarding

This document orients a new contributor (human or a fresh Claude Code account) taking over the project. It captures the context that is **not** obvious from the code or git history — read it first.

> **For a new Claude Code account:** the previous account's file-based memory does **not** transfer. This file + the specs/plans under `docs/superpowers/` are the source of truth. The repo follows the **superpowers** workflow: brainstorm → write spec → write plan → execute with `subagent-driven-development` → one PR per plan.

---

## What this is

**SupportSentinel** — an autopilot support-ticket agent for the **QwenCloud / Alibaba Cloud** hackathon, **Track 4: Autopilot Agent**. It turns inbound support emails into triaged, routed, and answered tickets end-to-end: **Qwen** (via Alibaba Cloud DashScope) classifies urgency + type, a deterministic Go state machine routes them, with **two human-in-the-loop checkpoints** (classification review + reply approval). Emphasis: production-readiness (auditability, resilience, evaluation), not a toy demo.

- **Repo:** https://github.com/lemonishi/supportsentinel (public, MIT)
- **Design spec:** `docs/superpowers/specs/2026-06-05-supportsentinel-design.md` (read this for the full architecture + locked decisions)
- **Plans:** `docs/superpowers/plans/` (one per increment; Plans 1–4 done, 5–7 pending)

## Status (as of 2026-06-10)

| Plan | What | State |
|---|---|---|
| 1 | Foundation & vertical slice (state machine, store, audit log, HTTP, both checkpoints — fake classifier) | ✅ merged (PR #1) |
| 2 | Qwen/DashScope client — the **Proof of Alibaba Cloud** file `internal/qwen/client.go` | ✅ merged (PR #2) |
| 3 | Tool layer — `lookup_customer` / `lookup_similar_tickets` via DashScope function-calling, recorded in `classifications.tools_used` | ✅ merged (PR #3) |
| 4 | React reviewer dashboard, embedded via `//go:embed` | ✅ merged (PR #4) |
| 5 | IMAP ingestion + Slack/email alerting | ⏭️ planned — `docs/superpowers/plans/2026-06-10-plan-5-imap-alerting.md` |
| 6 | Eval harness + gold dataset + threshold calibration | ⏭️ planned — `…/2026-06-10-plan-6-eval-harness.md` |
| 7 | Deployment (Alibaba Cloud ECS) + submission deliverables | ⏭️ planned — `…/2026-06-10-plan-7-deployment-submission.md` |

All of Plans 1–4 are live-validated against the **real DashScope endpoint** (classify, draft, and tool-calling all work end-to-end).

---

## ⚠️ Critical conventions (don't skip)

1. **Git author identity.** All commits MUST be authored as **`Lennon <lemoncode8888@gmail.com>`** (the owner's identity → GitHub `lemonishi`). The repo's local git config is already set to this — just use it; do **not** override the author email. (History was once polluted by a `sathwiik77@gmail.com` identity and had to be rewritten + force-pushed to remove a stray contributor. Don't reintroduce it.)
2. **Co-author trailer.** End commit messages with `Co-Authored-By: Claude <noreply@anthropic.com>` (adjust model name as appropriate).
3. **One PR per plan.** Branch `plan-N-...` off `main`, execute the plan, open a PR into `main`, merge. Docs/handover changes can go on a `docs-*` branch.
4. **Secrets.** The DashScope API key lives ONLY in the gitignored `app.env` (never `app.env.example`, never committed). If you ever see a key in a tracked file, scrub it before committing.

---

## Local environment

- **Go** 1.25 (toolchain auto-managed; module requires ≥1.25 due to pgx).
- **Node** 24 / npm 11 (for the `frontend/` dashboard).
- **PostgreSQL** runs locally via **Homebrew `postgresql@16` on port `5433`** (NOT the default 5432 — that port is held by another app's Docker container on the owner's machine). It's a brew service (auto-starts on login). Binaries at `/opt/homebrew/opt/postgresql@16/bin/`.
  - Role/creds: `postgres` / `postgres`. Databases: `supportsentinel` (dev) + `supportsentinel_test` (tests).
  - On a fresh machine: install Postgres, set port 5433, create the two DBs (`make test-db` does the latter), create a `postgres` superuser role with password `postgres`.
- **`app.env`** (gitignored; copy from `app.env.example`) holds:
  - `DATABASE_URL` / `TEST_DATABASE_URL` → `postgres://postgres:postgres@localhost:5433/supportsentinel[_test]?sslmode=disable`
  - `DASHSCOPE_API_KEY` → Alibaba Cloud Model Studio (DashScope) key. Region: **International (Singapore)**; default base URL `https://dashscope-intl.aliyuncs.com/compatible-mode/v1`.
  - `QWEN_MODEL` (default `qwen-max`), `CONFIDENCE_THRESHOLD` (default 0.75).
- The `Makefile` auto-loads `app.env` for `make` targets. Running a binary directly (`./bin/server`) does NOT load it — use `make` targets.

## Common commands (run from the repo root)

| Command | What |
|---|---|
| `make run` | Build the frontend + run the server natively with the dashboard at http://localhost:8080. **The one-command local run.** |
| `make dev` | Run the server natively (no frontend rebuild). |
| `make test` | `go test ./...` (DB-backed tests run because `app.env` sets `TEST_DATABASE_URL`; otherwise they skip). |
| `make frontend` | Build the React SPA into `internal/webui/dist` (embedded at compile time). |
| `make build` | **Linux/amd64** cross-compile for Alibaba Cloud ECS deploy — NOT runnable on macOS. Use `make run` locally. |
| `make eval` | (Plan 6) run the evaluation harness. |
| Live Qwen tests | `DASHSCOPE_API_KEY=… go test -tags live ./internal/qwen/ -run Live -v` (excluded from the normal suite). |

To populate the dashboard for a demo, POST a couple of emails (see any plan's validation section), e.g. a clear billing email (auto-routes → reply approval) and an "URGENT: production is down" email (parks at Checkpoint 1, fires an alert).

---

## Architecture in one screen

```
email ─▶ ingestion (HTTP /api/emails  + IMAP poller[Plan5]) ─▶ orchestrator (Go state machine)
                                                                   │  classify / draft
                                                                   ▼
                                          Qwen client (internal/qwen) ─▶ DashScope (Alibaba Cloud)
                                                                   │      + tools (internal/tools)
                                                                   ▼
                                          PostgreSQL (audit_log = every transition)
                                                                   │
        React dashboard (internal/webui, //go:embed) ◀─ read APIs ─┘   urgent ─▶ alerts (log; Slack/email[Plan5])
```

- **State machine:** `NEW → CLASSIFYING → {ROUTED | AWAITING_CLASSIFICATION_REVIEW} → DRAFTING → AWAITING_REPLY_APPROVAL → RESOLVED` (`FAILED` on unrecoverable error). Checkpoint 1 = confidence < threshold OR urgency == critical (critical ALWAYS parks + alerts). Checkpoint 2 = every reply human-approved.
- **Core principle:** **fail toward a human** — classifier/draft errors park the ticket for review, never drop it.
- **Every state change** goes through `store.Apply`, which writes the `audit_log` row in the same transaction. `classifications` / `replies` / `audit_log` are append-only.
- **Fixed taxonomies** (the hard contract — `internal/domain`): urgency `low|normal|high|critical`; type `billing|technical|account|feature_request|general`; department `billing|engineering|accounts|product|support_tier1`.

## Package map

```
cmd/server            main: wires config → store → classifier (Qwen if key else fake) → orchestrator → HTTP
internal/domain       taxonomies, core structs, Classifier interface (the shared contract)
internal/config       env config
internal/store        pgx; schema.sql; Apply (transactional transitions + audit); customers; dashboard reads
internal/classify     Fake (keyword) classifier — fallback when no DASHSCOPE_API_KEY
internal/qwen         DashScope client (★ Proof of Alibaba Cloud) — Classify (JSON + tool-calling loop), DraftReply
internal/tools        lookup_customer, lookup_similar_tickets (DashScope function-calling, store-backed)
internal/alert        Alerter interface; Log + Recording impls (Slack/email in Plan 5)
internal/orchestrator the agent core: state machine + the two checkpoints
internal/httpapi      ingestion + checkpoint actions + dashboard read endpoints; mounts webui at /
internal/webui        embeds the built SPA (//go:embed all:dist), serves it
frontend/             Vite + React + TS + Tailwind dashboard (built → internal/webui/dist)
eval/                 (Plan 6) gold dataset + metrics
deploy/               (Plan 7) systemd unit, nginx conf, deploy runbook
```

## Where to find things judges will ask about
- **Proof of Alibaba Cloud (code file):** `internal/qwen/client.go` (Bearer-auth DashScope calls) + RDS `DATABASE_URL`.
- **Tool invocation (Track 4 criterion):** `internal/qwen` Classify loop + `internal/tools`; persisted in `classifications.tools_used`.
- **Human-in-the-loop checkpoints:** `internal/orchestrator` (states `AWAITING_CLASSIFICATION_REVIEW`, `AWAITING_REPLY_APPROVAL`) + the dashboard controls.
- **Audit trail:** `audit_log` table; surfaced in the dashboard's per-ticket timeline.

## How to continue
Execute Plans 5 → 6 → 7 in order (each is self-contained, builds on the prior). Use `subagent-driven-development`: fresh implementer per task + spec-compliance then code-quality review, TDD on the Go side, one PR per plan. **Plan 7 is the finish line** — it gets the backend onto Alibaba Cloud ECS and assembles the submission package (README, architecture diagram, Proof-of-Alibaba-Cloud recording, ~3-min demo video, text description, Track identification).
