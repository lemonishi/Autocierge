import { useCallback, useEffect, useState } from "react";
import { api } from "./api";
import type { AuditEntry, TicketDetail as Detail, TicketSummary } from "./types";
import { TicketQueue } from "./components/TicketQueue";
import { TicketDetail } from "./components/TicketDetail";

const REVIEWER = "demo-agent";

export default function App() {
  const [tickets, setTickets] = useState<TicketSummary[]>([]);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [detail, setDetail] = useState<Detail | null>(null);
  const [audit, setAudit] = useState<AuditEntry[]>([]);
  const [error, setError] = useState<string | null>(null);

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
    <div className="flex h-screen flex-col bg-gray-50 text-gray-900">
      <header className="flex items-center justify-between border-b border-gray-200 bg-white px-6 py-3">
        <h1 className="text-lg font-bold">Autocierge <span className="text-gray-400">reviewer console</span></h1>
        {error && <span className="text-sm text-red-600">{error}</span>}
      </header>
      <div className="flex min-h-0 flex-1">
        <aside className="w-96 shrink-0 overflow-y-auto border-r border-gray-200 bg-white">
          <TicketQueue tickets={tickets} selectedId={selectedId} onSelect={setSelectedId} />
        </aside>
        <main className="min-w-0 flex-1 overflow-y-auto p-6">
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
            <div className="grid h-full place-items-center text-gray-400">Select a ticket from the queue</div>
          )}
        </main>
      </div>
    </div>
  );
}
