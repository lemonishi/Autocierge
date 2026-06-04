// Package classify holds Classifier implementations. Fake is a deterministic,
// keyword-based stand-in used to build and test the pipeline before the real
// Qwen client (Plan 2) exists.
package classify

import (
	"context"
	"strings"

	"github.com/lemonishi/supportsentinel/internal/domain"
)

type Fake struct{}

func NewFake() *Fake { return &Fake{} }

func (f *Fake) Classify(_ context.Context, e domain.Email) (domain.Classification, error) {
	text := strings.ToLower(e.Subject + " " + e.Body)

	typ := domain.TypeGeneral
	conf := 0.5 // ambiguous default → below threshold → Checkpoint 1
	switch {
	case containsAny(text, "invoice", "charge", "charged", "billing", "refund", "payment"):
		typ, conf = domain.TypeBilling, 0.88
	case containsAny(text, "error", "down", "crash", "bug", "broken", "fails", "failing"):
		typ, conf = domain.TypeTechnical, 0.86
	case containsAny(text, "login", "password", "account", "locked", "access"):
		typ, conf = domain.TypeAccount, 0.84
	case containsAny(text, "feature", "request", "would be nice", "suggestion"):
		typ, conf = domain.TypeFeatureRequest, 0.82
	}

	urg := domain.UrgencyNormal
	switch {
	case containsAny(text, "urgent", "asap", "down", "outage", "immediately", "critical"):
		urg = domain.UrgencyCritical
	case containsAny(text, "soon", "important", "blocked"):
		urg = domain.UrgencyHigh
	}

	return domain.Classification{
		Urgency:    urg,
		Type:       typ,
		Department: domain.DepartmentForType(typ),
		Confidence: conf,
		Reasoning:  "fake keyword classifier",
		Model:      "fake",
	}, nil
}

func (f *Fake) DraftReply(_ context.Context, _ domain.Ticket, e domain.Email) (string, error) {
	return "Hi,\n\nThanks for reaching out about \"" + e.Subject +
		"\". We've received your message and our team is looking into it.\n\nBest,\nSupport", nil
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
