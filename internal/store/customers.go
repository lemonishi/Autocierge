package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/lemonishi/autocierge/internal/domain"
)

// Customer is a seeded account record used by the lookup_customer tool.
type Customer struct {
	Email         string
	Name          string
	Tier          string
	AccountStatus string
}

// SimilarTicket is a past ticket surfaced by the lookup_similar_tickets tool.
type SimilarTicket struct {
	Subject string
	Type    domain.TicketType
	Urgency domain.Urgency
}

// UpsertCustomer inserts or updates a customer by email (idempotent).
func (s *Store) UpsertCustomer(ctx context.Context, c Customer) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO customers (email, name, tier, account_status)
		 VALUES ($1,$2,$3,$4)
		 ON CONFLICT (email) DO UPDATE SET
		   name = EXCLUDED.name, tier = EXCLUDED.tier, account_status = EXCLUDED.account_status`,
		c.Email, c.Name, c.Tier, c.AccountStatus)
	return err
}

// GetCustomer returns the customer with the given email, or ErrNotFound.
func (s *Store) GetCustomer(ctx context.Context, email string) (Customer, error) {
	var c Customer
	err := s.pool.QueryRow(ctx,
		`SELECT email, COALESCE(name,''), COALESCE(tier,''), COALESCE(account_status,'')
		 FROM customers WHERE email = $1`, email).
		Scan(&c.Email, &c.Name, &c.Tier, &c.AccountStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return Customer{}, ErrNotFound
	}
	if err != nil {
		return Customer{}, err
	}
	return c, nil
}

// FindSimilarTickets returns up to `limit` already-classified tickets whose email
// subject or body matches the query substring (most recent first). Used to show
// the model how comparable past tickets were classified.
func (s *Store) FindSimilarTickets(ctx context.Context, query string, limit int) ([]SimilarTicket, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT COALESCE(e.subject,''), t.type, t.urgency
		 FROM tickets t JOIN emails e ON e.ticket_id = t.id
		 WHERE t.type IS NOT NULL
		   AND (e.subject ILIKE '%' || $1 || '%' OR e.body ILIKE '%' || $1 || '%')
		 ORDER BY t.created_at DESC
		 LIMIT $2`, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SimilarTicket{}
	for rows.Next() {
		var st SimilarTicket
		if err := rows.Scan(&st.Subject, &st.Type, &st.Urgency); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}
