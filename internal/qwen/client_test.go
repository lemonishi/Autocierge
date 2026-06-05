package qwen

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// newTestClient points a Client at an httptest server with near-zero backoff.
func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := New("sk-test", srv.URL, "qwen-max", srv.Client())
	c.retryBackoff = time.Millisecond // keep tests fast
	return c
}

// chatReply is a minimal OpenAI-compatible completion response with the given content.
func chatReply(content string) string {
	b, _ := json.Marshal(map[string]any{
		"choices": []map[string]any{
			{"message": map[string]string{"role": "assistant", "content": content}},
		},
	})
	return string(b)
}

func TestDoChatRetriesOn500ThenSucceeds(t *testing.T) {
	var calls int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write([]byte(chatReply("hello")))
	})
	out, err := c.doChat(context.Background(), []chatMessage{{Role: "user", Content: "hi"}}, false)
	require.NoError(t, err)
	require.Equal(t, "hello", out)
	require.Equal(t, int32(2), atomic.LoadInt32(&calls))
}

func TestDoChatReturnsErrorAfterExhaustingRetries(t *testing.T) {
	var calls int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadGateway)
	})
	_, err := c.doChat(context.Background(), []chatMessage{{Role: "user", Content: "hi"}}, false)
	require.Error(t, err)
	require.Equal(t, int32(3), atomic.LoadInt32(&calls))
}

func TestDoChatRespectsContextCancellation(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError) // retryable, so it would loop
	})
	c.retryBackoff = 50 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before the call
	_, err := c.doChat(ctx, []chatMessage{{Role: "user", Content: "hi"}}, false)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

func TestDoChatDoesNotRetryOn400(t *testing.T) {
	var calls int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"bad"}`))
	})
	_, err := c.doChat(context.Background(), []chatMessage{{Role: "user", Content: "hi"}}, false)
	require.Error(t, err)
	require.Equal(t, int32(1), atomic.LoadInt32(&calls)) // 4xx is non-retryable
}

func TestDoChatSendsAuthAndModel(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer sk-test", r.Header.Get("Authorization"))
		var body chatRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, "qwen-max", body.Model)
		require.Len(t, body.Messages, 1)
		w.Write([]byte(chatReply("ok")))
	})
	out, err := c.doChat(context.Background(), []chatMessage{{Role: "user", Content: "hi"}}, false)
	require.NoError(t, err)
	require.Equal(t, "ok", out)
}

func TestDoChatJSONModeSetsResponseFormat(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body chatRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.NotNil(t, body.ResponseFormat)
		require.Equal(t, "json_object", body.ResponseFormat.Type)
		w.Write([]byte(chatReply("{}")))
	})
	_, err := c.doChat(context.Background(), []chatMessage{{Role: "user", Content: "hi"}}, true)
	require.NoError(t, err)
}
