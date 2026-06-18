import { useEffect, useState } from "react";
import type { TicketDetail as Detail, AuditEntry } from "../types";
import { Badge } from "../ui";
import { AuditTimeline } from "./AuditTimeline";

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

  // The draft is generated server-side and arrives on a later refetch of the
  // SAME ticket (Checkpoint 1 → 2), so the useState initializer above misses it
  // (no remount). Re-seed the textarea whenever the draft text changes; the
  // dependency means in-progress edits aren't clobbered by unchanged refetches.
  const draftText = detail.reply?.draft_text ?? "";
  useEffect(() => { setReplyText(draftText); }, [draftText]);

  const state = detail.ticket.state;
  const wrap = (fn: () => Promise<void>) => async () => {
    setBusy(true);
    try { await fn(); } finally { setBusy(false); }
  };

  return (
    <div className="space-y-6">
      <div>
        <div className="flex items-center gap-2">
          <h2 className="text-xl font-semibold text-gray-900">{detail.email.subject || "(no subject)"}</h2>
          <Badge kind="state" value={state} />
        </div>
        <p className="text-sm text-gray-500">From {detail.email.from}</p>
      </div>

      <section className="rounded-lg border border-gray-200 bg-white p-4">
        <h3 className="mb-2 text-sm font-semibold uppercase tracking-wide text-gray-500">Email</h3>
        <p className="whitespace-pre-wrap text-gray-800">{detail.email.body}</p>
      </section>

      {c && (
        <section className="rounded-lg border border-gray-200 bg-white p-4">
          <div className="mb-2 flex items-center justify-between">
            <h3 className="text-sm font-semibold uppercase tracking-wide text-gray-500">Qwen classification</h3>
            <span className="text-xs text-gray-400">
              {c.model} · confidence {(c.confidence * 100).toFixed(0)}%
            </span>
          </div>
          <div className="mb-2 flex flex-wrap gap-2">
            <Badge kind="urgency" value={c.urgency} />
            <span className="rounded bg-gray-100 px-2 py-0.5 text-xs text-gray-700">{c.type}</span>
            <span className="rounded bg-gray-100 px-2 py-0.5 text-xs text-gray-700">→ {c.department}</span>
          </div>
          <p className="text-sm italic text-gray-600">"{c.reasoning}"</p>
          {c.tools_used && Object.keys(c.tools_used).length > 0 && (
            <div className="mt-2 text-xs text-gray-500">
              <span className="font-medium">Tools used:</span> {Object.keys(c.tools_used).join(", ")}
            </div>
          )}
        </section>
      )}

      {/* Checkpoint 1 */}
      {state === "AWAITING_CLASSIFICATION_REVIEW" && (
        <section className="rounded-lg border-2 border-amber-300 bg-amber-50 p-4">
          <h3 className="mb-2 font-semibold text-amber-900">Checkpoint 1 — Validate routing</h3>
          <div className="flex flex-wrap gap-3">
            <Select label="Urgency" value={urgency} options={URGENCIES} onChange={setUrgency} />
            <Select label="Type" value={type} options={TYPES} onChange={setType} />
            <Select label="Department" value={department} options={DEPTS} onChange={setDepartment} />
          </div>
          <button
            disabled={busy}
            onClick={wrap(() => onReviewClassification({ urgency, type, department }))}
            className="mt-3 rounded bg-amber-600 px-4 py-2 font-medium text-white hover:bg-amber-700 disabled:opacity-50"
          >
            Confirm &amp; route
          </button>
        </section>
      )}

      {/* Checkpoint 2 */}
      {state === "AWAITING_REPLY_APPROVAL" && (
        <section className="rounded-lg border-2 border-amber-300 bg-amber-50 p-4">
          <h3 className="mb-2 font-semibold text-amber-900">Checkpoint 2 — Approve reply</h3>
          <textarea
            value={replyText}
            onChange={(e) => setReplyText(e.target.value)}
            rows={8}
            className="w-full rounded border border-gray-300 p-2 text-sm"
          />
          <div className="mt-3 flex gap-2">
            <button
              disabled={busy}
              onClick={wrap(() => onReplyApproval({ action: "approve", final_text: replyText }))}
              className="rounded bg-green-600 px-4 py-2 font-medium text-white hover:bg-green-700 disabled:opacity-50"
            >
              Approve &amp; send
            </button>
            <button
              disabled={busy}
              onClick={wrap(() => onReplyApproval({ action: "reject" }))}
              className="rounded bg-gray-200 px-4 py-2 font-medium text-gray-700 hover:bg-gray-300 disabled:opacity-50"
            >
              Reject &amp; redraft
            </button>
          </div>
        </section>
      )}

      {state === "RESOLVED" && detail.reply && (
        <section className="rounded-lg border border-green-200 bg-green-50 p-4">
          <h3 className="mb-2 font-semibold text-green-900">Resolved — sent reply</h3>
          <p className="whitespace-pre-wrap text-sm text-gray-800">{detail.reply.final_text || detail.reply.draft_text}</p>
        </section>
      )}

      <section className="rounded-lg border border-gray-200 bg-white p-4">
        <h3 className="mb-2 text-sm font-semibold uppercase tracking-wide text-gray-500">Audit timeline</h3>
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
      <span className="mb-1 block text-xs font-medium text-gray-600">{label}</span>
      <select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="rounded border border-gray-300 px-2 py-1"
      >
        {options.map((o) => <option key={o} value={o}>{o}</option>)}
      </select>
    </label>
  );
}
