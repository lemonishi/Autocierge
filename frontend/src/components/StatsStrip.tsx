import type { QueueStats } from "../stats";

const URGENCY_COLOR: Record<string, string> = {
  critical: "bg-critical", high: "bg-high", normal: "bg-normal", low: "bg-low",
};

function Stat({ label, value }: { label: string; value: number }) {
  return (
    <div className="flex items-baseline gap-1.5">
      <span className="font-mono text-sm font-semibold text-ink">{value}</span>
      <span className="text-xs text-muted">{label}</span>
    </div>
  );
}

export function StatsStrip({ stats }: { stats: QueueStats }) {
  const mix = stats.urgencyMix;
  const total = mix.critical + mix.high + mix.normal + mix.low;
  return (
    <div className="flex items-center gap-5">
      <Stat label="open" value={stats.open} />
      <Stat label="need review" value={stats.needsReview} />
      <Stat label="awaiting reply" value={stats.awaitingApproval} />
      <Stat label="resolved" value={stats.resolved} />
      {total > 0 && (
        <div className="flex h-1.5 w-28 overflow-hidden rounded-full bg-line" title="Urgency mix">
          {(["critical", "high", "normal", "low"] as const).map((k) =>
            mix[k] > 0 ? (
              <div key={k} className={URGENCY_COLOR[k]} style={{ width: `${(mix[k] / total) * 100}%` }} />
            ) : null,
          )}
        </div>
      )}
    </div>
  );
}
