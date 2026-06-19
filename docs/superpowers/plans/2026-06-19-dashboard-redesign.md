# Dashboard Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restyle the Autocierge reviewer dashboard into a polished dark/light dev-tool aesthetic (electric-blue/cyan accent) with a theme toggle, a header stats strip, a confidence meter, and a redesigned audit timeline — without changing flow, routes, API, or the state machine.

**Architecture:** A theme-aware design-token layer (CSS variables on `:root` / `[data-theme="dark"]`, mapped into Tailwind v4 via `@theme inline`) so components use semantic utilities (`bg-panel`, `text-ink`, `text-accent`, …) that auto-switch theme. A tiny `useTheme` hook + a no-flash inline script drive theming. Logic-heavy pieces (theme, stats, meter) are TDD'd; visual restyles are full-file rewrites guarded by render smoke tests plus the existing Checkpoint-2 behavior test.

**Tech Stack:** React 18 + TypeScript, Tailwind v4, Vite 6, vitest + @testing-library/react (jsdom), lucide-react, @fontsource (Inter + JetBrains Mono).

**Spec:** `docs/superpowers/specs/2026-06-19-dashboard-redesign-design.md`

**Working directory:** all paths are under `frontend/`. Run all commands from `frontend/`.

---

## File map

| File | Responsibility | Action |
|------|----------------|--------|
| `package.json` | add lucide-react + fontsource deps | modify |
| `index.html` | no-flash theme bootstrap script | modify |
| `src/index.css` | token system (light+dark), font imports, Tailwind theme mapping | rewrite |
| `src/theme.ts` | `useTheme` hook + storage key + helpers | create |
| `src/theme.test.ts` | tests for useTheme | create |
| `src/components/ThemeToggle.tsx` | sun/moon toggle button | create |
| `src/stats.ts` | `deriveStats(tickets)` pure logic | create |
| `src/stats.test.ts` | tests for deriveStats | create |
| `src/components/StatsStrip.tsx` | header stats strip UI | create |
| `src/components/ConfidenceMeter.tsx` | confidence bar | create |
| `src/components/ConfidenceMeter.test.tsx` | meter width test | create |
| `src/ui.tsx` | Badge restyle + icons | rewrite |
| `src/components/TicketQueue.tsx` | queue restyle | rewrite |
| `src/components/TicketDetail.tsx` | detail restyle (preserve CP2 fix) | rewrite |
| `src/components/AuditTimeline.tsx` | vertical timeline | rewrite |
| `src/App.tsx` | header shell (logo + stats + toggle) | modify |

> **Note on "Resolved today":** the spec listed a `Resolved today` stat, but `TicketSummary` carries only `created_at` (no resolution timestamp), and the spec forbids new API calls. To stay accurate and client-side-only, this plan ships a total **`Resolved`** count instead. This is a deliberate, minor deviation from the spec.

---

### Task 1: Add dependencies

**Files:**
- Modify: `frontend/package.json` (via npm)

- [ ] **Step 1: Install runtime + font deps**

Run (from `frontend/`):
```bash
npm install lucide-react@^0.460.0 @fontsource/inter@^5 @fontsource/jetbrains-mono@^5
```

- [ ] **Step 2: Verify install resolves and the build still passes**

Run:
```bash
npm run build
```
Expected: build completes, `✓ built in …`, no errors.

- [ ] **Step 3: Commit**

```bash
git add package.json package-lock.json
git commit -m "build(dashboard): add lucide-react + fontsource fonts"
```

---

### Task 2: Theme token system + fonts (index.css) + no-flash script (index.html)

**Files:**
- Rewrite: `frontend/src/index.css`
- Modify: `frontend/index.html`

CSS tokens are not unit-testable in jsdom; they are verified by `npm run build` here and by the manual visual pass in Task 10.

- [ ] **Step 1: Rewrite `src/index.css`**

```css
/* Font faces (self-hosted via @fontsource — works offline on the embedded deploy). */
@import "@fontsource/inter/400.css";
@import "@fontsource/inter/500.css";
@import "@fontsource/inter/600.css";
@import "@fontsource/inter/700.css";
@import "@fontsource/jetbrains-mono/400.css";
@import "@fontsource/jetbrains-mono/500.css";

@import "tailwindcss";

/* Make `dark:` utilities respond to our data-theme attribute (not OS media). */
@custom-variant dark (&:where([data-theme="dark"], [data-theme="dark"] *));

/* Light theme (default). */
:root {
  --canvas: #f7f8fa;
  --panel: #ffffff;
  --raised: #ffffff;
  --line: #e2e8f0;
  --ink: #0f172a;
  --muted: #475569;
  --faint: #94a3b8;
  --accent: #0ea5e9;
  --accent-hover: #0284c7;
  --critical: #dc2626;  --critical-soft: #fee2e2;
  --high: #ea580c;      --high-soft: #ffedd5;
  --normal: #2563eb;    --normal-soft: #dbeafe;
  --low: #64748b;       --low-soft: #f1f5f9;
  --resolved: #059669;  --resolved-soft: #d1fae5;
}

/* Dark theme. */
[data-theme="dark"] {
  --canvas: #0b0e14;
  --panel: #12161f;
  --raised: #161b26;
  --line: #222838;
  --ink: #e6e9ef;
  --muted: #9ba3b4;
  --faint: #6b7280;
  --accent: #38bdf8;
  --accent-hover: #0ea5e9;
  --critical: #f87171;  --critical-soft: #2a1416;
  --high: #fb923c;      --high-soft: #2a1e12;
  --normal: #60a5fa;    --normal-soft: #12203a;
  --low: #94a3b8;       --low-soft: #1a1f2b;
  --resolved: #34d399;  --resolved-soft: #0e2620;
}

/* Map raw variables into Tailwind tokens (inline => utilities reference the
   var, so they switch with data-theme at runtime). */
@theme inline {
  --color-canvas: var(--canvas);
  --color-panel: var(--panel);
  --color-raised: var(--raised);
  --color-line: var(--line);
  --color-ink: var(--ink);
  --color-muted: var(--muted);
  --color-faint: var(--faint);
  --color-accent: var(--accent);
  --color-accent-hover: var(--accent-hover);
  --color-critical: var(--critical);
  --color-critical-soft: var(--critical-soft);
  --color-high: var(--high);
  --color-high-soft: var(--high-soft);
  --color-normal: var(--normal);
  --color-normal-soft: var(--normal-soft);
  --color-low: var(--low);
  --color-low-soft: var(--low-soft);
  --color-resolved: var(--resolved);
  --color-resolved-soft: var(--resolved-soft);
  --font-sans: "Inter", ui-sans-serif, system-ui, sans-serif;
  --font-mono: "JetBrains Mono", ui-monospace, monospace;
}

html, body, #root { height: 100%; }
body {
  background-color: var(--canvas);
  color: var(--ink);
  font-family: var(--font-sans);
}
```

- [ ] **Step 2: Add the no-flash theme bootstrap to `index.html`**

Replace the `<head>` block in `frontend/index.html` with:
```html
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Autocierge</title>
    <script>
      // Set theme before first paint to avoid a flash of the wrong theme.
      (function () {
        try {
          var stored = localStorage.getItem("autocierge-theme");
          var theme = stored
            || (window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light");
          document.documentElement.setAttribute("data-theme", theme);
        } catch (e) {
          document.documentElement.setAttribute("data-theme", "light");
        }
      })();
    </script>
  </head>
```

- [ ] **Step 3: Verify build**

Run:
```bash
npm run build
```
Expected: build succeeds, no errors.

- [ ] **Step 4: Commit**

```bash
git add src/index.css index.html
git commit -m "feat(dashboard): theme-aware design tokens + no-flash bootstrap"
```

---

### Task 3: `useTheme` hook (TDD)

**Files:**
- Create: `frontend/src/theme.ts`
- Test: `frontend/src/theme.test.ts`

- [ ] **Step 1: Write the failing test**

`frontend/src/theme.test.ts`:
```ts
import { describe, it, expect, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useTheme, THEME_KEY } from "./theme";

beforeEach(() => {
  localStorage.clear();
  document.documentElement.removeAttribute("data-theme");
});

describe("useTheme", () => {
  it("uses a stored preference when present", () => {
    localStorage.setItem(THEME_KEY, "dark");
    const { result } = renderHook(() => useTheme());
    expect(result.current.theme).toBe("dark");
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
  });

  it("toggle flips the theme, sets data-theme, and persists", () => {
    localStorage.setItem(THEME_KEY, "light");
    const { result } = renderHook(() => useTheme());
    expect(result.current.theme).toBe("light");

    act(() => result.current.toggle());

    expect(result.current.theme).toBe("dark");
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
    expect(localStorage.getItem(THEME_KEY)).toBe("dark");
  });
});
```

- [ ] **Step 2: Run it to confirm it fails**

Run:
```bash
npx vitest run src/theme.test.ts
```
Expected: FAIL — `Failed to resolve import "./theme"`.

- [ ] **Step 3: Implement `src/theme.ts`**

```ts
import { useCallback, useEffect, useState } from "react";

export type Theme = "light" | "dark";
export const THEME_KEY = "autocierge-theme";

function initialTheme(): Theme {
  const attr = document.documentElement.getAttribute("data-theme");
  if (attr === "light" || attr === "dark") return attr;
  const stored = localStorage.getItem(THEME_KEY);
  if (stored === "light" || stored === "dark") return stored;
  return window.matchMedia?.("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

export function useTheme() {
  const [theme, setThemeState] = useState<Theme>(initialTheme);

  useEffect(() => {
    document.documentElement.setAttribute("data-theme", theme);
  }, [theme]);

  const setTheme = useCallback((t: Theme) => {
    localStorage.setItem(THEME_KEY, t);
    setThemeState(t);
  }, []);

  const toggle = useCallback(() => {
    setThemeState((prev) => {
      const next: Theme = prev === "dark" ? "light" : "dark";
      localStorage.setItem(THEME_KEY, next);
      return next;
    });
  }, []);

  return { theme, toggle, setTheme };
}
```

- [ ] **Step 4: Run it to confirm it passes**

Run:
```bash
npx vitest run src/theme.test.ts
```
Expected: PASS (2 tests).

- [ ] **Step 5: Commit**

```bash
git add src/theme.ts src/theme.test.ts
git commit -m "feat(dashboard): useTheme hook (follow-OS default, persisted override)"
```

---

### Task 4: `ThemeToggle` component

**Files:**
- Create: `frontend/src/components/ThemeToggle.tsx`

- [ ] **Step 1: Implement `ThemeToggle.tsx`**

```tsx
import { Moon, Sun } from "lucide-react";
import { useTheme } from "../theme";

export function ThemeToggle() {
  const { theme, toggle } = useTheme();
  const isDark = theme === "dark";
  return (
    <button
      onClick={toggle}
      aria-label={isDark ? "Switch to light theme" : "Switch to dark theme"}
      className="rounded-md border border-line bg-raised p-2 text-muted transition hover:text-ink hover:border-accent"
    >
      {isDark ? <Sun size={16} /> : <Moon size={16} />}
    </button>
  );
}
```

- [ ] **Step 2: Verify build**

Run:
```bash
npm run build
```
Expected: build succeeds.

- [ ] **Step 3: Commit**

```bash
git add src/components/ThemeToggle.tsx
git commit -m "feat(dashboard): ThemeToggle button"
```

---

### Task 5: `deriveStats` logic (TDD)

**Files:**
- Create: `frontend/src/stats.ts`
- Test: `frontend/src/stats.test.ts`

- [ ] **Step 1: Write the failing test**

`frontend/src/stats.test.ts`:
```ts
import { describe, it, expect } from "vitest";
import { deriveStats } from "./stats";
import type { TicketSummary } from "./types";

function t(state: string, urgency: string): TicketSummary {
  return {
    id: Math.random().toString(), state, urgency: urgency as TicketSummary["urgency"],
    type: "technical", department: "engineering", confidence: 0.9,
    subject: "s", from: "a@b.com", created_at: "2026-06-19T00:00:00Z",
  };
}

describe("deriveStats", () => {
  it("counts open / needs-review / awaiting-approval / resolved and the urgency mix", () => {
    const tickets = [
      t("AWAITING_CLASSIFICATION_REVIEW", "critical"),
      t("AWAITING_CLASSIFICATION_REVIEW", "high"),
      t("AWAITING_REPLY_APPROVAL", "normal"),
      t("ROUTED", "low"),
      t("RESOLVED", "normal"),
      t("FAILED", "high"),
    ];
    const s = deriveStats(tickets);
    expect(s.open).toBe(4);             // all except RESOLVED + FAILED
    expect(s.needsReview).toBe(2);
    expect(s.awaitingApproval).toBe(1);
    expect(s.resolved).toBe(1);
    expect(s.urgencyMix).toEqual({ critical: 1, high: 1, normal: 1, low: 1 });
  });

  it("handles an empty list", () => {
    const s = deriveStats([]);
    expect(s).toEqual({
      open: 0, needsReview: 0, awaitingApproval: 0, resolved: 0,
      urgencyMix: { critical: 0, high: 0, normal: 0, low: 0 },
    });
  });
});
```

- [ ] **Step 2: Run it to confirm it fails**

Run:
```bash
npx vitest run src/stats.test.ts
```
Expected: FAIL — `Failed to resolve import "./stats"`.

- [ ] **Step 3: Implement `src/stats.ts`**

```ts
import type { TicketSummary } from "./types";

export interface QueueStats {
  open: number;
  needsReview: number;
  awaitingApproval: number;
  resolved: number;
  urgencyMix: { critical: number; high: number; normal: number; low: number };
}

export function deriveStats(tickets: TicketSummary[]): QueueStats {
  const s: QueueStats = {
    open: 0, needsReview: 0, awaitingApproval: 0, resolved: 0,
    urgencyMix: { critical: 0, high: 0, normal: 0, low: 0 },
  };
  for (const t of tickets) {
    if (t.state !== "RESOLVED" && t.state !== "FAILED") s.open++;
    if (t.state === "AWAITING_CLASSIFICATION_REVIEW") s.needsReview++;
    if (t.state === "AWAITING_REPLY_APPROVAL") s.awaitingApproval++;
    if (t.state === "RESOLVED") s.resolved++;
    if (t.urgency in s.urgencyMix) s.urgencyMix[t.urgency as keyof QueueStats["urgencyMix"]]++;
  }
  return s;
}
```

- [ ] **Step 4: Run it to confirm it passes**

Run:
```bash
npx vitest run src/stats.test.ts
```
Expected: PASS (2 tests).

- [ ] **Step 5: Commit**

```bash
git add src/stats.ts src/stats.test.ts
git commit -m "feat(dashboard): deriveStats queue metrics"
```

---

### Task 6: `StatsStrip` component

**Files:**
- Create: `frontend/src/components/StatsStrip.tsx`

- [ ] **Step 1: Implement `StatsStrip.tsx`**

```tsx
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
```

- [ ] **Step 2: Verify build**

Run:
```bash
npm run build
```
Expected: build succeeds.

- [ ] **Step 3: Commit**

```bash
git add src/components/StatsStrip.tsx
git commit -m "feat(dashboard): StatsStrip header metrics"
```

---

### Task 7: `ConfidenceMeter` component (TDD)

**Files:**
- Create: `frontend/src/components/ConfidenceMeter.tsx`
- Test: `frontend/src/components/ConfidenceMeter.test.tsx`

- [ ] **Step 1: Write the failing test**

`frontend/src/components/ConfidenceMeter.test.tsx`:
```tsx
import { describe, it, expect } from "vitest";
import { render } from "@testing-library/react";
import { ConfidenceMeter } from "./ConfidenceMeter";

describe("ConfidenceMeter", () => {
  it("renders fill width and label from the confidence value", () => {
    const { container, getByText } = render(<ConfidenceMeter value={0.98} />);
    const fill = container.querySelector("[data-testid='meter-fill']") as HTMLElement;
    expect(fill.style.width).toBe("98%");
    expect(getByText("98%")).toBeTruthy();
  });

  it("clamps out-of-range values", () => {
    const { container } = render(<ConfidenceMeter value={1.5} />);
    const fill = container.querySelector("[data-testid='meter-fill']") as HTMLElement;
    expect(fill.style.width).toBe("100%");
  });
});
```

- [ ] **Step 2: Run it to confirm it fails**

Run:
```bash
npx vitest run src/components/ConfidenceMeter.test.tsx
```
Expected: FAIL — cannot resolve `./ConfidenceMeter`.

- [ ] **Step 3: Implement `ConfidenceMeter.tsx`**

```tsx
export function ConfidenceMeter({ value }: { value: number }) {
  const pct = Math.round(Math.max(0, Math.min(1, value)) * 100);
  return (
    <div className="flex items-center gap-2">
      <div className="h-1.5 w-32 overflow-hidden rounded-full bg-line">
        <div data-testid="meter-fill" className="h-full rounded-full bg-accent" style={{ width: `${pct}%` }} />
      </div>
      <span className="font-mono text-xs text-muted">{pct}%</span>
    </div>
  );
}
```

- [ ] **Step 4: Run it to confirm it passes**

Run:
```bash
npx vitest run src/components/ConfidenceMeter.test.tsx
```
Expected: PASS (2 tests).

- [ ] **Step 5: Commit**

```bash
git add src/components/ConfidenceMeter.tsx src/components/ConfidenceMeter.test.tsx
git commit -m "feat(dashboard): ConfidenceMeter component"
```

---

### Task 8: Restyle `Badge` + icons (ui.tsx)

**Files:**
- Rewrite: `frontend/src/ui.tsx`

- [ ] **Step 1: Rewrite `src/ui.tsx`**

```tsx
import { AlertTriangle, CheckCircle2, Circle, Clock, Inbox, Loader2, PenLine, XCircle } from "lucide-react";
import type { JSX } from "react";

const urgencyClass: Record<string, string> = {
  critical: "bg-critical-soft text-critical border-critical/30",
  high: "bg-high-soft text-high border-high/30",
  normal: "bg-normal-soft text-normal border-normal/30",
  low: "bg-low-soft text-low border-low/30",
  "": "bg-low-soft text-faint border-line",
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

const stateIcon: Record<string, JSX.Element> = {
  AWAITING_CLASSIFICATION_REVIEW: <AlertTriangle size={12} />,
  AWAITING_REPLY_APPROVAL: <PenLine size={12} />,
  RESOLVED: <CheckCircle2 size={12} />,
  ROUTED: <Circle size={12} />,
  CLASSIFYING: <Loader2 size={12} />,
  DRAFTING: <Loader2 size={12} />,
  NEW: <Inbox size={12} />,
  FAILED: <XCircle size={12} />,
};

export function Badge({ kind, value }: { kind: "urgency" | "state"; value: string }) {
  if (kind === "urgency") {
    return (
      <span className={`inline-flex items-center rounded-full border px-2 py-0.5 text-xs font-medium ${urgencyClass[value] ?? urgencyClass[""]}`}>
        {value || "—"}
      </span>
    );
  }
  const needsAction = value === "AWAITING_CLASSIFICATION_REVIEW" || value === "AWAITING_REPLY_APPROVAL";
  const resolved = value === "RESOLVED";
  const tone = needsAction
    ? "bg-accent/10 text-accent"
    : resolved
      ? "bg-resolved-soft text-resolved"
      : "bg-low-soft text-muted";
  return (
    <span className={`inline-flex items-center gap-1 rounded px-2 py-0.5 text-xs font-medium ${tone}`}>
      {stateIcon[value] ?? <Clock size={12} />}
      {stateLabel[value] ?? value}
    </span>
  );
}
```

- [ ] **Step 2: Run existing tests + build**

Run:
```bash
npx vitest run && npm run build
```
Expected: all tests pass; build succeeds.

- [ ] **Step 3: Commit**

```bash
git add src/ui.tsx
git commit -m "feat(dashboard): restyle Badge with tokens + icons"
```

---

### Task 9: Restyle `TicketQueue`

**Files:**
- Rewrite: `frontend/src/components/TicketQueue.tsx`

- [ ] **Step 1: Rewrite `TicketQueue.tsx`**

```tsx
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
```

- [ ] **Step 2: Run existing tests + build**

Run:
```bash
npx vitest run && npm run build
```
Expected: all pass; build succeeds.

- [ ] **Step 3: Commit**

```bash
git add src/components/TicketQueue.tsx
git commit -m "feat(dashboard): restyle TicketQueue rows"
```

---

### Task 10: Restyle `AuditTimeline` (vertical timeline)

**Files:**
- Rewrite: `frontend/src/components/AuditTimeline.tsx`

- [ ] **Step 1: Rewrite `AuditTimeline.tsx`**

```tsx
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
```

- [ ] **Step 2: Run existing tests + build**

Run:
```bash
npx vitest run && npm run build
```
Expected: all pass; build succeeds.

- [ ] **Step 3: Commit**

```bash
git add src/components/AuditTimeline.tsx
git commit -m "feat(dashboard): vertical audit timeline"
```

---

### Task 11: Restyle `TicketDetail` (preserve Checkpoint-2 fix + use ConfidenceMeter)

**Files:**
- Rewrite: `frontend/src/components/TicketDetail.tsx`

The existing `TicketDetail.test.tsx` (Checkpoint-2 draft render + edit-preservation) MUST stay green — the `useEffect` re-seed of `replyText` and the `value={replyText}` textarea are preserved exactly.

- [ ] **Step 1: Rewrite `TicketDetail.tsx`**

```tsx
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
                <span key={name} className="rounded bg-accent/10 px-1.5 py-0.5 font-mono text-accent">{name}</span>
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
            className="mt-3 rounded-md bg-accent px-4 py-2 font-medium text-white transition hover:bg-accent-hover disabled:opacity-50"
          >
            Confirm &amp; route
          </button>
        </section>
      )}

      {state === "AWAITING_REPLY_APPROVAL" && (
        <section className="rounded-lg border border-accent bg-accent/5 p-4 shadow-[0_0_0_1px_var(--accent)]">
          <h3 className="mb-2 font-semibold text-ink">Checkpoint 2 — Approve reply</h3>
          <textarea
            value={replyText}
            onChange={(e) => setReplyText(e.target.value)}
            rows={8}
            className="w-full rounded-md border border-line bg-canvas p-2 text-sm text-ink"
          />
          <div className="mt-3 flex gap-2">
            <button
              disabled={busy}
              onClick={wrap(() => onReplyApproval({ action: "approve", final_text: replyText }))}
              className="rounded-md bg-resolved px-4 py-2 font-medium text-white transition hover:opacity-90 disabled:opacity-50"
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
```

- [ ] **Step 2: Run existing tests (Checkpoint-2 must stay green) + build**

Run:
```bash
npx vitest run && npm run build
```
Expected: all tests pass (incl. the two `TicketDetail.test.tsx` cases); build succeeds.

- [ ] **Step 3: Commit**

```bash
git add src/components/TicketDetail.tsx
git commit -m "feat(dashboard): restyle TicketDetail + confidence meter + accent checkpoints"
```

---

### Task 12: App shell — header (logo + StatsStrip + ThemeToggle)

**Files:**
- Modify: `frontend/src/App.tsx`

- [ ] **Step 1: Update imports at the top of `App.tsx`**

Add below the existing imports:
```tsx
import { useMemo } from "react";
import { StatsStrip } from "./components/StatsStrip";
import { ThemeToggle } from "./components/ThemeToggle";
import { deriveStats } from "./stats";
```
(Adjust the existing `import { useCallback, useEffect, useState } from "react";` line to also include `useMemo`, or keep the separate import above — either compiles.)

- [ ] **Step 2: Compute stats inside the `App` component**

Immediately after the `const [error, setError] = useState<string | null>(null);` line, add:
```tsx
  const stats = useMemo(() => deriveStats(tickets), [tickets]);
```

- [ ] **Step 3: Replace the `<header>` element**

Replace the existing `<header>…</header>` block with:
```tsx
      <header className="flex items-center justify-between border-b border-line bg-panel px-6 py-3">
        <div className="flex items-center gap-6">
          <div className="flex items-center gap-2">
            <span className="grid h-6 w-6 place-items-center rounded-md bg-accent font-bold text-white">A</span>
            <h1 className="text-base font-semibold text-ink">Autocierge</h1>
          </div>
          <StatsStrip stats={stats} />
        </div>
        <div className="flex items-center gap-3">
          {error && <span className="text-sm text-critical">{error}</span>}
          <ThemeToggle />
        </div>
      </header>
```

- [ ] **Step 4: Update the two-pane container classes**

Replace:
```tsx
    <div className="flex h-screen flex-col bg-gray-50 text-gray-900">
```
with:
```tsx
    <div className="flex h-screen flex-col bg-canvas text-ink">
```
And replace the `<aside className="...">` and `<main className="...">` opening tags:
```tsx
        <aside className="w-96 shrink-0 overflow-y-auto border-r border-line bg-panel">
```
```tsx
        <main className="min-w-0 flex-1 overflow-y-auto p-6">
```
And the empty-state line:
```tsx
            <div className="grid h-full place-items-center text-faint">Select a ticket from the queue</div>
```

- [ ] **Step 5: Run full tests + build**

Run:
```bash
npx vitest run && npm run build
```
Expected: all tests pass; build succeeds.

- [ ] **Step 6: Commit**

```bash
git add src/App.tsx
git commit -m "feat(dashboard): header with logo, stats strip, theme toggle"
```

---

### Task 13: Final verification (build, tests, manual visual pass in both themes)

**Files:** none (verification only)

- [ ] **Step 1: Full test suite**

Run (from `frontend/`):
```bash
npm test
```
Expected: all test files pass (theme, stats, ConfidenceMeter, TicketDetail).

- [ ] **Step 2: Production build embeds the UI**

Run:
```bash
npm run build
```
Expected: `✓ built`, assets written to `../internal/webui/dist`.

- [ ] **Step 3: Manual visual pass**

From the repo root:
```bash
make dev
```
Open http://localhost:8080. Seed a few tickets (`./scripts/seed_demo.sh` in another terminal). Verify:
- Header shows logo, stats strip with live counts + urgency-mix bar, and a working theme toggle.
- Toggle flips light ↔ dark with no flash on reload; choice persists across reload.
- Queue rows show urgency edge bar + selected accent border.
- Detail shows the confidence meter, tools chips, and accent-bordered checkpoint panels.
- Approving a critical ticket at Checkpoint 1 renders the draft in Checkpoint 2 immediately (regression guard).
- Audit timeline renders as a vertical connected timeline.
- Stop the server (Ctrl-C).

- [ ] **Step 4: Go build sanity (embedded assets compile)**

From the repo root:
```bash
go build ./...
```
Expected: clean (confirms the embedded `internal/webui/dist` is valid).

---

## Self-review notes

- **Spec coverage:** tokens/dual-theme (T2), follow-OS default + persist + no-flash (T2/T3), toggle (T4), Inter+JetBrains Mono self-hosted (T1/T2), lucide icons (T7/T8), stats strip (T5/T6/T12), confidence meter (T7/T11), vertical timeline (T10), queue/detail/badge restyle (T8/T9/T11), header/logo (T12), preserve Checkpoint-2 fix (T11), tests for theme/stats/meter (T3/T5/T7), build/manual both themes (T13). "Resolved today" intentionally shipped as total "Resolved" (documented in the file map note).
- **Type consistency:** `QueueStats` shape and `deriveStats` signature match between T5 (definition), the test, and T6/T12 (consumers). `useTheme`/`THEME_KEY` consistent across T3/T4. Token utility names (`bg-panel`, `text-ink`, `text-muted`, `text-faint`, `border-line`, `text-accent`, `bg-accent`, `*-soft`) are defined in T2 and used unchanged thereafter.
- **No placeholders:** every code/test step contains complete code and exact commands.
