# Memoh Settings & Dashboards: Two-Column Master-Detail System

This document extends `Design-new-setting.md` and applies the intelligence of the `ui-ux-pro-max` UX guidelines specifically to **Two-Column Settings Layouts** (e.g., a left-side navigation rail with a right-side dynamic configuration panel). It strictly inherits the core setting laws while establishing absolute dimensional constraints to maintain layout parity across all configuration modules.

## Part 1: The 5 Core Setting & Dashboard Laws (Inherited)

Before applying any two-column specific rules, the foundational laws from `Design-new-setting.md` MUST be obeyed:

1. **Extreme Typography Restraint**: 
   - Never use large, bold headers (`text-lg` or `text-sm font-semibold`) for internal card sections.
   - Standard section headers MUST be `text-xs font-medium text-foreground`.
   - Helper/Description texts MUST be `text-[11px] text-muted-foreground`, with minimal line height (avoid `leading-relaxed` unless it's a long paragraph).
   - Only reserve `text-2xl font-semibold` for Core KPIs/Metrics.

2. **The "Box-in-Box" Bento Architecture**:
   - Group related settings or metrics into a unified outer container (`rounded-md border p-4 space-y-4`).
   - DO NOT use aggressive full-width separator lines (`divide-y` or `<Separator>`) inside cards if spacing (`space-y-4` or `pt-4`) can do the job.

3. **Progressive Disclosure**:
   - Hide complex diagnostics or advanced settings inside `Collapsible` components.
   - **Collapsible Sizing**: Triggers must be compact (`px-3 py-2 text-xs hover:bg-accent/40`), avoiding overly wide padding (`p-4`).

4. **Global Action Anchor**:
   - Page-level primary actions (e.g., "Save Settings", "Refresh") MUST NOT be hidden at the bottom of a scrolling page.
   - Anchor them to the Top Action Bar (Header) of the page (`flex items-start justify-between pb-4 border-b border-border/50`).

5. **Stoic Danger Zones**:
   - Destructive actions (Delete, Purge) do NOT get red backgrounds or thick red borders.
   - Use standard `border-border` and isolate them with whitespace (e.g., `pt-4`).
   - Only the title text (`text-destructive`) and the final confirmation button should indicate the destructive nature.

---

## Part 2: High-Density Two-Column Architecture (Strict Dimensions)

When combining a left navigation rail (Master) with a detailed form panel (Detail), the interface must maintain the illusion of a single, coherent "Bento Box" across all contexts. To achieve this, **strict dimensional parity is mandated**.

### 1. Absolute Grid Constraint & Physical Fusion
- **Global Wrapper (`max-w-4xl`):** The top-level wrapper MUST be strictly constrained to `max-w-4xl mx-auto` (approx. 896px). Use `absolute inset-0 py-6 px-4 w-full` to claim the viewport height within the tab pane, but enforce the `max-w-4xl` to center the dual-pane UI gracefully.
- **The Golden Ratio Split:** 
  - **Left Rail (Master):** Must be strictly fixed to `w-60` (240px). Do not use `w-64` or flexible percentages.
  - **Gutter:** Must be exactly `gap-6` (24px).
  - **Right Workspace (Detail):** Must use `flex-1 min-w-0`. This leaves the right-side form at an ideal reading measure of approximately 632px, perfectly mirroring the standard single-column settings experience.

### 2. Left Rail: The Master Navigation (L3)
The left sidebar is a navigation hub. It must remain visually "silent" to reduce cognitive load.
- **Extreme Noise Reduction:** DO NOT use aggressive solid backgrounds to indicate the active tab. Use a very subtle highlight (`bg-accent/40`), combined with `font-medium text-foreground`. Inactive items remain `text-muted-foreground`.
- **High-Density Spacing (`py-1.5`):** Compress vertical padding on list items to `px-3 py-1.5` (instead of standard `py-2.5`). This allows maximum items to fit above the fold.
- **Dirty State Tracking (The Asterisk):** If a child form contains unsaved changes, display a subtle indicator (like a bold, orange or standard `text-foreground` asterisk `*`) next to the entity's name in the rail, ensuring users don't lose track of modified states when switching tabs.

### 3. Right Workspace: Sovereign Context (L4 Header)
The right panel must act as a sovereign workspace.
- **Header Structure:** Define the exact boundary using `pb-4 border-b border-border/50 sticky top-0 bg-background/95 backdrop-blur z-10`. 
- **Contextual Ghost Micro-Copy:** When the form is "dirty", smoothly fade in a micro-copy badge (`text-[11px] text-muted-foreground px-2 py-0.5 bg-muted/40 rounded`) immediately to the left of the Save button to explicitly state the origin of the dirty state.

### 4. Right Workspace: Dynamic Form Body (L4 Body)
Follow the "Box-in-Box" philosophy (Law 2) adapted for dynamic panels.
- **The `grid-cols-2` Default:** Default dynamic forms to `md:grid-cols-2 gap-4`. Short inputs sit side-by-side; wide inputs dynamically span `md:col-span-2`.
- **Enforcing Progressive Disclosure (Law 3):** All non-required (Optional) fields MUST be hidden inside a `Collapsible` container.
  - The Collapsible Header must feature a clear title ("Advanced Settings") and compact action triggers (`h-7 px-2 text-xs`) for "Expand All" and "Collapse".
- **Read-Only Data:** For system-generated values (e.g., IDs, URLs), eschew standard inputs for solid blocks (`bg-muted p-2 rounded-md font-mono text-[11px]`) to establish visual friction against editability.

### 5. Right Workspace: The Stoic Danger Zone
Extending Law 5, destructive actions in a two-column setup are highly prone to accidental clicks due to the compressed scrolling area.
- **Absolute Isolation:** Place at the absolute bottom of the document flow, separated by an aggressive whitespace barrier (`pt-4` minimum padding top).
- **Structural Friction:** The destructive action MUST be wrapped in a `<ConfirmPopover>` to ensure a two-step confirmation process.

## Summary: The UI/UX Dimensional Checkpoints
1. [ ] Is the global wrapper strictly `max-w-4xl`?
2. [ ] Is the left rail exactly `w-60`?
3. [ ] Is the gap between rail and workspace exactly `gap-6`?
4. [ ] Are list items in the left rail dense (`py-1.5`) and visually muted?
5. [ ] Is the primary Save button anchored to the Top Header, bordered tightly at the bottom?
6. [ ] Is the Danger Zone stripped of red backgrounds and isolated at the bottom?
