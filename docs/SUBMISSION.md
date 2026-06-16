# Track 4 Submission — Autocierge

**Track:** Track 4 — Autopilot Agent (QwenCloud / Alibaba Cloud)
**Repo:** https://github.com/lemonishi/autocierge (public, MIT)
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

Autocierge is an autopilot support-ticket agent. It ingests support emails (HTTP or IMAP), classifies urgency and type with **Qwen on Alibaba Cloud Model Studio (DashScope)** — invoking tools over the **Model Context Protocol** to disambiguate hard cases — and routes them through a deterministic Go state machine with two human-in-the-loop checkpoints: low-confidence or critical tickets park for review, and every drafted reply is human-approved before sending. Every state change is written to an append-only audit log in the same transaction, so the whole lifecycle is replayable. It fails toward a human — classifier errors park for review rather than dropping the ticket. Classification quality is measured by a gold-dataset evaluation harness that calibrates the review threshold (not guessed). It deploys without Docker: single Go binaries + systemd + nginx on Alibaba Cloud ECS, with PostgreSQL on Alibaba Cloud RDS.

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
2. `systemctl status autocierge.service autocierge-mcp.service` — both active on ECS.
3. `journalctl -u autocierge.service -n 20` — show `classifier: Qwen via DashScope (model=...) with tools (MCP ...)`.
4. `curl -k -X POST https://<ecs-ip>/api/emails -H 'content-type: application/json' -d '{"from":"a@b.com","subject":"prod down","body":"everything is 500ing"}'` — a live request hitting Qwen on Alibaba Cloud, returning a classified ticket.
5. Show the Alibaba Cloud console: the ECS instance + the RDS PostgreSQL instance.
