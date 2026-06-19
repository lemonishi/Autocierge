import { AlertTriangle, CheckCircle2, Circle, Clock, Inbox, Loader2, PenLine, XCircle } from "lucide-react";
import type { JSX } from "react";

const urgencyClass: Record<string, string> = {
  critical: "bg-critical-soft text-critical border-critical/30",
  high: "bg-high-soft text-high border-high/30",
  normal: "bg-normal-soft text-normal border-normal/30",
  low: "bg-low-soft text-low border-low/30",
  "": "bg-low-soft text-faint border-line",
};

const stateLabel: Record<string, string> = {
  AWAITING_CLASSIFICATION_REVIEW: "Needs review",
  AWAITING_REPLY_APPROVAL: "Approve reply",
  RESOLVED: "Resolved",
  ROUTED: "Routed",
  CLASSIFYING: "Classifying",
  DRAFTING: "Drafting",
  NEW: "New",
  FAILED: "Failed",
};

const stateIcon: Record<string, JSX.Element> = {
  AWAITING_CLASSIFICATION_REVIEW: <AlertTriangle aria-hidden size={12} />,
  AWAITING_REPLY_APPROVAL: <PenLine aria-hidden size={12} />,
  RESOLVED: <CheckCircle2 aria-hidden size={12} />,
  ROUTED: <Circle aria-hidden size={12} />,
  CLASSIFYING: <Loader2 aria-hidden size={12} />,
  DRAFTING: <Loader2 aria-hidden size={12} />,
  NEW: <Inbox aria-hidden size={12} />,
  FAILED: <XCircle aria-hidden size={12} />,
};

export function Badge({ kind, value }: { kind: "urgency" | "state"; value: string }) {
  if (kind === "urgency") {
    return (
      <span className={`inline-flex items-center rounded-full border px-2 py-0.5 text-xs font-medium ${urgencyClass[value] ?? urgencyClass[""]}`}>
        {value || "—"}
      </span>
    );
  }
  const needsAction = value === "AWAITING_CLASSIFICATION_REVIEW" || value === "AWAITING_REPLY_APPROVAL";
  const resolved = value === "RESOLVED";
  const tone = needsAction
    ? "bg-accent/10 text-accent-text"
    : resolved
      ? "bg-resolved-soft text-resolved"
      : "bg-low-soft text-muted";
  return (
    <span className={`inline-flex items-center gap-1 rounded px-2 py-0.5 text-xs font-medium ${tone}`}>
      {stateIcon[value] ?? <Clock size={12} />}
      {stateLabel[value] ?? value}
    </span>
  );
}
