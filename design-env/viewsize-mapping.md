# Layout Standards: L3/L4 Multi-Sidebar & Single-Column Responsive Strategy

This document outlines the engineering standards and experience for handling complex multi-sidebar (Level 3 Rail + Level 4 Workspace) layouts, as well as single-column layouts, within restricted or extreme viewports.

## 1. Responsive Waterfall Strategy (Multi-Sidebar)

When a page contains nested sidebars (e.g., a platform-wide sidebar and a component-specific sidebar), the horizontal space is quickly consumed. 

### Breakpoint Mapping
- **Threshold**: `lg` (1024px).
- **Desktop (≥ 1024px)**: `flex-row`. Sidebar and Workspace are side-by-side.
- **Narrow/Mobile (< 1024px)**: `flex-col`. Sidebar stacks above the Workspace (Waterfall mode).

### Implementation Logic
```vue
<!-- Outer Container -->
<div class="flex flex-col lg:flex-row gap-4 absolute inset-0 max-w-6xl mx-auto px-4 pt-4 pb-6 w-full">
  <!-- L3 Sidebar: Fixed width on desktop, fixed height on mobile -->
  <div class="w-full h-48 lg:w-52 lg:h-full shrink-0"> ... </div>
  
  <!-- L4 Workspace: Full width, scrollable -->
  <div class="flex-1 min-w-0"> ... </div>
</div>
```

## 2. Single-Column Layouts (No L3 Sidebar)

For pages that only require a main workspace (L4) without a local navigation rail (e.g., Overview, Settings, Access), the layout should constrain its maximum width to maintain readability, rather than stretching across the entire screen.

### Implementation Logic
```vue
<!-- Outer Container -->
<div class="max-w-2xl mx-auto pb-6 space-y-5 px-4 pt-4 w-full">
  <!-- Sovereign Header -->
  <header class="pb-4 border-b border-border/50 sticky top-0 bg-background/95 backdrop-blur z-30 pt-4 -mt-4 flex items-center justify-between">
     ...
  </header>
  
  <!-- Content Sections -->
  <div class="space-y-4"> ... </div>
</div>
```

### Constraints & Behavior
- **Max Width**: Use `max-w-2xl` (42rem / 672px) or `max-w-4xl` depending on the density of the form or content. Do not use `max-w-6xl` unless L3 is present or a wide data table requires it.
- **Centering**: Always use `mx-auto` to horizontally center the column.
- **Responsive Flow**: Since there are no sidebars to stack, single-column layouts naturally adapt to smaller screens. Elements inside the column (like flex rows) should use `flex-wrap` or switch to `flex-col` on extreme narrow screens if they contain side-by-side inputs.

## 3. Dynamic Sidebar Sizing

To prevent UI "squeezing", sidebars must adapt their dimensioning based on the axis of flow:
- **Horizontal Axis (Desktop)**: Sidebar width should be optimized (e.g., `w-52` or `w-56`) to maximize L4 workspace area.
- **Vertical Axis (Narrow)**: Sidebar should switch to a fixed height (e.g., `h-48`) or become collapsible (`h-12`) to ensure the primary L4 form content remains usable.

## 4. Responsive Action Button Toggling

Action buttons (e.g., "Add", "New", "Import") should relocate based on layout density to maintain reachability and visual balance.

| Layout Mode | Button Location | Style | Responsive Class |
| :--- | :--- | :--- | :--- |
| **Desktop (Sidebar)** | Sidebar Footer | Full Width / Label + Icon | `hidden lg:block` |
| **Narrow (Card)** | Sidebar Header | Compact / Plus Icon Only | `lg:hidden` |

### Rationale
In waterfall mode, the sidebar behaves like a "Card". Placing a large button at the bottom of a card often requires unnecessary scrolling. Moving the primary action to the header (top-right) aligns with mobile card patterns and saves space.

## 5. Empty State Centering

Empty states within sidebars or single-column areas must be perfectly centered (vertically and horizontally) to avoid a "broken" or "top-heavy" look.

### CSS Standard
```vue
<div class="flex-1 flex flex-col items-center justify-center p-4 text-center">
  <span class="text-muted-foreground text-[11px]">No items found.</span>
</div>
```
- **Prerequisite**: The parent container must be `flex flex-col h-full` or the `ScrollArea` must occupy the remaining height.

## 6. Summary of Optimized Utility Classes
- `w-full h-48 lg:w-52 lg:h-full`: The standard responsive sidebar dimension.
- `flex flex-col lg:flex-row gap-4`: The standard waterfall container.
- `max-w-2xl mx-auto` or `max-w-4xl mx-auto`: Standard constraint for single-column (no L3) forms.
- `hidden lg:block` vs `lg:hidden`: The standard toggle for header-icon vs footer-button.
