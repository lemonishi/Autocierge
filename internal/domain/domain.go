// Package domain defines the fixed taxonomies, core entities, and the
// Classifier interface. The enums here are the hard contract shared by the
// classifier, orchestrator, store, dashboard, and eval harness.
package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Urgency string

const (
	UrgencyLow      Urgency = "low"
	UrgencyNormal   Urgency = "normal"
	UrgencyHigh     Urgency = "high"
	UrgencyCritical Urgency = "critical"
)

type TicketType string

const (
	TypeBilling        TicketType = "billing"
	TypeTechnical      TicketType = "technical"
	TypeAccount        TicketType = "account"
	TypeFeatureRequest TicketType = "feature_request"
	TypeGeneral        TicketType = "general"
)

type Department string

const (
	DeptBilling      Department = "billing"
	DeptEngineering  Department = "engineering"
	DeptAccounts     Department = "accounts"
	DeptProduct      Department = "product"
	DeptSupportTier1 Department = "support_tier1"
)

type State string

const (
	StateNew                          State = "NEW"
	StateClassifying                  State = "CLASSIFYING"
	StateAwaitingClassificationReview State = "AWAITING_CLASSIFICATION_REVIEW"
	StateRouted                       State = "ROUTED"
	StateDrafting                     State = "DRAFTING"
	StateAwaitingReplyApproval        State = "AWAITING_REPLY_APPROVAL"
	StateResolved                     State = "RESOLVED"
	StateFailed                       State = "FAILED"
)

// DepartmentForType is the default routing map (a human may override at Checkpoint 1).
func DepartmentForType(t TicketType) Department {
	switch t {
	case TypeBilling:
		return DeptBilling
	case TypeTechnical:
		return DeptEngineering
	case TypeAccount:
		return DeptAccounts
	case TypeFeatureRequest:
		return DeptProduct
	default:
		return DeptSupportTier1
	}
}

func ValidUrgency(u Urgency) bool {
	switch u {
	case UrgencyLow, UrgencyNormal, UrgencyHigh, UrgencyCritical:
		return true
	}
	return false
}

func ValidType(t TicketType) bool {
	switch t {
	case TypeBilling, TypeTechnical, TypeAccount, TypeFeatureRequest, TypeGeneral:
		return true
	}
	return false
}

func ValidDepartment(d Department) bool {
	switch d {
	case DeptBilling, DeptEngineering, DeptAccounts, DeptProduct, DeptSupportTier1:
		return true
	}
	return false
}

// Email is the normalized inbound message produced by every ingestion adapter.
type Email struct {
	ID         uuid.UUID
	TicketID   uuid.UUID
	FromAddr   string
	ToAddr     string
	Subject    string
	Body       string
	Raw        map[string]any
	DedupeKey  string // IMAP message-id or hash(from+subject+body); enforces idempotency
	ReceivedAt time.Time
}

type Ticket struct {
	ID         uuid.UUID
	State      State
	Source     string // "http" | "imap"
	Urgency    Urgency
	Type       TicketType
	Department Department
	Confidence float64
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Classification is one classifier output (append-only history in DB).
type Classification struct {
	Urgency    Urgency
	Type       TicketType
	Department Department
	Confidence float64
	Reasoning  string
	ToolsUsed  map[string]any
	Model      string
}

// Classifier is the AI brain. Faked in Plan 1; Qwen/DashScope in Plan 2.
type Classifier interface {
	Classify(ctx context.Context, e Email) (Classification, error)
	DraftReply(ctx context.Context, t Ticket, e Email) (string, error)
}
