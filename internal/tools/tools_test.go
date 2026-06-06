package tools

import (
	"context"
	"os"
	"testing"

	"github.com/lemonishi/supportsentinel/internal/domain"
	"github.com/lemonishi/supportsentinel/internal/store"
	"github.com/stretchr/testify/require"
)

func newStore(t *testing.T) *store.Store {
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
	return s
}

func TestDefinitionsExposesBothTools(t *testing.T) {
	b := New(newStore(t))
	defs := b.Definitions()
	names := []string{}
	for _, d := range defs {
		names = append(names, d.Name)
	}
	require.ElementsMatch(t, []string{"lookup_customer", "lookup_similar_tickets"}, names)
}

func TestInvokeLookupCustomer(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	require.NoError(t, s.UpsertCustomer(ctx, store.Customer{
		Email: "vip@acme.com", Name: "Acme VIP", Tier: "enterprise", AccountStatus: "active",
	}))
	b := New(s)

	out, err := b.Invoke(ctx, "lookup_customer", `{"email":"vip@acme.com"}`)
	require.NoError(t, err)
	require.Contains(t, out, "enterprise")

	// Unknown customer → a found:false result, not an error.
	out, err = b.Invoke(ctx, "lookup_customer", `{"email":"ghost@nowhere.com"}`)
	require.NoError(t, err)
	require.Contains(t, out, "false")
}

func TestInvokeLookupSimilarTickets(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	tk, err := s.CreateTicketWithEmail(ctx, "http", domain.Email{
		FromAddr: "a@b.com", Subject: "refund for invoice", Body: "x", DedupeKey: "t1",
	})
	require.NoError(t, err)
	urg, typ, dep := domain.UrgencyNormal, domain.TypeBilling, domain.DeptBilling
	require.NoError(t, s.Apply(ctx, store.Transition{
		TicketID: tk.ID, From: domain.StateNew, To: domain.StateRouted, Actor: "system",
		SetUrgency: &urg, SetType: &typ, SetDepartment: &dep,
	}))
	b := New(s)

	out, err := b.Invoke(ctx, "lookup_similar_tickets", `{"query":"invoice"}`)
	require.NoError(t, err)
	require.Contains(t, out, "billing")
}

func TestInvokeUnknownTool(t *testing.T) {
	b := New(newStore(t))
	_, err := b.Invoke(context.Background(), "no_such_tool", `{}`)
	require.Error(t, err)
}

func TestInvokeBadArgsJSON(t *testing.T) {
	b := New(newStore(t))
	_, err := b.Invoke(context.Background(), "lookup_customer", `not json`)
	require.Error(t, err)
}
