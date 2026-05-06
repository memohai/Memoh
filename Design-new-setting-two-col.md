# Memoh Settings & Dashboards: Two-Column Master-Detail System

This document extends `Design-new-setting.md` and applies the intelligence of the `ui-ux-pro-max` UX guidelines specifically to **Two-Column Settings Layouts** (e.g., a left-side navigation rail with a right-side dynamic configuration panel). It strictly inherits the core setting laws while resolving the tension between high-density information architecture and the requirement for "one-page context perception."

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

## Part 2: High-Density Two-Column Architecture

When combining a left navigation rail (Master) with a detailed form panel (Detail), the interface must maintain the illusion of a single, coherent "Bento Box" rather than two disconnected floating columns.

### 1. Container Constraint & Physical Fusion
- **Strict `max-w-4xl`:** The top-level wrapper MUST be strictly constrained to `max-w-4xl mx-auto` (approx. 896px). 
  - *Reasoning:* A standard single-column settings page is `max-w-2xl` (672px). When adding a `w-60` (240px) left rail and a `gap-6` (24px) gutter, a `max-w-4xl` container ensures the right-side form remains at an ideal reading measure (~632px), perfectly mirroring the single-column experience without unrestricted stretching.
- **Parent-Level Wrapping:** Use `absolute inset-0 py-6 px-4 w-full` to claim the viewport height within the tab pane, but enforce the `max-w-4xl mx-auto` to center the dual-pane UI gracefully.

### 2. Left Rail: The Silent Hub (L3)
The left sidebar in a settings context is a navigation hub, not a primary action area. It must remain visually "silent" to reduce cognitive load.
- **Extreme Noise Reduction:** DO NOT use aggressive solid backgrounds (e.g., solid `bg-accent` or primary brand colors) to indicate the active tab. Use a very subtle highlight (`bg-accent/40`), combined with `font-medium text-foreground`. Inactive items remain `text-muted-foreground`.
- **High-Density Spacing (`py-1.5`):** Compress vertical padding on list items to `py-1.5 px-3` (instead of standard `py-2.5`). This allows maximum items to fit above the fold.
- **Dirty State Tracking (The Asterisk):** If a child form contains unsaved changes, display a subtle indicator (like a bold, orange asterisk `*`) next to the entity's name in the rail, ensuring users don't lose track of modified states when switching tabs.

### 3. Right Workspace: Sovereign Context (L4 Header)
The right panel must act as a sovereign workspace. It must establish its context and ownership of the "Save" action immediately at the top (extending Law 4).
- **Header Structure:** Define the exact boundary using `pb-4 border-b border-border/50 sticky top-0 bg-background/95 backdrop-blur z-10`. 
- **Contextual Ghost Micro-Copy:** When the form is "dirty", smoothly fade in a micro-copy badge (`text-[11px] text-muted-foreground px-2 py-0.5 bg-muted/40 rounded`) immediately to the left of the Save button. 
  - *UX Routing Rule:* If the user navigates away to a different tab while changes remain unsaved, update the ghost copy to explicitly state the origin of the dirty state (e.g., "Unsaved changes in Telegram" with a clickable link to return).

### 4. Right Workspace: Dynamic Form Body (L4 Body)
Follow the "Box-in-Box" philosophy (Law 2) but adapt it for dynamically generated schemas.
- **The `grid-cols-2` Default:** Default dynamic forms to `md:grid-cols-2 gap-4`. Short inputs sit side-by-side; wide inputs (URLs, long tokens) dynamically span `md:col-span-2`.
- **Enforcing Progressive Disclosure (Law 3):** All non-required (Optional) fields MUST be hidden inside a `Collapsible` container.
  - The Collapsible Header must feature a clear title ("Advanced Settings") and compact action triggers (`h-7 px-2 text-xs`) for "Expand All" and "Collapse".
  - If a platform lacks optional fields, the section must still render to maintain layout parity, but in a disabled/ghosted state (e.g., "No advanced options available").
- **Read-Only Data:** For generated endpoints (like Webhook URLs), eschew standard inputs for solid blocks (`bg-muted p-2 rounded-md font-mono text-[11px]`) to establish visual friction against editability.

### 5. Right Workspace: The Stoic Danger Zone
Extending Law 5, destructive actions in a two-column setup are even more prone to accidental clicks due to the compressed scrolling area.
- **Absolute Isolation:** Place at the absolute bottom of the document flow, separated by an aggressive whitespace barrier (`pt-4` minimum padding top).
- **Structural Friction:** The destructive action MUST be wrapped in a `<ConfirmPopover>` to ensure a two-step confirmation process.

## Summary: The UI/UX Checkpoints
1. [ ] Is the global wrapper strictly `max-w-4xl`?
2. [ ] Are list items in the left rail dense (`py-1.5`) and visually muted?
3. [ ] Are un-saved tab states indicated in the rail and the top action bar?
4. [ ] Is the primary Save button anchored to the Top Header, bordered tightly at the bottom?
5. [ ] Are optional fields hidden by default inside a Collapsible box?
6. [ ] Is the Danger Zone stripped of red backgrounds and isolated at the bottom?