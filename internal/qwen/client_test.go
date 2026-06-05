package qwen

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lemonishi/supportsentinel/internal/domain"
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

func validClassificationJSON() string {
	return `{"urgency":"high","type":"billing","department":"billing","confidence":0.91,"reasoning":"double charge"}`
}

func TestClassifyParsesValidResponse(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(chatReply(validClassificationJSON())))
	})
	got, err := c.Classify(context.Background(), domain.Email{Subject: "charged twice", Body: "help"})
	require.NoError(t, err)
	require.Equal(t, domain.UrgencyHigh, got.Urgency)
	require.Equal(t, domain.TypeBilling, got.Type)
	require.Equal(t, domain.DeptBilling, got.Department)
	require.InEpsilon(t, 0.91, got.Confidence, 0.001)
	require.Equal(t, "qwen-max", got.Model)
}

func TestClassifyDerivesDepartmentWhenInvalid(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(chatReply(`{"urgency":"normal","type":"technical","department":"nonsense","confidence":0.8}`)))
	})
	got, err := c.Classify(context.Background(), domain.Email{Subject: "bug", Body: "x"})
	require.NoError(t, err)
	require.Equal(t, domain.DeptEngineering, got.Department) // derived from type
}

func TestClassifyClampsConfidence(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(chatReply(`{"urgency":"low","type":"general","department":"support_tier1","confidence":5}`)))
	})
	got, err := c.Classify(context.Background(), domain.Email{Subject: "hi", Body: "x"})
	require.NoError(t, err)
	require.Equal(t, 1.0, got.Confidence)
}

func TestClassifyClampsNegativeConfidence(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(chatReply(`{"urgency":"low","type":"general","department":"support_tier1","confidence":-0.5}`)))
	})
	got, err := c.Classify(context.Background(), domain.Email{Subject: "hi", Body: "x"})
	require.NoError(t, err)
	require.Equal(t, 0.0, got.Confidence)
}

func TestClassifyStripsCodeFences(t *testing.T) {
	fenced := "```json\n" + validClassificationJSON() + "\n```"
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(chatReply(fenced)))
	})
	got, err := c.Classify(context.Background(), domain.Email{Subject: "x", Body: "y"})
	require.NoError(t, err)
	require.Equal(t, domain.TypeBilling, got.Type)
}

func TestClassifyRepromptsOnMalformedThenSucceeds(t *testing.T) {
	var calls int32
	var secondRoles []string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 2 {
			var body chatRequest
			_ = json.NewDecoder(r.Body).Decode(&body)
			for _, m := range body.Messages {
				secondRoles = append(secondRoles, m.Role)
			}
		}
		if n == 1 {
			w.Write([]byte(chatReply("not json at all")))
			return
		}
		w.Write([]byte(chatReply(validClassificationJSON())))
	})
	got, err := c.Classify(context.Background(), domain.Email{Subject: "x", Body: "y"})
	require.NoError(t, err)
	require.Equal(t, domain.TypeBilling, got.Type)
	require.Equal(t, int32(2), atomic.LoadInt32(&calls))
	require.Equal(t, []string{"system", "user", "assistant", "user"}, secondRoles)
}

func TestClassifyErrorsOnPersistentMalformed(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(chatReply("still not json")))
	})
	_, err := c.Classify(context.Background(), domain.Email{Subject: "x", Body: "y"})
	require.Error(t, err) // orchestrator will park this for human review
}

func TestClassifyErrorsOnInvalidEnumAfterReprompt(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(chatReply(`{"urgency":"WAT","type":"billing","confidence":0.5}`)))
	})
	_, err := c.Classify(context.Background(), domain.Email{Subject: "x", Body: "y"})
	require.Error(t, err)
}

func TestDraftReplyReturnsText(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body chatRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Nil(t, body.ResponseFormat) // draft is free text, not JSON mode
		w.Write([]byte(chatReply("Hi, we've refunded the duplicate charge. Sorry for the trouble!")))
	})
	out, err := c.DraftReply(context.Background(),
		domain.Ticket{Urgency: domain.UrgencyHigh, Type: domain.TypeBilling},
		domain.Email{Subject: "charged twice", Body: "double charge"})
	require.NoError(t, err)
	require.Contains(t, out, "refunded")
}

func toolCallReply(id, name, args string) string {
	b, _ := json.Marshal(map[string]any{
		"choices": []map[string]any{{"message": map[string]any{
			"role":    "assistant",
			"content": "",
			"tool_calls": []map[string]any{{
				"id": id, "type": "function",
				"function": map[string]any{"name": name, "arguments": args},
			}},
		}}},
	})
	return string(b)
}

func TestDoChatRawSurfacesToolCalls(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body chatRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Len(t, body.Tools, 1)
		require.Equal(t, "lookup_customer", body.Tools[0].Function.Name)
		w.Write([]byte(toolCallReply("call_1", "lookup_customer", `{"email":"x@y.com"}`)))
	})
	tools := []toolDef{{Type: "function", Function: functionDef{
		Name: "lookup_customer", Description: "look up a customer", Parameters: map[string]any{"type": "object"},
	}}}
	msg, err := c.doChatRaw(context.Background(), []chatMessage{{Role: "user", Content: "hi"}}, false, tools)
	require.NoError(t, err)
	require.Len(t, msg.ToolCalls, 1)
	require.Equal(t, "lookup_customer", msg.ToolCalls[0].Function.Name)
	require.Equal(t, `{"email":"x@y.com"}`, msg.ToolCalls[0].Function.Arguments)
}
