import type { TicketSummary, TicketDetail, AuditEntry } from "./types";

async function get<T>(path: string): Promise<T> {
  const r = await fetch(path);
  if (!r.ok) throw new Error(`${path} → ${r.status}`);
  return r.json() as Promise<T>;
}

async function post<T>(path: string, body: unknown): Promise<T> {
  const r = await fetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!r.ok) throw new Error(`${path} → ${r.status}`);
  return r.json() as Promise<T>;
}

export const api = {
  listTickets: () => get<TicketSummary[]>("/api/tickets"),
  ticketDetail: (id: string) => get<TicketDetail>(`/api/tickets/${id}/detail`),
  ticketAudit: (id: string) => get<AuditEntry[]>(`/api/tickets/${id}/audit`),
  submitEmail: (e: { from: string; subject: string; body: string }) =>
    post("/api/emails", e),
  reviewClassification: (id: string, d: { urgency: string; type: string; department: string; reviewer: string }) =>
    post(`/api/tickets/${id}/classification-review`, d),
  replyApproval: (id: string, d: { action: "approve" | "reject"; final_text?: string; reviewer: string }) =>
    post(`/api/tickets/${id}/reply-approval`, d),
};
