import { useCallback, useEffect, useMemo, useState } from "react";
import { api } from "./api";
import type { AuditEntry, TicketDetail as Detail, TicketSummary } from "./types";
import { TicketQueue } from "./components/TicketQueue";
import { TicketDetail } from "./components/TicketDetail";
import { StatsStrip } from "./components/StatsStrip";
import { ThemeToggle } from "./components/ThemeToggle";
import { deriveStats } from "./stats";

const REVIEWER = "demo-agent";

export default function App() {
  const [tickets, setTickets] = useState<TicketSummary[]>([]);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [detail, setDetail] = useState<Detail | null>(null);
  const [audit, setAudit] = useState<AuditEntry[]>([]);
  const [error, setError] = useState<string | null>(null);
  const stats = useMemo(() => deriveStats(tickets), [tickets]);

  const refreshQueue = useCallback(async () => {
    try { setTickets(await api.listTickets()); } catch (e) { setError(String(e)); }
  }, []);

  const refreshDetail = useCallback(async (id: string) => {
    try {
      const [d, a] = await Promise.all([api.ticketDetail(id), api.ticketAudit(id)]);
      setDetail(d); setAudit(a);
    } catch (e) { setError(String(e)); }
  }, []);

  useEffect(() => { refreshQueue(); }, [refreshQueue]);
  useEffect(() => {
    const t = setInterval(refreshQueue, 4000); // keep the queue fresh during the demo
    return () => clearInterval(t);
  }, [refreshQueue]);
  useEffect(() => { if (selectedId) refreshDetail(selectedId); }, [selectedId, refreshDetail]);

  const afterAction = async () => {
    await refreshQueue();
    if (selectedId) await refreshDetail(selectedId);
  };

  return (
    <div className="flex h-screen flex-col bg-canvas text-ink">
      <header className="flex items-center justify-between border-b border-line bg-panel px-6 py-3">
        <div className="flex items-center gap-6">
          <div className="flex items-center gap-2">
            <span className="grid h-6 w-6 place-items-center rounded-md bg-accent font-bold text-on-accent">A</span>
            <h1 className="text-base font-semibold text-ink">Autocierge</h1>
          </div>
          <StatsStrip stats={stats} />
        </div>
        <div className="flex items-center gap-3">
          {error && <span role="status" aria-live="polite" className="text-sm text-critical">{error}</span>}
          <ThemeToggle />
        </div>
      </header>
      <div className="flex min-h-0 flex-1">
        <aside aria-label="Ticket queue" className="w-96 shrink-0 overflow-y-auto border-r border-line bg-panel">
          <TicketQueue tickets={tickets} selectedId={selectedId} onSelect={setSelectedId} />
        </aside>
        <main aria-label="Ticket detail" className="min-w-0 flex-1 overflow-y-auto p-6">
          {detail ? (
            <TicketDetail
              key={detail.ticket.id}
              detail={detail}
              audit={audit}
              onReviewClassification={async (d) => {
                await api.reviewClassification(detail.ticket.id, { ...d, reviewer: REVIEWER });
                await afterAction();
              }}
              onReplyApproval={async (d) => {
                await api.replyApproval(detail.ticket.id, { ...d, reviewer: REVIEWER });
                await afterAction();
              }}
            />
          ) : (
            <div className="grid h-full place-items-center text-faint">Select a ticket from the queue</div>
          )}
        </main>
      </div>
    </div>
  );
}
