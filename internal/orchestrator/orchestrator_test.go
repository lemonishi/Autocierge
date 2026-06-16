package orchestrator

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/lemonishi/autocierge/internal/alert"
	"github.com/lemonishi/autocierge/internal/classify"
	"github.com/lemonishi/autocierge/internal/domain"
	"github.com/lemonishi/autocierge/internal/store"
	"github.com/stretchr/testify/require"
)

// errClassifier always fails Classify, to exercise the fail-toward-a-human path.
type errClassifier struct{}

func (errClassifier) Classify(context.Context, domain.Email) (domain.Classification, error) {
	return domain.Classification{}, errors.New("boom")
}
func (errClassifier) DraftReply(context.Context, domain.Ticket, domain.Email) (string, error) {
	return "", nil
}

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
	require.Equal(t, domain.UrgencyCritical, rec.Sent[0].Urgency)
}

func TestIngestClassifyErrorParksForHuman(t *testing.T) {
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

	o := New(s, errClassifier{}, alert.NewRecording(), 0.75)
	tk, err := o.Ingest(context.Background(), "http", domain.Email{
		FromAddr: "c@x.com", Subject: "anything", Body: "anything", DedupeKey: "err1",
	})
	require.NoError(t, err)
	require.Equal(t, domain.StateAwaitingClassificationReview, tk.State)
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

func TestReviewClassificationRoutesAndDrafts(t *testing.T) {
	o, s, _ := newOrch(t)
	ctx := context.Background()
	tk, err := o.Ingest(ctx, "http", domain.Email{
		FromAddr: "c@x.com", Subject: "hello", Body: "I have a thing", DedupeKey: "r1",
	})
	require.NoError(t, err)
	require.Equal(t, domain.StateAwaitingClassificationReview, tk.State)

	err = o.ReviewClassification(ctx, tk.ID, ReviewDecision{
		Urgency: domain.UrgencyNormal, Type: domain.TypeAccount, Department: domain.DeptAccounts,
	}, "alice")
	require.NoError(t, err)

	got, err := s.GetTicket(ctx, tk.ID)
	require.NoError(t, err)
	require.Equal(t, domain.StateAwaitingReplyApproval, got.State)
	require.Equal(t, domain.TypeAccount, got.Type)
}

func TestApproveReplyResolves(t *testing.T) {
	o, s, _ := newOrch(t)
	ctx := context.Background()
	tk, err := o.Ingest(ctx, "http", domain.Email{
		FromAddr: "c@x.com", Subject: "invoice", Body: "charged twice", DedupeKey: "r2",
	})
	require.NoError(t, err)
	require.Equal(t, domain.StateAwaitingReplyApproval, tk.State)

	err = o.ApproveReply(ctx, tk.ID, "Resolved your billing issue.", "bob")
	require.NoError(t, err)
	got, err := s.GetTicket(ctx, tk.ID)
	require.NoError(t, err)
	require.Equal(t, domain.StateResolved, got.State)

	var status, final string
	err = s.Pool().QueryRow(ctx,
		`SELECT status, COALESCE(final_text,'') FROM replies WHERE ticket_id=$1 ORDER BY created_at DESC LIMIT 1`, tk.ID).
		Scan(&status, &final)
	require.NoError(t, err)
	require.Equal(t, "approved", status)
	require.Equal(t, "Resolved your billing issue.", final)
}

func TestRejectReplyReturnsToDrafting(t *testing.T) {
	o, s, _ := newOrch(t)
	ctx := context.Background()
	tk, err := o.Ingest(ctx, "http", domain.Email{
		FromAddr: "c@x.com", Subject: "invoice", Body: "charged twice", DedupeKey: "r3",
	})
	require.NoError(t, err)

	require.NoError(t, o.RejectReply(ctx, tk.ID, "bob"))
	got, err := s.GetTicket(ctx, tk.ID)
	require.NoError(t, err)
	// Re-draft runs immediately, so it parks at reply approval again.
	require.Equal(t, domain.StateAwaitingReplyApproval, got.State)

	var rejectedCount int
	err = s.Pool().QueryRow(ctx,
		`SELECT count(*) FROM replies WHERE ticket_id=$1 AND status='rejected'`, tk.ID).Scan(&rejectedCount)
	require.NoError(t, err)
	require.GreaterOrEqual(t, rejectedCount, 1)
}

func TestReviewClassificationPartialOverrideKeepsStoredValues(t *testing.T) {
	o, s, _ := newOrch(t)
	ctx := context.Background()
	// Low-confidence ambiguous ticket parks at classification review with the
	// fake classifier's stored values (TypeGeneral for this subject/body).
	tk, err := o.Ingest(ctx, "http", domain.Email{
		FromAddr: "c@x.com", Subject: "hello", Body: "I have a thing", DedupeKey: "po1",
	})
	require.NoError(t, err)
	require.Equal(t, domain.StateAwaitingClassificationReview, tk.State)

	// Override ONLY Department; Urgency and Type are left empty and must fall
	// back to the classifier's stored values via COALESCE in Apply.
	err = o.ReviewClassification(ctx, tk.ID, ReviewDecision{
		Department: domain.DeptEngineering,
	}, "alice")
	require.NoError(t, err)

	got, err := s.GetTicket(ctx, tk.ID)
	require.NoError(t, err)
	require.Equal(t, domain.TypeGeneral, got.Type)
	require.Equal(t, domain.DeptEngineering, got.Department)
}
