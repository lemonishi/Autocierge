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
