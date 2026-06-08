import type { TicketSummary } from "../types";
import { Badge } from "../ui";

export function TicketQueue({
  tickets, selectedId, onSelect,
}: {
  tickets: TicketSummary[];
  selectedId: string | null;
  onSelect: (id: string) => void;
}) {
  return (
    <div className="divide-y divide-gray-100">
      {tickets.map((t) => (
        <button
          key={t.id}
          onClick={() => onSelect(t.id)}
          className={`w-full text-left px-4 py-3 hover:bg-gray-50 ${selectedId === t.id ? "bg-blue-50" : ""}`}
        >
          <div className="flex items-center justify-between gap-2">
            <span className="truncate font-medium text-gray-900">{t.subject || "(no subject)"}</span>
            <Badge kind="urgency" value={t.urgency} />
          </div>
          <div className="mt-1 flex items-center justify-between gap-2">
            <span className="truncate text-sm text-gray-500">{t.from}</span>
            <Badge kind="state" value={t.state} />
          </div>
        </button>
      ))}
      {tickets.length === 0 && <div className="p-6 text-center text-gray-400">No tickets yet</div>}
    </div>
  );
}
