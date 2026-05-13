# Memoh Design System

**Core**: High-contrast, extremely flat, 1px crisp boundaries. Built on shadcn/ui + Tailwind CSS.

## 1. The 5 Absolute Laws
1. **Flat Atom**: Base elements (buttons, inputs) have `shadow-none`. Hierarchy relies strictly on `border-border` and background contrast.
2. **Monochrome Hover**: Standard hover uses `bg-accent text-foreground` (light gray/near-black).
3. **Purple Scarcity**: Brand purple (`bg-primary`) is STRICTLY reserved for core conversion actions (e.g., "Send") or tiny active indicators. General buttons use `bg-foreground`.
4. **Bimodal Elevation**: Zero shadow for canvas elements; strong shadows (`shadow-md`, `shadow-lg`) ONLY for floating/Z-index layers (toasts, modals, dropdowns, dragged items).
5. **a11y First**: Interactive elements MUST have `focus-visible:ring-2` and proper `aria-*` attributes.

## 2. Design Tokens (Never hardcode colors)
- **Background**: `bg-background` (Light: Pure white `#ffffff` | Dark: Near black `#09090b`)
- **Foreground**: `bg-foreground` / `text-foreground` (Light: Near black `#09090b` | Dark: Near white `#fafafa`. Primary actions & text)
- **Primary**: `bg-primary` (Purple `#7c3aed`. Extreme scarcity!) / `bg-primary-foreground` (Pure white `#ffffff`)
- **Muted/Accent**: `bg-muted` / `bg-accent` (Light: Gray `#f1f5f9` | Dark: Gray `#27272a`. Hover state, secondary panels)
- **Muted Text**: `text-muted-foreground` (Light: Slate `#64748b` | Dark: Zinc `#a1a1aa`. Secondary text, inactive icons)
- **Border**: `border-border` (Light: Slate `#e2e8f0` | Dark: Zinc `#27272a`. 1px structural dividers)
- **Destructive**: `text-destructive` / `border-destructive` (Pure red `#ef4444`. Text/icons only, no red backgrounds)
- **Ring**: `ring-ring` (Light: `#09090b` | Dark: `#d4d4d8`. Focus outline rings)

## 3. Typography
- **Fonts**: `font-sans` (Text) / `font-mono` (Code).
- **Weights**: `font-medium` (Headers, Buttons), `font-normal` (Body text).
- **Sizes**: Base `text-sm` (14px). Secondary `text-xs` (12px). 
- **CRITICAL**: All Inputs MUST use `text-base` (16px) to prevent iOS auto-zoom.

## 4. Interaction States (Mandatory)
- **Focus**: `focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2`
- **Active**: `active:scale-95`
- **Disabled**: `disabled:pointer-events-none disabled:opacity-50 aria-disabled="true"`
- **Error**: `border-destructive text-destructive aria-invalid="true" aria-errormessage="..."`
- **Selected**: `data-[state=checked]:bg-foreground aria-checked="true"`
- **Loading**: Use Spinner/Skeleton, add `aria-busy="true"`, disable interactions.

## 5. Component Specifics
- **Button**: Primary (`bg-foreground`), Secondary (`border-border`), Ghost. **No base shadow**.
- **Input**: `shadow-none border-border rounded-md text-base`.
- **Switch**: `shadow-none` container. Track: `h-6 w-11`, Thumb: `h-5 w-5`.
- **Table**: `border-border rounded-sm shadow-none`. Dragged rows dynamically add `shadow-lg z-50 bg-background`.
- **Toast**: `rounded-[10px] shadow-md`. Title `text-sm`, Desc `text-xs text-muted-foreground`.
