package store

import (
	"context"
	"testing"

	"github.com/lemonishi/supportsentinel/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestUpsertAndGetCustomer(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.UpsertCustomer(ctx, Customer{
		Email: "vip@acme.com", Name: "Acme VIP", Tier: "enterprise", AccountStatus: "active",
	}))
	// Upsert again with changed tier — should update, not duplicate.
	require.NoError(t, s.UpsertCustomer(ctx, Customer{
		Email: "vip@acme.com", Name: "Acme VIP", Tier: "enterprise_plus", AccountStatus: "active",
	}))

	got, err := s.GetCustomer(ctx, "vip@acme.com")
	require.NoError(t, err)
	require.Equal(t, "enterprise_plus", got.Tier)
	require.Equal(t, "Acme VIP", got.Name)
}

func TestGetCustomerNotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetCustomer(context.Background(), "nobody@nowhere.com")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestFindSimilarTickets(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Seed a resolved-ish ticket classified as billing with a matching subject.
	tk, err := s.CreateTicketWithEmail(ctx, "http", domain.Email{
		FromAddr: "a@b.com", Subject: "duplicate invoice charge", Body: "charged twice", DedupeKey: "sim-1",
	})
	require.NoError(t, err)
	urg, typ, dep := domain.UrgencyHigh, domain.TypeBilling, domain.DeptBilling
	require.NoError(t, s.Apply(ctx, Transition{
		TicketID: tk.ID, From: domain.StateNew, To: domain.StateRouted, Actor: "system",
		SetUrgency: &urg, SetType: &typ, SetDepartment: &dep,
	}))

	got, err := s.FindSimilarTickets(ctx, "invoice", 5)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(got), 1)
	require.Equal(t, domain.TypeBilling, got[0].Type)
	require.Contains(t, got[0].Subject, "invoice")

	// A query that matches nothing returns an empty slice, not an error.
	none, err := s.FindSimilarTickets(ctx, "zzzznomatch", 5)
	require.NoError(t, err)
	require.Len(t, none, 0)
}
