const urgencyColor: Record<string, string> = {
  critical: "bg-red-100 text-red-800 border-red-300",
  high: "bg-orange-100 text-orange-800 border-orange-300",
  normal: "bg-blue-100 text-blue-800 border-blue-300",
  low: "bg-gray-100 text-gray-700 border-gray-300",
  "": "bg-gray-100 text-gray-500 border-gray-300",
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

export function Badge({ kind, value }: { kind: "urgency" | "state"; value: string }) {
  if (kind === "urgency") {
    return (
      <span className={`inline-block rounded-full border px-2 py-0.5 text-xs font-medium ${urgencyColor[value] ?? urgencyColor[""]}`}>
        {value || "—"}
      </span>
    );
  }
  const needsAction = value === "AWAITING_CLASSIFICATION_REVIEW" || value === "AWAITING_REPLY_APPROVAL";
  return (
    <span className={`inline-block rounded px-2 py-0.5 text-xs font-medium ${needsAction ? "bg-amber-100 text-amber-800" : "bg-gray-100 text-gray-600"}`}>
      {stateLabel[value] ?? value}
    </span>
  );
}
