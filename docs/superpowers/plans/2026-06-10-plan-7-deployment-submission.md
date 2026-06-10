# SupportSentinel — Plan 7: Alibaba Cloud Deployment + Submission Deliverables

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans. This plan is infra- and docs-heavy (a runbook + checklist rather than TDD); follow it task-by-task, verify each artifact, one PR. Read `HANDOVER.md` first.

**Goal:** Get the backend running on **Alibaba Cloud ECS** (with PostgreSQL on **Alibaba Cloud RDS**) and assemble the complete **hackathon submission package**: public repo + OSS license (done), Proof-of-Alibaba-Cloud recording, architecture diagram, ~3-minute demo video, text description, and Track identification.

**Architecture (deploy):** `make build` cross-compiles a single linux/amd64 Go binary with the dashboard embedded (`//go:embed`). It's copied to an Alibaba Cloud ECS instance, run under **systemd** (auto-restart, boot-start, `journalctl` logs) reading secrets from an `EnvironmentFile`, behind **nginx** terminating TLS (Let's Encrypt) and proxying to `localhost:8080`. `DATABASE_URL` points at **Alibaba Cloud RDS PostgreSQL**; Qwen calls go to Alibaba Cloud DashScope. **No Docker.**

**Spec:** `docs/superpowers/specs/2026-06-05-supportsentinel-design.md` (§9 deployment, §11 deliverables). **Builds on:** all prior plans. Commit as `Lennon <lemoncode8888@gmail.com>` (see HANDOVER.md).

**Submission requirements (from the brief) — this plan satisfies each:**
1. Public repo + OSS license visible in About ✅ (MIT, already detected on `main`)
2. Proof of Alibaba Cloud: a code file using Alibaba services/APIs → `internal/qwen/client.go` (DashScope) + RDS config; **plus a short recording** of the backend running on Alibaba Cloud
3. Architecture diagram
4. ~3-minute public demo video (YouTube/Vimeo)
5. Text description of features/functionality
6. Track identification (Track 4: Autopilot Agent)
7. (Optional) blog/social post for the Blog Prize

---

## File Structure (Plan 7)

```
deploy/supportsentinel.service   → systemd unit (new)
deploy/nginx.conf                 → nginx reverse-proxy + TLS (new)
deploy/app.env.production.example → server env template (RDS, DashScope, IMAP/SMTP) (new)
deploy/RUNBOOK.md                 → step-by-step ECS + RDS provisioning & deploy (new)
deploy/deploy.sh                  → build → scp → restart (new)
Makefile                          → `deploy` target (modify)
README.md                         → the submission README (new/replace)
docs/architecture.md              → Mermaid architecture diagram source (new)
docs/architecture.png             → exported diagram image for the README/submission (new)
docs/SUBMISSION.md                → deliverables checklist + demo video script + proof-recording script (new)
```

---

## Task 1: Deploy assets (systemd, nginx, env template)

**Files:** create `deploy/supportsentinel.service`, `deploy/nginx.conf`, `deploy/app.env.production.example`.

- [ ] **`deploy/supportsentinel.service`:**
```ini
[Unit]
Description=SupportSentinel
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=supportsentinel
EnvironmentFile=/etc/supportsentinel/app.env
ExecStart=/opt/supportsentinel/server
Restart=on-failure
RestartSec=3
# Server listens on 8080; nginx fronts it on 80/443.

[Install]
WantedBy=multi-user.target
```

- [ ] **`deploy/nginx.conf`** (server block; certbot fills in the TLS lines):
```nginx
server {
    server_name YOUR_DOMAIN;
    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
    listen 80;
    # `certbot --nginx` will add listen 443 ssl + cert paths here.
}
```

- [ ] **`deploy/app.env.production.example`:** same keys as `app.env.example`, but `DATABASE_URL` points at RDS (`postgres://USER:PASS@RDS_HOST:5432/supportsentinel?sslmode=require`), with `DASHSCOPE_API_KEY`, `QWEN_MODEL`, `CONFIDENCE_THRESHOLD` (from Plan 6), and optional IMAP/SMTP/Slack (Plan 5). **Document that the real file lives at `/etc/supportsentinel/app.env` on the server (0600, owned by the service user) and is never committed.**

- [ ] Commit: `feat(deploy): systemd unit, nginx config, production env template`.

---

## Task 2: Deploy script + provisioning runbook

**Files:** create `deploy/deploy.sh`, `deploy/RUNBOOK.md`; modify `Makefile`.

- [ ] **`deploy/deploy.sh`** (parameterized by `SS_HOST` / `SS_USER` env or args):
```bash
#!/usr/bin/env bash
set -euo pipefail
: "${SS_HOST:?set SS_HOST=user@ecs-ip}"
echo "building linux binary…"; make build      # frontend + linux/amd64 → bin/server
echo "uploading…"
ssh "$SS_HOST" 'sudo mkdir -p /opt/supportsentinel && sudo chown $USER /opt/supportsentinel'
scp bin/server "$SS_HOST":/opt/supportsentinel/server.new
ssh "$SS_HOST" 'sudo mv /opt/supportsentinel/server.new /opt/supportsentinel/server && sudo systemctl restart supportsentinel && sudo systemctl --no-pager status supportsentinel | head -5'
echo "deployed."
```
Make it executable; add a `Makefile` target:
```make
deploy:
	SS_HOST=$(SS_HOST) ./deploy/deploy.sh
```

- [ ] **`deploy/RUNBOOK.md`** — the one-time provisioning steps (write as a clear checklist):
  1. **ECS:** create an instance (Ubuntu 22.04 or Alibaba Cloud Linux), open security-group ports 80/443 (and 22). Note the public IP.
  2. **RDS:** create a PostgreSQL instance; create the `supportsentinel` database; whitelist the ECS instance's IP; note host/user/pass. (The app applies its schema on startup via `schema.sql`.)
  3. **On ECS:** create user `supportsentinel`; `sudo mkdir /etc/supportsentinel /opt/supportsentinel`; put `app.env` at `/etc/supportsentinel/app.env` (chmod 600) with the RDS `DATABASE_URL` + `DASHSCOPE_API_KEY`; install the systemd unit (`/etc/systemd/system/supportsentinel.service`), `systemctl enable --now supportsentinel`.
  4. **nginx + TLS:** `apt install nginx certbot python3-certbot-nginx`; install `nginx.conf` with your domain; `certbot --nginx -d YOUR_DOMAIN`.
  5. **Deploy:** from a dev machine, `SS_HOST=ubuntu@ECS_IP make deploy`.
  6. **Verify:** `curl https://YOUR_DOMAIN/` returns the dashboard; `POST /api/emails` creates a ticket; `journalctl -u supportsentinel -f` shows the Qwen classifier line.

- [ ] **Deploy for real** and confirm the live URL works end-to-end (dashboard + a real Qwen-classified ticket).

- [ ] Commit: `feat(deploy): deploy script and provisioning runbook`.

---

## Task 3: README (the submission front door)

**Files:** create/replace `README.md`.

Write a strong README — judges read this first. Sections:
- [ ] **Title + one-liner** + the deployed demo URL + the demo video link.
- [ ] **Track:** "Track 4: Autopilot Agent" (explicit).
- [ ] **What it does** — the autopilot story: email → Qwen classification → routing → two human-in-the-loop checkpoints → resolved, fully audited. Screenshot/GIF of the dashboard.
- [ ] **Proof of Alibaba Cloud** — a prominent link to **`internal/qwen/client.go`** (DashScope calls) and a note that PostgreSQL runs on **Alibaba Cloud RDS**; link the proof recording (Task 5).
- [ ] **Architecture** — embed `docs/architecture.png` (Task 4) + a short prose walk-through (Qwen ↔ backend ↔ DB ↔ frontend).
- [ ] **Features** — fixed taxonomies; confidence-gated + critical-always-parks checkpoints; **agent invokes external tools** (lookup_customer / lookup_similar_tickets) recorded in `tools_used`; fail-toward-a-human resilience; append-only audit log; evaluation metrics (cite the Plan 6 numbers); IMAP ingestion + Slack/email alerts.
- [ ] **Run it locally** — prerequisites (Go, Node, Postgres on 5433, `app.env` with a DashScope key), then `make run` → http://localhost:8080; `make test`; `make eval`.
- [ ] **Deploy** — point to `deploy/RUNBOOK.md`.
- [ ] **License** — MIT.

- [ ] Commit: `docs: submission README`.

---

## Task 4: Architecture diagram

**Files:** create `docs/architecture.md` (Mermaid source) + `docs/architecture.png` (export).

- [ ] **`docs/architecture.md`:** a Mermaid diagram showing the full system — email sources (HTTP + IMAP) → orchestrator state machine → Qwen client → DashScope (Alibaba Cloud) + tools; orchestrator → PostgreSQL/RDS (audit log); React dashboard (embedded) reading via APIs; urgent → Slack/email alerts. Annotate the Alibaba Cloud boundary (ECS + RDS + DashScope).
- [ ] **Export to `docs/architecture.png`** (Mermaid CLI `mmdc`, or paste into mermaid.live and export) so it renders in the README and the submission even where Mermaid isn't supported.
- [ ] Commit: `docs: architecture diagram`.

---

## Task 5: Submission package (checklist, demo video script, proof recording)

**Files:** create `docs/SUBMISSION.md`.

- [ ] **Deliverables checklist** — map each requirement to its artifact (repo URL, license, proof file link, proof recording link, architecture diagram, demo video link, text description, Track 4). Keep this as the single source of truth for what to paste into the submission form.

- [ ] **Proof-of-Alibaba-Cloud recording script** (separate from the demo, per the brief): a short screen recording on the ECS box showing the backend running on Alibaba Cloud — e.g.:
  1. `ssh` into the ECS instance; `systemctl status supportsentinel` (active/running).
  2. `journalctl -u supportsentinel -n 20` showing the Qwen classifier startup line.
  3. From your laptop, `curl https://YOUR_DOMAIN/api/emails -d '{…}'` and show the ticket created; show `journalctl` logging the DashScope call.
  4. Show the RDS console (or `psql` to RDS) with the `tickets` row. 
  Provide this as a link to a file in the repo that demonstrates Alibaba Cloud usage (the brief asks for a code-file link too → `internal/qwen/client.go`).

- [ ] **~3-minute demo video script** — scripted shots that make the autopilot + HITL + tools story legible:
  1. (0:00) One-line problem + "Track 4 Autopilot Agent". 
  2. (0:20) POST / show an inbound support email arrive (IMAP or the submit form); Qwen classifies it — show **reasoning + confidence + tools-used** in the dashboard.
  3. (1:00) A **critical** email parks at **Checkpoint 1**; a Slack/email alert fires; the reviewer validates routing.
  4. (1:40) The drafted reply at **Checkpoint 2**; reviewer edits + approves → **RESOLVED**; show the **audit timeline** (system → qwen → human).
  5. (2:20) Show the **eval numbers** (accuracy / per-class / calibrated threshold) and the **Proof of Alibaba Cloud** (running on ECS, DashScope, RDS).
  6. (2:50) Close. Upload public to YouTube/Vimeo; link in README + SUBMISSION.md.

- [ ] **Text description** — a few paragraphs (features + functionality) for the submission form; can mirror the README intro.

- [ ] (Optional) **Blog post** — a short build-journal for the Blog Prize.

- [ ] Commit: `docs: submission checklist, demo script, proof-recording script`.

---

## Plan 7 Definition of Done
- [ ] Backend live on **Alibaba Cloud ECS** behind nginx/TLS, DB on **Alibaba Cloud RDS**; `https://YOUR_DOMAIN/` serves the dashboard and classifies a real ticket via Qwen.
- [ ] `deploy/` has the systemd unit, nginx conf, deploy script, and runbook; `make deploy` works.
- [ ] `README.md` complete with Track 4 ID, Proof-of-Alibaba-Cloud link, architecture diagram, features, run/deploy instructions, license.
- [ ] `docs/architecture.png` exists and is embedded.
- [ ] `docs/SUBMISSION.md` checklist complete; proof recording captured; ~3-min demo video uploaded (public) and linked; text description written.
- [ ] Final pass: every hackathon requirement in the brief has a corresponding, linkable artifact.

## After Plan 7
The project is submission-ready. Remaining polish (more gold cases, auto-send for trivial replies, k8s, multi-reviewer auth) is explicitly out of hackathon scope (spec §12) — pursue only if time allows.
