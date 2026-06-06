package store

import "context"

// SeedDemoCustomers upserts a small set of demo customers so the lookup_customer
// tool returns meaningful data in demos. Idempotent.
func (s *Store) SeedDemoCustomers(ctx context.Context) error {
	demo := []Customer{
		{Email: "vip@acme.com", Name: "Acme Corp", Tier: "enterprise", AccountStatus: "active"},
		{Email: "smb@widgets.io", Name: "Widgets Inc", Tier: "business", AccountStatus: "active"},
		{Email: "free@gmail.com", Name: "Casual User", Tier: "free", AccountStatus: "active"},
		{Email: "overdue@latepay.com", Name: "LatePay Ltd", Tier: "business", AccountStatus: "past_due"},
	}
	for _, c := range demo {
		if err := s.UpsertCustomer(ctx, c); err != nil {
			return err
		}
	}
	return nil
}
