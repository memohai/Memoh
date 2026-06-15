# Memoh Web Refactor — Reference

Concrete recipes, the dirty→clean diagnostic table, the reference-page map, and the
component picker. Read `SKILL.md` first for the principles; this file is the lookup.

## Reference map — copy these, by page shape

| Your page is… | Copy this refactored reference | Anti-example to compare against |
|---|---|---|
| A sparse, few-card page | `pages/about/index.vue` (centered group, footer meta) | — |
| A settings page: titled card groups of rows | `pages/bots/components/bot-overview.vue`, `pages/usage/index.vue` | `pages/bots/components/bot-tool-approval.vue` |
| A list of backends/items → detail | `pages/web-search/index.vue` (`useViewSwap` + `SwapTransition` + `BackendCard` + `DetailPane`) | — |
| A dashboard with stats + chart | `pages/bots/components/bot-overview.vue` (stat tiles + echarts, black/white/gray only) | `bot-tool-approval.vue` invented "metrics" cards |
| The full component catalog | `pages/dev/components/` (the wall — `Cmd/Ctrl+Shift+D`). Each `<Specimen note="…">` states *when* to use a component and its anti-pattern. | — |

`bot-tool-approval.vue` is the canonical un-refactored page: it stacks tinted fills,
hairline-alpha borders, off-scale text, `scale-90`, `shadow-none`, colored focus rings,
and an invented metrics dashboard. Treat it as the "what dirty looks like" exhibit.

## Recipes (verified against the refactored pages)

### Page shell (settings/list page)

```vue
<section class="mx-auto max-w-3xl px-6 pt-10 pb-12">
  <h1 class="mb-6 px-2 text-lg font-semibold">{{ $t('feature.title') }}</h1>
  <div class="space-y-8">
    <!-- SettingsSection groups go here -->
  </div>
</section>
```

A bot tab shell differs slightly (`mx-auto max-w-3xl pt-6 pb-8`, the tab container adds
its own `px-6`); mirror the sibling tabs, don't invent a new width.

### Spacing ladder (reuse these rungs — don't free-style margins)

| Gap between… | Value | Notes |
|---|---|---|
| pane edge ↔ content (horizontal) | `px-6` | the shell gutter; content never touches the edge |
| top of pane ↔ title | `pt-10` | text starts well below the top (About centers vertically instead) |
| bottom of content ↔ pane bottom | `pb-12` | |
| content column width / centering | `mx-auto max-w-3xl` | centered, ~768px cap — never full-bleed |
| title ↔ first card | `mb-6` | Profile uses `mb-8` |
| card group ↔ card group | `space-y-8` | the big section gap |
| section label ↔ its card | `space-y-2.5` | label is `px-2 text-[13px] text-muted-foreground` |
| row ↔ row inside a card | `border-b border-border` + `py-3`, `min-h-[3.75rem]` | hairline dividers, `last:border-b-0` |
| label ↔ its description | `mt-0.5` | |
| inside a padded card block | `p-4`/`p-5`, `space-y-4` | for non-row card content |

The `px-2` on the title and on section labels deliberately matches the visual left edge of
card content, so the title, the section labels, and the rows all line up on one invisible
left margin.

### The card + row primitives (use the shared components — do not hand-roll)

```vue
<SettingsSection :title="$t('feature.sectionTitle')">
  <SettingsRow :label="$t('feature.rowLabel')" :description="rowDescription">
    <Switch v-model="enabled" />
  </SettingsRow>
  <SettingsRow :label="…" :description="…">
    <Button variant="outline" size="sm">{{ $t('feature.action') }}</Button>
  </SettingsRow>
</SettingsSection>
```

What they render (so you can match them when a bespoke layout is unavoidable):

- `SettingsSection` card: `overflow-hidden rounded-[var(--radius-menu-shell)] border border-border bg-card`
- `SettingsSection` title (above the card): `px-2 text-[13px] font-medium text-muted-foreground`
- `SettingsRow`: `mx-4 flex min-h-[3.75rem] items-center justify-between gap-4 border-b border-border py-3 last:border-b-0`
  - label: `text-sm font-medium text-foreground` · description: `mt-0.5 text-xs text-muted-foreground`

### Stat tiles (hairline-divided grid, not bordered boxes-in-a-box)

```vue
<section class="space-y-2.5">
  <h2 class="px-2 text-[13px] font-medium text-muted-foreground">{{ $t('feature.overview') }}</h2>
  <div class="grid grid-cols-2 gap-px overflow-hidden rounded-[var(--radius-menu-shell)] border border-border bg-border sm:grid-cols-4">
    <div class="bg-card px-4 py-3.5">
      <p class="text-xs text-muted-foreground">{{ label }}</p>
      <p class="mt-1 text-xl font-semibold tabular-nums">{{ value }}</p>
    </div>
    <!-- … -->
  </div>
</section>
```

The `gap-px` + `bg-border` parent + `bg-card` children draws hairline dividers between
tiles with no per-tile border. (Contrast: the dirty page wraps each tile in its own
`rounded-md border` and tints the active one — card-in-card.)

### Empty / loading that holds the frame

```vue
<Empty class="rounded-[var(--radius-menu-shell)] border border-dashed border-border py-16">
  <EmptyHeader><EmptyMedia variant="icon"><Globe /></EmptyMedia></EmptyHeader>
  <EmptyTitle>{{ $t('feature.emptyTitle') }}</EmptyTitle>
  <EmptyDescription>{{ $t('feature.emptyDescription') }}</EmptyDescription>
  <EmptyContent><Button variant="outline">…</Button></EmptyContent>
</Empty>
```

In a table, keep the table drawn and use a full-width empty cell:

```vue
<TableRow v-else-if="rows.length === 0">
  <TableCell :colspan="N" class="p-0">
    <div class="flex h-[480px] items-center justify-center text-muted-foreground">
      {{ $t('feature.noRecords') }}
    </div>
  </TableCell>
</TableRow>
```

Never replace a section with a lone `<p class="py-12 text-center text-muted-foreground">No data</p>`
if it leaves the page looking broken — that is the empty-state anti-pattern.

### Search box + action, same height, same row

```vue
<div class="flex items-center gap-2">
  <div class="w-44 sm:w-56">
    <InputGroup class="w-full">
      <InputGroupAddon align="inline-start"><Search class="size-3.5 text-muted-foreground" /></InputGroupAddon>
      <InputGroupInput v-model="searchQuery" :placeholder="t('feature.searchPlaceholder')" />
    </InputGroup>
  </div>
  <Button><Plus class="size-4" /> {{ t('feature.add') }}</Button>
</div>
```

Consider hiding the search entirely until the list is long enough to need it
(`v-if="items.length > 8"`) — a search box over four rows is noise.

### List ↔ detail directional swap

```vue
<SwapTransition :direction="direction">
  <ListView v-if="view === 'list'" @open="openDetail" />
  <DetailPane v-else :back-label="t('feature.title')" @back="backToList()" />
</SwapTransition>
```

```ts
const { view, direction, openDetail, backToList } = useViewSwap()
```

`openDetail()` sets `forward` (list exits left, detail enters right); `backToList()` sets
`back` (reverse). Keyframes live in `style.css`; no `appear`, so landing on the page is a
plain cut and only the swap moves.

## Dirty → clean diagnostic table

Each left-column pattern is a real sin from `bot-tool-approval.vue` (and friends). When you
see it, replace it with the right column. This is your strip-list when refactoring.

| Dirty (strip it) | Why it's wrong | Clean (do this) |
|---|---|---|
| `bg-muted/40`, `bg-background/70`, `bg-success/5` baked tints | a fill grayer/colored vs the `bg-card` parent → "inside ≠ outside"; semantic color as decoration | inherit `bg-card`; tint only as a rationed signal |
| `border-border/50`, `border-*/40`, `border-success/20` | hand-written alpha + per-control structural borders | one `border-border` hairline; control edges via the field-edge / `--border-hairline` family |
| `text-[11px]`, `text-[10px]`, `text-[9px]` | off the type scale | the `--text-*` ladder (`text-body`/`text-label`/`text-caption`, etc.) |
| `rounded-full` status pills, bare `rounded`, mixed `rounded-md`/`rounded` | off the radius role-map | role-map radius (badge/tooltip 6 → control 8 → menu-shell 12 → card 14) |
| `<Switch class="scale-90">` | resizing a control with a transform | use the control's real size prop; never `scale-*` a control |
| `class="shadow-none"` fighting an inherited shadow | flat controls/cards carry no shadow | drop it; elevation is a token, used only on floating layers |
| `focus-visible:ring-success/30`, `…ring-warning/30` | colored focus rings; ring as emphasis | the `--ring` keyboard focus only; field commit swaps the edge in place |
| `opacity-50 grayscale` for disabled | muddy disabled treatment | `opacity-40` (the contract's single disabled rule) |
| invented "metrics" cards w/ `text-2xl` numbers, status tints | dashboard chrome that isn't the language | stat-tile grid recipe above, black/white/gray |
| sticky `bg-background/95 backdrop-blur` "sovereign header" | invented page chrome | the plain page-shell `h1` + a save action where it belongs |
| `"+"` / `"×"` glyphs, hand-rolled icon hover bg, hand-rolled tooltip | not real components; can't receive size/stroke tokens | lucide components in `<Button size="icon">`; `@memohai/ui` `Tooltip` |
| `Transition name="fade"` + ad-hoc `transition-all duration-300` | lazy catch-all motion | the directional swap / token durations; transition the real property |

## Component picker

| Need | Use | Not |
|---|---|---|
| Pick one value from a menu | `Select` | a hand-rolled popover list |
| Searchable pick (single or many) | `Combobox` (with `multiple`) | re-skinning `Select`; bespoke search dropdown |
| Switch a mode/filter, returns a value, no panels | `SegmentedControl` | `Tabs` re-skinned as a pill |
| Switch between content panels | `Tabs` (underline) | `SegmentedControl` |
| Simple native dropdown (few static options) | `NativeSelect` | a full `Select` for 3 options if native suffices |
| Toolbar icon action | `<Button variant="ghost" size="icon">` | a bare clickable `<svg>` with manual hover bg |
| Standalone icon action | `<Button variant="outline" size="icon">` | ghost (reads as toolbar) |
| Clickable low-emphasis text w/ hover chip | `TextButton` (ghost @ `size="text"`) | a `<span @click>` with a hand-rolled hover |
| High-emphasis CTA | `<Button>` (charcoal default) | `variant="brand"` purple unless it's a true brand CTA |
| Destructive action | `<Button variant="destructive">` (filled) | `variant="ghost"` + `text-destructive` |
| Count / unread / overflow badge | `BadgeCount` (`destructive` alert · `default` neutral) | a hand-built rounded-full number pill |
| Empty surface | `Empty` (+ framed) | bare centered muted `<p>` |
| Status that aligns to a section title | `Badge` (gives the status a box edge) | a loose dot + text floating with nothing to align to |

### Icon & badge specifics (from the wall)

- Icons scale on one ladder with the control: default control **16px** (`size-4`); small
  in-field **14px** (`size-3.5`); text/badge rung **12px** (`size-3`). Pick the rung; don't
  free-set sizes.
- `BadgeCount`: `destructive` is the red alert dot pinned to an **icon corner**
  (`absolute -right-1.5 -top-1.5`) for unread/needs-attention; `default` is a neutral count
  that rides a tab/filter/segment label; in a flat list row, a count is calmer as a plain
  muted numeral (`text-caption tabular-nums text-muted-foreground`), no bubble. Overflow caps
  at `:max` (default 99 → `99+`).

## Lessons baked into the reference pages (worth stealing)

From `bot-overview.vue` — these are the judgment calls that make a page read calm:

- **A healthy state says nothing.** Don't tell the user "you connected Telegram" — they did
  it. Surface a block only when it's actionable (nothing set up yet, or there's an issue).
- **No card-in-card.** A single row of metric tiles wrapped in a `SettingsSection` reads as
  a big bordered box moated around small boxes → "mostly empty." Let the tiles be the content.
- **A Badge beats a loose dot+text for status**, because the badge gives the status a box to
  align against the section title instead of floating.
- **`—`, never a faked `0`.** If the backend didn't sample a metric, show `—`. If there's no
  metric grid, say *why* in one honest line — don't pad with empty tiles.
- **Charts are black/white/gray.** `--primary` is a violet in theme; charts use `--foreground`
  + `--muted-foreground`, no brand/accent. (See the token→canvas color round-trip note in the
  page — echarts can't read oklch tokens directly.)

## Guard & commands

- `mise run lint` — runs `scripts/check-ui-contract.mjs` (HARD-fails raw colors, off-scale
  arbitrary radius, invented box-shadows; WARNs on structural borders on controls, invented
  shadows, ring-offset selection). Run before declaring a page done.
- The component wall (`Cmd/Ctrl+Shift+D` on desktop, or the `memoh:dev-tools` localStorage
  flag on web) is the living catalog — verify your component choice against its `note=`
  annotations before inventing anything.
