import { useEffect, useState } from "react";
import type { TicketDetail as Detail, AuditEntry } from "../types";
import { Badge } from "../ui";
import { AuditTimeline } from "./AuditTimeline";
import { ConfidenceMeter } from "./ConfidenceMeter";

const URGENCIES = ["low", "normal", "high", "critical"];
const TYPES = ["billing", "technical", "account", "feature_request", "general"];
const DEPTS = ["billing", "engineering", "accounts", "product", "support_tier1"];

export function TicketDetail({
  detail, audit, onReviewClassification, onReplyApproval,
}: {
  detail: Detail;
  audit: AuditEntry[];
  onReviewClassification: (d: { urgency: string; type: string; department: string }) => Promise<void>;
  onReplyApproval: (d: { action: "approve" | "reject"; final_text?: string }) => Promise<void>;
}) {
  const c = detail.classification;
  const [urgency, setUrgency] = useState<string>(c?.urgency ?? "normal");
  const [type, setType] = useState<string>(c?.type ?? "general");
  const [department, setDepartment] = useState<string>(c?.department ?? "support_tier1");
  const [replyText, setReplyText] = useState(detail.reply?.draft_text ?? "");
  const [busy, setBusy] = useState(false);

  // Preserved Checkpoint-2 fix: re-seed the textarea when the draft arrives on
  // a later refetch of the same ticket (no remount), without clobbering edits.
  const draftText = detail.reply?.draft_text ?? "";
  useEffect(() => { setReplyText(draftText); }, [draftText]);

  const state = detail.ticket.state;
  const wrap = (fn: () => Promise<void>) => async () => {
    setBusy(true);
    try { await fn(); } finally { setBusy(false); }
  };

  return (
    <div className="space-y-5">
      <div>
        <div className="flex items-center gap-2">
          <h2 className="text-xl font-semibold text-ink">{detail.email.subject || "(no subject)"}</h2>
          <Badge kind="state" value={state} />
        </div>
        <p className="text-sm text-muted">From {detail.email.from}</p>
      </div>

      <section className="rounded-lg border border-line bg-panel p-4">
        <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-faint">Email</h3>
        <p className="whitespace-pre-wrap text-ink">{detail.email.body}</p>
      </section>

      {c && (
        <section className="rounded-lg border border-line bg-panel p-4">
          <div className="mb-3 flex items-center justify-between">
            <h3 className="text-xs font-semibold uppercase tracking-wide text-faint">Qwen classification</h3>
            <span className="font-mono text-xs text-faint">{c.model}</span>
          </div>
          <div className="mb-3 flex flex-wrap items-center gap-2">
            <Badge kind="urgency" value={c.urgency} />
            <span className="rounded bg-low-soft px-2 py-0.5 text-xs text-muted">{c.type}</span>
            <span className="rounded bg-low-soft px-2 py-0.5 text-xs text-muted">→ {c.department}</span>
          </div>
          <ConfidenceMeter value={c.confidence} />
          <p className="mt-3 text-sm italic text-muted">"{c.reasoning}"</p>
          {c.tools_used && Object.keys(c.tools_used).length > 0 && (
            <div className="mt-2 flex flex-wrap gap-1.5 text-xs">
              <span className="text-faint">Tools:</span>
              {Object.keys(c.tools_used).map((name) => (
                <span key={name} className="rounded bg-accent/10 px-1.5 py-0.5 font-mono text-accent-text">{name}</span>
              ))}
            </div>
          )}
        </section>
      )}

      {state === "AWAITING_CLASSIFICATION_REVIEW" && (
        <section className="rounded-lg border border-accent bg-accent/5 p-4 shadow-[0_0_0_1px_var(--accent)]">
          <h3 className="mb-2 font-semibold text-ink">Checkpoint 1 — Validate routing</h3>
          <div className="flex flex-wrap gap-3">
            <Select label="Urgency" value={urgency} options={URGENCIES} onChange={setUrgency} />
            <Select label="Type" value={type} options={TYPES} onChange={setType} />
            <Select label="Department" value={department} options={DEPTS} onChange={setDepartment} />
          </div>
          <button
            disabled={busy}
            onClick={wrap(() => onReviewClassification({ urgency, type, department }))}
            className="mt-3 rounded-md bg-accent px-4 py-2 font-medium text-on-accent transition hover:bg-accent-hover disabled:opacity-50"
          >
            Confirm &amp; route
          </button>
        </section>
      )}

      {state === "AWAITING_REPLY_APPROVAL" && (
        <section className="rounded-lg border border-accent bg-accent/5 p-4 shadow-[0_0_0_1px_var(--accent)]">
          <h3 className="mb-2 font-semibold text-ink">Checkpoint 2 — Approve reply</h3>
          <textarea
            aria-label="Reply draft"
            value={replyText}
            onChange={(e) => setReplyText(e.target.value)}
            rows={8}
            className="w-full rounded-md border border-line bg-canvas p-2 text-sm text-ink"
          />
          <div className="mt-3 flex gap-2">
            <button
              disabled={busy}
              onClick={wrap(() => onReplyApproval({ action: "approve", final_text: replyText }))}
              className="rounded-md bg-resolved px-4 py-2 font-medium text-on-accent transition hover:opacity-90 disabled:opacity-50"
            >
              Approve &amp; send
            </button>
            <button
              disabled={busy}
              onClick={wrap(() => onReplyApproval({ action: "reject" }))}
              className="rounded-md border border-line bg-raised px-4 py-2 font-medium text-muted transition hover:text-ink disabled:opacity-50"
            >
              Reject &amp; redraft
            </button>
          </div>
        </section>
      )}

      {state === "RESOLVED" && detail.reply && (
        <section className="rounded-lg border border-resolved/30 bg-resolved-soft p-4">
          <h3 className="mb-2 font-semibold text-resolved">Resolved — sent reply</h3>
          <p className="whitespace-pre-wrap text-sm text-ink">{detail.reply.final_text || detail.reply.draft_text}</p>
        </section>
      )}

      <section className="rounded-lg border border-line bg-panel p-4">
        <h3 className="mb-3 text-xs font-semibold uppercase tracking-wide text-faint">Audit timeline</h3>
        <AuditTimeline entries={audit} />
      </section>
    </div>
  );
}

function Select({
  label, value, options, onChange,
}: { label: string; value: string; options: string[]; onChange: (v: string) => void }) {
  return (
    <label className="text-sm">
      <span className="mb-1 block text-xs font-medium text-muted">{label}</span>
      <select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="rounded-md border border-line bg-canvas px-2 py-1 text-ink"
      >
        {options.map((o) => <option key={o} value={o}>{o}</option>)}
      </select>
    </label>
  );
}
