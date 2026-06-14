.PHONY: dev run test test-db build tidy frontend eval eval-live

# Auto-load app.env (gitignored) so DATABASE_URL / TEST_DATABASE_URL are set
# without manual exporting. Override per-invocation by setting the var inline.
ifneq (,$(wildcard ./app.env))
include app.env
export
endif

tidy:
	go mod tidy

# Run the server locally (native, app.env auto-loaded). Does NOT build the
# frontend first — use `make run` for the full dashboard.
dev:
	go run ./cmd/server

# Build the dashboard, then run the server locally with the embedded UI.
# This is the one-command "see the dashboard at http://localhost:8080".
run: frontend
	go run ./cmd/server

frontend:
	cd frontend && npm install && npm run build
	touch internal/webui/dist/.gitkeep

# Cross-compiles a linux/amd64 binary for Alibaba Cloud ECS deploy (NOT runnable
# on macOS). For local use, run `make run` instead.
build: frontend
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/server ./cmd/server

# Unit tests + DB-backed tests (DB tests run when TEST_DATABASE_URL is set,
# which it is via app.env above; otherwise they skip).
test:
	go test ./...

# Convenience: create the local dev + test databases on the Homebrew instance (port 5433).
test-db:
	/opt/homebrew/opt/postgresql@16/bin/createdb -h localhost -p 5433 -O postgres supportsentinel || true
	/opt/homebrew/opt/postgresql@16/bin/createdb -h localhost -p 5433 -O postgres supportsentinel_test || true

# Classification quality report. `make eval` replays the committed cache
# (eval/recorded.json) — free, deterministic, no API key. `make eval-live`
# calls real Qwen on the gold set and refreshes the cache (spends quota).
eval:
	go run ./cmd/eval

eval-live:
	go run ./cmd/eval --live
