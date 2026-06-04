// Package orchestrator is the agent core: a deterministic state machine that
// drives a ticket from ingestion through both human-in-the-loop checkpoints.
// It gates routing on confidence and criticality, and FAILS TOWARD A HUMAN —
// any classifier error parks the ticket for review rather than dropping it.
package orchestrator

import (
	"context"
	"log"

	"github.com/google/uuid"
	"github.com/lemonishi/supportsentinel/internal/alert"
	"github.com/lemonishi/supportsentinel/internal/domain"
	"github.com/lemonishi/supportsentinel/internal/store"
)

type Orchestrator struct {
	store     *store.Store
	clf       domain.Classifier
	alerter   alert.Alerter
	threshold float64
}

func New(s *store.Store, clf domain.Classifier, a alert.Alerter, threshold float64) *Orchestrator {
	return &Orchestrator{store: s, clf: clf, alerter: a, threshold: threshold}
}

// Ingest creates the ticket and runs classification synchronously.
func (o *Orchestrator) Ingest(ctx context.Context, source string, e domain.Email) (domain.Ticket, error) {
	tk, err := o.store.CreateTicketWithEmail(ctx, source, e)
	if err != nil {
		return domain.Ticket{}, err
	}
	if tk.State != domain.StateNew {
		// Idempotent re-delivery of an already-processed email; return as-is.
		return tk, nil
	}
	if err := o.runClassify(ctx, tk.ID); err != nil {
		return domain.Ticket{}, err
	}
	return o.store.GetTicket(ctx, tk.ID)
}

func (o *Orchestrator) runClassify(ctx context.Context, ticketID uuid.UUID) error {
	if err := o.store.Apply(ctx, store.Transition{
		TicketID: ticketID, From: domain.StateNew, To: domain.StateClassifying, Actor: "system",
	}); err != nil {
		return err
	}

	email, err := o.store.GetEmailByTicket(ctx, ticketID)
	if err != nil {
		return err
	}

	c, err := o.clf.Classify(ctx, email)
	if err != nil {
		// FAIL TOWARD A HUMAN: park for review with the error in the payload.
		log.Printf("classify error for %s: %v", ticketID, err)
		return o.store.Apply(ctx, store.Transition{
			TicketID: ticketID, From: domain.StateClassifying,
			To: domain.StateAwaitingClassificationReview, Actor: "system",
			Payload: map[string]any{"classify_error": err.Error()},
		})
	}

	if err := o.store.SaveClassification(ctx, ticketID, c); err != nil {
		return err
	}

	urg, typ, dep, conf := c.Urgency, c.Type, c.Department, c.Confidence
	parks := c.Urgency == domain.UrgencyCritical || c.Confidence < o.threshold
	if parks {
		if err := o.store.Apply(ctx, store.Transition{
			TicketID: ticketID, From: domain.StateClassifying,
			To: domain.StateAwaitingClassificationReview, Actor: "qwen",
			SetUrgency: &urg, SetType: &typ, SetDepartment: &dep, SetConfidence: &conf,
			Payload: c,
		}); err != nil {
			return err
		}
		if c.Urgency == domain.UrgencyCritical {
			tk, err := o.store.GetTicket(ctx, ticketID)
			if err == nil {
				_ = o.alerter.Alert(ctx, tk) // best-effort
			}
		}
		return nil
	}

	// High-confidence, non-critical → auto-route, then draft.
	if err := o.store.Apply(ctx, store.Transition{
		TicketID: ticketID, From: domain.StateClassifying, To: domain.StateRouted, Actor: "qwen",
		SetUrgency: &urg, SetType: &typ, SetDepartment: &dep, SetConfidence: &conf, Payload: c,
	}); err != nil {
		return err
	}
	return o.runDraft(ctx, ticketID)
}

func (o *Orchestrator) runDraft(ctx context.Context, ticketID uuid.UUID) error {
	if err := o.store.Apply(ctx, store.Transition{
		TicketID: ticketID, From: domain.StateRouted, To: domain.StateDrafting, Actor: "system",
	}); err != nil {
		return err
	}
	tk, err := o.store.GetTicket(ctx, ticketID)
	if err != nil {
		return err
	}
	email, err := o.store.GetEmailByTicket(ctx, ticketID)
	if err != nil {
		return err
	}
	draft, err := o.clf.DraftReply(ctx, tk, email)
	if err != nil {
		log.Printf("draft error for %s: %v", ticketID, err)
		// Stay in DRAFTING is risky; park at reply approval with empty draft so a human writes one.
		draft = ""
	}
	if _, err := o.store.SaveReplyDraft(ctx, ticketID, draft); err != nil {
		return err
	}
	return o.store.Apply(ctx, store.Transition{
		TicketID: ticketID, From: domain.StateDrafting, To: domain.StateAwaitingReplyApproval, Actor: "system",
	})
}
