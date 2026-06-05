// Package httpapi exposes the ingestion endpoint and the two human-in-the-loop
// checkpoint actions. The React dashboard (Plan 4) renders on top of these.
package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/lemonishi/supportsentinel/internal/domain"
	"github.com/lemonishi/supportsentinel/internal/orchestrator"
	"github.com/lemonishi/supportsentinel/internal/store"
)

func NewServer(o *orchestrator.Orchestrator, s *store.Store) http.Handler {
	mux := http.NewServeMux()
	h := &handlers{o: o, s: s}
	mux.HandleFunc("POST /api/emails", h.submitEmail)
	mux.HandleFunc("GET /api/tickets/{id}", h.getTicket)
	mux.HandleFunc("POST /api/tickets/{id}/classification-review", h.reviewClassification)
	mux.HandleFunc("POST /api/tickets/{id}/reply-approval", h.replyApproval)
	return mux
}

type handlers struct {
	o *orchestrator.Orchestrator
	s *store.Store
}

func writeTicket(w http.ResponseWriter, t domain.Ticket) {
	writeJSON(w, http.StatusOK, map[string]any{
		"id": t.ID.String(), "state": string(t.State), "urgency": string(t.Urgency),
		"type": string(t.Type), "department": string(t.Department), "confidence": t.Confidence,
	})
}

func writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, orchestrator.ErrInvalidReview), errors.Is(err, orchestrator.ErrEmptyReply):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.Is(err, store.ErrStateConflict):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, store.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *handlers) submitEmail(w http.ResponseWriter, r *http.Request) {
	var in struct {
		From    string `json:"from"`
		To      string `json:"to"`
		Subject string `json:"subject"`
		Body    string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if in.From == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "from is required"})
		return
	}
	email := domain.Email{
		FromAddr: in.From, ToAddr: in.To, Subject: in.Subject, Body: in.Body,
		DedupeKey: hashKey(in.From, in.Subject, in.Body),
	}
	tk, err := h.o.Ingest(r.Context(), "http", email)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeTicket(w, tk)
}

func (h *handlers) getTicket(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	tk, err := h.s.GetTicket(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeTicket(w, tk)
}

func (h *handlers) reviewClassification(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	var in struct {
		Urgency    string `json:"urgency"`
		Type       string `json:"type"`
		Department string `json:"department"`
		Reviewer   string `json:"reviewer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	err = h.o.ReviewClassification(r.Context(), id, orchestrator.ReviewDecision{
		Urgency: domain.Urgency(in.Urgency), Type: domain.TicketType(in.Type),
		Department: domain.Department(in.Department),
	}, fallback(in.Reviewer, "anon"))
	if err != nil {
		writeErr(w, err)
		return
	}
	tk, err := h.s.GetTicket(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeTicket(w, tk)
}

func (h *handlers) replyApproval(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	var in struct {
		Action    string `json:"action"`
		FinalText string `json:"final_text"`
		Reviewer  string `json:"reviewer"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	rev := fallback(in.Reviewer, "anon")
	switch in.Action {
	case "approve":
		err = h.o.ApproveReply(r.Context(), id, in.FinalText, rev)
	case "reject":
		err = h.o.RejectReply(r.Context(), id, rev)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "action must be approve|reject"})
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	tk, err := h.s.GetTicket(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeTicket(w, tk)
}

func fallback(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
