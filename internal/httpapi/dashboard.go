package httpapi

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/lemonishi/supportsentinel/internal/store"
)

func (h *handlers) listTickets(w http.ResponseWriter, r *http.Request) {
	items, err := h.s.ListTickets(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, t := range items {
		out = append(out, map[string]any{
			"id": t.ID.String(), "state": string(t.State), "urgency": string(t.Urgency),
			"type": string(t.Type), "department": string(t.Department), "confidence": t.Confidence,
			"subject": t.Subject, "from": t.FromAddr, "created_at": t.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handlers) ticketDetail(w http.ResponseWriter, r *http.Request) {
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
	email, err := h.s.GetEmailByTicket(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	detail := map[string]any{
		"ticket": map[string]any{
			"id": tk.ID.String(), "state": string(tk.State), "urgency": string(tk.Urgency),
			"type": string(tk.Type), "department": string(tk.Department), "confidence": tk.Confidence,
			"source": tk.Source, "created_at": tk.CreatedAt, "updated_at": tk.UpdatedAt,
		},
		"email": map[string]any{
			"from": email.FromAddr, "to": email.ToAddr, "subject": email.Subject,
			"body": email.Body, "received_at": email.ReceivedAt,
		},
		"classification": nil,
		"reply":          nil,
	}
	if cr, err := h.s.GetLatestClassification(r.Context(), id); err == nil {
		detail["classification"] = map[string]any{
			"urgency": string(cr.Urgency), "type": string(cr.Type), "department": string(cr.Department),
			"confidence": cr.Confidence, "reasoning": cr.Reasoning, "model": cr.Model,
			"tools_used": cr.ToolsUsed, "created_at": cr.CreatedAt,
		}
	} else if !errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if rr, err := h.s.GetLatestReply(r.Context(), id); err == nil {
		detail["reply"] = map[string]any{
			"draft_text": rr.DraftText, "final_text": rr.FinalText, "status": rr.Status, "created_at": rr.CreatedAt,
		}
	} else if !errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (h *handlers) ticketAudit(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	rows, err := h.s.AuditLog(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, a := range rows {
		out = append(out, map[string]any{
			"from_state": string(a.From), "to_state": string(a.To), "actor": a.Actor, "created_at": a.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, out)
}
