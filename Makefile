.PHONY: dev test test-db build tidy

# Auto-load app.env (gitignored) so DATABASE_URL / TEST_DATABASE_URL are set
# without manual exporting. Override per-invocation by setting the var inline.
ifneq (,$(wildcard ./app.env))
include app.env
export
endif

tidy:
	go mod tidy

dev:
	go run ./cmd/server

build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/server ./cmd/server

# Unit tests + DB-backed tests (DB tests run when TEST_DATABASE_URL is set,
# which it is via app.env above; otherwise they skip).
test:
	go test ./...

# Convenience: create the local dev + test databases on the Homebrew instance (port 5433).
test-db:
	/opt/homebrew/opt/postgresql@16/bin/createdb -h localhost -p 5433 -O postgres supportsentinel || true
	/opt/homebrew/opt/postgresql@16/bin/createdb -h localhost -p 5433 -O postgres supportsentinel_test || true
