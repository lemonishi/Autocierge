# SupportSentinel — Plan 1: Foundation & Core Vertical Slice

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up the Go project and a fully-audited state-machine pipeline that takes an email via HTTP and walks it through both human-in-the-loop checkpoints to `RESOLVED`, persisting every transition in PostgreSQL — using a *fake* classifier (no Qwen yet).

**Architecture:** A deterministic Go state machine (`orchestrator`) owns all control flow and gates HITL by confidence/criticality. A `Classifier` interface decouples the brain (faked here, Qwen in Plan 2). A `Store` (pgx) persists tickets/emails/classifications/replies and an append-only `audit_log`; every state change goes through one transactional `Apply` method that records the audit row atomically. An HTTP layer exposes ingestion + the two checkpoint actions.

**Tech Stack:** Go 1.22+, jackc/pgx v5 (pgxpool), google/uuid, stretchr/testify, net/http ServeMux. PostgreSQL (local for tests via `TEST_DATABASE_URL`; Alibaba Cloud RDS in prod). No Docker.

**Spec:** `docs/superpowers/specs/2026-06-05-supportsentinel-design.md`

**Module path:** `github.com/sathwiik/supportsentinel` (adjust to your actual GitHub repo before pushing; if changed, update every import in this plan accordingly).

---

## File Structure (Plan 1)

```
go.mod  go.sum
Makefile
LICENSE                      → MIT
CLAUDE.md                    → seeded project memory (impl task #1)
app.env.example
cmd/server/main.go           → wires config, store, fake classifier, orchestrator, HTTP
internal/domain/domain.go    → taxonomies (enums), core structs, Classifier interface
internal/domain/domain_test.go
internal/config/config.go    → env loading
internal/config/config_test.go
internal/store/schema.sql     → embedded DDL
internal/store/store.go       → pgxpool, schema apply, Apply(transition), CRUD
internal/store/store_test.go
internal/classify/fake.go     → deterministic fake classifier
internal/classify/fake_test.go
internal/alert/alert.go       → Alerter interface + recording/noop impl
internal/orchestrator/orchestrator.go
internal/orchestrator/orchestrator_test.go
internal/httpapi/server.go    → HTTP handlers
internal/httpapi/server_test.go  → end-to-end vertical slice test
```

---

## Task 0: Project skeleton, CLAUDE.md, Makefile, LICENSE

**Files:**
- Create: `go.mod`, `Makefile`, `LICENSE`, `CLAUDE.md`, `app.env.example`
- Modify: `.gitignore` (already exists from spec commit)

- [ ] **Step 1: Initialize the Go module**

Run:
```bash
go mod init github.com/sathwiik/supportsentinel
go get github.com/jackc/pgx/v5@latest
go get github.com/google/uuid@latest
go get github.com/stretchr/testify@latest
```
Expected: `go.mod` created listing the three deps; `go.sum` populated.

- [ ] **Step 2: Create `LICENSE` (MIT)**

```text
MIT License

Copyright (c) 2026 SupportSentinel

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

- [ ] **Step 3: Create `app.env.example`**

```bash
# Server
PORT=8080
# HITL classification confidence gate (0..1). Calibrated later via the eval harness (Plan 6).
CONFIDENCE_THRESHOLD=0.75
# PostgreSQL connection (prod: Alibaba Cloud RDS)
DATABASE_URL=postgres://postgres:postgres@localhost:5432/supportsentinel?sslmode=disable
# Tests use a separate database; leave unset to SKIP DB-backed tests.
TEST_DATABASE_URL=postgres://postgres:postgres@localhost:5432/supportsentinel_test?sslmode=disable
# Qwen / DashScope (added in Plan 2)
# DASHSCOPE_API_KEY=
```

- [ ] **Step 4: Create `Makefile`**

```make
.PHONY: dev test test-db build tidy

tidy:
	go mod tidy

dev:
	go run ./cmd/server

build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/server ./cmd/server

# Unit tests + DB-backed tests (set TEST_DATABASE_URL to enable the latter).
test:
	go test ./...

# Convenience: create the local test database (requires a running local Postgres).
test-db:
	createdb supportsentinel_test || true
```

- [ ] **Step 5: Create seeded `CLAUDE.md`**

```markdown
# SupportSentinel — Project Memory

Autopilot support-ticket agent. Hackathon Track 4 (QwenCloud / Alibaba Cloud).
Full design: `docs/superpowers/specs/2026-06-05-supportsentinel-design.md`.
Plans: `docs/superpowers/plans/`.

## Stack
- Backend: Go 1.22+ (net/http ServeMux, jackc/pgx v5, google/uuid, testify).
- AI core: Qwen via Alibaba Cloud DashScope (added in Plan 2). The Qwen client
  (`internal/qwen/client.go`) is the primary "Proof of Alibaba Cloud" artifact.
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
```

- [ ] **Step 6: Verify it builds and commit**

Run:
```bash
go build ./... && echo OK
```
Expected: `OK` (no packages yet beyond module — succeeds with no output then `OK`).

```bash
git add -A
git commit -m "chore: project skeleton, CLAUDE.md, Makefile, MIT license"
```

---

## Task 1: Domain taxonomies, core structs, Classifier interface

**Files:**
- Create: `internal/domain/domain.go`
- Test: `internal/domain/domain_test.go`

- [ ] **Step 1: Write the failing test**

`internal/domain/domain_test.go`:
```go
package domain

import "testing"

func TestDepartmentForType(t *testing.T) {
	cases := map[TicketType]Department{
		TypeBilling:        DeptBilling,
		TypeTechnical:      DeptEngineering,
		TypeAccount:        DeptAccounts,
		TypeFeatureRequest: DeptProduct,
		TypeGeneral:        DeptSupportTier1,
	}
	for tt, want := range cases {
		if got := DepartmentForType(tt); got != want {
			t.Fatalf("DepartmentForType(%q) = %q, want %q", tt, got, want)
		}
	}
}

func TestValidUrgency(t *testing.T) {
	if !ValidUrgency("critical") {
		t.Fatal("critical should be valid")
	}
	if ValidUrgency("meltdown") {
		t.Fatal("meltdown should be invalid")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/ -run TestDepartmentForType -v`
Expected: FAIL — undefined: `DepartmentForType`, `TypeBilling`, etc.

- [ ] **Step 3: Write the implementation**

`internal/domain/domain.go`:
```go
// Package domain defines the fixed taxonomies, core entities, and the
// Classifier interface. The enums here are the hard contract shared by the
// classifier, orchestrator, store, dashboard, and eval harness.
package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Urgency string

const (
	UrgencyLow      Urgency = "low"
	UrgencyNormal   Urgency = "normal"
	UrgencyHigh     Urgency = "high"
	UrgencyCritical Urgency = "critical"
)

type TicketType string

const (
	TypeBilling        TicketType = "billing"
	TypeTechnical      TicketType = "technical"
	TypeAccount        TicketType = "account"
	TypeFeatureRequest TicketType = "feature_request"
	TypeGeneral        TicketType = "general"
)

type Department string

const (
	DeptBilling      Department = "billing"
	DeptEngineering  Department = "engineering"
	DeptAccounts     Department = "accounts"
	DeptProduct      Department = "product"
	DeptSupportTier1 Department = "support_tier1"
)

type State string

const (
	StateNew                          State = "NEW"
	StateClassifying                  State = "CLASSIFYING"
	StateAwaitingClassificationReview State = "AWAITING_CLASSIFICATION_REVIEW"
	StateRouted                       State = "ROUTED"
	StateDrafting                     State = "DRAFTING"
	StateAwaitingReplyApproval        State = "AWAITING_REPLY_APPROVAL"
	StateResolved                     State = "RESOLVED"
	StateFailed                       State = "FAILED"
)

// DepartmentForType is the default routing map (a human may override at Checkpoint 1).
func DepartmentForType(t TicketType) Department {
	switch t {
	case TypeBilling:
		return DeptBilling
	case TypeTechnical:
		return DeptEngineering
	case TypeAccount:
		return DeptAccounts
	case TypeFeatureRequest:
		return DeptProduct
	default:
		return DeptSupportTier1
	}
}

func ValidUrgency(u Urgency) bool {
	switch u {
	case UrgencyLow, UrgencyNormal, UrgencyHigh, UrgencyCritical:
		return true
	}
	return false
}

func ValidType(t TicketType) bool {
	switch t {
	case TypeBilling, TypeTechnical, TypeAccount, TypeFeatureRequest, TypeGeneral:
		return true
	}
	return false
}

func ValidDepartment(d Department) bool {
	switch d {
	case DeptBilling, DeptEngineering, DeptAccounts, DeptProduct, DeptSupportTier1:
		return true
	}
	return false
}

// Email is the normalized inbound message produced by every ingestion adapter.
type Email struct {
	ID         uuid.UUID
	TicketID   uuid.UUID
	FromAddr   string
	ToAddr     string
	Subject    string
	Body       string
	Raw        map[string]any
	DedupeKey  string // IMAP message-id or hash(from+subject+body); enforces idempotency
	ReceivedAt time.Time
}

type Ticket struct {
	ID         uuid.UUID
	State      State
	Source     string // "http" | "imap"
	Urgency    Urgency
	Type       TicketType
	Department Department
	Confidence float64
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Classification is one classifier output (append-only history in DB).
type Classification struct {
	Urgency    Urgency
	Type       TicketType
	Department Department
	Confidence float64
	Reasoning  string
	ToolsUsed  map[string]any
	Model      string
}

// Classifier is the AI brain. Faked in Plan 1; Qwen/DashScope in Plan 2.
type Classifier interface {
	Classify(ctx context.Context, e Email) (Classification, error)
	DraftReply(ctx context.Context, t Ticket, e Email) (string, error)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/domain/ -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add internal/domain/
git commit -m "feat(domain): fixed taxonomies, core structs, Classifier interface"
```

---

## Task 2: Config loading

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test**

`internal/config/config_test.go`:
```go
package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("PORT", "")
	t.Setenv("CONFIDENCE_THRESHOLD", "")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Port != "8080" {
		t.Fatalf("Port = %q, want 8080", c.Port)
	}
	if c.ConfidenceThreshold != 0.75 {
		t.Fatalf("ConfidenceThreshold = %v, want 0.75", c.ConfidenceThreshold)
	}
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected error when DATABASE_URL is empty")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -v`
Expected: FAIL — undefined: `Load`.

- [ ] **Step 3: Write the implementation**

`internal/config/config.go`:
```go
// Package config loads runtime configuration from environment variables.
package config

import (
	"errors"
	"os"
	"strconv"
)

type Config struct {
	Port                string
	DatabaseURL         string
	ConfidenceThreshold float64
}

func Load() (Config, error) {
	c := Config{
		Port:                getenv("PORT", "8080"),
		DatabaseURL:         os.Getenv("DATABASE_URL"),
		ConfidenceThreshold: 0.75,
	}
	if c.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if v := os.Getenv("CONFIDENCE_THRESHOLD"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return Config{}, errors.New("CONFIDENCE_THRESHOLD must be a float")
		}
		c.ConfidenceThreshold = f
	}
	return c, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat(config): env-based configuration loader"
```

---

## Task 3: Store — schema, connection, and the transactional `Apply`

**Files:**
- Create: `internal/store/schema.sql`, `internal/store/store.go`
- Test: `internal/store/store_test.go`

> DB-backed tests skip automatically unless `TEST_DATABASE_URL` is set. Run a local Postgres and `make test-db` first to exercise them.

- [ ] **Step 1: Create the embedded schema**

`internal/store/schema.sql`:
```sql
CREATE TABLE IF NOT EXISTS tickets (
  id          UUID PRIMARY KEY,
  state       TEXT NOT NULL,
  source      TEXT NOT NULL,
  urgency     TEXT,
  type        TEXT,
  department  TEXT,
  confidence  NUMERIC,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS emails (
  id          UUID PRIMARY KEY,
  ticket_id   UUID NOT NULL REFERENCES tickets(id),
  from_addr   TEXT NOT NULL,
  to_addr     TEXT,
  subject     TEXT,
  body        TEXT,
  raw         JSONB,
  dedupe_key  TEXT UNIQUE,
  received_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS classifications (
  id         UUID PRIMARY KEY,
  ticket_id  UUID NOT NULL REFERENCES tickets(id),
  urgency    TEXT,
  type       TEXT,
  department TEXT,
  confidence NUMERIC,
  reasoning  TEXT,
  tools_used JSONB,
  model      TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS replies (
  id         UUID PRIMARY KEY,
  ticket_id  UUID NOT NULL REFERENCES tickets(id),
  draft_text TEXT,
  final_text TEXT,
  status     TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS audit_log (
  id         UUID PRIMARY KEY,
  ticket_id  UUID NOT NULL REFERENCES tickets(id),
  from_state TEXT,
  to_state   TEXT NOT NULL,
  actor      TEXT NOT NULL,
  payload    JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS customers (
  email          TEXT PRIMARY KEY,
  name           TEXT,
  tier           TEXT,
  account_status TEXT
);
```

- [ ] **Step 2: Write the failing test**

`internal/store/store_test.go`:
```go
package store

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/sathwiik/supportsentinel/internal/domain"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping DB-backed test")
	}
	s, err := New(context.Background(), url)
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	// Clean tables for isolation.
	_, err = s.pool.Exec(context.Background(),
		`TRUNCATE audit_log, replies, classifications, emails, tickets, customers RESTART IDENTITY CASCADE`)
	require.NoError(t, err)
	return s
}

func TestCreateTicketWithEmailIsIdempotent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	e := domain.Email{
		FromAddr: "a@b.com", Subject: "help", Body: "thing broke",
		DedupeKey: "dk-1",
	}
	tk1, err := s.CreateTicketWithEmail(ctx, "http", e)
	require.NoError(t, err)
	require.Equal(t, domain.StateNew, tk1.State)

	// Same dedupe key returns the SAME ticket, not a new one.
	tk2, err := s.CreateTicketWithEmail(ctx, "http", e)
	require.NoError(t, err)
	require.Equal(t, tk1.ID, tk2.ID)
}

func TestApplyTransitionWritesAuditAndEnforcesFromState(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	tk, err := s.CreateTicketWithEmail(ctx, "http",
		domain.Email{FromAddr: "a@b.com", Subject: "s", Body: "b", DedupeKey: "dk-2"})
	require.NoError(t, err)

	urg := domain.UrgencyHigh
	typ := domain.TypeTechnical
	dep := domain.DeptEngineering
	conf := 0.9
	err = s.Apply(ctx, Transition{
		TicketID: tk.ID, From: domain.StateNew, To: domain.StateClassifying,
		Actor: "system",
	})
	require.NoError(t, err)

	err = s.Apply(ctx, Transition{
		TicketID: tk.ID, From: domain.StateClassifying, To: domain.StateRouted,
		Actor: "qwen", SetUrgency: &urg, SetType: &typ, SetDepartment: &dep, SetConfidence: &conf,
	})
	require.NoError(t, err)

	got, err := s.GetTicket(ctx, tk.ID)
	require.NoError(t, err)
	require.Equal(t, domain.StateRouted, got.State)
	require.Equal(t, domain.UrgencyHigh, got.Urgency)
	require.Equal(t, domain.DeptEngineering, got.Department)

	// Wrong From must conflict.
	err = s.Apply(ctx, Transition{
		TicketID: tk.ID, From: domain.StateNew, To: domain.StateResolved, Actor: "system",
	})
	require.ErrorIs(t, err, ErrStateConflict)

	// Audit log recorded both successful transitions.
	rows, err := s.AuditLog(ctx, tk.ID)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, domain.StateRouted, rows[1].To)
}

func TestSaveClassificationAndReply(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	tk, err := s.CreateTicketWithEmail(ctx, "http",
		domain.Email{FromAddr: "a@b.com", Subject: "s", Body: "b", DedupeKey: "dk-3"})
	require.NoError(t, err)

	require.NoError(t, s.SaveClassification(ctx, tk.ID, domain.Classification{
		Urgency: domain.UrgencyNormal, Type: domain.TypeBilling,
		Department: domain.DeptBilling, Confidence: 0.8, Reasoning: "mentions invoice",
		Model: "fake",
	}))

	id, err := s.SaveReplyDraft(ctx, tk.ID, "Hi, we are looking into it.")
	require.NoError(t, err)
	require.NoError(t, s.FinalizeReply(ctx, id, "approved", "Hi, fixed!"))
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/store/ -v`
Expected: FAIL — undefined `New`, `Store`, `Transition`, etc. (Or SKIP if `TEST_DATABASE_URL` unset — set it to actually drive the test.)

- [ ] **Step 4: Write the implementation**

`internal/store/store.go`:
```go
// Package store is the PostgreSQL persistence layer. Every state change goes
// through Apply, which updates the ticket and writes the audit_log row in one
// transaction, guaranteeing a complete, replayable history.
package store

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sathwiik/supportsentinel/internal/domain"
)

//go:embed schema.sql
var schemaSQL string

var ErrStateConflict = errors.New("ticket not in expected state")
var ErrNotFound = errors.New("not found")

type Store struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, url string) (*Store, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if _, err := pool.Exec(ctx, schemaSQL); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

// CreateTicketWithEmail inserts a NEW ticket plus its email atomically. If an
// email with the same DedupeKey already exists, it returns the existing ticket
// (idempotent ingestion).
func (s *Store) CreateTicketWithEmail(ctx context.Context, source string, e domain.Email) (domain.Ticket, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Ticket{}, err
	}
	defer tx.Rollback(ctx)

	if e.DedupeKey != "" {
		var existing uuid.UUID
		err := tx.QueryRow(ctx, `SELECT ticket_id FROM emails WHERE dedupe_key = $1`, e.DedupeKey).Scan(&existing)
		if err == nil {
			_ = tx.Commit(ctx)
			return s.GetTicket(ctx, existing)
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return domain.Ticket{}, err
		}
	}

	ticketID := uuid.New()
	now := time.Now().UTC()
	_, err = tx.Exec(ctx,
		`INSERT INTO tickets (id, state, source, created_at, updated_at) VALUES ($1,$2,$3,$4,$4)`,
		ticketID, domain.StateNew, source, now)
	if err != nil {
		return domain.Ticket{}, err
	}
	rawJSON, _ := json.Marshal(e.Raw)
	_, err = tx.Exec(ctx,
		`INSERT INTO emails (id, ticket_id, from_addr, to_addr, subject, body, raw, dedupe_key, received_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		uuid.New(), ticketID, e.FromAddr, e.ToAddr, e.Subject, e.Body, rawJSON, nullStr(e.DedupeKey), now)
	if err != nil {
		return domain.Ticket{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Ticket{}, err
	}
	return domain.Ticket{ID: ticketID, State: domain.StateNew, Source: source, CreatedAt: now, UpdatedAt: now}, nil
}

func (s *Store) GetTicket(ctx context.Context, id uuid.UUID) (domain.Ticket, error) {
	var t domain.Ticket
	var urg, typ, dep *string
	var conf *float64
	err := s.pool.QueryRow(ctx,
		`SELECT id, state, source, urgency, type, department, confidence, created_at, updated_at
		 FROM tickets WHERE id = $1`, id).
		Scan(&t.ID, &t.State, &t.Source, &urg, &typ, &dep, &conf, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Ticket{}, ErrNotFound
	}
	if err != nil {
		return domain.Ticket{}, err
	}
	if urg != nil {
		t.Urgency = domain.Urgency(*urg)
	}
	if typ != nil {
		t.Type = domain.TicketType(*typ)
	}
	if dep != nil {
		t.Department = domain.Department(*dep)
	}
	if conf != nil {
		t.Confidence = *conf
	}
	return t, nil
}

func (s *Store) GetEmailByTicket(ctx context.Context, ticketID uuid.UUID) (domain.Email, error) {
	var e domain.Email
	var rawJSON []byte
	var dedupe *string
	err := s.pool.QueryRow(ctx,
		`SELECT id, ticket_id, from_addr, COALESCE(to_addr,''), COALESCE(subject,''), COALESCE(body,''), raw, dedupe_key, received_at
		 FROM emails WHERE ticket_id = $1`, ticketID).
		Scan(&e.ID, &e.TicketID, &e.FromAddr, &e.ToAddr, &e.Subject, &e.Body, &rawJSON, &dedupe, &e.ReceivedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Email{}, ErrNotFound
	}
	if err != nil {
		return domain.Email{}, err
	}
	if len(rawJSON) > 0 {
		_ = json.Unmarshal(rawJSON, &e.Raw)
	}
	if dedupe != nil {
		e.DedupeKey = *dedupe
	}
	return e, nil
}

// Transition describes a single state change plus optional ticket field updates.
type Transition struct {
	TicketID      uuid.UUID
	From          domain.State // expected current state; "" skips the check
	To            domain.State
	Actor         string // "system" | "qwen" | "human:<id>"
	Payload       any
	SetUrgency    *domain.Urgency
	SetType       *domain.TicketType
	SetDepartment *domain.Department
	SetConfidence *float64
}

// Apply performs the transition and writes the audit row in one transaction.
func (s *Store) Apply(ctx context.Context, tr Transition) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var current domain.State
	if err := tx.QueryRow(ctx, `SELECT state FROM tickets WHERE id = $1 FOR UPDATE`, tr.TicketID).Scan(&current); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if tr.From != "" && current != tr.From {
		return ErrStateConflict
	}

	_, err = tx.Exec(ctx,
		`UPDATE tickets SET state=$1,
		   urgency=COALESCE($2, urgency),
		   type=COALESCE($3, type),
		   department=COALESCE($4, department),
		   confidence=COALESCE($5, confidence),
		   updated_at=now()
		 WHERE id=$6`,
		tr.To, ptrStr(tr.SetUrgency), ptrStr(tr.SetType), ptrStr(tr.SetDepartment), tr.SetConfidence, tr.TicketID)
	if err != nil {
		return err
	}

	payloadJSON, _ := json.Marshal(tr.Payload)
	_, err = tx.Exec(ctx,
		`INSERT INTO audit_log (id, ticket_id, from_state, to_state, actor, payload)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		uuid.New(), tr.TicketID, nullStr(string(current)), tr.To, tr.Actor, payloadJSON)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) SaveClassification(ctx context.Context, ticketID uuid.UUID, c domain.Classification) error {
	toolsJSON, _ := json.Marshal(c.ToolsUsed)
	_, err := s.pool.Exec(ctx,
		`INSERT INTO classifications (id, ticket_id, urgency, type, department, confidence, reasoning, tools_used, model)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		uuid.New(), ticketID, c.Urgency, c.Type, c.Department, c.Confidence, c.Reasoning, toolsJSON, c.Model)
	return err
}

func (s *Store) SaveReplyDraft(ctx context.Context, ticketID uuid.UUID, draft string) (uuid.UUID, error) {
	id := uuid.New()
	_, err := s.pool.Exec(ctx,
		`INSERT INTO replies (id, ticket_id, draft_text, status) VALUES ($1,$2,$3,'draft')`,
		id, ticketID, draft)
	return id, err
}

func (s *Store) FinalizeReply(ctx context.Context, replyID uuid.UUID, status, finalText string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE replies SET status=$1, final_text=$2 WHERE id=$3`, status, finalText, replyID)
	return err
}

type AuditRow struct {
	From      domain.State
	To        domain.State
	Actor     string
	CreatedAt time.Time
}

func (s *Store) AuditLog(ctx context.Context, ticketID uuid.UUID) ([]AuditRow, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT COALESCE(from_state,''), to_state, actor, created_at
		 FROM audit_log WHERE ticket_id=$1 ORDER BY created_at ASC`, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditRow
	for rows.Next() {
		var r AuditRow
		if err := rows.Scan(&r.From, &r.To, &r.Actor, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// ptrStr converts a typed-string pointer (Urgency/TicketType/Department) to a
// nullable any for COALESCE updates.
func ptrStr[T ~string](p *T) any {
	if p == nil {
		return nil
	}
	return string(*p)
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `TEST_DATABASE_URL` must be set (see `app.env.example`); then:
```bash
go test ./internal/store/ -v
```
Expected: PASS (all three). Without `TEST_DATABASE_URL`: SKIP.

- [ ] **Step 6: Commit**

```bash
git add internal/store/
git commit -m "feat(store): pgx schema, idempotent ingest, transactional Apply with audit log"
```

---

## Task 4: Fake classifier

**Files:**
- Create: `internal/classify/fake.go`
- Test: `internal/classify/fake_test.go`

- [ ] **Step 1: Write the failing test**

`internal/classify/fake_test.go`:
```go
package classify

import (
	"context"
	"testing"

	"github.com/sathwiik/supportsentinel/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestFakeClassifyBilling(t *testing.T) {
	f := NewFake()
	c, err := f.Classify(context.Background(), domain.Email{
		Subject: "Question about my invoice", Body: "I was charged twice",
	})
	require.NoError(t, err)
	require.Equal(t, domain.TypeBilling, c.Type)
	require.Equal(t, domain.DeptBilling, c.Department)
	require.GreaterOrEqual(t, c.Confidence, 0.8)
}

func TestFakeClassifyCritical(t *testing.T) {
	f := NewFake()
	c, err := f.Classify(context.Background(), domain.Email{
		Subject: "URGENT: production is down", Body: "everything is failing",
	})
	require.NoError(t, err)
	require.Equal(t, domain.UrgencyCritical, c.Urgency)
}

func TestFakeClassifyAmbiguousIsLowConfidence(t *testing.T) {
	f := NewFake()
	c, err := f.Classify(context.Background(), domain.Email{
		Subject: "hello", Body: "I have a thing",
	})
	require.NoError(t, err)
	require.Less(t, c.Confidence, 0.75)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/classify/ -v`
Expected: FAIL — undefined `NewFake`.

- [ ] **Step 3: Write the implementation**

`internal/classify/fake.go`:
```go
// Package classify holds Classifier implementations. Fake is a deterministic,
// keyword-based stand-in used to build and test the pipeline before the real
// Qwen client (Plan 2) exists.
package classify

import (
	"context"
	"strings"

	"github.com/sathwiik/supportsentinel/internal/domain"
)

type Fake struct{}

func NewFake() *Fake { return &Fake{} }

func (f *Fake) Classify(_ context.Context, e domain.Email) (domain.Classification, error) {
	text := strings.ToLower(e.Subject + " " + e.Body)

	typ := domain.TypeGeneral
	conf := 0.5 // ambiguous default → below threshold → Checkpoint 1
	switch {
	case containsAny(text, "invoice", "charge", "charged", "billing", "refund", "payment"):
		typ, conf = domain.TypeBilling, 0.88
	case containsAny(text, "error", "down", "crash", "bug", "broken", "fails", "failing"):
		typ, conf = domain.TypeTechnical, 0.86
	case containsAny(text, "login", "password", "account", "locked", "access"):
		typ, conf = domain.TypeAccount, 0.84
	case containsAny(text, "feature", "request", "would be nice", "suggestion"):
		typ, conf = domain.TypeFeatureRequest, 0.82
	}

	urg := domain.UrgencyNormal
	switch {
	case containsAny(text, "urgent", "asap", "down", "outage", "immediately", "critical"):
		urg = domain.UrgencyCritical
	case containsAny(text, "soon", "important", "blocked"):
		urg = domain.UrgencyHigh
	}

	return domain.Classification{
		Urgency:    urg,
		Type:       typ,
		Department: domain.DepartmentForType(typ),
		Confidence: conf,
		Reasoning:  "fake keyword classifier",
		Model:      "fake",
	}, nil
}

func (f *Fake) DraftReply(_ context.Context, _ domain.Ticket, e domain.Email) (string, error) {
	return "Hi,\n\nThanks for reaching out about \"" + e.Subject +
		"\". We've received your message and our team is looking into it.\n\nBest,\nSupport", nil
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/classify/ -v`
Expected: PASS (all three).

- [ ] **Step 5: Commit**

```bash
git add internal/classify/
git commit -m "feat(classify): deterministic fake classifier for pipeline development"
```

---

## Task 5: Alerter interface

**Files:**
- Create: `internal/alert/alert.go`
- Test: `internal/alert/alert_test.go`

- [ ] **Step 1: Write the failing test**

`internal/alert/alert_test.go`:
```go
package alert

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/sathwiik/supportsentinel/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestRecordingAlerter(t *testing.T) {
	a := NewRecording()
	tk := domain.Ticket{ID: uuid.New(), Urgency: domain.UrgencyCritical}
	require.NoError(t, a.Alert(context.Background(), tk))
	require.Len(t, a.Sent, 1)
	require.Equal(t, tk.ID, a.Sent[0].ID)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/alert/ -v`
Expected: FAIL — undefined `NewRecording`.

- [ ] **Step 3: Write the implementation**

`internal/alert/alert.go`:
```go
// Package alert sends best-effort, one-way urgent notifications. Real Slack/email
// delivery arrives in Plan 5; alerting must NEVER block or fail the pipeline.
package alert

import (
	"context"
	"log"

	"github.com/sathwiik/supportsentinel/internal/domain"
)

type Alerter interface {
	Alert(ctx context.Context, t domain.Ticket) error
}

// Log writes alerts to stdout (default impl until Plan 5).
type Log struct{}

func NewLog() *Log { return &Log{} }

func (l *Log) Alert(_ context.Context, t domain.Ticket) error {
	log.Printf("[ALERT] urgent ticket %s (urgency=%s) needs review", t.ID, t.Urgency)
	return nil
}

// Recording captures alerts for tests.
type Recording struct{ Sent []domain.Ticket }

func NewRecording() *Recording { return &Recording{} }

func (r *Recording) Alert(_ context.Context, t domain.Ticket) error {
	r.Sent = append(r.Sent, t)
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/alert/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/alert/
git commit -m "feat(alert): best-effort Alerter interface with log and recording impls"
```

---

## Task 6: Orchestrator — ingest + classify gate

**Files:**
- Create: `internal/orchestrator/orchestrator.go`
- Test: `internal/orchestrator/orchestrator_test.go`

> Orchestrator tests use the real Store (skip without `TEST_DATABASE_URL`) so transitions + persistence are covered together.

- [ ] **Step 1: Write the failing test**

`internal/orchestrator/orchestrator_test.go`:
```go
package orchestrator

import (
	"context"
	"os"
	"testing"

	"github.com/sathwiik/supportsentinel/internal/alert"
	"github.com/sathwiik/supportsentinel/internal/classify"
	"github.com/sathwiik/supportsentinel/internal/domain"
	"github.com/sathwiik/supportsentinel/internal/store"
	"github.com/stretchr/testify/require"
)

func newOrch(t *testing.T) (*Orchestrator, *store.Store, *alert.Recording) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	s, err := store.New(context.Background(), url)
	require.NoError(t, err)
	t.Cleanup(s.Close)
	_, err = s.Pool().Exec(context.Background(),
		`TRUNCATE audit_log, replies, classifications, emails, tickets, customers RESTART IDENTITY CASCADE`)
	require.NoError(t, err)
	rec := alert.NewRecording()
	o := New(s, classify.NewFake(), rec, 0.75)
	return o, s, rec
}

func TestIngestHighConfidenceAutoRoutesToDraft(t *testing.T) {
	o, s, rec := newOrch(t)
	ctx := context.Background()
	tk, err := o.Ingest(ctx, "http", domain.Email{
		FromAddr: "c@x.com", Subject: "invoice problem", Body: "charged twice", DedupeKey: "o1",
	})
	require.NoError(t, err)

	got, err := s.GetTicket(ctx, tk.ID)
	require.NoError(t, err)
	// High-confidence, non-critical → auto-routed then drafted → parked at reply approval.
	require.Equal(t, domain.StateAwaitingReplyApproval, got.State)
	require.Equal(t, domain.TypeBilling, got.Type)
	require.Empty(t, rec.Sent)
}

func TestIngestCriticalAlwaysParksAtReviewAndAlerts(t *testing.T) {
	o, s, rec := newOrch(t)
	ctx := context.Background()
	tk, err := o.Ingest(ctx, "http", domain.Email{
		FromAddr: "c@x.com", Subject: "URGENT outage", Body: "production down", DedupeKey: "o2",
	})
	require.NoError(t, err)

	got, err := s.GetTicket(ctx, tk.ID)
	require.NoError(t, err)
	require.Equal(t, domain.StateAwaitingClassificationReview, got.State)
	require.Len(t, rec.Sent, 1) // alert fired for critical
}

func TestIngestLowConfidenceParksAtReview(t *testing.T) {
	o, s, _ := newOrch(t)
	ctx := context.Background()
	tk, err := o.Ingest(ctx, "http", domain.Email{
		FromAddr: "c@x.com", Subject: "hello", Body: "I have a thing", DedupeKey: "o3",
	})
	require.NoError(t, err)
	got, err := s.GetTicket(ctx, tk.ID)
	require.NoError(t, err)
	require.Equal(t, domain.StateAwaitingClassificationReview, got.State)
}
```

- [ ] **Step 2: Add `Pool()` accessor to the store** (needed by orchestrator tests to truncate)

In `internal/store/store.go`, add:
```go
// Pool exposes the underlying pool for test setup/teardown. Not for app use.
func (s *Store) Pool() *pgxpool.Pool { return s.pool }
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/orchestrator/ -v`
Expected: FAIL — undefined `New`, `Orchestrator`, `Ingest`.

- [ ] **Step 4: Write the implementation**

`internal/orchestrator/orchestrator.go`:
```go
// Package orchestrator is the agent core: a deterministic state machine that
// drives a ticket from ingestion through both human-in-the-loop checkpoints.
// It gates routing on confidence and criticality, and FAILS TOWARD A HUMAN —
// any classifier error parks the ticket for review rather than dropping it.
package orchestrator

import (
	"context"
	"log"

	"github.com/google/uuid"
	"github.com/sathwiik/supportsentinel/internal/alert"
	"github.com/sathwiik/supportsentinel/internal/domain"
	"github.com/sathwiik/supportsentinel/internal/store"
)

type Orchestrator struct {
	store     *store.Store
	clf       domain.Classifier
	alerter   alert.Alerter
	threshold float64
}

func New(s *store.Store, clf domain.Classifier, a alert.Alerter, threshold float64) *Orchestrator {
	return &Orchestrator{store: s, clf: clf, alerter: a, threshold: threshold}
}

// Ingest creates the ticket and runs classification synchronously.
func (o *Orchestrator) Ingest(ctx context.Context, source string, e domain.Email) (domain.Ticket, error) {
	tk, err := o.store.CreateTicketWithEmail(ctx, source, e)
	if err != nil {
		return domain.Ticket{}, err
	}
	if tk.State != domain.StateNew {
		// Idempotent re-delivery of an already-processed email; return as-is.
		return tk, nil
	}
	if err := o.runClassify(ctx, tk.ID); err != nil {
		return domain.Ticket{}, err
	}
	return o.store.GetTicket(ctx, tk.ID)
}

func (o *Orchestrator) runClassify(ctx context.Context, ticketID uuid.UUID) error {
	if err := o.store.Apply(ctx, store.Transition{
		TicketID: ticketID, From: domain.StateNew, To: domain.StateClassifying, Actor: "system",
	}); err != nil {
		return err
	}

	email, err := o.store.GetEmailByTicket(ctx, ticketID)
	if err != nil {
		return err
	}

	c, err := o.clf.Classify(ctx, email)
	if err != nil {
		// FAIL TOWARD A HUMAN: park for review with the error in the payload.
		log.Printf("classify error for %s: %v", ticketID, err)
		return o.store.Apply(ctx, store.Transition{
			TicketID: ticketID, From: domain.StateClassifying,
			To: domain.StateAwaitingClassificationReview, Actor: "system",
			Payload: map[string]any{"classify_error": err.Error()},
		})
	}

	if err := o.store.SaveClassification(ctx, ticketID, c); err != nil {
		return err
	}

	urg, typ, dep, conf := c.Urgency, c.Type, c.Department, c.Confidence
	parks := c.Urgency == domain.UrgencyCritical || c.Confidence < o.threshold
	if parks {
		if err := o.store.Apply(ctx, store.Transition{
			TicketID: ticketID, From: domain.StateClassifying,
			To: domain.StateAwaitingClassificationReview, Actor: "qwen",
			SetUrgency: &urg, SetType: &typ, SetDepartment: &dep, SetConfidence: &conf,
			Payload: c,
		}); err != nil {
			return err
		}
		if c.Urgency == domain.UrgencyCritical {
			tk, err := o.store.GetTicket(ctx, ticketID)
			if err == nil {
				_ = o.alerter.Alert(ctx, tk) // best-effort
			}
		}
		return nil
	}

	// High-confidence, non-critical → auto-route, then draft.
	if err := o.store.Apply(ctx, store.Transition{
		TicketID: ticketID, From: domain.StateClassifying, To: domain.StateRouted, Actor: "qwen",
		SetUrgency: &urg, SetType: &typ, SetDepartment: &dep, SetConfidence: &conf, Payload: c,
	}); err != nil {
		return err
	}
	return o.runDraft(ctx, ticketID)
}

func (o *Orchestrator) runDraft(ctx context.Context, ticketID uuid.UUID) error {
	if err := o.store.Apply(ctx, store.Transition{
		TicketID: ticketID, From: domain.StateRouted, To: domain.StateDrafting, Actor: "system",
	}); err != nil {
		return err
	}
	tk, err := o.store.GetTicket(ctx, ticketID)
	if err != nil {
		return err
	}
	email, err := o.store.GetEmailByTicket(ctx, ticketID)
	if err != nil {
		return err
	}
	draft, err := o.clf.DraftReply(ctx, tk, email)
	if err != nil {
		log.Printf("draft error for %s: %v", ticketID, err)
		// Stay in DRAFTING is risky; park at reply approval with empty draft so a human writes one.
		draft = ""
	}
	if _, err := o.store.SaveReplyDraft(ctx, ticketID, draft); err != nil {
		return err
	}
	return o.store.Apply(ctx, store.Transition{
		TicketID: ticketID, From: domain.StateDrafting, To: domain.StateAwaitingReplyApproval, Actor: "system",
	})
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run (with `TEST_DATABASE_URL` set): `go test ./internal/orchestrator/ -v`
Expected: PASS (all three).

- [ ] **Step 6: Commit**

```bash
git add internal/orchestrator/ internal/store/store.go
git commit -m "feat(orchestrator): ingest + classify gate (critical/low-confidence park, alert)"
```

---

## Task 7: Orchestrator — Checkpoint 1 review, Checkpoint 2 approval

**Files:**
- Modify: `internal/orchestrator/orchestrator.go`
- Modify: `internal/orchestrator/orchestrator_test.go`

- [ ] **Step 1: Add failing tests**

Append to `internal/orchestrator/orchestrator_test.go`:
```go
func TestReviewClassificationRoutesAndDrafts(t *testing.T) {
	o, s, _ := newOrch(t)
	ctx := context.Background()
	tk, err := o.Ingest(ctx, "http", domain.Email{
		FromAddr: "c@x.com", Subject: "hello", Body: "I have a thing", DedupeKey: "r1",
	})
	require.NoError(t, err)
	require.Equal(t, domain.StateAwaitingClassificationReview, tk.State)

	err = o.ReviewClassification(ctx, tk.ID, ReviewDecision{
		Urgency: domain.UrgencyNormal, Type: domain.TypeAccount, Department: domain.DeptAccounts,
	}, "alice")
	require.NoError(t, err)

	got, err := s.GetTicket(ctx, tk.ID)
	require.NoError(t, err)
	require.Equal(t, domain.StateAwaitingReplyApproval, got.State)
	require.Equal(t, domain.TypeAccount, got.Type)
}

func TestApproveReplyResolves(t *testing.T) {
	o, s, _ := newOrch(t)
	ctx := context.Background()
	tk, err := o.Ingest(ctx, "http", domain.Email{
		FromAddr: "c@x.com", Subject: "invoice", Body: "charged twice", DedupeKey: "r2",
	})
	require.NoError(t, err)
	require.Equal(t, domain.StateAwaitingReplyApproval, tk.State)

	err = o.ApproveReply(ctx, tk.ID, "Resolved your billing issue.", "bob")
	require.NoError(t, err)
	got, err := s.GetTicket(ctx, tk.ID)
	require.NoError(t, err)
	require.Equal(t, domain.StateResolved, got.State)
}

func TestRejectReplyReturnsToDrafting(t *testing.T) {
	o, s, _ := newOrch(t)
	ctx := context.Background()
	tk, err := o.Ingest(ctx, "http", domain.Email{
		FromAddr: "c@x.com", Subject: "invoice", Body: "charged twice", DedupeKey: "r3",
	})
	require.NoError(t, err)

	require.NoError(t, o.RejectReply(ctx, tk.ID, "bob"))
	got, err := s.GetTicket(ctx, tk.ID)
	require.NoError(t, err)
	// Re-draft runs immediately, so it parks at reply approval again.
	require.Equal(t, domain.StateAwaitingReplyApproval, got.State)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/orchestrator/ -run 'TestReviewClassification|TestApproveReply|TestRejectReply' -v`
Expected: FAIL — undefined `ReviewDecision`, `ReviewClassification`, `ApproveReply`, `RejectReply`.

- [ ] **Step 3: Implement the checkpoint methods**

Append to `internal/orchestrator/orchestrator.go`:
```go
// ReviewDecision is the human's Checkpoint-1 input. Empty fields fall back to
// the model's stored values (handled by COALESCE in Apply when pointers are nil).
type ReviewDecision struct {
	Urgency    domain.Urgency
	Type       domain.TicketType
	Department domain.Department
}

// ReviewClassification (Checkpoint 1): a human confirms/overrides the routing,
// after which the ticket routes and a reply is drafted.
func (o *Orchestrator) ReviewClassification(ctx context.Context, ticketID uuid.UUID, d ReviewDecision, reviewer string) error {
	tr := store.Transition{
		TicketID: ticketID, From: domain.StateAwaitingClassificationReview,
		To: domain.StateRouted, Actor: "human:" + reviewer, Payload: d,
	}
	if d.Urgency != "" {
		u := d.Urgency
		tr.SetUrgency = &u
	}
	if d.Type != "" {
		t := d.Type
		tr.SetType = &t
	}
	if d.Department != "" {
		dep := d.Department
		tr.SetDepartment = &dep
	}
	if err := o.store.Apply(ctx, tr); err != nil {
		return err
	}
	return o.runDraft(ctx, ticketID)
}

// ApproveReply (Checkpoint 2): human approves/edits the draft; ticket resolves.
func (o *Orchestrator) ApproveReply(ctx context.Context, ticketID uuid.UUID, finalText, reviewer string) error {
	replyID, err := o.store.LatestReplyID(ctx, ticketID)
	if err != nil {
		return err
	}
	if err := o.store.FinalizeReply(ctx, replyID, "approved", finalText); err != nil {
		return err
	}
	return o.store.Apply(ctx, store.Transition{
		TicketID: ticketID, From: domain.StateAwaitingReplyApproval,
		To: domain.StateResolved, Actor: "human:" + reviewer,
		Payload: map[string]any{"final_text": finalText},
	})
}

// RejectReply (Checkpoint 2): human rejects the draft; re-draft and park again.
func (o *Orchestrator) RejectReply(ctx context.Context, ticketID uuid.UUID, reviewer string) error {
	replyID, err := o.store.LatestReplyID(ctx, ticketID)
	if err != nil {
		return err
	}
	if err := o.store.FinalizeReply(ctx, replyID, "rejected", ""); err != nil {
		return err
	}
	if err := o.store.Apply(ctx, store.Transition{
		TicketID: ticketID, From: domain.StateAwaitingReplyApproval,
		To: domain.StateDrafting, Actor: "human:" + reviewer,
	}); err != nil {
		return err
	}
	// Re-draft from DRAFTING. runDraft expects ROUTED→DRAFTING, so issue the draft directly here.
	return o.redraftFromDrafting(ctx, ticketID)
}

// redraftFromDrafting drafts a reply when already in DRAFTING and parks for approval.
func (o *Orchestrator) redraftFromDrafting(ctx context.Context, ticketID uuid.UUID) error {
	tk, err := o.store.GetTicket(ctx, ticketID)
	if err != nil {
		return err
	}
	email, err := o.store.GetEmailByTicket(ctx, ticketID)
	if err != nil {
		return err
	}
	draft, err := o.clf.DraftReply(ctx, tk, email)
	if err != nil {
		draft = ""
	}
	if _, err := o.store.SaveReplyDraft(ctx, ticketID, draft); err != nil {
		return err
	}
	return o.store.Apply(ctx, store.Transition{
		TicketID: ticketID, From: domain.StateDrafting,
		To: domain.StateAwaitingReplyApproval, Actor: "system",
	})
}
```

- [ ] **Step 4: Add `LatestReplyID` to the store**

In `internal/store/store.go`, add:
```go
func (s *Store) LatestReplyID(ctx context.Context, ticketID uuid.UUID) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.pool.QueryRow(ctx,
		`SELECT id FROM replies WHERE ticket_id=$1 ORDER BY created_at DESC LIMIT 1`, ticketID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	return id, err
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/orchestrator/ -v`
Expected: PASS (all six).

- [ ] **Step 6: Commit**

```bash
git add internal/orchestrator/ internal/store/store.go
git commit -m "feat(orchestrator): Checkpoint 1 review and Checkpoint 2 approve/reject"
```

---

## Task 8: HTTP API + end-to-end vertical slice

**Files:**
- Create: `internal/httpapi/server.go`, `cmd/server/main.go`
- Test: `internal/httpapi/server_test.go`

- [ ] **Step 1: Write the failing end-to-end test**

`internal/httpapi/server_test.go`:
```go
package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/sathwiik/supportsentinel/internal/alert"
	"github.com/sathwiik/supportsentinel/internal/classify"
	"github.com/sathwiik/supportsentinel/internal/orchestrator"
	"github.com/sathwiik/supportsentinel/internal/store"
	"github.com/stretchr/testify/require"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	s, err := store.New(context.Background(), url)
	require.NoError(t, err)
	t.Cleanup(s.Close)
	_, err = s.Pool().Exec(context.Background(),
		`TRUNCATE audit_log, replies, classifications, emails, tickets, customers RESTART IDENTITY CASCADE`)
	require.NoError(t, err)
	o := orchestrator.New(s, classify.NewFake(), alert.NewRecording(), 0.75)
	srv := httptest.NewServer(NewServer(o, s))
	t.Cleanup(srv.Close)
	return srv
}

func postJSON(t *testing.T, url string, body any) map[string]any {
	t.Helper()
	b, _ := json.Marshal(body)
	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Less(t, resp.StatusCode, 300)
	var out map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	return out
}

func TestEndToEndSliceAutoRouteThenApprove(t *testing.T) {
	srv := newTestServer(t)

	// 1. Submit a clear billing email → auto-routes → parks at reply approval.
	created := postJSON(t, srv.URL+"/api/emails", map[string]string{
		"from": "cust@acme.com", "subject": "invoice issue", "body": "charged twice",
	})
	id := created["id"].(string)
	require.Equal(t, "AWAITING_REPLY_APPROVAL", created["state"])

	// 2. Approve the reply → RESOLVED.
	approved := postJSON(t, srv.URL+"/api/tickets/"+id+"/reply-approval", map[string]string{
		"action": "approve", "final_text": "All sorted.", "reviewer": "alice",
	})
	require.Equal(t, "RESOLVED", approved["state"])
}

func TestEndToEndSliceCriticalReviewThenApprove(t *testing.T) {
	srv := newTestServer(t)

	created := postJSON(t, srv.URL+"/api/emails", map[string]string{
		"from": "cust@acme.com", "subject": "URGENT outage", "body": "everything is down",
	})
	id := created["id"].(string)
	require.Equal(t, "AWAITING_CLASSIFICATION_REVIEW", created["state"])

	// Checkpoint 1: human validates routing.
	reviewed := postJSON(t, srv.URL+"/api/tickets/"+id+"/classification-review", map[string]string{
		"urgency": "critical", "type": "technical", "department": "engineering", "reviewer": "alice",
	})
	require.Equal(t, "AWAITING_REPLY_APPROVAL", reviewed["state"])

	// Checkpoint 2: approve reply.
	approved := postJSON(t, srv.URL+"/api/tickets/"+id+"/reply-approval", map[string]string{
		"action": "approve", "final_text": "We restored service.", "reviewer": "alice",
	})
	require.Equal(t, "RESOLVED", approved["state"])
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/httpapi/ -v`
Expected: FAIL — undefined `NewServer`.

- [ ] **Step 3: Implement the HTTP server**

`internal/httpapi/server.go`:
```go
// Package httpapi exposes the ingestion endpoint and the two human-in-the-loop
// checkpoint actions. The React dashboard (Plan 4) renders on top of these.
package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/sathwiik/supportsentinel/internal/domain"
	"github.com/sathwiik/supportsentinel/internal/orchestrator"
	"github.com/sathwiik/supportsentinel/internal/store"
)

func NewServer(o *orchestrator.Orchestrator, s *store.Store) http.Handler {
	mux := http.NewServeMux()
	h := &handlers{o: o, s: s}
	mux.HandleFunc("POST /api/emails", h.submitEmail)
	mux.HandleFunc("GET /api/tickets/{id}", h.getTicket)
	mux.HandleFunc("POST /api/tickets/{id}/classification-review", h.reviewClassification)
	mux.HandleFunc("POST /api/tickets/{id}/reply-approval", h.replyApproval)
	return mux
}

type handlers struct {
	o *orchestrator.Orchestrator
	s *store.Store
}

func writeTicket(w http.ResponseWriter, t domain.Ticket) {
	writeJSON(w, http.StatusOK, map[string]any{
		"id": t.ID.String(), "state": string(t.State), "urgency": string(t.Urgency),
		"type": string(t.Type), "department": string(t.Department), "confidence": t.Confidence,
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *handlers) submitEmail(w http.ResponseWriter, r *http.Request) {
	var in struct{ From, To, Subject, Body string }
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if in.From == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "from is required"})
		return
	}
	email := domain.Email{
		FromAddr: in.From, ToAddr: in.To, Subject: in.Subject, Body: in.Body,
		DedupeKey: hashKey(in.From, in.Subject, in.Body),
	}
	tk, err := h.o.Ingest(r.Context(), "http", email)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeTicket(w, tk)
}

func (h *handlers) getTicket(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	tk, err := h.s.GetTicket(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeTicket(w, tk)
}

func (h *handlers) reviewClassification(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	var in struct{ Urgency, Type, Department, Reviewer string }
	_ = json.NewDecoder(r.Body).Decode(&in)
	err = h.o.ReviewClassification(r.Context(), id, orchestrator.ReviewDecision{
		Urgency: domain.Urgency(in.Urgency), Type: domain.TicketType(in.Type),
		Department: domain.Department(in.Department),
	}, fallback(in.Reviewer, "anon"))
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	tk, _ := h.s.GetTicket(r.Context(), id)
	writeTicket(w, tk)
}

func (h *handlers) replyApproval(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	var in struct{ Action, FinalText, Reviewer string }
	_ = json.NewDecoder(r.Body).Decode(&in)
	rev := fallback(in.Reviewer, "anon")
	switch in.Action {
	case "approve":
		err = h.o.ApproveReply(r.Context(), id, in.FinalText, rev)
	case "reject":
		err = h.o.RejectReply(r.Context(), id, rev)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "action must be approve|reject"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	tk, _ := h.s.GetTicket(r.Context(), id)
	writeTicket(w, tk)
}

func fallback(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
```

- [ ] **Step 4: Add the dedupe-key helper**

Create `internal/httpapi/hash.go`:
```go
package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
)

func hashKey(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/httpapi/ -v`
Expected: PASS (both end-to-end tests).

- [ ] **Step 6: Wire `cmd/server/main.go`**

`cmd/server/main.go`:
```go
package main

import (
	"context"
	"log"
	"net/http"

	"github.com/sathwiik/supportsentinel/internal/alert"
	"github.com/sathwiik/supportsentinel/internal/classify"
	"github.com/sathwiik/supportsentinel/internal/config"
	"github.com/sathwiik/supportsentinel/internal/httpapi"
	"github.com/sathwiik/supportsentinel/internal/orchestrator"
	"github.com/sathwiik/supportsentinel/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	ctx := context.Background()
	s, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer s.Close()

	// Plan 1 uses the fake classifier; Plan 2 swaps in the Qwen client.
	o := orchestrator.New(s, classify.NewFake(), alert.NewLog(), cfg.ConfidenceThreshold)
	srv := httpapi.NewServer(o, s)

	log.Printf("SupportSentinel listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, srv); err != nil {
		log.Fatal(err)
	}
}
```

- [ ] **Step 7: Verify the whole thing builds and all tests pass**

Run:
```bash
go vet ./...
go build ./...
go test ./...
```
Expected: build clean; tests PASS (DB-backed ones run if `TEST_DATABASE_URL` set, else SKIP).

- [ ] **Step 8: Commit**

```bash
git add cmd/ internal/httpapi/
git commit -m "feat(httpapi): ingestion + checkpoint endpoints, server wiring, e2e slice test"
```

---

## Plan 1 Definition of Done

- [ ] `go build ./...` and `go vet ./...` clean.
- [ ] `go test ./...` passes with `TEST_DATABASE_URL` set (all DB-backed tests run, not skipped).
- [ ] `make dev` boots; `POST /api/emails` returns a ticket and the audit_log shows every transition.
- [ ] `CLAUDE.md`, `LICENSE`, `Makefile`, `app.env.example` committed.
- [ ] Manual smoke: submit a clear email → `AWAITING_REPLY_APPROVAL`; submit "URGENT outage" → `AWAITING_CLASSIFICATION_REVIEW`.

---

## Roadmap — Subsequent Plans (detailed after Plan 1 lands)

> Written once the foundation's exact types/signatures exist, so they reference real code rather than guesses.

- **Plan 2 — Qwen/DashScope integration.** `internal/qwen/client.go` (the Alibaba Cloud proof file): OpenAI-compatible DashScope calls, structured JSON output via schema, `Classify`/`DraftReply`. Bounded exponential-backoff retry; one re-prompt on malformed JSON; on exhaustion → fail toward `AWAITING_CLASSIFICATION_REVIEW` (the orchestrator path already exists). Swap `classify.NewFake()` for the Qwen client in `main.go`. Add `DASHSCOPE_API_KEY` to config. Contract tests against recorded fixtures + one build-tagged live smoke test.
- **Plan 3 — Tool layer.** `internal/tools`: `lookup_customer(email)` and `lookup_similar_tickets(text)` over Postgres (+ seed `customers`). Wire as DashScope function-calling tools available during `Classify`; record invocations into `classifications.tools_used`.
- **Plan 4 — React dashboard.** Queue + detail (email, reasoning, confidence, tools used), Checkpoint 1 + 2 controls, audit timeline (`GET /api/tickets/{id}/audit`). Build → `//go:embed` into the binary; Go serves the SPA.
- **Plan 5 — IMAP ingestion + alerting.** `internal/ingest/imap` poller feeding the same `Ingest` path (message-id dedupe). Real Slack webhook + SMTP email in `internal/alert` (replace `Log`), still best-effort.
- **Plan 6 — Eval harness + gold dataset.** `/eval`: labeled gold set, `make eval` → accuracy, per-class precision/recall/F1, confusion matrix, confidence-vs-accuracy calibration → set `CONFIDENCE_THRESHOLD`.
- **Plan 7 — Deployment + submission deliverables.** systemd unit, nginx TLS conf, `make deploy` (build → embed → cross-compile → scp → restart), RDS wiring, README (features + Track 4 + Alibaba Cloud proof link), architecture diagram image, proof recording.
