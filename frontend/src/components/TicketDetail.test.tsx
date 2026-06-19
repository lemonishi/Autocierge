import { describe, it, expect } from "vitest";
import { render, fireEvent } from "@testing-library/react";
import { TicketDetail } from "./TicketDetail";
import type { TicketDetail as Detail } from "../types";

// Build a detail for one ticket id in a given state, optionally with a draft.
function makeDetail(state: string, draftText: string | null): Detail {
  return {
    ticket: {
      id: "t-1", state, urgency: "critical", type: "technical",
      department: "engineering", confidence: 0.98, source: "http",
      created_at: "2026-06-18T00:00:00Z", updated_at: "2026-06-18T00:00:00Z",
    },
    email: { from: "vip@acme.com", to: "support@acme.com", subject: "prod down", body: "everything 500s", received_at: "2026-06-18T00:00:00Z" },
    classification: {
      urgency: "critical", type: "technical", department: "engineering",
      confidence: 0.98, reasoning: "outage", model: "qwen3-max",
      tools_used: null, created_at: "2026-06-18T00:00:00Z",
    },
    reply: draftText === null ? null : { draft_text: draftText, final_text: "", status: "draft", created_at: "2026-06-18T00:00:00Z" },
  };
}

const noop = async () => {};

describe("TicketDetail Checkpoint 2 draft rendering", () => {
  it("shows the drafted reply when a parked ticket transitions to AWAITING_REPLY_APPROVAL without remounting", () => {
    // Same component instance (same ticket id), mirroring App.tsx's key={ticket.id}:
    // first parked at Checkpoint 1 with no draft, then the draft arrives after routing.
    const { rerender, container } = render(
      <TicketDetail detail={makeDetail("AWAITING_CLASSIFICATION_REVIEW", null)} audit={[]} onReviewClassification={noop} onReplyApproval={noop} />,
    );
    expect(container.querySelector("textarea")).toBeNull(); // Checkpoint 2 not shown yet

    rerender(
      <TicketDetail detail={makeDetail("AWAITING_REPLY_APPROVAL", "Hi — we're on the outage now.")} audit={[]} onReviewClassification={noop} onReplyApproval={noop} />,
    );

    const textarea = container.querySelector("textarea") as HTMLTextAreaElement;
    expect(textarea).not.toBeNull();
    expect(textarea.value).toBe("Hi — we're on the outage now.");
  });

  it("does not clobber an in-progress edit when an unrelated refetch returns the same draft", () => {
    const drafted = makeDetail("AWAITING_REPLY_APPROVAL", "Original draft.");
    const { rerender, container } = render(
      <TicketDetail detail={drafted} audit={[]} onReviewClassification={noop} onReplyApproval={noop} />,
    );
    const textarea = container.querySelector("textarea") as HTMLTextAreaElement;

    // Reviewer edits the draft.
    fireEvent.change(textarea, { target: { value: "Reviewer's edited reply." } });
    expect(textarea.value).toBe("Reviewer's edited reply.");

    // A later refetch returns a new object but the SAME draft_text (e.g. queue poll).
    rerender(
      <TicketDetail detail={makeDetail("AWAITING_REPLY_APPROVAL", "Original draft.")} audit={[]} onReviewClassification={noop} onReplyApproval={noop} />,
    );
    expect(textarea.value).toBe("Reviewer's edited reply."); // edit preserved
  });
});
