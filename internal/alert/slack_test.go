package alert

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/lemonishi/autocierge/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestSlackAlerter_Success(t *testing.T) {
	var capturedBody []byte
	var capturedContentType string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedContentType = r.Header.Get("Content-Type")
		var err error
		capturedBody, err = io.ReadAll(r.Body)
		require.NoError(t, err)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tk := domain.Ticket{
		ID:      uuid.New(),
		Urgency: domain.UrgencyCritical,
		Type:    domain.TypeTechnical,
	}

	a := NewSlack(srv.URL, "http://app.example.com", srv.Client())
	err := a.Alert(context.Background(), tk)
	require.NoError(t, err)

	// Content-Type must be application/json
	require.Equal(t, "application/json", capturedContentType)

	// Body must be valid JSON with a "text" field
	var payload map[string]string
	require.NoError(t, json.Unmarshal(capturedBody, &payload))
	text, ok := payload["text"]
	require.True(t, ok, "payload must have a 'text' key")

	// Payload must mention the ticket ID and urgency
	require.True(t, strings.Contains(text, tk.ID.String()),
		"text must contain ticket ID, got: %s", text)
	require.True(t, strings.Contains(text, string(tk.Urgency)),
		"text must contain urgency, got: %s", text)
	// And the dashboard link
	require.True(t, strings.Contains(text, tk.ID.String()),
		"text must contain a link with ticket ID, got: %s", text)
}

func TestSlackAlerter_NonTwoxx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	tk := domain.Ticket{
		ID:      uuid.New(),
		Urgency: domain.UrgencyCritical,
		Type:    domain.TypeTechnical,
	}

	a := NewSlack(srv.URL, "", srv.Client())
	err := a.Alert(context.Background(), tk)
	require.Error(t, err)
	require.Contains(t, err.Error(), "500")
}
