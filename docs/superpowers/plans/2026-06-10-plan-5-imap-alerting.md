# Autocierge — Plan 5: IMAP Ingestion + Slack/Email Alerting

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Add a real **inbound-email source** — an IMAP poller that watches a mailbox and feeds the same `orchestrator.Ingest` path (idempotent via RFC822 Message-ID) — and real **urgent alerting** (Slack webhook + SMTP email) replacing the log-only alerter, still best-effort. This is the "production-real, not a toy" proof for Track 4.

**Architecture:** A pure `ParseRFC822` turns a raw message into a normalized `domain.Email` (From/Subject/plain-text Body/Message-ID dedupe key). `internal/ingest/imap` polls unseen messages on an interval, parses each, calls `orchestrator.Ingest(ctx, "imap", email)`, and marks them seen — running as a background goroutine started by `cmd/server` only when IMAP is configured. `internal/alert` gains `Slack` and `Email` alerters plus a `Multi` fan-out built from config; all alerting stays best-effort (failures logged, never block the pipeline). The orchestrator and existing HTTP ingestion are unchanged.

**Tech Stack:** Go 1.25; `github.com/emersion/go-imap/v2` (+ `go-message` for MIME parsing) for IMAP; stdlib `net/smtp` + `net/http` for alerts. Tests: pure parsing via `.eml` fixtures; Slack via `httptest`; Email via an injected send func; IMAP poller validated manually against a real test inbox.

**Spec:** `docs/superpowers/specs/2026-06-05-autocierge-design.md` (§3 ingestion adapters + alerting, §7 best-effort alerts). **Builds on:** Plans 1–4 (`orchestrator.Ingest`, `alert.Alerter`, `domain.Email.DedupeKey`). **Module path:** `github.com/lemonishi/autocierge`. **Env:** Postgres on 5433 (`TEST_DATABASE_URL`). Commit with the repo's existing git config; do not override the author email.

---

## File Structure (Plan 5)

```
internal/config/config.go            → + IMAP*, SMTP*, SlackWebhookURL settings (modify)
internal/ingest/email.go             → ParseRFC822 (pure) (new)
internal/ingest/email_test.go         → .eml fixture tests (new)
internal/ingest/testdata/*.eml        → fixtures (new)
internal/ingest/imap/poller.go        → IMAP poller goroutine (new)
internal/alert/slack.go               → Slack webhook alerter (new)
internal/alert/slack_test.go          → httptest (new)
internal/alert/email.go               → SMTP alerter (injectable sender) (new)
internal/alert/email_test.go          → message-build test (new)
internal/alert/multi.go               → Multi fan-out + FromConfig (new)
internal/alert/multi_test.go          → best-effort fan-out test (new)
cmd/server/main.go                    → build alerter from config; start IMAP poller if configured (modify)
app.env.example                       → + IMAP/SMTP/Slack vars (modify)
CLAUDE.md                             → note IMAP + alerting (modify)
```

---

## Task 1: Config — IMAP, SMTP, Slack settings

**Files:** modify `internal/config/config.go`, `internal/config/config_test.go`, `app.env.example`.

Add optional fields (all empty by default; features activate only when set):
```go
// IMAP ingestion (optional)
IMAPHost     string
IMAPPort     string // default "993"
IMAPUsername string
IMAPPassword string
IMAPMailbox  string // default "INBOX"
IMAPPollSeconds int  // default 30
// Alerting (optional)
SMTPHost     string
SMTPPort     string // default "587"
SMTPUsername string
SMTPPassword string
SMTPFrom     string
SlackWebhookURL string
```
In `Load()`, read each via `getenv`/`os.Getenv` with the noted defaults; `IMAPPollSeconds` parsed with a default of 30. **None are required.** Add a helper `(c Config) IMAPEnabled() bool { return c.IMAPHost != "" }`.

- [ ] Test: defaults (IMAPPort "993", IMAPMailbox "INBOX", IMAPPollSeconds 30 when unset; IMAPEnabled false when host empty, true when set). TDD (failing test → implement → pass).
- [ ] Update `app.env.example` with a commented block documenting all the new vars.
- [ ] Commit: `feat(config): IMAP, SMTP, and Slack settings`.

---

## Task 2: ParseRFC822 — raw message → domain.Email

**Files:** create `internal/ingest/email.go`, `internal/ingest/email_test.go`, `internal/ingest/testdata/{plain,multipart}.eml`.

Pure, dependency-light parsing so it's fully unit-testable.

- [ ] **Fixtures:** add two `.eml` files under `testdata/`: a simple `text/plain` message and a `multipart/alternative` (text + html) message. Each has `From:`, `Subject:`, `Message-ID:` headers and a known body.

- [ ] **Test** (`email_test.go`): `ParseRFC822` on each fixture returns the expected `FromAddr` (address only, parsed via `net/mail`), `Subject`, plain-text `Body` (the text/plain part for multipart; html stripped/ignored), and `DedupeKey` = the `Message-ID` header. A message with no `Message-ID` falls back to `hash(from+subject+body)` (reuse the same scheme as the HTTP adapter — extract that hash helper into `internal/ingest` or duplicate the sha256 logic). TDD.

- [ ] **Implement** `ParseRFC822(raw []byte) (domain.Email, error)`:
  - Use `net/mail.ReadMessage` for headers; `mail.ParseAddress` for the From address.
  - For the body, use `github.com/emersion/go-message/mail` to walk parts and pick the `text/plain` part (fallback: strip tags from `text/html`, or use the raw body for non-MIME).
  - `DedupeKey`: trimmed `Message-ID` if present, else `hashKey(from, subject, body)`.
  - Set `Raw` to a small map of useful headers (message-id, date) for traceability.

- [ ] Commit: `feat(ingest): ParseRFC822 normalizes raw email to domain.Email`.

---

## Task 3: Slack alerter

**Files:** create `internal/alert/slack.go`, `internal/alert/slack_test.go`.

- [ ] **Test** (`httptest`): a `Slack` alerter POSTs to the webhook URL a JSON body containing the ticket id, urgency, type, and a dashboard link; returns nil on 200; returns an error on non-2xx (but the caller treats it best-effort). Assert the payload includes the urgency and ticket id.

- [ ] **Implement** `internal/alert/slack.go`:
```go
type Slack struct {
	webhookURL string
	httpClient *http.Client
	baseAppURL string // for the dashboard link, e.g. "" → relative
}
var _ Alerter = (*Slack)(nil)
func NewSlack(webhookURL, baseAppURL string, hc *http.Client) *Slack { ... } // nil hc → 10s client
func (s *Slack) Alert(ctx context.Context, t domain.Ticket) error {
	text := fmt.Sprintf(":rotating_light: *Urgent ticket* `%s` — urgency=%s type=%s. Review: %s/tickets/%s",
		t.ID, t.Urgency, t.Type, s.baseAppURL, t.ID)
	body, _ := json.Marshal(map[string]string{"text": text})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, s.webhookURL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.httpClient.Do(req); if err != nil { return err }
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 { return fmt.Errorf("slack webhook status %d", resp.StatusCode) }
	return nil
}
```

- [ ] Commit: `feat(alert): Slack webhook alerter`.

---

## Task 4: SMTP email alerter

**Files:** create `internal/alert/email.go`, `internal/alert/email_test.go`.

Make the actual network send injectable so the message construction is unit-testable.

- [ ] **Test:** with an injected `sendFunc` capturing args, `Email.Alert` builds an RFC822 message with the right `From`, `To` (an ops/escalation address), `Subject` (e.g. "[URGENT] ticket <id>"), and a body mentioning urgency/type/link; calls send once. TDD.

- [ ] **Implement** `internal/alert/email.go`:
```go
type sendFunc func(addr string, auth smtp.Auth, from string, to []string, msg []byte) error
type Email struct {
	host, port, username, password, from, to, baseAppURL string
	send sendFunc // defaults to smtp.SendMail
}
var _ Alerter = (*Email)(nil)
func NewEmail(host, port, user, pass, from, to, baseAppURL string) *Email { ... } // send = smtp.SendMail
func (e *Email) Alert(ctx context.Context, t domain.Ticket) error {
	subject := fmt.Sprintf("[URGENT] support ticket %s (%s)", t.ID, t.Urgency)
	body := fmt.Sprintf("Urgent ticket needs review.\nID: %s\nUrgency: %s\nType: %s\nReview: %s/tickets/%s\n",
		t.ID, t.Urgency, t.Type, e.baseAppURL, t.ID)
	msg := []byte(fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s", e.from, e.to, subject, body))
	auth := smtp.PlainAuth("", e.username, e.password, e.host)
	return e.send(e.host+":"+e.port, auth, e.from, []string{e.to}, msg)
}
```
(Note: `ctx` is accepted for interface symmetry; `smtp.SendMail` is not ctx-aware — acceptable for best-effort alerts.)

- [ ] Commit: `feat(alert): SMTP email alerter`.

---

## Task 5: Multi alerter + config wiring

**Files:** create `internal/alert/multi.go`, `internal/alert/multi_test.go`; modify `cmd/server/main.go`.

- [ ] **Test** (`multi_test.go`): `Multi` calls every child alerter; if one returns an error the others STILL run (best-effort) and `Multi.Alert` returns nil (logs the failures). Use `Recording` + a failing stub. TDD.

- [ ] **Implement** `internal/alert/multi.go`:
```go
type Multi struct{ alerters []Alerter }
var _ Alerter = (*Multi)(nil)
func NewMulti(a ...Alerter) *Multi { return &Multi{alerters: a} }
func (m *Multi) Alert(ctx context.Context, t domain.Ticket) error {
	for _, a := range m.alerters {
		if err := a.Alert(ctx, t); err != nil {
			log.Printf("alert: %T failed for ticket %s: %v", a, t.ID, err)
		}
	}
	return nil // best-effort: never propagate
}

// FromConfig builds the alerter chain: always Log, plus Slack and/or Email when configured.
func FromConfig(c config.Config) Alerter {
	as := []Alerter{NewLog()}
	if c.SlackWebhookURL != "" { as = append(as, NewSlack(c.SlackWebhookURL, "", nil)) }
	if c.SMTPHost != "" && c.SMTPFrom != "" {
		as = append(as, NewEmail(c.SMTPHost, c.SMTPPort, c.SMTPUsername, c.SMTPPassword, c.SMTPFrom, c.SMTPFrom, ""))
	}
	return NewMulti(as...)
}
```
(If importing `config` into `alert` creates a cycle, instead pass primitive args to `FromConfig` or build the `Multi` in `main`. Verify no cycle; `config` imports nothing app-specific, so `alert → config` is fine.)

- [ ] **Wire `main.go`:** replace `alert.NewLog()` with `alert.FromConfig(cfg)`.

- [ ] Commit: `feat(alert): best-effort Multi fan-out built from config`.

---

## Task 6: IMAP poller

**Files:** create `internal/ingest/imap/poller.go`; modify `cmd/server/main.go`, `CLAUDE.md`.

The poller is integration-level (validated manually against a real inbox). Keep the testable logic (parse + ingest) in `ParseRFC822` (Task 2); the poller is thin glue.

- [ ] **Implement** `internal/ingest/imap/poller.go`:
```go
// Poller watches an IMAP mailbox and feeds unseen messages into the orchestrator.
type Poller struct {
	cfg  config.Config
	ing  Ingestor // interface: Ingest(ctx, source string, e domain.Email) (domain.Ticket, error)
	every time.Duration
}
type Ingestor interface {
	Ingest(ctx context.Context, source string, e domain.Email) (domain.Ticket, error)
}
func New(cfg config.Config, ing Ingestor) *Poller { ... }

// Run blocks, polling until ctx is cancelled. Best-effort: logs and retries on errors.
func (p *Poller) Run(ctx context.Context) {
	t := time.NewTicker(p.every); defer t.Stop()
	for {
		if err := p.pollOnce(ctx); err != nil { log.Printf("imap poll: %v", err) }
		select {
		case <-ctx.Done(): return
		case <-t.C:
		}
	}
}

// pollOnce: dial+TLS to IMAPHost:IMAPPort, login, select mailbox, search UNSEEN,
// fetch each raw message, ParseRFC822, p.ing.Ingest(ctx,"imap",email) (idempotent
// via DedupeKey), then mark the message \Seen. Use github.com/emersion/go-imap/v2.
```
- The orchestrator already implements `Ingest(ctx, source, e)` — it satisfies `Ingestor`. Idempotency: re-polling a message that was processed but not yet marked Seen is safe (dedupe key → existing ticket returned).
- **Confirm the exact go-imap/v2 API at implementation time** (it evolves); the structure above is the contract.

- [ ] **Wire `main.go`:** after building the orchestrator, if `cfg.IMAPEnabled()` start the poller in a goroutine:
```go
if cfg.IMAPEnabled() {
	poller := imap.New(cfg, o)
	go poller.Run(ctx)
	log.Printf("imap poller watching %s/%s every %ds", cfg.IMAPHost, cfg.IMAPMailbox, cfg.IMAPPollSeconds)
}
```

- [ ] **Manual validation:** point `IMAP*` at a dedicated test inbox (e.g. a Gmail account with an app password), send it an email, confirm a ticket appears in the dashboard sourced `imap`, and that re-sending the same Message-ID does not duplicate it.

- [ ] Update `CLAUDE.md` (ingestion now has http + imap; alerting now Slack/email).
- [ ] Commit: `feat(ingest): IMAP poller feeding the orchestrator`.

---

## Plan 5 Definition of Done
- [ ] `go vet ./...`, `go build ./...` clean; `go test ./...` green (parse/slack/email/multi tested; IMAP poller is glue).
- [ ] With Slack/SMTP configured, a critical ticket fires a real alert (best-effort; failures never block routing).
- [ ] With IMAP configured, emails sent to the watched mailbox become tickets (source `imap`), idempotent on Message-ID.
- [ ] Features stay OFF when unconfigured (server runs exactly as before with only `DASHSCOPE_API_KEY`/DB set).

## Next
- **Plan 6 — Eval harness + gold dataset + threshold calibration** (`…-plan-6-eval-harness.md`).
- **Plan 7 — Deployment + submission deliverables** (`…-plan-7-deployment-submission.md`).
