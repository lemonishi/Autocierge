package store

import (
	"context"
	"os"
	"testing"

	"github.com/lemonishi/autocierge/internal/domain"
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
