import type { TicketSummary } from "./types";

export interface QueueStats {
  open: number;
  needsReview: number;
  awaitingApproval: number;
  resolved: number;
  urgencyMix: { critical: number; high: number; normal: number; low: number };
}

export function deriveStats(tickets: TicketSummary[]): QueueStats {
  const s: QueueStats = {
    open: 0, needsReview: 0, awaitingApproval: 0, resolved: 0,
    urgencyMix: { critical: 0, high: 0, normal: 0, low: 0 },
  };
  for (const t of tickets) {
    const open = t.state !== "RESOLVED" && t.state !== "FAILED";
    if (open) s.open++;
    if (t.state === "AWAITING_CLASSIFICATION_REVIEW") s.needsReview++;
    if (t.state === "AWAITING_REPLY_APPROVAL") s.awaitingApproval++;
    if (t.state === "RESOLVED") s.resolved++;
    // Urgency mix describes the OPEN queue only (excludes RESOLVED/FAILED).
    if (open && t.urgency in s.urgencyMix) s.urgencyMix[t.urgency as keyof QueueStats["urgencyMix"]]++;
  }
  return s;
}
