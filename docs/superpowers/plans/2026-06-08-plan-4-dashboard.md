# Autocierge — Plan 4: React Dashboard (reviewer console)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A web dashboard — the reviewer's console and the demo centerpiece — that lists the ticket queue and, for a selected ticket, shows the email, Qwen's classification (urgency/type/department/confidence/**reasoning**/**tools used**), the drafted reply, the two human-in-the-loop checkpoint controls, and the audit timeline. Built with Vite/React/TS/Tailwind and **embedded into the Go binary via `//go:embed`** so the single binary serves both the API and the UI.

**Architecture:** New Go read endpoints (`GET /api/tickets` queue, `GET /api/tickets/{id}/detail`, `GET /api/tickets/{id}/audit`) backed by new store queries shape JSON DTOs the frontend renders. A `internal/webui` package embeds the built SPA (`//go:embed all:dist`) and serves it with SPA fallback; `cmd/server` mounts it at `/` while `/api/*` keeps precedence (Go 1.22 ServeMux specificity). The React app is a two-pane master–detail: queue on the left, detail + actions on the right. The existing checkpoint endpoints (`classification-review`, `reply-approval`) handle the actions — no orchestrator change.

**Tech Stack:** Go 1.25 (`embed`, `net/http`), pgx. Frontend: Vite + React 18 + TypeScript + Tailwind v4 (`@tailwindcss/vite`). Backend tests via `httptest`+DB; frontend gated on typecheck+build (the dashboard's real validation is manual/demo per the design spec).

**Spec:** `docs/superpowers/specs/2026-06-05-autocierge-design.md` (§1 dashboard, §3 component 6). **Builds on:** Plans 1–3. **Module path:** `github.com/lemonishi/autocierge`. **Env:** Postgres on 5433 (`TEST_DATABASE_URL`), Node 24/npm. Commit with the repo's git config (`Lennon <lemoncode8888@gmail.com>`); never override the author email.

---

## File Structure (Plan 4)

```
internal/store/dashboard.go         → TicketSummary/ClassificationRecord/ReplyRecord + ListTickets/GetLatestClassification/GetLatestReply (new)
internal/store/dashboard_test.go     → DB tests (new)
internal/httpapi/dashboard.go        → queue/detail/audit handlers + DTOs (new)
internal/httpapi/dashboard_test.go   → httptest+DB tests (new)
internal/httpapi/server.go           → register the 3 read routes + mount webui at "/" (modify)
internal/webui/embed.go              → //go:embed all:dist + SPA Handler (new)
internal/webui/embed_test.go         → serve tests (new)
internal/webui/dist/.gitkeep         → keeps the embed target present pre-build (new)
frontend/                            → Vite React TS app (new): package.json, vite.config.ts, tsconfig*, index.html, src/*
.gitignore                           → ignore built frontend output except .gitkeep (modify)
Makefile                             → frontend build target; build depends on it (modify)
CLAUDE.md                            → dashboard note (modify)
```

---

## Task 1: Store — dashboard read queries

**Files:**
- Create: `internal/store/dashboard.go`, `internal/store/dashboard_test.go`

- [ ] **Step 1: Write the failing test**

`internal/store/dashboard_test.go`:
```go
package store

import (
	"context"
	"testing"

	"github.com/lemonishi/autocierge/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestListTicketsReturnsSummaries(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	tk, err := s.CreateTicketWithEmail(ctx, "http", domain.Email{
		FromAddr: "a@b.com", Subject: "billing help", Body: "x", DedupeKey: "d1",
	})
	require.NoError(t, err)
	urg, typ, dep, conf := domain.UrgencyHigh, domain.TypeBilling, domain.DeptBilling, 0.9
	require.NoError(t, s.Apply(ctx, Transition{
		TicketID: tk.ID, From: domain.StateNew, To: domain.StateAwaitingReplyApproval, Actor: "qwen",
		SetUrgency: &urg, SetType: &typ, SetDepartment: &dep, SetConfidence: &conf,
	}))

	list, err := s.ListTickets(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, tk.ID, list[0].ID)
	require.Equal(t, "billing help", list[0].Subject)
	require.Equal(t, "a@b.com", list[0].FromAddr)
	require.Equal(t, domain.UrgencyHigh, list[0].Urgency)
	require.Equal(t, domain.StateAwaitingReplyApproval, list[0].State)
}

func TestGetLatestClassificationAndReply(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	tk, err := s.CreateTicketWithEmail(ctx, "http", domain.Email{
		FromAddr: "a@b.com", Subject: "s", Body: "b", DedupeKey: "d2",
	})
	require.NoError(t, err)

	// no classification yet → ErrNotFound
	_, err = s.GetLatestClassification(ctx, tk.ID)
	require.ErrorIs(t, err, ErrNotFound)

	require.NoError(t, s.SaveClassification(ctx, tk.ID, domain.Classification{
		Urgency: domain.UrgencyNormal, Type: domain.TypeBilling, Department: domain.DeptBilling,
		Confidence: 0.8, Reasoning: "mentions invoice", Model: "qwen-max",
		ToolsUsed: map[string]any{"lookup_customer": map[string]any{"result": "enterprise"}},
	}))
	cr, err := s.GetLatestClassification(ctx, tk.ID)
	require.NoError(t, err)
	require.Equal(t, "mentions invoice", cr.Reasoning)
	require.Equal(t, "qwen-max", cr.Model)
	require.Contains(t, cr.ToolsUsed, "lookup_customer")

	id, err := s.SaveReplyDraft(ctx, tk.ID, "draft text")
	require.NoError(t, err)
	require.NoError(t, s.FinalizeReply(ctx, id, "approved", "final text"))
	rr, err := s.GetLatestReply(ctx, tk.ID)
	require.NoError(t, err)
	require.Equal(t, "approved", rr.Status)
	require.Equal(t, "final text", rr.FinalText)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `TEST_DATABASE_URL='postgres://postgres:postgres@localhost:5433/autocierge_test?sslmode=disable' go test ./internal/store/ -run 'ListTickets|LatestClassification' -v`
Expected: FAIL — undefined `ListTickets`, etc.

- [ ] **Step 3: Implement**

`internal/store/dashboard.go`:
```go
package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/lemonishi/autocierge/internal/domain"
)

// TicketSummary is one row in the dashboard queue.
type TicketSummary struct {
	ID         uuid.UUID
	State      domain.State
	Urgency    domain.Urgency
	Type       domain.TicketType
	Department domain.Department
	Confidence float64
	Subject    string
	FromAddr   string
	CreatedAt  time.Time
}

// ClassificationRecord is a stored classification (with metadata) for the detail view.
type ClassificationRecord struct {
	Urgency    domain.Urgency
	Type       domain.TicketType
	Department domain.Department
	Confidence float64
	Reasoning  string
	Model      string
	ToolsUsed  map[string]any
	CreatedAt  time.Time
}

// ReplyRecord is a stored reply for the detail view.
type ReplyRecord struct {
	DraftText string
	FinalText string
	Status    string
	CreatedAt time.Time
}

// ListTickets returns all tickets for the queue, ordered so items needing human
// action and higher urgency surface first, then newest.
func (s *Store) ListTickets(ctx context.Context) ([]TicketSummary, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT t.id, t.state, COALESCE(t.urgency,''), COALESCE(t.type,''), COALESCE(t.department,''),
		        COALESCE(t.confidence,0), COALESCE(e.subject,''), e.from_addr, t.created_at
		 FROM tickets t JOIN emails e ON e.ticket_id = t.id
		 ORDER BY
		   CASE t.state
		     WHEN 'AWAITING_CLASSIFICATION_REVIEW' THEN 0
		     WHEN 'AWAITING_REPLY_APPROVAL' THEN 1
		     ELSE 2 END,
		   CASE t.urgency
		     WHEN 'critical' THEN 0 WHEN 'high' THEN 1 WHEN 'normal' THEN 2 ELSE 3 END,
		   t.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TicketSummary{}
	for rows.Next() {
		var ts TicketSummary
		if err := rows.Scan(&ts.ID, &ts.State, &ts.Urgency, &ts.Type, &ts.Department,
			&ts.Confidence, &ts.Subject, &ts.FromAddr, &ts.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, ts)
	}
	return out, rows.Err()
}

// GetLatestClassification returns the most recent classification, or ErrNotFound.
func (s *Store) GetLatestClassification(ctx context.Context, ticketID uuid.UUID) (ClassificationRecord, error) {
	var cr ClassificationRecord
	var toolsJSON []byte
	err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(urgency,''), COALESCE(type,''), COALESCE(department,''), COALESCE(confidence,0),
		        COALESCE(reasoning,''), COALESCE(model,''), tools_used, created_at
		 FROM classifications WHERE ticket_id = $1 ORDER BY created_at DESC LIMIT 1`, ticketID).
		Scan(&cr.Urgency, &cr.Type, &cr.Department, &cr.Confidence, &cr.Reasoning, &cr.Model, &toolsJSON, &cr.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ClassificationRecord{}, ErrNotFound
	}
	if err != nil {
		return ClassificationRecord{}, err
	}
	if len(toolsJSON) > 0 {
		_ = json.Unmarshal(toolsJSON, &cr.ToolsUsed)
	}
	return cr, nil
}

// GetLatestReply returns the most recent reply, or ErrNotFound.
func (s *Store) GetLatestReply(ctx context.Context, ticketID uuid.UUID) (ReplyRecord, error) {
	var rr ReplyRecord
	err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(draft_text,''), COALESCE(final_text,''), status, created_at
		 FROM replies WHERE ticket_id = $1 ORDER BY created_at DESC LIMIT 1`, ticketID).
		Scan(&rr.DraftText, &rr.FinalText, &rr.Status, &rr.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReplyRecord{}, ErrNotFound
	}
	if err != nil {
		return ReplyRecord{}, err
	}
	return rr, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `TEST_DATABASE_URL='...' go test ./internal/store/ -v`
Expected: PASS (all). `go vet ./internal/store/` clean.

- [ ] **Step 5: Commit**

```bash
git add internal/store/dashboard.go internal/store/dashboard_test.go
git commit -m "feat(store): dashboard read queries (queue, latest classification/reply)"
```

---

## Task 2: Backend read endpoints

**Files:**
- Create: `internal/httpapi/dashboard.go`, `internal/httpapi/dashboard_test.go`
- Modify: `internal/httpapi/server.go`

- [ ] **Step 1: Write the failing test**

`internal/httpapi/dashboard_test.go`:
```go
package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQueueAndDetailAndAudit(t *testing.T) {
	srv := newTestServer(t)

	// Create a ticket via the pipeline (fake classifier auto-routes a billing email).
	created := postJSON(t, srv.URL+"/api/emails", map[string]string{
		"from": "cust@acme.com", "subject": "invoice issue", "body": "charged twice",
	})
	id := created["id"].(string)

	// Queue lists it.
	resp, err := http.Get(srv.URL + "/api/tickets")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)
	var queue []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&queue))
	require.GreaterOrEqual(t, len(queue), 1)
	require.Equal(t, "invoice issue", queue[0]["subject"])

	// Detail returns email + classification + reply.
	dresp, err := http.Get(srv.URL + "/api/tickets/" + id + "/detail")
	require.NoError(t, err)
	defer dresp.Body.Close()
	require.Equal(t, 200, dresp.StatusCode)
	var detail map[string]any
	require.NoError(t, json.NewDecoder(dresp.Body).Decode(&detail))
	require.NotNil(t, detail["email"])
	require.NotNil(t, detail["classification"])
	email := detail["email"].(map[string]any)
	require.Equal(t, "charged twice", email["body"])

	// Audit returns the transition timeline.
	aresp, err := http.Get(srv.URL + "/api/tickets/" + id + "/audit")
	require.NoError(t, err)
	defer aresp.Body.Close()
	require.Equal(t, 200, aresp.StatusCode)
	var audit []map[string]any
	require.NoError(t, json.NewDecoder(aresp.Body).Decode(&audit))
	require.GreaterOrEqual(t, len(audit), 2) // NEW->CLASSIFYING, ...
	require.Equal(t, "NEW", audit[0]["from_state"])
}

func TestDetailNotFound(t *testing.T) {
	srv := newTestServer(t)
	resp, err := http.Get(srv.URL + "/api/tickets/11111111-1111-1111-1111-111111111111/detail")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 404, resp.StatusCode)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `TEST_DATABASE_URL='...' go test ./internal/httpapi/ -run 'QueueAndDetail|DetailNotFound' -v`
Expected: FAIL — routes 404 (handlers not registered).

- [ ] **Step 3: Implement the handlers**

`internal/httpapi/dashboard.go`:
```go
package httpapi

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/lemonishi/autocierge/internal/store"
)

func (h *handlers) listTickets(w http.ResponseWriter, r *http.Request) {
	items, err := h.s.ListTickets(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, t := range items {
		out = append(out, map[string]any{
			"id": t.ID.String(), "state": string(t.State), "urgency": string(t.Urgency),
			"type": string(t.Type), "department": string(t.Department), "confidence": t.Confidence,
			"subject": t.Subject, "from": t.FromAddr, "created_at": t.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handlers) ticketDetail(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	tk, err := h.s.GetTicket(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	email, err := h.s.GetEmailByTicket(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	detail := map[string]any{
		"ticket": map[string]any{
			"id": tk.ID.String(), "state": string(tk.State), "urgency": string(tk.Urgency),
			"type": string(tk.Type), "department": string(tk.Department), "confidence": tk.Confidence,
			"source": tk.Source, "created_at": tk.CreatedAt, "updated_at": tk.UpdatedAt,
		},
		"email": map[string]any{
			"from": email.FromAddr, "to": email.ToAddr, "subject": email.Subject,
			"body": email.Body, "received_at": email.ReceivedAt,
		},
		"classification": nil,
		"reply":          nil,
	}
	if cr, err := h.s.GetLatestClassification(r.Context(), id); err == nil {
		detail["classification"] = map[string]any{
			"urgency": string(cr.Urgency), "type": string(cr.Type), "department": string(cr.Department),
			"confidence": cr.Confidence, "reasoning": cr.Reasoning, "model": cr.Model,
			"tools_used": cr.ToolsUsed, "created_at": cr.CreatedAt,
		}
	} else if !errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if rr, err := h.s.GetLatestReply(r.Context(), id); err == nil {
		detail["reply"] = map[string]any{
			"draft_text": rr.DraftText, "final_text": rr.FinalText, "status": rr.Status, "created_at": rr.CreatedAt,
		}
	} else if !errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (h *handlers) ticketAudit(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	rows, err := h.s.AuditLog(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, a := range rows {
		out = append(out, map[string]any{
			"from_state": string(a.From), "to_state": string(a.To), "actor": a.Actor, "created_at": a.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, out)
}
```

- [ ] **Step 4: Register the routes**

In `internal/httpapi/server.go`, in `NewServer`, add (before the catch-all webui mount that Task 3 adds):
```go
	mux.HandleFunc("GET /api/tickets", h.listTickets)
	mux.HandleFunc("GET /api/tickets/{id}/detail", h.ticketDetail)
	mux.HandleFunc("GET /api/tickets/{id}/audit", h.ticketAudit)
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `TEST_DATABASE_URL='...' go test ./internal/httpapi/ -v`
Expected: PASS (all incl. the new dashboard tests). `go vet ./internal/httpapi/` clean.

- [ ] **Step 6: Commit**

```bash
git add internal/httpapi/dashboard.go internal/httpapi/dashboard_test.go internal/httpapi/server.go
git commit -m "feat(httpapi): queue, detail, and audit read endpoints"
```

---

## Task 3: Embed package + SPA serving + Makefile

**Files:**
- Create: `internal/webui/embed.go`, `internal/webui/embed_test.go`, `internal/webui/dist/.gitkeep`
- Modify: `internal/httpapi/server.go`, `.gitignore`, `Makefile`

- [ ] **Step 1: Create the embed target placeholder**

Create an empty file `internal/webui/dist/.gitkeep` (so `//go:embed all:dist` compiles before the frontend is built).

- [ ] **Step 2: Write the failing test**

`internal/webui/embed_test.go`:
```go
package webui

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHandlerServesSomethingAtRoot(t *testing.T) {
	srv := httptest.NewServer(Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/")
	require.NoError(t, err)
	defer resp.Body.Close()
	// Whether the SPA is built (index.html) or not (placeholder), root returns 200.
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestHandlerServesIndexForUnknownClientRoute(t *testing.T) {
	srv := httptest.NewServer(Handler())
	defer srv.Close()
	// A non-asset path (client-side route) should fall back to 200, not 404.
	resp, err := http.Get(srv.URL + "/tickets/abc")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/webui/ -v`
Expected: FAIL — undefined `Handler`.

- [ ] **Step 4: Implement the embed handler**

`internal/webui/embed.go`:
```go
// Package webui embeds the built React dashboard (Vite output in dist/) and
// serves it as a single-page app. Before the frontend is built, dist/ contains
// only .gitkeep and the handler serves a friendly placeholder.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

const placeholder = `<!doctype html><html><body style="font-family:sans-serif;padding:2rem">
<h1>Autocierge</h1><p>Dashboard not built yet. Run <code>make frontend</code> (or <code>make build</code>) and restart.</p>
</body></html>`

// Handler serves the embedded SPA: real files when present, with a fallback to
// index.html for client-side routes; a placeholder when the app isn't built.
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(sub))

	indexHTML, indexErr := fs.ReadFile(sub, "index.html")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Not built yet → placeholder.
		if indexErr != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(placeholder))
			return
		}
		clean := strings.TrimPrefix(r.URL.Path, "/")
		if clean == "" {
			serveIndex(w, indexHTML)
			return
		}
		// If the requested file exists in the build, serve it; otherwise SPA fallback.
		if f, err := sub.Open(clean); err == nil {
			_ = f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}
		serveIndex(w, indexHTML)
	})
}

func serveIndex(w http.ResponseWriter, indexHTML []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(indexHTML)
}
```

- [ ] **Step 5: Mount the SPA in the server**

In `internal/httpapi/server.go`: add the import `"github.com/lemonishi/autocierge/internal/webui"`, and at the END of `NewServer` (after all `/api/...` routes) register the catch-all:
```go
	mux.Handle("/", webui.Handler())
```
(Go 1.22 ServeMux: the specific `/api/...` patterns take precedence over `/`.)

- [ ] **Step 6: Update `.gitignore`**

Append:
```gitignore
# Built frontend (embedded at build time); keep the embed target placeholder.
internal/webui/dist/*
!internal/webui/dist/.gitkeep
frontend/node_modules/
frontend/dist/
```

- [ ] **Step 7: Update the `Makefile`**

Add a `frontend` target and make `build` depend on it. Add near the other targets:
```make
frontend:
	cd frontend && npm install && npm run build
	touch internal/webui/dist/.gitkeep

build: frontend
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/server ./cmd/server
```
(Replace the existing `build:` target with this one. Keep the other targets. The `touch` re-creates the placeholder that Vite's `emptyOutDir` removes, so the embed target always exists.)

- [ ] **Step 8: Verify and commit**

Run:
```bash
go vet ./internal/webui/ ./internal/httpapi/
go test ./internal/webui/ -v
go build ./...
```
Expected: pass; root serves the placeholder (frontend not built yet).

```bash
git add internal/webui/ internal/httpapi/server.go .gitignore Makefile
git commit -m "feat(webui): embed and serve the SPA with placeholder fallback"
```

---

## Task 4: Frontend scaffold (Vite + React + TS + Tailwind)

**Files:**
- Create: `frontend/package.json`, `frontend/vite.config.ts`, `frontend/tsconfig.json`, `frontend/tsconfig.node.json`, `frontend/index.html`, `frontend/src/main.tsx`, `frontend/src/index.css`, `frontend/src/types.ts`, `frontend/src/api.ts`, `frontend/src/App.tsx` (minimal placeholder)

- [ ] **Step 1: Create the project files**

`frontend/package.json`:
```json
{
  "name": "autocierge-dashboard",
  "private": true,
  "version": "0.1.0",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc -b && vite build",
    "preview": "vite preview"
  },
  "dependencies": {
    "react": "^18.3.1",
    "react-dom": "^18.3.1"
  },
  "devDependencies": {
    "@tailwindcss/vite": "^4.0.0",
    "@types/react": "^18.3.12",
    "@types/react-dom": "^18.3.1",
    "@vitejs/plugin-react": "^4.3.4",
    "tailwindcss": "^4.0.0",
    "typescript": "^5.6.3",
    "vite": "^6.0.3"
  }
}
```

`frontend/vite.config.ts`:
```ts
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    outDir: "../internal/webui/dist",
    emptyOutDir: true,
  },
  server: {
    proxy: { "/api": "http://localhost:8080" },
  },
});
```

`frontend/tsconfig.json`:
```json
{
  "compilerOptions": {
    "target": "ES2020",
    "useDefineForClassFields": true,
    "lib": ["ES2020", "DOM", "DOM.Iterable"],
    "module": "ESNext",
    "skipLibCheck": true,
    "moduleResolution": "bundler",
    "allowImportingTsExtensions": true,
    "noEmit": true,
    "jsx": "react-jsx",
    "strict": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true
  },
  "include": ["src"],
  "references": [{ "path": "./tsconfig.node.json" }]
}
```

`frontend/tsconfig.node.json`:
```json
{
  "compilerOptions": {
    "composite": true,
    "skipLibCheck": true,
    "module": "ESNext",
    "moduleResolution": "bundler",
    "allowSyntheticDefaultImports": true,
    "strict": true,
    "noEmit": true
  },
  "include": ["vite.config.ts"]
}
```

`frontend/index.html`:
```html
<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Autocierge</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
```

`frontend/src/index.css`:
```css
@import "tailwindcss";
```

`frontend/src/main.tsx`:
```tsx
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "./index.css";
import App from "./App";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>
);
```

`frontend/src/types.ts`:
```ts
export type Urgency = "low" | "normal" | "high" | "critical";
export type TicketType = "billing" | "technical" | "account" | "feature_request" | "general";
export type Department = "billing" | "engineering" | "accounts" | "product" | "support_tier1";

export interface TicketSummary {
  id: string;
  state: string;
  urgency: Urgency | "";
  type: TicketType | "";
  department: Department | "";
  confidence: number;
  subject: string;
  from: string;
  created_at: string;
}

export interface Classification {
  urgency: Urgency;
  type: TicketType;
  department: Department;
  confidence: number;
  reasoning: string;
  model: string;
  tools_used: Record<string, unknown> | null;
  created_at: string;
}

export interface Reply {
  draft_text: string;
  final_text: string;
  status: string;
  created_at: string;
}

export interface TicketDetail {
  ticket: {
    id: string; state: string; urgency: Urgency | ""; type: TicketType | "";
    department: Department | ""; confidence: number; source: string;
    created_at: string; updated_at: string;
  };
  email: { from: string; to: string; subject: string; body: string; received_at: string };
  classification: Classification | null;
  reply: Reply | null;
}

export interface AuditEntry {
  from_state: string;
  to_state: string;
  actor: string;
  created_at: string;
}
```

`frontend/src/api.ts`:
```ts
import type { TicketSummary, TicketDetail, AuditEntry } from "./types";

async function get<T>(path: string): Promise<T> {
  const r = await fetch(path);
  if (!r.ok) throw new Error(`${path} → ${r.status}`);
  return r.json() as Promise<T>;
}

async function post<T>(path: string, body: unknown): Promise<T> {
  const r = await fetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!r.ok) throw new Error(`${path} → ${r.status}`);
  return r.json() as Promise<T>;
}

export const api = {
  listTickets: () => get<TicketSummary[]>("/api/tickets"),
  ticketDetail: (id: string) => get<TicketDetail>(`/api/tickets/${id}/detail`),
  ticketAudit: (id: string) => get<AuditEntry[]>(`/api/tickets/${id}/audit`),
  submitEmail: (e: { from: string; subject: string; body: string }) =>
    post("/api/emails", e),
  reviewClassification: (id: string, d: { urgency: string; type: string; department: string; reviewer: string }) =>
    post(`/api/tickets/${id}/classification-review`, d),
  replyApproval: (id: string, d: { action: "approve" | "reject"; final_text?: string; reviewer: string }) =>
    post(`/api/tickets/${id}/reply-approval`, d),
};
```

`frontend/src/App.tsx` (minimal placeholder — replaced in Task 5):
```tsx
export default function App() {
  return <div className="p-8 text-2xl font-bold">Autocierge (scaffold)</div>;
}
```

- [ ] **Step 2: Install + typecheck + build**

Run:
```bash
cd frontend && npm install && npm run build
```
Expected: `tsc -b` passes, Vite builds into `../internal/webui/dist` (index.html + assets).

- [ ] **Step 3: Verify the binary embeds and serves it**

Run (from repo root):
```bash
go build ./... && go test ./internal/webui/ -v
```
Expected: pass; `TestHandlerServesSomethingAtRoot` now serves the real index.html.

- [ ] **Step 4: Commit**

```bash
git add frontend/ internal/webui/dist/.gitkeep
git commit -m "feat(frontend): Vite/React/TS/Tailwind scaffold + API client and types"
```
(Note: built output under `internal/webui/dist/` is gitignored except `.gitkeep`; `frontend/dist` and `node_modules` are gitignored.)

---

## Task 5: Frontend components + two-pane app

**Files:**
- Create: `frontend/src/components/TicketQueue.tsx`, `frontend/src/components/TicketDetail.tsx`, `frontend/src/components/AuditTimeline.tsx`, `frontend/src/ui.tsx`
- Modify: `frontend/src/App.tsx`

- [ ] **Step 1: Shared UI helpers**

`frontend/src/ui.tsx`:
```tsx
const urgencyColor: Record<string, string> = {
  critical: "bg-red-100 text-red-800 border-red-300",
  high: "bg-orange-100 text-orange-800 border-orange-300",
  normal: "bg-blue-100 text-blue-800 border-blue-300",
  low: "bg-gray-100 text-gray-700 border-gray-300",
  "": "bg-gray-100 text-gray-500 border-gray-300",
};

const stateLabel: Record<string, string> = {
  AWAITING_CLASSIFICATION_REVIEW: "Needs review",
  AWAITING_REPLY_APPROVAL: "Approve reply",
  RESOLVED: "Resolved",
  ROUTED: "Routed",
  CLASSIFYING: "Classifying",
  DRAFTING: "Drafting",
  NEW: "New",
  FAILED: "Failed",
};

export function Badge({ kind, value }: { kind: "urgency" | "state"; value: string }) {
  if (kind === "urgency") {
    return (
      <span className={`inline-block rounded-full border px-2 py-0.5 text-xs font-medium ${urgencyColor[value] ?? urgencyColor[""]}`}>
        {value || "—"}
      </span>
    );
  }
  const needsAction = value === "AWAITING_CLASSIFICATION_REVIEW" || value === "AWAITING_REPLY_APPROVAL";
  return (
    <span className={`inline-block rounded px-2 py-0.5 text-xs font-medium ${needsAction ? "bg-amber-100 text-amber-800" : "bg-gray-100 text-gray-600"}`}>
      {stateLabel[value] ?? value}
    </span>
  );
}
```

- [ ] **Step 2: Queue component**

`frontend/src/components/TicketQueue.tsx`:
```tsx
import type { TicketSummary } from "../types";
import { Badge } from "../ui";

export function TicketQueue({
  tickets, selectedId, onSelect,
}: {
  tickets: TicketSummary[];
  selectedId: string | null;
  onSelect: (id: string) => void;
}) {
  return (
    <div className="divide-y divide-gray-100">
      {tickets.map((t) => (
        <button
          key={t.id}
          onClick={() => onSelect(t.id)}
          className={`w-full text-left px-4 py-3 hover:bg-gray-50 ${selectedId === t.id ? "bg-blue-50" : ""}`}
        >
          <div className="flex items-center justify-between gap-2">
            <span className="truncate font-medium text-gray-900">{t.subject || "(no subject)"}</span>
            <Badge kind="urgency" value={t.urgency} />
          </div>
          <div className="mt-1 flex items-center justify-between gap-2">
            <span className="truncate text-sm text-gray-500">{t.from}</span>
            <Badge kind="state" value={t.state} />
          </div>
        </button>
      ))}
      {tickets.length === 0 && <div className="p-6 text-center text-gray-400">No tickets yet</div>}
    </div>
  );
}
```

- [ ] **Step 3: Audit timeline component**

`frontend/src/components/AuditTimeline.tsx`:
```tsx
import type { AuditEntry } from "../types";

export function AuditTimeline({ entries }: { entries: AuditEntry[] }) {
  return (
    <ol className="space-y-2">
      {entries.map((e, i) => (
        <li key={i} className="flex items-start gap-2 text-sm">
          <span className="mt-1 h-2 w-2 shrink-0 rounded-full bg-gray-300" />
          <div>
            <span className="text-gray-700">
              {e.from_state || "(new)"} → <span className="font-medium">{e.to_state}</span>
            </span>
            <span className="ml-2 rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-500">{e.actor}</span>
            <div className="text-xs text-gray-400">{new Date(e.created_at).toLocaleString()}</div>
          </div>
        </li>
      ))}
    </ol>
  );
}
```

- [ ] **Step 4: Detail component (with both checkpoints)**

`frontend/src/components/TicketDetail.tsx`:
```tsx
import { useState } from "react";
import type { TicketDetail as Detail, AuditEntry } from "../types";
import { Badge } from "../ui";
import { AuditTimeline } from "./AuditTimeline";

const URGENCIES = ["low", "normal", "high", "critical"];
const TYPES = ["billing", "technical", "account", "feature_request", "general"];
const DEPTS = ["billing", "engineering", "accounts", "product", "support_tier1"];

export function TicketDetail({
  detail, audit, onReviewClassification, onReplyApproval,
}: {
  detail: Detail;
  audit: AuditEntry[];
  onReviewClassification: (d: { urgency: string; type: string; department: string }) => Promise<void>;
  onReplyApproval: (d: { action: "approve" | "reject"; final_text?: string }) => Promise<void>;
}) {
  const c = detail.classification;
  const [urgency, setUrgency] = useState(c?.urgency ?? "normal");
  const [type, setType] = useState(c?.type ?? "general");
  const [department, setDepartment] = useState(c?.department ?? "support_tier1");
  const [replyText, setReplyText] = useState(detail.reply?.draft_text ?? "");
  const [busy, setBusy] = useState(false);

  const state = detail.ticket.state;
  const wrap = (fn: () => Promise<void>) => async () => {
    setBusy(true);
    try { await fn(); } finally { setBusy(false); }
  };

  return (
    <div className="space-y-6">
      <div>
        <div className="flex items-center gap-2">
          <h2 className="text-xl font-semibold text-gray-900">{detail.email.subject || "(no subject)"}</h2>
          <Badge kind="state" value={state} />
        </div>
        <p className="text-sm text-gray-500">From {detail.email.from}</p>
      </div>

      <section className="rounded-lg border border-gray-200 bg-white p-4">
        <h3 className="mb-2 text-sm font-semibold uppercase tracking-wide text-gray-500">Email</h3>
        <p className="whitespace-pre-wrap text-gray-800">{detail.email.body}</p>
      </section>

      {c && (
        <section className="rounded-lg border border-gray-200 bg-white p-4">
          <div className="mb-2 flex items-center justify-between">
            <h3 className="text-sm font-semibold uppercase tracking-wide text-gray-500">Qwen classification</h3>
            <span className="text-xs text-gray-400">
              {c.model} · confidence {(c.confidence * 100).toFixed(0)}%
            </span>
          </div>
          <div className="mb-2 flex flex-wrap gap-2">
            <Badge kind="urgency" value={c.urgency} />
            <span className="rounded bg-gray-100 px-2 py-0.5 text-xs text-gray-700">{c.type}</span>
            <span className="rounded bg-gray-100 px-2 py-0.5 text-xs text-gray-700">→ {c.department}</span>
          </div>
          <p className="text-sm italic text-gray-600">"{c.reasoning}"</p>
          {c.tools_used && Object.keys(c.tools_used).length > 0 && (
            <div className="mt-2 text-xs text-gray-500">
              <span className="font-medium">Tools used:</span> {Object.keys(c.tools_used).join(", ")}
            </div>
          )}
        </section>
      )}

      {/* Checkpoint 1 */}
      {state === "AWAITING_CLASSIFICATION_REVIEW" && (
        <section className="rounded-lg border-2 border-amber-300 bg-amber-50 p-4">
          <h3 className="mb-2 font-semibold text-amber-900">Checkpoint 1 — Validate routing</h3>
          <div className="flex flex-wrap gap-3">
            <Select label="Urgency" value={urgency} options={URGENCIES} onChange={setUrgency} />
            <Select label="Type" value={type} options={TYPES} onChange={setType} />
            <Select label="Department" value={department} options={DEPTS} onChange={setDepartment} />
          </div>
          <button
            disabled={busy}
            onClick={wrap(() => onReviewClassification({ urgency, type, department }))}
            className="mt-3 rounded bg-amber-600 px-4 py-2 font-medium text-white hover:bg-amber-700 disabled:opacity-50"
          >
            Confirm &amp; route
          </button>
        </section>
      )}

      {/* Checkpoint 2 */}
      {state === "AWAITING_REPLY_APPROVAL" && (
        <section className="rounded-lg border-2 border-amber-300 bg-amber-50 p-4">
          <h3 className="mb-2 font-semibold text-amber-900">Checkpoint 2 — Approve reply</h3>
          <textarea
            value={replyText}
            onChange={(e) => setReplyText(e.target.value)}
            rows={8}
            className="w-full rounded border border-gray-300 p-2 text-sm"
          />
          <div className="mt-3 flex gap-2">
            <button
              disabled={busy}
              onClick={wrap(() => onReplyApproval({ action: "approve", final_text: replyText }))}
              className="rounded bg-green-600 px-4 py-2 font-medium text-white hover:bg-green-700 disabled:opacity-50"
            >
              Approve &amp; send
            </button>
            <button
              disabled={busy}
              onClick={wrap(() => onReplyApproval({ action: "reject" }))}
              className="rounded bg-gray-200 px-4 py-2 font-medium text-gray-700 hover:bg-gray-300 disabled:opacity-50"
            >
              Reject &amp; redraft
            </button>
          </div>
        </section>
      )}

      {state === "RESOLVED" && detail.reply && (
        <section className="rounded-lg border border-green-200 bg-green-50 p-4">
          <h3 className="mb-2 font-semibold text-green-900">Resolved — sent reply</h3>
          <p className="whitespace-pre-wrap text-sm text-gray-800">{detail.reply.final_text || detail.reply.draft_text}</p>
        </section>
      )}

      <section className="rounded-lg border border-gray-200 bg-white p-4">
        <h3 className="mb-2 text-sm font-semibold uppercase tracking-wide text-gray-500">Audit timeline</h3>
        <AuditTimeline entries={audit} />
      </section>
    </div>
  );
}

function Select({
  label, value, options, onChange,
}: { label: string; value: string; options: string[]; onChange: (v: string) => void }) {
  return (
    <label className="text-sm">
      <span className="mb-1 block text-xs font-medium text-gray-600">{label}</span>
      <select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="rounded border border-gray-300 px-2 py-1"
      >
        {options.map((o) => <option key={o} value={o}>{o}</option>)}
      </select>
    </label>
  );
}
```

- [ ] **Step 5: The two-pane App**

Replace `frontend/src/App.tsx`:
```tsx
import { useCallback, useEffect, useState } from "react";
import { api } from "./api";
import type { AuditEntry, TicketDetail as Detail, TicketSummary } from "./types";
import { TicketQueue } from "./components/TicketQueue";
import { TicketDetail } from "./components/TicketDetail";

const REVIEWER = "demo-agent";

export default function App() {
  const [tickets, setTickets] = useState<TicketSummary[]>([]);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [detail, setDetail] = useState<Detail | null>(null);
  const [audit, setAudit] = useState<AuditEntry[]>([]);
  const [error, setError] = useState<string | null>(null);

  const refreshQueue = useCallback(async () => {
    try { setTickets(await api.listTickets()); } catch (e) { setError(String(e)); }
  }, []);

  const refreshDetail = useCallback(async (id: string) => {
    try {
      const [d, a] = await Promise.all([api.ticketDetail(id), api.ticketAudit(id)]);
      setDetail(d); setAudit(a);
    } catch (e) { setError(String(e)); }
  }, []);

  useEffect(() => { refreshQueue(); }, [refreshQueue]);
  useEffect(() => {
    const t = setInterval(refreshQueue, 4000); // keep the queue fresh during the demo
    return () => clearInterval(t);
  }, [refreshQueue]);
  useEffect(() => { if (selectedId) refreshDetail(selectedId); }, [selectedId, refreshDetail]);

  const afterAction = async () => {
    await refreshQueue();
    if (selectedId) await refreshDetail(selectedId);
  };

  return (
    <div className="flex h-screen flex-col bg-gray-50 text-gray-900">
      <header className="flex items-center justify-between border-b border-gray-200 bg-white px-6 py-3">
        <h1 className="text-lg font-bold">Autocierge <span className="text-gray-400">reviewer console</span></h1>
        {error && <span className="text-sm text-red-600">{error}</span>}
      </header>
      <div className="flex min-h-0 flex-1">
        <aside className="w-96 shrink-0 overflow-y-auto border-r border-gray-200 bg-white">
          <TicketQueue tickets={tickets} selectedId={selectedId} onSelect={setSelectedId} />
        </aside>
        <main className="min-w-0 flex-1 overflow-y-auto p-6">
          {detail ? (
            <TicketDetail
              detail={detail}
              audit={audit}
              onReviewClassification={async (d) => {
                await api.reviewClassification(detail.ticket.id, { ...d, reviewer: REVIEWER });
                await afterAction();
              }}
              onReplyApproval={async (d) => {
                await api.replyApproval(detail.ticket.id, { ...d, reviewer: REVIEWER });
                await afterAction();
              }}
            />
          ) : (
            <div className="grid h-full place-items-center text-gray-400">Select a ticket from the queue</div>
          )}
        </main>
      </div>
    </div>
  );
}
```

- [ ] **Step 6: Typecheck + build + embed verification**

Run:
```bash
cd frontend && npm run build && cd ..
go build ./... && go vet ./...
```
Expected: `tsc -b` clean (the build uses `noUnusedLocals`/`noUnusedParameters`, so remove any unused import/local if tsc flags it). Vite build succeeds; Go build embeds the app.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/
git commit -m "feat(frontend): two-pane reviewer console with both checkpoints and audit timeline"
```

---

## Task 6: Build integration, validation, docs

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Full build + suite**

Run:
```bash
go vet ./...
go build ./...
TEST_DATABASE_URL='postgres://postgres:postgres@localhost:5433/autocierge_test?sslmode=disable' go test ./...
```
Expected: all green (backend tests; webui serves the built app).

- [ ] **Step 2: Manual end-to-end validation**

Build the binary and exercise the dashboard against real data:
```bash
make frontend                 # builds the SPA into internal/webui/dist
go build -o /tmp/ss ./cmd/server
# start with the local dev DB (set DATABASE_URL); seed + submit a few sample emails
```
Then:
1. Open `http://localhost:8080/` — the queue renders.
2. `POST /api/emails` a clear billing email and an "URGENT outage" email (the latter parks at Checkpoint 1).
3. In the UI: select the urgent ticket → validate classification (Checkpoint 1) → it moves to reply approval → edit & approve (Checkpoint 2) → it resolves. The audit timeline updates; classification shows reasoning/confidence (and tools-used when Qwen is enabled).

Capture a screenshot/recording for the demo. (This manual check is the dashboard's primary validation per the design spec.)

- [ ] **Step 3: Update `CLAUDE.md`**

Add under "## Stack":
```markdown
- Dashboard: `frontend/` (Vite + React + TS + Tailwind v4), two-pane reviewer console
  (queue + detail with reasoning/confidence/tools-used + both checkpoint controls +
  audit timeline). Built into `internal/webui/dist` and embedded via `//go:embed`;
  served at `/` by the Go binary. Read endpoints: `GET /api/tickets`,
  `/api/tickets/{id}/detail`, `/api/tickets/{id}/audit`. Dev: `cd frontend && npm run dev`
  (proxies /api → :8080). Build: `make frontend` then `go build`.
```

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: document the embedded React dashboard"
```

---

## Plan 4 Definition of Done

- [ ] `go vet ./...`, `go build ./...` clean; `go test ./...` green.
- [ ] `cd frontend && npm run build` succeeds (tsc + vite); output embedded; `go build` self-contains the UI.
- [ ] The binary serves the dashboard at `/`: queue lists tickets; selecting one shows email + Qwen reasoning/confidence/tools-used + the relevant checkpoint control + audit timeline.
- [ ] Driving both checkpoints from the UI advances a ticket to RESOLVED, reflected in the queue and audit timeline.
- [ ] Manual demo screenshot/recording captured.

---

## Roadmap — Subsequent Plans

- **Plan 5 — IMAP ingestion + Slack/email alerting.**
- **Plan 6 — Eval harness + gold dataset + threshold calibration.**
- **Plan 7 — Deployment (ECS/systemd/nginx) + submission deliverables (README, architecture diagram, proof recording, demo video).**
