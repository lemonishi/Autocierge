// Package store is the PostgreSQL persistence layer. Every state change goes
// through Apply, which updates the ticket and writes the audit_log row in one
// transaction, guaranteeing a complete, replayable history.
package store

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lemonishi/autocierge/internal/domain"
)

//go:embed schema.sql
var schemaSQL string

var ErrStateConflict = errors.New("ticket not in expected state")
var ErrNotFound = errors.New("not found")
var ErrInvalidState = errors.New("invalid target state")

type Store struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, url string) (*Store, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if _, err := pool.Exec(ctx, schemaSQL); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

// Pool exposes the underlying pool for test setup/teardown. Not for app use.
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// CreateTicketWithEmail inserts a NEW ticket plus its email atomically. If an
// email with the same DedupeKey already exists, it returns the existing ticket
// (idempotent ingestion).
func (s *Store) CreateTicketWithEmail(ctx context.Context, source string, e domain.Email) (domain.Ticket, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Ticket{}, err
	}
	defer tx.Rollback(ctx)

	if e.DedupeKey != "" {
		var existing uuid.UUID
		err := tx.QueryRow(ctx, `SELECT ticket_id FROM emails WHERE dedupe_key = $1`, e.DedupeKey).Scan(&existing)
		if err == nil {
			if err := tx.Commit(ctx); err != nil {
				return domain.Ticket{}, err
			}
			return s.GetTicket(ctx, existing)
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return domain.Ticket{}, err
		}
	}

	ticketID := uuid.New()
	now := time.Now().UTC()
	_, err = tx.Exec(ctx,
		`INSERT INTO tickets (id, state, source, created_at, updated_at) VALUES ($1,$2,$3,$4,$4)`,
		ticketID, domain.StateNew, source, now)
	if err != nil {
		return domain.Ticket{}, err
	}
	rawJSON, err := json.Marshal(e.Raw)
	if err != nil {
		return domain.Ticket{}, fmt.Errorf("marshal raw: %w", err)
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO emails (id, ticket_id, from_addr, to_addr, subject, body, raw, dedupe_key, received_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		uuid.New(), ticketID, e.FromAddr, e.ToAddr, e.Subject, e.Body, rawJSON, nullStr(e.DedupeKey), now)
	if err != nil {
		return domain.Ticket{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Ticket{}, err
	}
	return domain.Ticket{ID: ticketID, State: domain.StateNew, Source: source, CreatedAt: now, UpdatedAt: now}, nil
}

func (s *Store) GetTicket(ctx context.Context, id uuid.UUID) (domain.Ticket, error) {
	var t domain.Ticket
	var urg, typ, dep *string
	var conf *float64
	err := s.pool.QueryRow(ctx,
		`SELECT id, state, source, urgency, type, department, confidence, created_at, updated_at
		 FROM tickets WHERE id = $1`, id).
		Scan(&t.ID, &t.State, &t.Source, &urg, &typ, &dep, &conf, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Ticket{}, ErrNotFound
	}
	if err != nil {
		return domain.Ticket{}, err
	}
	if urg != nil {
		t.Urgency = domain.Urgency(*urg)
	}
	if typ != nil {
		t.Type = domain.TicketType(*typ)
	}
	if dep != nil {
		t.Department = domain.Department(*dep)
	}
	if conf != nil {
		t.Confidence = *conf
	}
	return t, nil
}

func (s *Store) GetEmailByTicket(ctx context.Context, ticketID uuid.UUID) (domain.Email, error) {
	var e domain.Email
	var rawJSON []byte
	var dedupe *string
	err := s.pool.QueryRow(ctx,
		`SELECT id, ticket_id, from_addr, COALESCE(to_addr,''), COALESCE(subject,''), COALESCE(body,''), raw, dedupe_key, received_at
		 FROM emails WHERE ticket_id = $1 ORDER BY received_at LIMIT 1`, ticketID).
		Scan(&e.ID, &e.TicketID, &e.FromAddr, &e.ToAddr, &e.Subject, &e.Body, &rawJSON, &dedupe, &e.ReceivedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Email{}, ErrNotFound
	}
	if err != nil {
		return domain.Email{}, err
	}
	if len(rawJSON) > 0 {
		_ = json.Unmarshal(rawJSON, &e.Raw)
	}
	if dedupe != nil {
		e.DedupeKey = *dedupe
	}
	return e, nil
}

// Transition describes a single state change plus optional ticket field updates.
type Transition struct {
	TicketID      uuid.UUID
	From          domain.State // expected current state; "" skips the check
	To            domain.State
	Actor         string // "system" | "qwen" | "human:<id>"
	Payload       any
	SetUrgency    *domain.Urgency
	SetType       *domain.TicketType
	SetDepartment *domain.Department
	SetConfidence *float64
}

// Apply performs the transition and writes the audit row in one transaction.
func (s *Store) Apply(ctx context.Context, tr Transition) error {
	if !domain.ValidState(tr.To) {
		return ErrInvalidState
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var current domain.State
	if err := tx.QueryRow(ctx, `SELECT state FROM tickets WHERE id = $1 FOR UPDATE`, tr.TicketID).Scan(&current); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if tr.From != "" && current != tr.From {
		return ErrStateConflict
	}

	_, err = tx.Exec(ctx,
		`UPDATE tickets SET state=$1,
		   urgency=COALESCE($2, urgency),
		   type=COALESCE($3, type),
		   department=COALESCE($4, department),
		   confidence=COALESCE($5, confidence),
		   updated_at=now()
		 WHERE id=$6`,
		tr.To, ptrStr(tr.SetUrgency), ptrStr(tr.SetType), ptrStr(tr.SetDepartment), tr.SetConfidence, tr.TicketID)
	if err != nil {
		return err
	}

	payloadJSON, err := json.Marshal(tr.Payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO audit_log (id, ticket_id, from_state, to_state, actor, payload)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		uuid.New(), tr.TicketID, nullStr(string(current)), tr.To, tr.Actor, payloadJSON)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) SaveClassification(ctx context.Context, ticketID uuid.UUID, c domain.Classification) error {
	toolsJSON, err := json.Marshal(c.ToolsUsed)
	if err != nil {
		return fmt.Errorf("marshal tools_used: %w", err)
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO classifications (id, ticket_id, urgency, type, department, confidence, reasoning, tools_used, model)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		uuid.New(), ticketID, c.Urgency, c.Type, c.Department, c.Confidence, c.Reasoning, toolsJSON, c.Model)
	return err
}

func (s *Store) SaveReplyDraft(ctx context.Context, ticketID uuid.UUID, draft string) (uuid.UUID, error) {
	id := uuid.New()
	_, err := s.pool.Exec(ctx,
		`INSERT INTO replies (id, ticket_id, draft_text, status) VALUES ($1,$2,$3,'draft')`,
		id, ticketID, draft)
	return id, err
}

func (s *Store) FinalizeReply(ctx context.Context, replyID uuid.UUID, status, finalText string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE replies SET status=$1, final_text=$2 WHERE id=$3`, status, finalText, replyID)
	return err
}

type AuditRow struct {
	From      domain.State
	To        domain.State
	Actor     string
	CreatedAt time.Time
}

func (s *Store) AuditLog(ctx context.Context, ticketID uuid.UUID) ([]AuditRow, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT COALESCE(from_state,''), to_state, actor, created_at
		 FROM audit_log WHERE ticket_id=$1 ORDER BY created_at ASC`, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditRow
	for rows.Next() {
		var r AuditRow
		if err := rows.Scan(&r.From, &r.To, &r.Actor, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) LatestReplyID(ctx context.Context, ticketID uuid.UUID) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.pool.QueryRow(ctx,
		`SELECT id FROM replies WHERE ticket_id=$1 ORDER BY created_at DESC LIMIT 1`, ticketID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	return id, err
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// ptrStr converts a typed-string pointer (Urgency/TicketType/Department) to a
// nullable any for COALESCE updates.
func ptrStr[T ~string](p *T) any {
	if p == nil {
		return nil
	}
	return string(*p)
}
