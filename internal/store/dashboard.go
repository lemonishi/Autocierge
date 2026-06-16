package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/lemonishi/autocierge/internal/domain"
)

// TicketSummary is one row in the dashboard queue.
type TicketSummary struct {
	ID         uuid.UUID
	State      domain.State
	Urgency    domain.Urgency
	Type       domain.TicketType
	Department domain.Department
	Confidence float64
	Subject    string
	FromAddr   string
	CreatedAt  time.Time
}

// ClassificationRecord is a stored classification (with metadata) for the detail view.
type ClassificationRecord struct {
	Urgency    domain.Urgency
	Type       domain.TicketType
	Department domain.Department
	Confidence float64
	Reasoning  string
	Model      string
	ToolsUsed  map[string]any
	CreatedAt  time.Time
}

// ReplyRecord is a stored reply for the detail view.
type ReplyRecord struct {
	DraftText string
	FinalText string
	Status    string
	CreatedAt time.Time
}

// ListTickets returns all tickets for the queue, ordered so items needing human
// action and higher urgency surface first, then newest.
func (s *Store) ListTickets(ctx context.Context) ([]TicketSummary, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT t.id, t.state, COALESCE(t.urgency,''), COALESCE(t.type,''), COALESCE(t.department,''),
		        COALESCE(t.confidence,0), COALESCE(e.subject,''), e.from_addr, t.created_at
		 FROM tickets t JOIN emails e ON e.ticket_id = t.id
		 ORDER BY
		   CASE t.state
		     WHEN 'AWAITING_CLASSIFICATION_REVIEW' THEN 0
		     WHEN 'AWAITING_REPLY_APPROVAL' THEN 1
		     ELSE 2 END,
		   CASE t.urgency
		     WHEN 'critical' THEN 0 WHEN 'high' THEN 1 WHEN 'normal' THEN 2 ELSE 3 END,
		   t.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TicketSummary{}
	for rows.Next() {
		var ts TicketSummary
		if err := rows.Scan(&ts.ID, &ts.State, &ts.Urgency, &ts.Type, &ts.Department,
			&ts.Confidence, &ts.Subject, &ts.FromAddr, &ts.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, ts)
	}
	return out, rows.Err()
}

// GetLatestClassification returns the most recent classification, or ErrNotFound.
func (s *Store) GetLatestClassification(ctx context.Context, ticketID uuid.UUID) (ClassificationRecord, error) {
	var cr ClassificationRecord
	var toolsJSON []byte
	err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(urgency,''), COALESCE(type,''), COALESCE(department,''), COALESCE(confidence,0),
		        COALESCE(reasoning,''), COALESCE(model,''), tools_used, created_at
		 FROM classifications WHERE ticket_id = $1 ORDER BY created_at DESC LIMIT 1`, ticketID).
		Scan(&cr.Urgency, &cr.Type, &cr.Department, &cr.Confidence, &cr.Reasoning, &cr.Model, &toolsJSON, &cr.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ClassificationRecord{}, ErrNotFound
	}
	if err != nil {
		return ClassificationRecord{}, err
	}
	if len(toolsJSON) > 0 {
		_ = json.Unmarshal(toolsJSON, &cr.ToolsUsed)
	}
	return cr, nil
}

// GetLatestReply returns the most recent reply, or ErrNotFound.
func (s *Store) GetLatestReply(ctx context.Context, ticketID uuid.UUID) (ReplyRecord, error) {
	var rr ReplyRecord
	err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(draft_text,''), COALESCE(final_text,''), status, created_at
		 FROM replies WHERE ticket_id = $1 ORDER BY created_at DESC LIMIT 1`, ticketID).
		Scan(&rr.DraftText, &rr.FinalText, &rr.Status, &rr.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReplyRecord{}, ErrNotFound
	}
	if err != nil {
		return ReplyRecord{}, err
	}
	return rr, nil
}
