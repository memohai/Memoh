# Atomic Design System (otakugard Redesign)

**Core**: High-density, industrial-static, extremely flat. Built on shadcn/ui + Tailwind CSS. Focuses on governance-grade configuration and "Box-in-Box" bento architecture.

## 0. Foundations (Unified Standards)

### Typography Hierarchy
- **L1 (Main Title)**: `text-sm font-semibold` (Page headers, high-level context).
- **L2 (Section Title)**: `text-xs font-medium` (Bento card headers, group labels).
- **L3 (Primary Content)**: `text-sm font-normal` (Standard text, list items).
- **L4 (Helper/Description)**: `text-[11px] leading-snug` (Contextual help, sub-labels).
- **L5 (Micro/Nano)**: `text-[10px]` (Status labels) / `text-[9px]` (Technical metadata, trust indicators).
- **Data (Technical)**: `font-mono` (IDs, tool names, parameters).
- **Case Policy**: Strictly avoid all-caps. Use standard sentence/title case.

### Border Standards
- **Structural**: `1px solid border-border` (Outer container boundaries).
- **Subtle**: `border-border/50` (Sticky header lines, internal dividers).
- **High-Density**: `border-border/40` (Bento unit borders, list item separators).
- **Focus**: `ring-2 ring-ring ring-offset-2`.

### Radius (Rounded) Standards
- **Large (Layout)**: `rounded-lg` (Main navigation rails, detail workspace containers).
- **Medium (Components)**: `rounded-md` (Bento cards, action buttons, inputs).
- **Small (Elements)**: `rounded` (Inner list items, micro-badges, tags).
- **Circular (Functional)**: `rounded-full` (Avatars, status dots, indicator badges).

---

## 1. The 5 Absolute Laws
1. **Flat Atom**: Zero shadows for surface elements. Hierarchy relies on `border-border` and background contrast (`bg-muted/20` clusters vs `bg-background` units).
2. **Monochrome Hover**: Standard hover uses `bg-accent/40` or `bg-accent/60`. No scaling/transforms on navigation/list items to maintain a "static industrial" feel.
3. **Primary Scarcity**: `bg-primary` is strictly reserved for confirmation actions. Primary action buttons default to `bg-foreground`.
4. **Bimodal Elevation**: Zero shadow for canvas; `shadow-md/lg` ONLY for floating Z-index layers (popovers, toasts, modals).
5. **Progressive Disclosure**: Hide complex policy rules or advanced technical configurations within compact containers or togglable sections.

## 2. Design Tokens
- **Background**: `bg-background` (Pure surface)
- **Foreground**: `bg-foreground` / `text-foreground` (Primary text & active actions)
- **Muted Surface**: `bg-muted/20` or `bg-muted/25` (Outer Bento containers)
- **Accent/Selection**: `bg-accent/40` or `bg-accent/60` (Selection/Hover highlights)
- **Destructive**: `text-destructive` (Alerts & danger identifiers).

## 3. Layout Architectures

### A. Single-Column (Configuration Bento)
- **Container**: `max-w-2xl mx-auto pb-6 space-y-4` or `space-y-5`.
- **Sovereign Header**: `pb-4 border-b border-border/50 sticky top-0 bg-background/95 backdrop-blur z-30 pt-4 -mt-4`.
- **Bento Stat Unit**: `rounded-md border p-3 flex flex-col justify-between min-h-[110px] bg-background/70`.

### B. Two-Column (Master-Detail System)
- **Global Wrapper**: `max-w-4xl mx-auto flex gap-6 absolute inset-0 py-6 px-4`.
- **Master Rail (L3)**: `w-60 shrink-0 flex flex-col border border-border rounded-lg bg-background overflow-hidden`.
- **Detail Workspace (L4)**: `flex-1 min-w-0 flex flex-col border border-border rounded-lg bg-background overflow-hidden`.
- **Rail Item**: `px-3 py-1.5 text-xs rounded-md transition-colors`.

## 4. Component Specifics
- **Action Button**: `h-8 text-xs font-medium px-4 shadow-none`.
- **Micro Button**: `h-5 px-2 text-[9px]` or `h-7 px-3 text-[10px]`.
- **Standard Input**: `h-8 text-xs bg-transparent border-border rounded-md shadow-none`.
- **Search Input**: `pl-8 h-8 text-xs bg-transparent shadow-none`.
- **Bento Unit (Inner)**: `rounded-md border border-border/60 bg-background p-3 shadow-none hover:border-border`.
- **Switch**: `scale-90` or `scale-75` (Micro-toggles).
- **Micro-Badge**: `h-5 text-[10px] px-1.5 shadow-none rounded-full`.

## 5. Interaction & Indicators
- **Dirty State**: Asterisk `*` appended to labels or a `size-1.5 rounded-full bg-muted-foreground` dot.
- **Status Dot**: `size-2 rounded-full` (Indicator for connectivity/health).
- **Risk Indicator**: `text-destructive` without red background for destructive actions; isolated via `pt-4`.
- **Progressive Action**: Popover-based menus (`PopoverContent class="w-48 p-2"`) for secondary trusts/actions.
