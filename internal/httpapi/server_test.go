package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/lemonishi/supportsentinel/internal/alert"
	"github.com/lemonishi/supportsentinel/internal/classify"
	"github.com/lemonishi/supportsentinel/internal/orchestrator"
	"github.com/lemonishi/supportsentinel/internal/store"
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
