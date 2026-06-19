import type { TicketSummary } from "../types";
import { Badge } from "../ui";

const urgencyBar: Record<string, string> = {
  critical: "bg-critical", high: "bg-high", normal: "bg-normal", low: "bg-low", "": "bg-line",
};

export function TicketQueue({
  tickets, selectedId, onSelect,
}: {
  tickets: TicketSummary[];
  selectedId: string | null;
  onSelect: (id: string) => void;
}) {
  return (
    <div className="flex flex-col">
      {tickets.map((t) => {
        const active = selectedId === t.id;
        return (
          <button
            key={t.id}
            onClick={() => onSelect(t.id)}
            className={`relative w-full border-b border-line px-4 py-3 text-left transition
              ${active ? "bg-raised" : "hover:bg-raised/60"}`}
          >
            <span className={`absolute inset-y-0 left-0 w-0.5 ${active ? "bg-accent" : urgencyBar[t.urgency] ?? "bg-line"}`} />
            <div className="flex items-center justify-between gap-2">
              <span className="truncate font-medium text-ink">{t.subject || "(no subject)"}</span>
              <Badge kind="urgency" value={t.urgency} />
            </div>
            <div className="mt-1 flex items-center justify-between gap-2">
              <span className="truncate text-sm text-muted">{t.from}</span>
              <Badge kind="state" value={t.state} />
            </div>
          </button>
        );
      })}
      {tickets.length === 0 && <div className="p-6 text-center text-faint">No tickets yet</div>}
    </div>
  );
}
