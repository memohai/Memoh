# Memoh Settings & Dashboards Design System

This document extends `Design-new.md` specifically for complex data panels, settings forms, and dashboard overview pages. It strictly inherits the core laws (Flat Atom, Monochrome Hover, Bimodal Elevation) while establishing rules for high-density information architecture.

## 1. The 5 Setting & Dashboard Laws

1. **Extreme Typography Restraint**: 
   - Never use large, bold headers (`text-lg` or `text-sm font-semibold`) for internal card sections.
   - Standard section headers MUST be `text-xs font-medium text-foreground`.
   - Helper/Description texts MUST be `text-[11px] text-muted-foreground`, with minimal line height (avoid `leading-relaxed` unless it's a long paragraph).
   - Only reserve `text-2xl font-semibold` for **Core KPIs/Metrics** (e.g., total count, percentage).

2. **The "Box-in-Box" Bento Architecture**:
   - Group related settings or metrics into a unified outer container (`rounded-md border p-4 space-y-4`).
   - Internal sections should either use strict grid layouts (`grid gap-3 sm:grid-cols-3`) with muted backgrounds (`bg-background/70` or `bg-muted/20`) or seamless stacking. 
   - DO NOT use aggressive full-width separator lines (`divide-y` or `<Separator>`) inside cards if spacing (`space-y-4` or `pt-4`) can do the job. Use "Floating Ticks" (short vertical borders) for grid division if needed.

3. **Progressive Disclosure**:
   - Hide complex diagnostics or advanced settings inside `Collapsible` components.
   - **Collapsible Sizing**: Triggers must be compact (`px-3 py-2 text-xs hover:bg-accent/40`), avoiding overly wide padding (`p-4`) which wastes vertical space.
   - **Smart Defaults**: Auto-expand items requiring immediate attention (e.g., Errors); auto-collapse and visually suppress (e.g., `opacity-60`) items that are stable or healthy.

4. **Global Action Anchor**:
   - Page-level primary actions (e.g., "Save Settings", "Refresh") MUST NOT be hidden at the bottom of a scrolling page.
   - Anchor them to the Top Action Bar (Header) of the page (`flex items-start justify-between pb-4 border-b border-border/50`).

5. **Stoic Danger Zones**:
   - Destructive actions (Delete, Purge) do NOT get red backgrounds or thick red borders.
   - Use standard `border-border` and isolate them with whitespace (e.g., `pt-4` or `mt-8`).
   - Only the title text (`text-destructive`) and the final confirmation button should indicate the destructive nature.

## 2. Layout & Spacing Defaults

- **Page Container**: `max-w-2xl mx-auto pb-6 space-y-4` (Prevent ultrawide stretching).
- **Card Spacing**: `space-y-4` between major outer cards.
- **Inner Form Spacing**: `space-y-3` between form elements, `space-y-1.5` between Label and Input, `space-y-0.5` between Title and Description.
- **Inputs & Selects**: Use `h-8 text-xs bg-transparent shadow-none`.

## 3. Micro-Interactions (Static Feel)

- **No Scaling**: Remove `active:scale-95` on standard list items or accordions to maintain an industrial, static feel.
- **Skeleton over Spinner**: For initial page loads, use block Skeletons (`animate-pulse bg-muted/10 rounded-md border`) instead of simple Spinners to prevent layout jumping.
- **Semantic Feedback**: Rely on localized toasts (e.g., `toast.info('Status updated')`) rather than obtrusive inline success banners.
