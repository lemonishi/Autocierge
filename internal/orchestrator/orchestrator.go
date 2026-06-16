// Package orchestrator is the agent core: a deterministic state machine that
// drives a ticket from ingestion through both human-in-the-loop checkpoints.
// It gates routing on confidence and criticality, and FAILS TOWARD A HUMAN —
// any classifier error parks the ticket for review rather than dropping it.
package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/google/uuid"
	"github.com/lemonishi/autocierge/internal/alert"
	"github.com/lemonishi/autocierge/internal/domain"
	"github.com/lemonishi/autocierge/internal/store"
)

// ErrInvalidReview is returned when a human review decision contains an invalid enum value.
var ErrInvalidReview = errors.New("invalid review decision")

// ErrEmptyReply is returned when an approved reply has no text.
var ErrEmptyReply = errors.New("final reply text must not be empty")

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
	// Park if critical, or if confidence is strictly below threshold
	// (confidence == threshold is treated as confident enough to auto-route).
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
			if err != nil {
				log.Printf("alert: GetTicket %s failed: %v; skipping alert", ticketID, err)
			} else {
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
	return o.draftAndPark(ctx, ticketID)
}

// draftAndPark drafts a reply for a ticket already in DRAFTING and parks it for
// human approval. On a draft error it logs and falls back to an empty draft so a
// human writes the reply (fail toward a human).
func (o *Orchestrator) draftAndPark(ctx context.Context, ticketID uuid.UUID) error {
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
		draft = ""
	}
	if _, err := o.store.SaveReplyDraft(ctx, ticketID, draft); err != nil {
		return err
	}
	return o.store.Apply(ctx, store.Transition{
		TicketID: ticketID, From: domain.StateDrafting,
		To: domain.StateAwaitingReplyApproval, Actor: "system",
	})
}

// ReviewDecision is the human's Checkpoint-1 input. Empty fields fall back to
// the model's stored values (handled by COALESCE in Apply when pointers are nil).
type ReviewDecision struct {
	Urgency    domain.Urgency
	Type       domain.TicketType
	Department domain.Department
}

// ReviewClassification (Checkpoint 1): a human confirms/overrides the routing,
// after which the ticket routes and a reply is drafted.
func (o *Orchestrator) ReviewClassification(ctx context.Context, ticketID uuid.UUID, d ReviewDecision, reviewer string) error {
	if d.Urgency != "" && !domain.ValidUrgency(d.Urgency) {
		return fmt.Errorf("%w: urgency %q", ErrInvalidReview, d.Urgency)
	}
	if d.Type != "" && !domain.ValidType(d.Type) {
		return fmt.Errorf("%w: type %q", ErrInvalidReview, d.Type)
	}
	if d.Department != "" && !domain.ValidDepartment(d.Department) {
		return fmt.Errorf("%w: department %q", ErrInvalidReview, d.Department)
	}
	tr := store.Transition{
		TicketID: ticketID, From: domain.StateAwaitingClassificationReview,
		To: domain.StateRouted, Actor: "human:" + reviewer, Payload: d,
	}
	if d.Urgency != "" {
		u := d.Urgency
		tr.SetUrgency = &u
	}
	if d.Type != "" {
		t := d.Type
		tr.SetType = &t
	}
	if d.Department != "" {
		dep := d.Department
		tr.SetDepartment = &dep
	}
	if err := o.store.Apply(ctx, tr); err != nil {
		return err
	}
	return o.runDraft(ctx, ticketID)
}

// ApproveReply (Checkpoint 2): human approves/edits the draft; ticket resolves.
func (o *Orchestrator) ApproveReply(ctx context.Context, ticketID uuid.UUID, finalText, reviewer string) error {
	if strings.TrimSpace(finalText) == "" {
		return ErrEmptyReply
	}
	replyID, err := o.store.LatestReplyID(ctx, ticketID)
	if err != nil {
		return err
	}
	// NOTE: FinalizeReply and Apply are separate steps (consistent with the
	// codebase's "save then Apply" pattern). A crash between them leaves a
	// transient inconsistency; the planned orchestrator crash-recovery (re-scan of
	// DRAFTING/CLASSIFYING on restart) is the intended holistic remedy.
	if err := o.store.FinalizeReply(ctx, replyID, "approved", finalText); err != nil {
		return err
	}
	return o.store.Apply(ctx, store.Transition{
		TicketID: ticketID, From: domain.StateAwaitingReplyApproval,
		To: domain.StateResolved, Actor: "human:" + reviewer,
		Payload: map[string]any{"final_text": finalText},
	})
}

// RejectReply (Checkpoint 2): human rejects the draft; re-draft and park again.
func (o *Orchestrator) RejectReply(ctx context.Context, ticketID uuid.UUID, reviewer string) error {
	replyID, err := o.store.LatestReplyID(ctx, ticketID)
	if err != nil {
		return err
	}
	// NOTE: FinalizeReply and Apply are separate steps (consistent with the
	// codebase's "save then Apply" pattern). A crash between them leaves a
	// transient inconsistency; the planned orchestrator crash-recovery (re-scan of
	// DRAFTING/CLASSIFYING on restart) is the intended holistic remedy.
	if err := o.store.FinalizeReply(ctx, replyID, "rejected", ""); err != nil {
		return err
	}
	if err := o.store.Apply(ctx, store.Transition{
		TicketID: ticketID, From: domain.StateAwaitingReplyApproval,
		To: domain.StateDrafting, Actor: "human:" + reviewer,
	}); err != nil {
		return err
	}
	// Re-draft from DRAFTING. runDraft expects ROUTED→DRAFTING, so draft directly.
	return o.draftAndPark(ctx, ticketID)
}
