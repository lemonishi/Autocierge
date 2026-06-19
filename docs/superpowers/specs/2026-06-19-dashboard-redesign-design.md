# Autocierge Dashboard Redesign — Design Spec

**Date:** 2026-06-19
**Status:** Approved (brainstorming) — pending implementation plan.

## Goal

Make the reviewer dashboard look like a polished, serious triage console instead of
a generic Tailwind-default ("vibecoded") UI — primarily to make the demo impressive —
**without changing the two-pane flow, routes, API, or the orchestrator/state machine.**
Purely presentational, plus a few high-signal informational visuals.

## Aesthetic direction

**Dark dev-tool** (Linear / Raycast / Vercel-dark family): low-chrome canvas, one vibrant
accent, mono numerics, clear hierarchy, subtle depth. **Accent: electric blue / cyan.**
Ships with **both light and dark themes** and a user toggle.

## Scope

In scope: a dark+light design-token system applied to all existing components, a header
stats strip, a confidence meter, a redesigned vertical audit timeline, iconography, and a
theme toggle.

Out of scope (explicitly): structural features (search/filter, collapsible panels, keyboard
nav, command palette), any backend/API/state-machine change, new pages or routes.

## Architecture / implementation approach

**Theme-aware design tokens via CSS variables + Tailwind v4 `@theme`.**

- Semantic tokens are defined as CSS custom properties: `--color-canvas`, `--color-panel`,
  `--color-raised`, `--color-border`, `--color-text`, `--color-text-muted`,
  `--color-text-faint`, `--color-accent`, `--color-accent-hover`, plus status pairs
  (`--color-critical`, `--color-high`, `--color-normal`, `--color-low`, `--color-resolved`)
  and their subtle background variants.
- **Light values on `:root`; dark values under `[data-theme="dark"]`** on the `<html>`
  element. These are mapped into Tailwind's theme via `@theme inline` so components use
  semantic utility classes (e.g. `bg-panel`, `text-muted`, `border-border`, `text-accent`).
- Components reference **semantic tokens only** — no component hardcodes a hex or a
  Tailwind palette color. Switching themes is a single attribute flip on `<html>`; tweaking
  a palette is editing one variable.

Rejected alternative: hardcoding `bg-slate-900` etc. across components — faster to start but
scatters the palette and reproduces the maintainability problem we're fixing. Rejected:
adding a component library (shadcn/MUI) — overkill and churn for a restyle.

## Design tokens

### Palettes

**Dark** (`[data-theme="dark"]`):
- canvas `#0B0E14`, panel `#12161F`, raised `#161B26`, border `#222838`
- text `#E6E9EF`, muted `#9BA3B4`, faint `#6B7280`
- accent `#38BDF8`, accent-hover `#0EA5E9` (with a soft accent glow on focus/action elements)

**Light** (`:root`):
- canvas `#F7F8FA`, panel `#FFFFFF`, raised `#FFFFFF`, border `#E2E8F0`
- text `#0F172A`, muted `#475569`, faint `#94A3B8`
- accent `#0EA5E9`, accent-hover `#0284C7` (soft shadows instead of glow)

**Status colors** (reserved — never used as the accent; tuned per theme):
- critical = red, high = amber/orange, normal = muted blue, low = neutral gray,
  resolved = emerald. Each has a foreground + subtle background variant for chips.

### Typography
- **Inter** for UI text, **JetBrains Mono** for numerics, IDs, timestamps, confidence.
- Bundled via `@fontsource/inter` and `@fontsource/jetbrains-mono` (npm, self-hosted) —
  **not** a CDN — so fonts work offline on the embedded `//go:embed` / ECS deploy.

### Depth, radius, spacing
- 1px hairline borders + subtle shadow (light) / faint accent glow (dark); `rounded-lg`
  cards; consistent 4/8px spacing rhythm.

### Icons
- `lucide-react` (tree-shakeable). Leading icons on states, urgency, tools-used, actions,
  and the theme toggle (sun/moon).

## Theme toggle behavior

- Control: a sun/moon icon button in the header.
- **Default: follow the OS** via `prefers-color-scheme`.
- An explicit user choice **persists to `localStorage`** and overrides the OS default.
- The `data-theme` attribute is set on `<html>` **before first paint** (a tiny inline script
  in `index.html`) to avoid a flash of the wrong theme.
- Implemented as a small `useTheme` hook; no new dependency beyond the lucide icon.

## Component changes

- **App shell / header** (`src/App.tsx`): dark/light top bar with a CSS/SVG **logo mark +
  "Autocierge" wordmark** (no image asset), the **stats strip** (below), and the **theme
  toggle**. Two-pane layout unchanged.
- **Queue** (`src/components/TicketQueue.tsx`): denser rows; urgency as a colored **left-edge
  bar + dot**; selected row gets an accent left-border + raised surface + (dark) glow;
  hover state; mono timestamp.
- **Detail** (`src/components/TicketDetail.tsx`): sectioned cards; classification card shows
  the **confidence meter**, tools-used as icon chips, and model in mono; the two **checkpoint**
  panels get an **accent border + subtle glow** (so "this needs you" reads instantly) instead
  of the current amber box. The Checkpoint-2 reply textarea behavior (the merged fix) is
  preserved.
- **Badges** (`src/ui.tsx`): restyled for both themes via tokens, each with a small leading
  icon per state/urgency.

## New high-signal visuals

- **Stats strip** (header): counts for `Open · Needs review · Awaiting approval ·
  Resolved today` plus a small **urgency-mix bar**. Computed **client-side** from the
  already-fetched tickets list — no new API call.
- **Confidence meter:** a horizontal bar with accent fill and the `%` in mono, replacing the
  plain "confidence 98%" text.
- **Audit timeline** (`src/components/AuditTimeline.tsx`): a real vertical timeline — connected
  dots color-coded by state, mono timestamps, actor chips.

## Testing

- The existing **vitest** suite (incl. the Checkpoint-2 draft-render regression test) must stay
  green after the restyle.
- Add light render/smoke tests:
  - **theme toggle**: flipping sets `data-theme` on `<html>` and persists to `localStorage`;
    follows OS when no stored preference.
  - **stats strip**: counts are derived correctly from a sample tickets list.
  - **confidence meter**: rendered fill width reflects the confidence value.
- Manual visual pass in both themes via `make run`; `npm run build` stays clean (test files
  remain excluded from the production `tsc -b`).

## Locked decisions

- Dark dev-tool aesthetic; **electric blue/cyan** accent.
- **Both** light + dark themes; **default follows OS**, explicit choice persisted.
- New dependencies limited to `lucide-react`, `@fontsource/inter`, `@fontsource/jetbrains-mono`.
  No component library.
- Flow, routes, API, and orchestrator/state machine unchanged.
