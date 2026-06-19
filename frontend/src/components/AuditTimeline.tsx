import type { AuditEntry } from "../types";

const dotColor: Record<string, string> = {
  RESOLVED: "bg-resolved",
  FAILED: "bg-critical",
  AWAITING_CLASSIFICATION_REVIEW: "bg-accent",
  AWAITING_REPLY_APPROVAL: "bg-accent",
};

export function AuditTimeline({ entries }: { entries: AuditEntry[] }) {
  return (
    <ol className="relative ml-1.5 border-l border-line">
      {entries.map((e, i) => (
        <li key={i} className="relative pl-5 pb-4 last:pb-0">
          <span className={`absolute -left-[5px] top-1 h-2.5 w-2.5 rounded-full ring-2 ring-panel ${dotColor[e.to_state] ?? "bg-faint"}`} />
          <div className="text-sm text-ink">
            {e.from_state || "(new)"} <span className="text-faint">→</span> <span className="font-medium">{e.to_state}</span>
            <span className="ml-2 rounded bg-low-soft px-1.5 py-0.5 font-mono text-xs text-muted">{e.actor}</span>
          </div>
          <div className="font-mono text-xs text-faint">{new Date(e.created_at).toLocaleString()}</div>
        </li>
      ))}
      {entries.length === 0 && <li className="pl-5 text-sm text-faint">No history yet</li>}
    </ol>
  );
}
