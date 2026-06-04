# SupportSentinel — Design Spec

**Hackathon track:** Track 4 — Autopilot Agent (QwenCloud / Alibaba Cloud)
**Date:** 2026-06-05
**Status:** Approved for planning

## 1. Summary

SupportSentinel is an autopilot agent that turns inbound support emails into
triaged, routed, and answered tickets end-to-end. It classifies each email's
**urgency** and **type**, routes it to the right department, drafts a reply, and
inserts **human-in-the-loop checkpoints** at the two critical decision points
(classification/routing and outbound reply). The AI core is **Qwen** via Alibaba
Cloud Model Studio / DashScope. The orchestration spine is a deterministic,
fully-audited **GoLang** state machine. Persistence is **PostgreSQL on Alibaba
Cloud RDS**.

Design emphasis follows the Track 4 judging criteria: **production-readiness over
toy demos** — auditability, resilience, evaluation/metrics, and a credible
deployment story.

## 2. Locked Decisions

| Area | Decision |
|---|---|
| AI core | Qwen-first via Alibaba Cloud Model Studio / DashScope |
| Backend language | GoLang |
| Ingestion | Two adapters feeding one pipeline: HTTP submit (built first, also test fixtures) + real IMAP poller |
| Human-in-the-loop | Two checkpoints — (1) validate classification/routing, (2) approve outbound reply. Both mandatory in this build. |
| Critical urgency | Always parks at Checkpoint 1 (human review) AND fires an alert, even at high confidence |
| Reply approval | Universal — every reply is human-approved before "send" |
| Reviewer interface | React web dashboard = approval surface (system of record); Slack/email = fire-and-forget urgent alerts only |
| Agent architecture | Deterministic Go state machine spine + Qwen tool-augmented reasoning at the classify step (Approach "A+C") |
| Taxonomies | Fixed enums (see §5) |
| Evaluation | Gold dataset + `make eval` harness with metrics + confidence calibration |
| Deployment | No Docker. `//go:embed` single Go binary (serves API + built React) + systemd + nginx (TLS) on Alibaba Cloud ECS; PostgreSQL on Alibaba Cloud RDS |

## 3. Architecture

```
                    ┌─────────────────────────────────────────────┐
   Email sources    │              Alibaba Cloud ECS               │
   ┌──────────┐     │  ┌────────────┐      ┌──────────────────┐    │
   │ IMAP poll│────▶│  │  Go binary │      │  React Dashboard │    │
   └──────────┘     │  │ (API +     │◀────▶│  (embedded,      │    │
   ┌──────────┐     │  │  orchestr.)│      │   reviewer UI)   │    │
   │POST /emails│──▶│  └─────┬──────┘      └──────────────────┘    │
   └──────────┘     │        │  classify / draft / tool calls      │
                    │        ▼                                      │
                    │  ┌────────────┐        ┌──────────────────┐  │
                    │  │ Qwen client│───────▶│ DashScope (Qwen) │  │ ◀ Alibaba Cloud AI
                    │  └────────────┘        │  Model Studio    │  │
                    │        │               └──────────────────┘  │
                    │        ▼                                      │
                    │  ┌────────────────────────────────────┐      │
                    │  │ PostgreSQL  ───────────────────────▶│──────┼─▶ Alibaba Cloud RDS
                    │  └────────────────────────────────────┘      │
                    │  nginx (TLS) ─▶ localhost:8080 │ systemd unit │
                    └─────────────────────────────────────────────┘
                             │  (urgent escalations, best-effort)
                             ▼
                       Slack / email alert  ──▶ link back to dashboard
```

### Components (each one clear job)

1. **Ingestion layer** — `http` submit adapter + `imap` poller adapter, both
   normalize to one internal `Email` struct and hand off to the orchestrator.
   Idempotent via a dedupe key (IMAP message-id, or hash(from+subject+body)).
2. **Orchestrator (the Agent core, Go)** — state machine; owns control flow,
   confidence thresholds, HITL gating; persists every transition.
3. **Qwen client** — wraps DashScope (OpenAI-compatible endpoint). `Classify`
   (structured JSON via schema/function-calling, may invoke tools) and
   `DraftReply`. Model pinned in config (`qwen-max` primary, `qwen-plus`
   fallback). **This file is the primary Alibaba Cloud proof artifact.**
4. **Tool layer** — external tools Qwen may call during classify:
   `lookup_customer(email)` and `lookup_similar_tickets(text)`, backed by
   Postgres. Satisfies "invokes external tools / handles ambiguous inputs."
5. **Routing & alerting** — sets department + state; fires best-effort Slack/email
   alert for urgent tickets (one-way; never blocks routing).
6. **Dashboard (React, embedded)** — ticket queue + detail (email, Qwen reasoning,
   confidence, tools used) + the two HITL controls + per-ticket audit timeline.
7. **Persistence (PostgreSQL / RDS)** — tickets, emails, classifications, replies,
   audit log, customers seed.
8. **Evaluation harness + gold dataset** — `make eval`: runs gold set through live
   Qwen, prints accuracy, per-class precision/recall/F1, confusion matrix, and
   confidence calibration used to set the HITL threshold.

## 4. Data Flow & State Machine

```
  NEW
   │  ingestion adapter normalizes email → creates ticket (idempotent)
   ▼
  CLASSIFYING ──────────────────────────────────────────────┐ may call tools:
   │   Qwen.Classify(email, tools)                           │  lookup_customer
   │   → {urgency, type, department, confidence, reasoning}  │  lookup_similar_tickets
   ▼                                                         └──────────────────────
  ┌──── confidence ≥ threshold  AND  urgency != critical ? ────┐
  │ YES                                                  NO     │
  ▼                                                             ▼
 ROUTED                                      AWAITING_CLASSIFICATION_REVIEW ◀ CHECKPOINT 1
  │  department set;                                │  dashboard: human confirms/
  │  if urgency=critical → fire alert               │  overrides urgency/type/dept
  │                                                 │  human submits →
  │◀─────────────────────────────────────────────────┘
  ▼
  DRAFTING
   │  Qwen.DraftReply(ticket) → proposed response text
   ▼
  AWAITING_REPLY_APPROVAL  ◀───────────────────────────────── CHECKPOINT 2
   │  dashboard: human approves / edits / rejects draft
   │  approve/edit →
   ▼
  RESOLVED  (reply recorded + marked resolved; optional real SMTP send; logged)
```

### Key behaviors
- **Confidence gate (Checkpoint 1):** `confidence < threshold` → parks in
  `AWAITING_CLASSIFICATION_REVIEW` instead of auto-routing. Threshold is
  calibrated by the eval harness, not guessed.
- **Critical override:** `urgency == critical` ALWAYS parks at Checkpoint 1 and
  fires the alert — never auto-routes regardless of confidence.
- **Checkpoint 2 universal:** every outbound reply is human-approved before send.
- **"Sent":** record final reply + mark resolved; optionally send real email via
  the IMAP account's SMTP creds (bonus, not core).
- **Audit log:** every transition writes `{ticket_id, from_state, to_state,
  actor (system|qwen|human:<id>), payload, created_at}`. Powers the dashboard
  timeline and the production-readiness narrative.

## 5. Fixed Taxonomies

- **Urgency:** `low` · `normal` · `high` · `critical`
- **Type:** `billing` · `technical` · `account` · `feature_request` · `general`
- **Department** (derived from type, overridable by human): `billing` ·
  `engineering` · `accounts` · `product` · `support_tier1`

These enums are the shared contract across classifier, router, dashboard, and eval.

## 6. Data Model (PostgreSQL)

**`tickets`** — core entity / current state
- `id` uuid PK, `state` text (NEW, CLASSIFYING, AWAITING_CLASSIFICATION_REVIEW,
  ROUTED, DRAFTING, AWAITING_REPLY_APPROVAL, RESOLVED, FAILED), `source` text
  (http|imap), `urgency` text (nullable), `type` text (nullable), `department`
  text (nullable), `confidence` numeric, `created_at`/`updated_at` timestamptz.

**`emails`** — raw inbound message (1:1 at creation)
- `id` uuid PK, `ticket_id` uuid FK, `from_addr`, `to_addr`, `subject`, `body`
  (plain-text normalized), `raw` jsonb (headers/metadata), `received_at` timestamptz.

**`classifications`** — append-only, every attempt
- `id` uuid PK, `ticket_id` uuid FK, `urgency`/`type`/`department` text,
  `confidence` numeric, `reasoning` text, `tools_used` jsonb, `model` text,
  `created_at` timestamptz.

**`replies`** — append-only drafts/approvals
- `id` uuid PK, `ticket_id` uuid FK, `draft_text` text, `final_text` text
  (nullable), `status` text (draft|approved|rejected), `created_at` timestamptz.

**`audit_log`** — the spine
- `id` uuid PK, `ticket_id` uuid FK, `from_state`/`to_state` text, `actor` text
  (system|qwen|human:<reviewer>), `payload` jsonb, `created_at` timestamptz.

**`customers`** — seed data so tool lookups return real signal
- `email` PK, `name`, `tier`, `account_status`.

Append-only classifications/replies/audit_log enable full ticket-life replay.

## 7. Error Handling & Resilience

Principle: **fail toward a human, never silently drop.**

- **Qwen call failure** (network/5xx/rate-limit): bounded exponential-backoff
  retry (≈3 attempts); on exhaustion → park in `AWAITING_CLASSIFICATION_REVIEW`
  flagged for a human.
- **Malformed model output:** one re-prompt with schema nudge; still bad → human
  review. Never crash the pipeline.
- **Ingestion idempotency:** dedupe key prevents duplicate tickets on IMAP re-poll.
- **Crash recovery:** state lives in Postgres; restarted orchestrator re-scans
  transient states (`CLASSIFYING`, `DRAFTING`) and resumes. No in-memory state.
- **Alert delivery:** best-effort; failure logged, never blocks routing.

### Alibaba Cloud / Qwen integration
- Go client → DashScope OpenAI-compatible endpoint; structured output via JSON
  schema / function-calling.
- **Proof of Alibaba Cloud** = `internal/qwen/client.go` (visible DashScope calls
  w/ Alibaba Cloud key) + RDS connection config as a second Alibaba service.
- Secrets via env (`EnvironmentFile` for systemd); `.env`/`app.env` gitignored,
  `app.env.example` documents required vars.
- Observability: structured JSON logs (request id, ticket id, state); `audit_log`
  is the domain-level observability surface.

## 8. Testing Strategy

- **Orchestrator state machine (priority, TDD):** table-driven tests over every
  transition incl. gated paths (below-threshold → review, critical-always-parks,
  Qwen-error → human fallback, invalid-JSON → re-prompt → fallback). Qwen mocked.
- **Qwen client:** contract tests for JSON parsing + re-prompt path against
  fixtures; one optional live smoke test behind a build tag.
- **Tool layer (TDD):** unit tests against a throwaway test Postgres.
- **Ingestion idempotency:** duplicate-email test.
- **Eval harness:** `make eval` quality report (not pass/fail) — accuracy,
  per-class F1, confusion matrix, confidence calibration. Source of demo metrics.
- **Frontend:** light component tests on review controls; otherwise manual/demo.

## 9. Deployment (No Docker)

- **Artifact:** React `npm run build` → `dist/`, embedded via `//go:embed`;
  cross-compiled single Go binary (`GOOS=linux GOARCH=amd64`).
- **ECS:** `systemd` service (auto-restart, start-on-boot, `journalctl` logs);
  `nginx` reverse proxy terminates TLS (Let's Encrypt/certbot) → `localhost:8080`.
- **Secrets:** `EnvironmentFile=/etc/supportsentinel/app.env` (Qwen key, RDS
  `DATABASE_URL`, IMAP creds).
- **Database:** Alibaba Cloud managed RDS PostgreSQL.
- **Deploy flow (`make deploy`):** build frontend → embed → cross-compile → scp →
  `systemctl restart supportsentinel`. Optional GitHub Action on push to `main`.
- **Local dev (`make dev`):** run binary against a local/dev Postgres
  (`DATABASE_URL` documented in `app.env.example`).

## 10. Repository Layout

```
/cmd/server            → main.go (wires everything, //go:embed frontend)
/internal/orchestrator → state machine
/internal/qwen         → DashScope client  (★ Alibaba Cloud proof file)
/internal/tools        → lookup_customer, lookup_similar_tickets
/internal/ingest       → http + imap adapters
/internal/store        → Postgres/RDS access, migrations
/internal/alert        → slack/email notifier
/frontend              → React app (built → embedded)
/eval                  → gold dataset + eval harness (make eval)
/deploy                → systemd unit, nginx conf, make deploy script
/docs                  → architecture diagram, specs
README.md  LICENSE(MIT)  app.env.example  Makefile  CLAUDE.md
```

`CLAUDE.md` is created as **implementation task #1** (seeded by hand from these
locked decisions: stack, taxonomies, make commands, no-Docker deploy facts,
"fail toward a human" principle, link to this spec) — not via `/init` on an empty
repo.

## 11. Submission Deliverables Map

| Requirement | Satisfied by |
|---|---|
| Public repo + OSS license (visible in About) | `LICENSE` (MIT) at root + set on GitHub repo |
| Proof of Alibaba Cloud (code file link) | `internal/qwen/client.go` (DashScope/Qwen) + RDS config; linked in README |
| Proof recording (backend on Alibaba Cloud) | Screen capture: `systemctl status` + `journalctl` + live request on ECS |
| Architecture diagram | Polished §3 diagram, committed as image + in README |
| ~3-min demo video (YouTube/Vimeo) | Scripted: ambiguous email → Qwen classify w/ tool lookups → low-confidence Checkpoint 1 → human validate → draft → Checkpoint 2 approve → resolved; show audit timeline + eval metrics |
| Text description | README features/functionality section |
| Track identification | README: "Track 4: Autopilot Agent" |
| Optional blog post | Stretch — build journal |

## 12. Out of Scope (YAGNI for this build)

- Auto-send replies without human approval (Checkpoint 2 is universal).
- Multi-tenant / auth-heavy reviewer accounts (single reviewer identity is fine).
- Kubernetes/container orchestration.
- Real outbound email is optional, not core.
- Agentic free-running LLM control loop (we use the deterministic spine).
