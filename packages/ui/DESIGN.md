# Design System

## Overview

A modern, clean, and highly functional interface designed for the Memoh AI Agent platform. The design emphasizes a "high-contrast, flat" aesthetic. It moves away from mushy, ubiquitous soft shadows (`shadow-sm`) and brand-colored primary buttons, favoring a starker monochrome foundation with pure whites, light greys, and pure blacks. Brand colors are used extremely sparingly for distinct active states and primary conversion actions (like the chat "Send" button).

## Colors

- **Primary** (`#8b56e3`): Brand purple. Its usage is heavily restricted to specific active indicators (sidebar selection, "Send" action, status dots). It is **not** used for generic primary buttons.
- **Primary Foreground** (`#fafafa`): Used heavily as the background for sidebars (Navigation, Session List, Metadata) to separate them from the main content area.
- **Background** (`#ffffff`): Pure white, used for the central chat area, cards, alerts, and major input fields.
- **Foreground** (`#171717` or `#0a0a0a`): Near black. Now acts as the default visual weight for **Primary Buttons** and **Default Badges**.
- **Muted Foreground** (`#737373`): Mid-grey, extensively used for timestamps, secondary labels, placeholders, and inactive icons.
- **Accent** (`#f5f5f5`): Light grey, used for hover states, secondary buttons, code/badge backgrounds, and model selectors.
- **Border** (`#e5e5e5`): Very light grey, used uniformly for all structural dividers and component borders.
- **Destructive** (`#dc2626`): Pure red. Used exclusively for text or icons in critical actions (e.g., delete). It does not use colored backgrounds (e.g., no `bg-red-50`).

## Typography

- **Headline Font**: Inter / Noto Sans SC / Noto Sans JP
- **Body Font**: Inter / Noto Sans SC / Noto Sans JP
- **Mono Font**: Geist Mono / Menlo (used for code blocks, component names in collapsibles, and tags like `thinking`, `tool_call`)

Headlines and key labels use medium/semibold weights. Body text uses regular weight at 14px. Inputs use 16px to prevent iOS zoom. Secondary information uses 11px or 12px.

## Elevation

The system employs a **bimodal elevation strategy**:

1. **Absolutely Flat (Zero Elevation):** Interactive and atomic elements like Buttons, Inputs, Alerts, Badges, and Collapsibles have **zero shadows**. The default `shadow-sm` from Shadcn must be stripped out. Hierarchy is defined purely by 1px borders and background contrast.
2. **High Floating (Shadow-lg/md):**
   - Large layout containers that sit above the main canvas (like a central Login `Card`) use a pronounced `shadow-lg` to starkly differentiate themselves from the flat UI underneath.
   - Popovers, Dropdowns, and Combobox menus use a tightly controlled `shadow-md` without thick borders.

## Components

### Base Atoms

- **Avatars**: 26px rounded full. Default fallback uses `accent`. Status badges require a 2px knockout border matching the background.
- **Badges**: 4px radius (`rounded-sm`). Default is pure black (`bg-foreground text-background`). Secondary is `bg-accent text-foreground`. Destructive is pure red background. Warning is `bg-amber-500 text-white`. Outline is `bg-background border-border text-foreground`. Supports `size="sm"` (`text-[11px] px-2`) alongside default.
- **BadgeCount**: Circular numeric counter. `rounded-full`, `h-[18px] min-w-[18px] px-1`, `text-[11px] font-medium`. Variants: default (`bg-foreground text-background`), destructive (`bg-destructive text-destructive-foreground`), secondary (`bg-accent text-foreground`). Values above 99 display as "99+".
- **Buttons**:
  - Primary (Default): 8px radius, **`foreground` (black) background**, white text. Flat.
  - Secondary / Outline: 8px radius, `accent` background, 1px `border`. Flat.
  - Ghost: Icon buttons use `muted-foreground` defaulting to `foreground` on hover.
  - Grouped (`ButtonGroup`): Unified outer border (`border-border`, 8px radius). Internal items divided by `border-r` or `border-b`, no individual borders or radiuses.
  - Brand Primary: Use `variant="primary"` explicitly for Send button or active CTA. This is the **only** case where `bg-primary` is used.
- **Inputs**: Flat design. 1px `border`, pure `background` fill, 8px/10px outer radius. Text must be `text-[16px]`. Focus state uses a stark black/grey ring, **not** primary purple.
- **Checkboxes/Radios**: Flat, pure white background (`bg-background`). 1px `border`. The checked state marker (tick/dot) is `foreground` (black), avoiding brand colors.
- **Separators**: Uniformly use the 1px `border` color. Do not use padding/margins that detach them from the container edges (e.g., use full width `border-b` in menus instead of `<Separator mx-1>`).

### Containers & Layouts

- **Cards**: The exception to the flat rule. 12px radius (`rounded-xl`), pure `background`, 1px `border`, and strong `shadow-lg`. Content spacing is tight (`p-6`, with 6px gap between title and description).
- **Alerts**: Flat (`bg-background`), 1px `border`, 10px radius (`rounded-[10px]`). No variant-specific colored backgrounds; errors use red text/icons but keep the white background.
- **Collapsible**: Flat sub-panels. `bg-background` inside a `border-border` container, items divided by `border-b` (no bottom border on last child). Uses `font-mono` for code-related items.
- **Dropdowns / Popovers / Comboboxes**:
  - `bg-background` with `border-border` and `shadow-md`.
  - 8px outer radius.
  - Items use 4px inner radius and `text-sm font-normal text-foreground`.
  - Hover states use `bg-accent` and `text-foreground` (no brand color tints).
  - Group separators must be full-bleed `border-b`.
- **Breadcrumb**: Minimalist. 14px icons, 4px-6px gaps. Active page is `text-foreground font-normal`, inactive is `text-muted-foreground`.
- **Sidebar**:
  - Container uses `primary-foreground` background.
  - Active items use `accent` background with a 2px `primary` left border indicator.

### Items

- Global: `rounded-lg` (8px), `p-4`, `shadow-none`, `gap-4`.
- Variants:
  - Default: `bg-background`, no border.
  - Outline: `bg-background` + `border border-border`.
  - Muted: `bg-accent`, no border.

### Pagination

- Default page buttons: `bg-background border border-border text-foreground`, `rounded-lg`, `size-9`.
- Active/current page: `bg-foreground text-background border-foreground` (black bg, white text — no brand purple).
- Hover: `hover:bg-accent`.
- Disabled: `opacity-50 text-muted-foreground`.
- Previous/Next: ghost style, transparent bg, hover `bg-accent`.

### Complex / Domain Specific

- **Sonner (Toast Notifications)**: Uses `bg-background` with `border border-border` and `shadow-md`. Border radius is `var(--radius-lg)` (10px). Title: `text-sm font-medium text-foreground`. Description: `text-xs text-muted-foreground`. Status icons are `size-4`. Action buttons use `bg-foreground text-background rounded-sm text-xs font-medium`. Supports 6 states: default (with action button), loading (spinner), success, info, warning, error.
- **Table**: Wrapped in a container with `border border-border rounded-sm` (6px). Header row height 41px with `py-[10px]`, data rows 38px with `py-[8.5px]`. Footer uses `bg-muted/50`. Empty state centers an icon + `text-muted-foreground` message in `h-[204px]`. Caption is centered `text-sm text-muted-foreground`.
- **TagsInput**: Flat design. `rounded-lg` (no shadow). Focus state uses `ring-ring/20 ring-2` (not `ring-[3px]`).
- **Chat Input (Main Area)**:
  - Flat container (`shadow-none`), `border-border`, 10px radius.
  - Divided horizontally by top/bottom 1px borders (`border-t`, `border-b`).
  - Send button uniquely retains `bg-primary` for high conversion focus.
  - Model selector (Badge style) floats inside the top border area, using `bg-accent` or `bg-background` depending on state.
- **OTP Input (PinInput)**: Flat group built on reka-ui `PinInput`. Each cell is `size-9 bg-background text-sm shadow-none`. The first cell gets `border border-border rounded-l-lg`, middle cells use `border-y border-r border-border` (no left border to avoid doubling), and the last cell adds `rounded-r-lg`. Focus state applies `border-2 border-ring` (dark ring, not primary purple). Supports an optional `PinInputSeparator` between groups.

## AI Image Generation & Component Reuse Rules

To ensure visual consistency when generating images or extrapolating designs via AI based on this system, strictly adhere to the following composition rules:

1. **The "Flat Atom" Law:** If it's a basic interactive element (button, badge, input, toggle, checkbox), it **cannot** have a shadow. It must rely on `#e5e5e5` borders and background contrast (`#ffffff` vs `#f5f5f5` vs `#0a0a0a`).
2. **The "Menu Underline" Law:** Whenever rendering a list of items inside a container (Dropdowns, Comboboxes, Sidebar Sub-menus, Collapsibles), items must be divided by a full-width, 1px horizontal line (`border-b`), terminating cleanly on the last item. Do not leave floating separators with margins.
3. **The "Monochrome Hover" Law:** Focus and hover states for secondary items must always snap to `bg-[#f5f5f5]` and `text-[#0a0a0a]`. Never apply the brand purple (`#8b56e3`) as a background highlight for menus.
4. **The "Bimodal Elevation" Trigger:** Only use heavy, soft shadows (like `shadow-lg`) when rendering a standalone focal piece (like a login modal or a central dashboard card) that sits distinctly above a complex, flat background layer.
5. **The "Purple Scarcity" Law:** The brand purple (`#8b56e3`) is the most expensive pixel on the screen. It can only be used for:
   - The absolute primary completion action on a screen (e.g., the Chat "Send" button).
   - Micro-indicators of "current active state" (e.g., a 2px vertical bar next to an active sidebar menu item, or a 5px status dot).

## Do's and Don'ts

- **Do** use `bg-foreground` (black) for standard primary actions like "Login" or "Register" instead of the brand purple.
- **Don't** use `shadow-sm` on buttons, inputs, or alerts. Keep atomic components strictly flat.
- **Do** use `shadow-lg` for large focal cards to make them pop against the flat UI.
- **Don't** use colored backgrounds for Alert or Badge variants (like `bg-red-50`). Keep them white and use text/icon color to convey meaning (or solid red for destructive badges).
- **Do** maintain the 1px `#e5e5e5` (`--border`) across all structural dividing lines, ensuring they connect edge-to-edge (no floating separators).
- **Don't** use primary colors for hover or active states in standard dropdowns and menus; stick to `bg-accent` and `text-foreground`.
