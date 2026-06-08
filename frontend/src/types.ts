export type Urgency = "low" | "normal" | "high" | "critical";
export type TicketType = "billing" | "technical" | "account" | "feature_request" | "general";
export type Department = "billing" | "engineering" | "accounts" | "product" | "support_tier1";

export interface TicketSummary {
  id: string;
  state: string;
  urgency: Urgency | "";
  type: TicketType | "";
  department: Department | "";
  confidence: number;
  subject: string;
  from: string;
  created_at: string;
}

export interface Classification {
  urgency: Urgency;
  type: TicketType;
  department: Department;
  confidence: number;
  reasoning: string;
  model: string;
  tools_used: Record<string, unknown> | null;
  created_at: string;
}

export interface Reply {
  draft_text: string;
  final_text: string;
  status: string;
  created_at: string;
}

export interface TicketDetail {
  ticket: {
    id: string; state: string; urgency: Urgency | ""; type: TicketType | "";
    department: Department | ""; confidence: number; source: string;
    created_at: string; updated_at: string;
  };
  email: { from: string; to: string; subject: string; body: string; received_at: string };
  classification: Classification | null;
  reply: Reply | null;
}

export interface AuditEntry {
  from_state: string;
  to_state: string;
  actor: string;
  created_at: string;
}
