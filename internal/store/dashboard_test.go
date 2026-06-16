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
