package orchestrator

import (
	"context"
	"os"
	"testing"

	"github.com/lemonishi/supportsentinel/internal/alert"
	"github.com/lemonishi/supportsentinel/internal/classify"
	"github.com/lemonishi/supportsentinel/internal/domain"
	"github.com/lemonishi/supportsentinel/internal/store"
	"github.com/stretchr/testify/require"
)

func newOrch(t *testing.T) (*Orchestrator, *store.Store, *alert.Recording) {
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
	rec := alert.NewRecording()
	o := New(s, classify.NewFake(), rec, 0.75)
	return o, s, rec
}

func TestIngestHighConfidenceAutoRoutesToDraft(t *testing.T) {
	o, s, rec := newOrch(t)
	ctx := context.Background()
	tk, err := o.Ingest(ctx, "http", domain.Email{
		FromAddr: "c@x.com", Subject: "invoice problem", Body: "charged twice", DedupeKey: "o1",
	})
	require.NoError(t, err)

	got, err := s.GetTicket(ctx, tk.ID)
	require.NoError(t, err)
	// High-confidence, non-critical → auto-routed then drafted → parked at reply approval.
	require.Equal(t, domain.StateAwaitingReplyApproval, got.State)
	require.Equal(t, domain.TypeBilling, got.Type)
	require.Empty(t, rec.Sent)
}

func TestIngestCriticalAlwaysParksAtReviewAndAlerts(t *testing.T) {
	o, s, rec := newOrch(t)
	ctx := context.Background()
	tk, err := o.Ingest(ctx, "http", domain.Email{
		FromAddr: "c@x.com", Subject: "URGENT outage", Body: "production down", DedupeKey: "o2",
	})
	require.NoError(t, err)

	got, err := s.GetTicket(ctx, tk.ID)
	require.NoError(t, err)
	require.Equal(t, domain.StateAwaitingClassificationReview, got.State)
	require.Len(t, rec.Sent, 1) // alert fired for critical
}

func TestIngestLowConfidenceParksAtReview(t *testing.T) {
	o, s, _ := newOrch(t)
	ctx := context.Background()
	tk, err := o.Ingest(ctx, "http", domain.Email{
		FromAddr: "c@x.com", Subject: "hello", Body: "I have a thing", DedupeKey: "o3",
	})
	require.NoError(t, err)
	got, err := s.GetTicket(ctx, tk.ID)
	require.NoError(t, err)
	require.Equal(t, domain.StateAwaitingClassificationReview, got.State)
}
