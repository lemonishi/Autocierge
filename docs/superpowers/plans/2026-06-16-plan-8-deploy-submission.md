# Plan 8 — Alibaba Cloud Deployment & Submission Deliverables

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship everything needed to deploy SupportSentinel on Alibaba Cloud ECS (no Docker — single Go binaries + systemd + nginx, PostgreSQL on RDS) and to submit it for Track 4: a centerpiece README with architecture/state diagrams, deployment artifacts + runbook, a submission deliverables map with demo/proof scripts, and a demo seed script — all framed to surface the four judging criteria.

**Architecture:** Two cross-compiled binaries (`server`, `mcp-server`) run as systemd units on one ECS instance; nginx terminates TLS (self-signed, over the public IP — no domain) and reverse-proxies to the main server on `localhost:8080`; the MCP tool server stays localhost-only on `:8090`; PostgreSQL is Alibaba Cloud RDS (schema auto-applied on startup). Submission docs live in the repo: README.md (with Mermaid diagrams), `docs/SUBMISSION.md`, and `deploy/`.

**Tech Stack:** Go 1.25 cross-compile (`CGO_ENABLED=0 GOOS=linux GOARCH=amd64`); systemd; nginx + OpenSSL self-signed TLS; Alibaba Cloud ECS + RDS PostgreSQL; Bash deploy/seed scripts; Mermaid diagrams (render natively on GitHub).

**Design decisions (locked in brainstorming, 2026-06-16):**
- **No Docker** — single binaries + systemd + nginx (per the project spec §9).
- **No domain → self-signed TLS over the ECS public IP** (nginx terminates; browser cert warning is acceptable for the demo).
- **Two systemd units** — `supportsentinel.service` (API + embedded dashboard) and `supportsentinel-mcp.service` (MCP tool server, localhost-only). Prod sets `MCP_SERVER_URL=http://127.0.0.1:8090/mcp`.
- **Deploy = config files + `scripts/deploy.sh` + `deploy/README.md` runbook** (re-deploys are one `make deploy`; first-time setup is documented).
- **Architecture + state-machine diagrams as Mermaid** (render on GitHub; user may export a polished PNG later for the video).
- **Schema is auto-applied** by `store.New` (embedded `schema.sql`, idempotent) — no manual migration step.
- The README is the centerpiece for **Presentation & Documentation (15%)** and must make the existing depth (Qwen tool-calling, MCP, eval calibration, state machine, audit log) legible for **Technical Depth (30%)** and **Innovation (30%)**.

**Judging criteria to surface (user-provided 2026-06-16):** Technical Depth & Engineering 30% · Innovation & AI Creativity 30% · Problem Value & Impact 25% · Presentation & Documentation 15%.

**Manual user steps (NOT in this plan — documented for the user):** provision ECS + RDS, run `make deploy`, generate the cert, record the ~3-min demo video + the Proof-of-Alibaba-Cloud recording, submit with the Track ID.

---

## Project facts (use verbatim in the artifacts)

- Repo: `https://github.com/lemonishi/supportsentinel` (public, MIT). Module `github.com/lemonishi/supportsentinel`, Go 1.25.
- Main binary: `cmd/server` → serves API + embedded React dashboard on `PORT` (default 8080). MCP binary: `cmd/mcp-server` → MCP Streamable HTTP on `MCP_LISTEN_ADDR` (default `:8090`, path `/mcp`).
- HTTP API: `POST /api/emails`, `GET /api/tickets`, `GET /api/tickets/{id}`, `GET /api/tickets/{id}/detail`, `GET /api/tickets/{id}/audit`, `POST /api/tickets/{id}/classification-review`, `POST /api/tickets/{id}/reply-approval`, `GET /` (dashboard).
- Env vars: `PORT`, `CONFIDENCE_THRESHOLD`, `DATABASE_URL` (required), `DASHSCOPE_API_KEY`, `DASHSCOPE_BASE_URL`, `QWEN_MODEL`, `MCP_SERVER_URL`, `MCP_LISTEN_ADDR`, `IMAP_*`, `SMTP_*`, `SLACK_WEBHOOK_URL`.
- Make targets: `dev`, `run`, `frontend`, `build`, `test`, `test-db`, `eval`, `eval-live`, `mcp`.
- Taxonomies: urgency `low|normal|high|critical`; type `billing|technical|account|feature_request|general`; department `billing|engineering|accounts|product|support_tier1`.
- State machine: `NEW → CLASSIFYING → {ROUTED | AWAITING_CLASSIFICATION_REVIEW} → DRAFTING → AWAITING_REPLY_APPROVAL → RESOLVED` (`FAILED` on unrecoverable error). Checkpoint 1: confidence < threshold OR urgency == critical → park. Checkpoint 2: every reply human-approved.
- Eval metrics (live qwen3.7-plus, 30-email gold set, reproduce with `make eval`): **type 86.7%**, **urgency 83.3%**, both-correct ~70%; calibration recommended `CONFIDENCE_THRESHOLD=0.95`.
- Proof of Alibaba Cloud: `internal/qwen/client.go` (DashScope/Qwen client) + RDS via `DATABASE_URL`.

---

## File structure

```
README.md                         → centerpiece (overview, diagrams, quickstart, deploy, criteria) (new)
docs/architecture.md              → larger Mermaid architecture + state diagrams + prose (new)
docs/SUBMISSION.md                → deliverables map + demo-video script + proof-recording script (new)
deploy/supportsentinel.service    → systemd unit (API + dashboard) (new)
deploy/supportsentinel-mcp.service→ systemd unit (MCP tool server, localhost) (new)
deploy/nginx.conf                 → reverse proxy + self-signed TLS over public IP (new)
deploy/app.env.prod.example       → production env template (new)
deploy/README.md                  → first-time ECS+RDS provisioning runbook (new)
scripts/deploy.sh                 → cross-compile + scp + restart over SSH (new)
scripts/seed_demo.sh              → POST representative emails to populate the dashboard (new)
Makefile                          → build both binaries; add `deploy` target (modify)
CLAUDE.md                         → add Deployment + Submission docs pointers (modify)
```

---

## Task 1: README.md + architecture diagrams

**Files:**
- Create: `README.md`
- Create: `docs/architecture.md`

- [ ] **Step 1: Write `README.md`** with exactly this content:

````markdown
# SupportSentinel

> Autopilot support-ticket agent — turns inbound support emails into triaged, routed, and drafted-reply tickets with two human-in-the-loop checkpoints. Built on **Qwen via Alibaba Cloud Model Studio (DashScope)**.

**Hackathon Track 4: Autopilot Agent (QwenCloud / Alibaba Cloud).** Open source under the MIT License.

SupportSentinel ingests support emails (HTTP or IMAP), classifies **urgency** and **type** with Qwen — invoking tools over the **Model Context Protocol** to disambiguate hard cases — routes them through a deterministic state machine, and parks low-confidence or critical tickets for human review. Every reply is human-approved before it would be sent. Every state change is written to an append-only audit log in the same transaction. **It fails toward a human — it never silently drops a ticket.**

## Why this matters

Support teams drown in inbound email: triage is slow, inconsistent, and the urgent ticket hides among the routine ones. SupportSentinel automates the triage-and-draft loop while keeping a human in control of anything risky — a production-shaped workflow (auditability, resilience, evaluation), not a chatbot demo. It is built to be deployed (single binary + systemd + nginx on Alibaba Cloud ECS, PostgreSQL on RDS) and to be measured (a calibrated evaluation harness, below).

## Architecture

```mermaid
flowchart TB
    subgraph Sources["Email sources"]
        HTTP["POST /api/emails"]
        IMAP["IMAP poller<br/>(UNSEEN → ingest → Seen)"]
    end

    subgraph ECS["Alibaba Cloud ECS (no Docker)"]
        NGINX["nginx<br/>(TLS, reverse proxy)"]
        subgraph SERVER["supportsentinel.service :8080"]
            ORCH["Orchestrator<br/>deterministic state machine"]
            QWEN["Qwen client<br/>(DashScope, JSON-mode + retry)"]
            UI["Embedded React dashboard"]
        end
        MCP["supportsentinel-mcp.service :8090<br/>MCP tool server (lookup_customer,<br/>lookup_similar_tickets)"]
    end

    ALI["Alibaba Cloud Model Studio<br/>(Qwen / DashScope)"]
    RDS[("Alibaba Cloud RDS<br/>PostgreSQL")]
    ALERTS["Slack webhook / SMTP<br/>(critical-ticket alerts)"]
    REVIEWER(["Human reviewer"])

    HTTP --> ORCH
    IMAP --> ORCH
    ORCH --> QWEN
    QWEN -->|tools over MCP| MCP
    QWEN <-->|classify / draft| ALI
    MCP --- RDS
    ORCH --- RDS
    ORCH -->|critical| ALERTS
    NGINX --> SERVER
    REVIEWER -->|checkpoints| UI
    UI --- ORCH
```

The classifier consumes its tools over MCP at runtime (`MCP_SERVER_URL`), falling back to in-process tools if the MCP service is unset or unreachable — the classify loop is identical either way.

## State machine

```mermaid
stateDiagram-v2
    [*] --> NEW
    NEW --> CLASSIFYING
    CLASSIFYING --> ROUTED: confident & not critical
    CLASSIFYING --> AWAITING_CLASSIFICATION_REVIEW: low confidence OR critical OR classify error
    AWAITING_CLASSIFICATION_REVIEW --> ROUTED: human confirms (Checkpoint 1)
    ROUTED --> DRAFTING
    DRAFTING --> AWAITING_REPLY_APPROVAL
    AWAITING_REPLY_APPROVAL --> RESOLVED: human approves (Checkpoint 2)
    AWAITING_REPLY_APPROVAL --> DRAFTING: human rejects → re-draft
    CLASSIFYING --> FAILED: unrecoverable
    RESOLVED --> [*]
```

- **Checkpoint 1:** `confidence < CONFIDENCE_THRESHOLD` OR `urgency == critical` → park for human review (critical always parks **and** alerts).
- **Checkpoint 2:** every drafted reply is human-approved (or rejected → re-drafted) before it would be sent.
- Every transition goes through `store.Apply`, which writes the `audit_log` row in the **same transaction**. `classifications` / `replies` / `audit_log` are append-only (full replay).

## Proof of Alibaba Cloud

The AI core is **Qwen on Alibaba Cloud Model Studio (DashScope)** — see [`internal/qwen/client.go`](internal/qwen/client.go): an OpenAI-compatible DashScope client doing JSON-mode classification with schema validation + one re-prompt, function-calling (tools), and bounded exponential-backoff retry. Persistence is **Alibaba Cloud RDS PostgreSQL** via `DATABASE_URL`. In production both run on **Alibaba Cloud ECS** (see [deploy/](deploy/)).

## Sophisticated Qwen usage

- **Function-calling tool loop** — during classification the model can call `lookup_customer` (account tier/status by email) and `lookup_similar_tickets` (how past tickets were classified) to disambiguate; invocations are recorded in `classifications.tools_used`.
- **Tools over MCP** — those tools are exposed via the **Model Context Protocol** (`internal/mcp`, `cmd/mcp-server`, built on `mark3labs/mcp-go`, Streamable HTTP). The classifier consumes them over MCP at runtime, so the tool layer is reusable by any MCP host. Schemas surfaced over MCP are verified identical to the in-process definitions.
- **JSON-mode + validation + re-prompt**, **bounded retry** (4xx non-retryable), and a **fake classifier fallback** when no API key is set (keeps the pipeline runnable offline).

## Evaluation (calibrated, not guessed)

`make eval` runs a 30-email gold dataset through the classifier and prints accuracy, per-class precision/recall/F1, confusion matrices, and a confidence-threshold calibration sweep. Latest live run (qwen3.7-plus):

| Dimension | Accuracy |
|---|---|
| Type | 86.7% |
| Urgency | 83.3% |
| Both correct | ~70% |

The calibration sweep recommends the HITL threshold (`CONFIDENCE_THRESHOLD`); urgency is the weaker axis, so the harness recommends routing conservatively. `make eval` replays a committed cache (free, offline); `make eval-live` refreshes it against live Qwen.

## Quickstart (local)

```bash
# Prereqs: Go 1.25+, Node 18+ (for the dashboard build), PostgreSQL.
cp app.env.example app.env          # fill in DATABASE_URL and DASHSCOPE_API_KEY
make test-db                        # create local dev + test databases
make run                            # build dashboard + run server → http://localhost:8080
```

Without `DASHSCOPE_API_KEY` the server uses a deterministic fake classifier, so the pipeline runs offline. Optional: `make mcp` (tool server), `make eval` (quality report).

### Submit a ticket

```bash
curl -sX POST localhost:8080/api/emails -H 'content-type: application/json' \
  -d '{"from":"customer@acme.com","subject":"Production is down","body":"All API calls 500 since 14:00, checkout is broken."}'
```

Then open http://localhost:8080 to review, approve, or override.

## API

| Method | Path | Purpose |
|---|---|---|
| POST | `/api/emails` | Ingest an email (creates + classifies a ticket) |
| GET | `/api/tickets` | List tickets (review queue) |
| GET | `/api/tickets/{id}/detail` | Ticket detail (reasoning, confidence, tools used) |
| GET | `/api/tickets/{id}/audit` | Append-only audit timeline |
| POST | `/api/tickets/{id}/classification-review` | Checkpoint 1: confirm/override routing |
| POST | `/api/tickets/{id}/reply-approval` | Checkpoint 2: approve/reject the drafted reply |

## Tech stack

Go 1.25 (net/http, jackc/pgx v5, google/uuid) · Qwen via Alibaba Cloud DashScope · Model Context Protocol (mark3labs/mcp-go) · PostgreSQL (local / Alibaba RDS) · Vite + React + TypeScript + Tailwind (embedded via `//go:embed`) · systemd + nginx on Alibaba Cloud ECS (no Docker).

See [docs/architecture.md](docs/architecture.md) for a deeper walkthrough and [deploy/README.md](deploy/README.md) for the deployment runbook.

## Deployment

No Docker — a single cross-compiled binary per service, systemd, and nginx on Alibaba Cloud ECS, with PostgreSQL on Alibaba Cloud RDS. One-command re-deploy:

```bash
DEPLOY_HOST=<ecs-public-ip> DEPLOY_USER=<user> make deploy
```

First-time setup (ECS, RDS, self-signed TLS, systemd units) is documented in [deploy/README.md](deploy/README.md).

## License

[MIT](LICENSE).
````

- [ ] **Step 2: Write `docs/architecture.md`** with exactly this content:

````markdown
# Architecture

SupportSentinel is a deterministic agent: a Go state machine drives every ticket from ingestion through two human-in-the-loop checkpoints, calling Qwen for the non-deterministic parts (classification, reply drafting) and recording every step in an append-only audit log.

## Components

| Package | Responsibility |
|---|---|
| `internal/domain` | Fixed taxonomies (urgency/type/department), the `Ticket` model, and the `Classifier`/`ToolBox` interfaces. |
| `internal/orchestrator` | The state machine. Gates routing on confidence + criticality; fails toward a human. |
| `internal/qwen` | Qwen/DashScope client: JSON-mode classification (validate + one re-prompt), function-calling tool loop, bounded retry, free-text reply drafting. **The Alibaba Cloud proof file.** |
| `internal/tools` | The agent's tools (`lookup_customer`, `lookup_similar_tickets`), store-backed. |
| `internal/mcp` | The same tools exposed over the Model Context Protocol + an MCP-client-backed `ToolBox` the classifier consumes at runtime. |
| `internal/ingest`, `internal/ingest/imap` | Email parsing (RFC 822, RFC 2047 subjects, HTML→text) and the IMAP poller. |
| `internal/alert` | Best-effort Slack + SMTP fan-out for critical tickets (failures never block the pipeline). |
| `internal/store` | PostgreSQL access (pgx), transactional `Apply`, embedded schema. |
| `internal/httpapi`, `internal/webui` | HTTP API + the embedded React dashboard. |
| `internal/eval`, `cmd/eval` | Gold-dataset evaluation harness + confidence calibration. |

## Data flow

```mermaid
flowchart LR
    E["Email<br/>(HTTP or IMAP)"] --> I["Ingest<br/>dedupe on Message-ID"]
    I --> C["Classify (Qwen)<br/>+ tools over MCP"]
    C -->|confident| R["Route → Draft (Qwen)"]
    C -->|low conf / critical| P1["Park: Checkpoint 1"]
    P1 -->|human| R
    R --> P2["Park: Checkpoint 2<br/>approve reply"]
    P2 --> DONE["Resolved"]
    C -. error .-> P1
    classDef park fill:#fde,stroke:#b36;
    class P1,P2 park;
```

## Core principles

- **Fail toward a human.** Classifier errors park the ticket for review (`AWAITING_CLASSIFICATION_REVIEW`) with the error recorded — never dropped.
- **Transactional audit.** Every state change goes through `store.Apply`, which writes the `audit_log` row in the same DB transaction. `classifications` / `replies` / `audit_log` are append-only, enabling full replay.
- **Deterministic core, probabilistic edges.** The state machine and routing are deterministic Go; only classification and drafting call the model.

## Deployment topology

Two systemd-managed binaries on one ECS instance — `supportsentinel.service` (API + embedded dashboard, `:8080`) and `supportsentinel-mcp.service` (MCP tool server, `:8090`, localhost-only). nginx terminates TLS and reverse-proxies the public interface to `:8080`. PostgreSQL is Alibaba Cloud RDS. See [../deploy/README.md](../deploy/README.md).
````

- [ ] **Step 3: Verify the Mermaid blocks are well-formed.** Run a quick fence/sanity check:

```bash
grep -c '```mermaid' README.md docs/architecture.md
```
Expected: `README.md:2` and `docs/architecture.md:1`. Then visually confirm each ```mermaid block is closed with a ``` fence (open `README.md`/`docs/architecture.md` and check the diagrams look syntactically balanced — node ids, arrows, and `subgraph/end` pairing). Do NOT leave an unterminated fence.

- [ ] **Step 4: Commit:**
```bash
git add README.md docs/architecture.md
git commit -m "docs: README centerpiece + architecture & state-machine diagrams

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 2: Deployment artifacts (systemd, nginx, prod env) + build both binaries

**Files:**
- Create: `deploy/supportsentinel.service`, `deploy/supportsentinel-mcp.service`, `deploy/nginx.conf`, `deploy/app.env.prod.example`
- Modify: `Makefile` (build both binaries)

- [ ] **Step 1: Create `deploy/supportsentinel.service`:**
```ini
[Unit]
Description=SupportSentinel API + dashboard
After=network-online.target supportsentinel-mcp.service
Wants=network-online.target

[Service]
Type=simple
User=supportsentinel
EnvironmentFile=/etc/supportsentinel/app.env
ExecStart=/opt/supportsentinel/server
Restart=on-failure
RestartSec=3
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/opt/supportsentinel

[Install]
WantedBy=multi-user.target
```

- [ ] **Step 2: Create `deploy/supportsentinel-mcp.service`:**
```ini
[Unit]
Description=SupportSentinel MCP tool server (localhost only)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=supportsentinel
EnvironmentFile=/etc/supportsentinel/app.env
ExecStart=/opt/supportsentinel/mcp-server
Restart=on-failure
RestartSec=3
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true

[Install]
WantedBy=multi-user.target
```

- [ ] **Step 3: Create `deploy/nginx.conf`** (self-signed TLS over the public IP; does NOT expose the MCP server):
```nginx
# SupportSentinel — nginx reverse proxy with self-signed TLS over the ECS public IP.
# Install to /etc/nginx/conf.d/supportsentinel.conf, then `nginx -t && systemctl reload nginx`.
# The MCP server (:8090) is intentionally NOT proxied — it stays localhost-only.

server {
    listen 80 default_server;
    server_name _;
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl default_server;
    server_name _;

    ssl_certificate     /etc/nginx/ssl/supportsentinel.crt;
    ssl_certificate_key /etc/nginx/ssl/supportsentinel.key;
    ssl_protocols TLSv1.2 TLSv1.3;

    client_max_body_size 2m;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

- [ ] **Step 4: Create `deploy/app.env.prod.example`:**
```bash
# SupportSentinel production environment (Alibaba Cloud ECS).
# Install the filled-in copy to /etc/supportsentinel/app.env (chmod 600, owned by
# the supportsentinel user). NEVER commit the filled-in version — secrets live
# only on the server.

PORT=8080
# Calibrated via `make eval` (urgency is the weak axis → route conservatively).
CONFIDENCE_THRESHOLD=0.95

# Alibaba Cloud RDS PostgreSQL (managed RDS requires TLS).
DATABASE_URL=postgres://USER:PASSWORD@RDS_HOST:5432/supportsentinel?sslmode=require

# Qwen / Alibaba Cloud Model Studio (DashScope, International endpoint).
DASHSCOPE_API_KEY=
DASHSCOPE_BASE_URL=https://dashscope-intl.aliyuncs.com/compatible-mode/v1
QWEN_MODEL=qwen-plus

# MCP tool server (runs as supportsentinel-mcp.service on localhost).
MCP_LISTEN_ADDR=:8090
MCP_SERVER_URL=http://127.0.0.1:8090/mcp

# Optional — leave blank to disable.
IMAP_HOST=
IMAP_PORT=993
IMAP_USERNAME=
IMAP_PASSWORD=
IMAP_MAILBOX=INBOX
SMTP_HOST=
SMTP_PORT=587
SMTP_USERNAME=
SMTP_PASSWORD=
SMTP_FROM=
SMTP_TO=
SLACK_WEBHOOK_URL=
```

- [ ] **Step 5: Update `Makefile` to build BOTH binaries.** Replace the current `build` target:
```make
build: frontend
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/server ./cmd/server
```
with:
```make
build: frontend
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/server ./cmd/server
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/mcp-server ./cmd/mcp-server
```

- [ ] **Step 6: Verify the cross-compile produces both binaries:**
```bash
make build && file bin/server bin/mcp-server
```
Expected: both exist and report `ELF 64-bit ... x86-64` (linux binaries cross-compiled from macOS). (`make build` runs `make frontend` first — needs npm; if npm is unavailable in this environment, instead run the two `go build` lines directly and confirm both binaries are produced.)

- [ ] **Step 7: Commit:**
```bash
git add deploy/supportsentinel.service deploy/supportsentinel-mcp.service deploy/nginx.conf deploy/app.env.prod.example Makefile
git commit -m "feat(deploy): systemd units, nginx self-signed TLS, prod env, build both binaries

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 3: Deploy + demo-seed scripts and `make deploy`

**Files:**
- Create: `scripts/deploy.sh`, `scripts/seed_demo.sh`
- Modify: `Makefile` (add `deploy` to `.PHONY` + target)

- [ ] **Step 1: Create `scripts/deploy.sh`:**
```bash
#!/usr/bin/env bash
# Deploy SupportSentinel to an Alibaba Cloud ECS instance: cross-compile both
# binaries, copy them over, and restart the systemd services.
#
#   DEPLOY_HOST=<ecs-public-ip> DEPLOY_USER=<user> ./scripts/deploy.sh
#
# Assumes first-time setup (user, /opt/supportsentinel, /etc/supportsentinel/app.env,
# systemd units, nginx) is already done — see deploy/README.md.
set -euo pipefail

: "${DEPLOY_HOST:?set DEPLOY_HOST to the ECS public IP or hostname}"
DEPLOY_USER="${DEPLOY_USER:-root}"
REMOTE_DIR="/opt/supportsentinel"
SSH_TARGET="${DEPLOY_USER}@${DEPLOY_HOST}"

echo "==> building frontend + linux/amd64 binaries"
make build

echo "==> uploading binaries to ${SSH_TARGET}:${REMOTE_DIR}"
ssh "${SSH_TARGET}" "sudo install -d -o '${DEPLOY_USER}' '${REMOTE_DIR}'"
scp bin/server bin/mcp-server "${SSH_TARGET}:${REMOTE_DIR}/"

echo "==> restarting services (mcp first, then api)"
ssh "${SSH_TARGET}" "sudo systemctl restart supportsentinel-mcp.service supportsentinel.service && sudo systemctl --no-pager --lines=0 status supportsentinel.service"

echo "==> done — check: https://${DEPLOY_HOST}/  (self-signed cert warning is expected)"
```

- [ ] **Step 2: Create `scripts/seed_demo.sh`** (populates the dashboard with representative tickets for the demo video):
```bash
#!/usr/bin/env bash
# Seed the dashboard with a spread of representative support emails so the demo
# isn't empty. Posts to the running server's ingest endpoint.
#
#   BASE_URL=http://localhost:8080 ./scripts/seed_demo.sh
set -euo pipefail
BASE_URL="${BASE_URL:-http://localhost:8080}"

post() {
  local from="$1" subject="$2" body="$3"
  curl -fsS -X POST "${BASE_URL}/api/emails" \
    -H 'content-type: application/json' \
    -d "$(printf '{"from":%s,"subject":%s,"body":%s}' \
          "$(json "$from")" "$(json "$subject")" "$(json "$body")")" >/dev/null
  echo "  seeded: ${subject}"
}

# JSON-escape a string via the shell (no jq dependency).
json() { printf '%s' "$1" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))'; }

echo "==> seeding demo tickets to ${BASE_URL}"
post "frank@acme.com"  "Production API returning 500 for all calls" "Every request has 500'd for 20 minutes. Checkout is completely down."
post "kate@acme.com"   "Cannot log in - password reset never arrives" "I requested a reset three times, no email. Locked out of my account."
post "alice@acme.com"  "Charged twice for my subscription"            "I was billed \$49 twice this month. Please refund the duplicate."
post "pat@acme.com"    "Feature request: dark mode"                   "A dark theme for the dashboard would be great for late-night work."
post "bea@acme.com"    "Security: possible unauthorized access"       "I see logins from a country we do not operate in. Please lock it down now."
post "wes@acme.com"    "Thanks for the great product"                 "Just wanted to say the team loves the new release."
echo "==> done — open ${BASE_URL} to review the queue"
```

- [ ] **Step 3: Make both scripts executable:**
```bash
chmod +x scripts/deploy.sh scripts/seed_demo.sh
```

- [ ] **Step 4: Syntax-check both scripts** (no execution — they need a server/SSH):
```bash
bash -n scripts/deploy.sh && bash -n scripts/seed_demo.sh && echo "syntax OK"
```
Expected: `syntax OK`. If `shellcheck` is installed, also run `shellcheck scripts/deploy.sh scripts/seed_demo.sh` and address any error-level findings (warnings about `ssh`/`scp` quoting are acceptable if intentional).

- [ ] **Step 5: Add the `deploy` Make target.** Change the `.PHONY` line (currently ends `... eval eval-live mcp`) to add `deploy`:
```make
.PHONY: dev run test test-db build tidy frontend eval eval-live mcp deploy
```
Append at the end of `Makefile`:
```make
# Deploy to Alibaba Cloud ECS (cross-compile + scp + restart). Requires DEPLOY_HOST.
# First-time server setup is documented in deploy/README.md.
deploy:
	DEPLOY_HOST=$(DEPLOY_HOST) DEPLOY_USER=$(DEPLOY_USER) ./scripts/deploy.sh
```

- [ ] **Step 6: Verify the Make target wiring** (dry parse, no real deploy):
```bash
make -n deploy DEPLOY_HOST=example.invalid DEPLOY_USER=demo
```
Expected: prints the `./scripts/deploy.sh` invocation with the env vars (it must not actually connect).

- [ ] **Step 7: Commit:**
```bash
git add scripts/deploy.sh scripts/seed_demo.sh Makefile
git commit -m "feat(deploy): deploy.sh + seed_demo.sh + make deploy target

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 4: First-time deployment runbook

**Files:**
- Create: `deploy/README.md`

- [ ] **Step 1: Write `deploy/README.md`** with exactly this content:

````markdown
# Deploying SupportSentinel on Alibaba Cloud (no Docker)

One ECS instance runs two systemd services; nginx terminates TLS and proxies to the API; PostgreSQL is Alibaba Cloud RDS. Re-deploys are one `make deploy`; this runbook covers the one-time setup.

## 0. Provision

- **ECS:** a small Linux instance (Ubuntu 22.04 / Alibaba Cloud Linux). Open security-group inbound ports **80** and **443** to the internet, **22** to your IP. Do NOT expose 8080/8090.
- **RDS:** an Alibaba Cloud RDS for PostgreSQL instance. Create a database `supportsentinel` and a user. Add the ECS instance's IP/VPC to the RDS whitelist. Note the connection string; managed RDS requires TLS (`sslmode=require`). The app applies its schema automatically on first connect (embedded `schema.sql`) — no manual migration.

## 1. Server user + directory

```bash
sudo useradd --system --no-create-home --shell /usr/sbin/nologin supportsentinel
sudo install -d -o "$USER" /opt/supportsentinel
sudo install -d /etc/supportsentinel
```

## 2. Environment file

Copy `deploy/app.env.prod.example` to the server, fill in `DATABASE_URL` (RDS) and `DASHSCOPE_API_KEY` (and optionally IMAP/SMTP/Slack), and install it locked down:

```bash
sudo install -m 600 -o supportsentinel app.env.prod /etc/supportsentinel/app.env
```

## 3. systemd units

```bash
sudo cp deploy/supportsentinel.service deploy/supportsentinel-mcp.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now supportsentinel-mcp.service supportsentinel.service
sudo systemctl --no-pager status supportsentinel.service
journalctl -u supportsentinel.service -n 50 --no-pager   # look for "listening on :8080"
```

## 4. Self-signed TLS + nginx

```bash
sudo install -d /etc/nginx/ssl
sudo openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
  -keyout /etc/nginx/ssl/supportsentinel.key \
  -out    /etc/nginx/ssl/supportsentinel.crt \
  -subj "/CN=$(curl -s ifconfig.me)"
sudo cp deploy/nginx.conf /etc/nginx/conf.d/supportsentinel.conf
sudo nginx -t && sudo systemctl reload nginx
```

Visit `https://<ecs-public-ip>/` — accept the self-signed cert warning. The dashboard loads.

## 5. First binary deploy

From your laptop (repo root):

```bash
DEPLOY_HOST=<ecs-public-ip> DEPLOY_USER=<ssh-user> make deploy
```

This cross-compiles `server` + `mcp-server`, copies them to `/opt/supportsentinel/`, and restarts the services. Re-run it for every subsequent deploy.

## Verify

- `systemctl status supportsentinel.service supportsentinel-mcp.service` — both `active (running)`.
- `journalctl -u supportsentinel.service` shows `classifier: Qwen via DashScope (model=...) with tools (MCP http://127.0.0.1:8090/mcp)`.
- `curl -k https://<ip>/api/tickets` returns JSON.
- POST a ticket: `curl -k -X POST https://<ip>/api/emails -H 'content-type: application/json' -d '{"from":"a@b.com","subject":"test","body":"hello"}'`.

## Troubleshooting

- **Service crash-loops:** `journalctl -u supportsentinel.service -n 100` — usually a bad `DATABASE_URL` or missing `DASHSCOPE_API_KEY` (the latter falls back to the fake classifier, not a crash).
- **MCP fallback:** if `supportsentinel-mcp.service` is down, the API logs `mcp: dial ... failed; falling back to in-process tools` and still serves — tools just run in-process.
- **RDS connection refused:** check the RDS whitelist includes the ECS IP and that `sslmode=require` is set.
````

- [ ] **Step 2: Verify the doc's fenced code blocks are balanced:**
```bash
grep -c '```' deploy/README.md
```
Expected: an even number (every fence opened is closed).

- [ ] **Step 3: Commit:**
```bash
git add deploy/README.md
git commit -m "docs(deploy): first-time ECS + RDS + TLS provisioning runbook

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 5: Submission deliverables map + demo/proof scripts

**Files:**
- Create: `docs/SUBMISSION.md`

- [ ] **Step 1: Write `docs/SUBMISSION.md`** with exactly this content:

````markdown
# Track 4 Submission — SupportSentinel

**Track:** Track 4 — Autopilot Agent (QwenCloud / Alibaba Cloud)
**Repo:** https://github.com/lemonishi/supportsentinel (public, MIT)
**Proof of Alibaba Cloud:** [`internal/qwen/client.go`](../internal/qwen/client.go) (Qwen via DashScope) + Alibaba Cloud RDS (`DATABASE_URL`) + Alibaba Cloud ECS deploy ([deploy/](../deploy/)).

## Deliverables checklist

- [ ] Public repo + OSS license — ✅ in repo (MIT).
- [ ] Proof of Alibaba Cloud (code file) — `internal/qwen/client.go`, linked in README.
- [ ] Proof recording (backend on Alibaba Cloud) — see "Proof recording script" below.
- [ ] Architecture diagram — Mermaid in [README](../README.md) + [docs/architecture.md](architecture.md) (export a PNG if a static image is required).
- [ ] ~3-min demo video — see "Demo video script" below.
- [ ] Text description — see "Description" below (reuse README).
- [ ] Track identification — Track 4: Autopilot Agent.

## Description (text submission)

SupportSentinel is an autopilot support-ticket agent. It ingests support emails (HTTP or IMAP), classifies urgency and type with **Qwen on Alibaba Cloud Model Studio (DashScope)** — invoking tools over the **Model Context Protocol** to disambiguate hard cases — and routes them through a deterministic Go state machine with two human-in-the-loop checkpoints: low-confidence or critical tickets park for review, and every drafted reply is human-approved before sending. Every state change is written to an append-only audit log in the same transaction, so the whole lifecycle is replayable. It fails toward a human — classifier errors park for review rather than dropping the ticket. Classification quality is measured by a gold-dataset evaluation harness that calibrates the review threshold (not guessed). It deploys without Docker: single Go binaries + systemd + nginx on Alibaba Cloud ECS, with PostgreSQL on Alibaba Cloud RDS.

## How it maps to the judging criteria

- **Technical Depth & Engineering (30%):** Qwen function-calling tool loop (JSON-mode + validation + re-prompt, bounded retry); tools exposed and consumed over MCP at runtime; a deterministic state machine with transactional, append-only auditing; a calibrated evaluation harness.
- **Innovation & AI Creativity (30%):** clean modular architecture behind interfaces (`Classifier`, `ToolBox`, `Ingestor`, `Alerter`); "fail toward a human" resilience; the MCP tool layer makes the agent's skills reusable by any MCP host.
- **Problem Value & Impact (25%):** automates a real, costly workflow (support triage) while keeping humans in control of risk; deployable and measurable; open source.
- **Presentation & Documentation (15%):** README with architecture + state-machine diagrams, this submission map, and a deployment runbook.

## Demo video script (~3 min)

1. **(0:00) Problem & overview** — one line on support-triage pain; show the README architecture diagram.
2. **(0:25) Ingest** — `./scripts/seed_demo.sh` (or POST one email live); show the dashboard queue populate.
3. **(0:50) Classification + tools** — open a ticket detail: Qwen's reasoning, confidence, and **tools used** (`lookup_customer` / `lookup_similar_tickets`) recorded. Mention they run over MCP.
4. **(1:20) Checkpoint 1** — open a low-confidence or **critical** ticket parked for review; confirm/override the routing.
5. **(1:50) Checkpoint 2** — show the drafted reply; edit + approve → ticket resolves.
6. **(2:15) Audit timeline** — show the append-only audit log for that ticket (full lifecycle).
7. **(2:35) Evaluation** — run `make eval`; show accuracy, per-class F1, confusion matrix, and the recommended threshold (the "calibrated, not guessed" story).
8. **(2:55) Close** — "Deployed on Alibaba Cloud ECS + RDS, Qwen via DashScope."

## Proof-of-Alibaba-Cloud recording script

Record a screen capture on the ECS instance (or SSH'd in):

1. `cat internal/qwen/client.go | head -40` — show the DashScope base URL + model (the proof file).
2. `systemctl status supportsentinel.service supportsentinel-mcp.service` — both active on ECS.
3. `journalctl -u supportsentinel.service -n 20` — show `classifier: Qwen via DashScope (model=...) with tools (MCP ...)`.
4. `curl -k -X POST https://<ecs-ip>/api/emails -H 'content-type: application/json' -d '{"from":"a@b.com","subject":"prod down","body":"everything is 500ing"}'` — a live request hitting Qwen on Alibaba Cloud, returning a classified ticket.
5. Show the Alibaba Cloud console: the ECS instance + the RDS PostgreSQL instance.
````

- [ ] **Step 2: Verify fenced blocks balance:**
```bash
grep -c '```' docs/SUBMISSION.md
```
Expected: even number.

- [ ] **Step 3: Commit:**
```bash
git add docs/SUBMISSION.md
git commit -m "docs: Track 4 submission deliverables map + demo & proof scripts

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 6: CLAUDE.md pointers + full verification

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Update the `Deploy:` line in `CLAUDE.md`.** It currently reads:
```markdown
- Deploy: NO Docker. Single Go binary + systemd + nginx on Alibaba Cloud ECS (Plan 7).
```
Replace with:
```markdown
- Deploy: NO Docker. Two cross-compiled binaries (`server` + `mcp-server`) as systemd
  units + nginx (self-signed TLS over the public IP) on Alibaba Cloud ECS; PostgreSQL
  on Alibaba Cloud RDS (schema auto-applied). Artifacts + one-time runbook in `deploy/`;
  re-deploy with `DEPLOY_HOST=<ip> DEPLOY_USER=<user> make deploy`. Submission deliverables
  (Track 4 map, demo + proof scripts) in `docs/SUBMISSION.md`; overview + diagrams in
  `README.md` / `docs/architecture.md`.
```

- [ ] **Step 2: Full verification.** Run and confirm:
  - `go vet ./...` — clean
  - `go build ./...` — clean
  - `go test ./...` — all packages green (no code behavior changed in this plan; this guards against accidental breakage). Run `make test` too for DB-backed coverage; report which ran.
  - `bash -n scripts/deploy.sh scripts/seed_demo.sh` — syntax OK
  - Confirm no secrets committed: `git grep -nE 'sk-|PASSWORD=.|WEBHOOK_URL=.' -- ':!*.example' ':!docs/*'` returns nothing meaningful (the `.example`/docs templates with blank values are fine).

- [ ] **Step 3: Commit:**
```bash
git add CLAUDE.md
git commit -m "docs: point CLAUDE.md at deploy artifacts + submission docs

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Verification checklist

- [ ] README renders on GitHub with both Mermaid diagrams (architecture + state machine), the eval metrics table, the Qwen/Alibaba Cloud proof section, and the Track 4 identification.
- [ ] `make build` produces `bin/server` AND `bin/mcp-server` as linux/amd64 binaries.
- [ ] `deploy/` has both systemd units, nginx self-signed TLS config, the prod env template (secrets blank), and a first-time runbook; the MCP server is NOT exposed publicly.
- [ ] `scripts/deploy.sh` and `scripts/seed_demo.sh` pass `bash -n`; `make -n deploy` shows the right invocation.
- [ ] `docs/SUBMISSION.md` maps every Track 4 deliverable and includes the demo + proof recording scripts.
- [ ] `go vet ./...`, `go build ./...`, `go test ./...` all clean/green; no secrets committed.
- [ ] CLAUDE.md points at the deploy artifacts and submission docs.

## Manual follow-up (user — the actual submission)

- [ ] Provision ECS + RDS; run the `deploy/README.md` runbook + `make deploy`.
- [ ] Generate the self-signed cert; confirm the dashboard loads over HTTPS on the public IP.
- [ ] Record the ~3-min demo video (script in `docs/SUBMISSION.md`) and the Proof-of-Alibaba-Cloud recording.
- [ ] (Optional) Export a polished PNG of the architecture diagram for the video.
- [ ] Submit with **Track 4: Autopilot Agent** and the repo link.
