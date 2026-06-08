import type { AuditEntry } from "../types";

export function AuditTimeline({ entries }: { entries: AuditEntry[] }) {
  return (
    <ol className="space-y-2">
      {entries.map((e, i) => (
        <li key={i} className="flex items-start gap-2 text-sm">
          <span className="mt-1 h-2 w-2 shrink-0 rounded-full bg-gray-300" />
          <div>
            <span className="text-gray-700">
              {e.from_state || "(new)"} → <span className="font-medium">{e.to_state}</span>
            </span>
            <span className="ml-2 rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-500">{e.actor}</span>
            <div className="text-xs text-gray-400">{new Date(e.created_at).toLocaleString()}</div>
          </div>
        </li>
      ))}
    </ol>
  );
}
