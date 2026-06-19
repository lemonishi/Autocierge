import { describe, it, expect } from "vitest";
import { deriveStats } from "./stats";
import type { TicketSummary } from "./types";

function t(state: string, urgency: string): TicketSummary {
  return {
    id: Math.random().toString(), state, urgency: urgency as TicketSummary["urgency"],
    type: "technical", department: "engineering", confidence: 0.9,
    subject: "s", from: "a@b.com", created_at: "2026-06-19T00:00:00Z",
  };
}

describe("deriveStats", () => {
  it("counts open / needs-review / awaiting-approval / resolved and the urgency mix", () => {
    const tickets = [
      t("AWAITING_CLASSIFICATION_REVIEW", "critical"),
      t("AWAITING_CLASSIFICATION_REVIEW", "high"),
      t("AWAITING_REPLY_APPROVAL", "normal"),
      t("ROUTED", "low"),
      t("RESOLVED", "normal"),
      t("FAILED", "high"),
    ];
    const s = deriveStats(tickets);
    expect(s.open).toBe(4);             // all except RESOLVED + FAILED
    expect(s.needsReview).toBe(2);
    expect(s.awaitingApproval).toBe(1);
    expect(s.resolved).toBe(1);
    expect(s.urgencyMix).toEqual({ critical: 1, high: 1, normal: 1, low: 1 });
  });

  it("handles an empty list", () => {
    const s = deriveStats([]);
    expect(s).toEqual({
      open: 0, needsReview: 0, awaitingApproval: 0, resolved: 0,
      urgencyMix: { critical: 0, high: 0, normal: 0, low: 0 },
    });
  });
});
