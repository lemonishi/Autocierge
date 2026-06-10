package ingest_test

import (
	"os"
	"strings"
	"testing"

	"github.com/lemonishi/supportsentinel/internal/ingest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	require.NoError(t, err)
	return raw
}

func TestParseRFC822_Plain(t *testing.T) {
	raw := readFixture(t, "plain.eml")
	email, err := ingest.ParseRFC822(raw)
	require.NoError(t, err)

	assert.Equal(t, "alice@example.com", email.FromAddr)
	assert.Equal(t, "Cannot log in to my account", email.Subject)
	assert.Contains(t, email.Body, "unable to log in")
	// DedupeKey should be the Message-ID (trimmed)
	assert.Equal(t, "<unique-001@mail.example.com>", email.DedupeKey)
	// Raw map should contain message-id
	assert.NotEmpty(t, email.Raw["message-id"])
}

func TestParseRFC822_Multipart(t *testing.T) {
	raw := readFixture(t, "multipart.eml")
	email, err := ingest.ParseRFC822(raw)
	require.NoError(t, err)

	assert.Equal(t, "bob@example.com", email.FromAddr)
	assert.Equal(t, "Billing charge is incorrect", email.Subject)
	// Should extract text/plain, not HTML
	assert.Contains(t, email.Body, "charged twice")
	assert.NotContains(t, email.Body, "<html>")
	// DedupeKey from Message-ID
	assert.Equal(t, "<unique-002@mail.example.com>", email.DedupeKey)
	assert.NotEmpty(t, email.Raw["message-id"])
}

func TestParseRFC822_NoMsgID(t *testing.T) {
	raw := readFixture(t, "nomsgid.eml")
	email, err := ingest.ParseRFC822(raw)
	require.NoError(t, err)

	assert.Equal(t, "carol@example.com", email.FromAddr)
	assert.Equal(t, "Feature request: dark mode", email.Subject)
	assert.Contains(t, email.Body, "dark mode")

	// DedupeKey should be hash fallback when no Message-ID
	assert.NotEmpty(t, email.DedupeKey)
	// It must be consistent (deterministic)
	email2, err := ingest.ParseRFC822(raw)
	require.NoError(t, err)
	assert.Equal(t, email.DedupeKey, email2.DedupeKey)
	// Must NOT look like a Message-ID (no angle brackets)
	assert.False(t, strings.HasPrefix(email.DedupeKey, "<"), "expected hash, not message-id format")
	// sha256 hex is 64 chars
	assert.Len(t, email.DedupeKey, 64)
}
