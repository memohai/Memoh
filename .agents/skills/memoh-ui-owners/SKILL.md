---
name: memoh-ui-owners
description: Read this BEFORE writing or editing any apps/web (Memoh web frontend) settings page, bot-detail tab, dialog/popover/sheet form, config panel, list row, or detail surface — i.e. almost any Vue markup that is not pure chat. Memoh has ten owner components (SettingsRow, SettingsSection, FieldStack, PageShell, MetricReadout, PersonaTile, CalloutBanner, ExpandableSettingsRow, BackendCard, FormStack) that own all recurring spacing. Use this skill whenever you are about to build a settings row, a form field (label + input), a section card, a Save/Cancel footer, a stat tile, a warning banner, an "Advanced" disclosure, an empty state, or a whole page frame — even if the user just says "add a settings page", "make a dialog", "add a toggle row", "build this form", or "add a field", without mentioning spacing or components. Hand-writing a row/field/card/tile out of raw `<div class="flex … border-b …">` is the #1 recurring mistake in this codebase (同形异码); compose an owner instead so spacing can never drift. The skill also says when a shape must STAY hand-written (a genuinely different spatial relationship, e.g. a denser list row). Pairs with memoh-web (page-level) and packages/ui/AGENTS.md (atom-level).
---

# Memoh UI — Spacing Owner Vocabulary

This skill is the **composition layer** between the page-level rules in
`memoh-web/SKILL.md` and the atom-level token law in `packages/ui/AGENTS.md`.

`memoh-web` tells you *how a page reads*. `packages/ui/AGENTS.md` tells you *how one
control looks*. This file tells you the piece in between: **when you need a row, a
field, a section, a tile, a disclosure, an object card, or a warning — you compose one
of ten owner components instead of hand-writing it from `<div>`s.**

## Why this exists (the one idea)

The debt this vocabulary kills is **same-shape-different-code** (同形异码): the same
visual shape — a settings row, a form field — written out by hand in slightly different
Tailwind classes on every page. It is invisible to grep (the classes differ) and it
drifts: one row is `py-3`, its twin is `py-2.5`, a third adds `min-h-[3.75rem]`, and the
surface slowly stops looking like one system.

The fix is **not** "tokenize every `gap-4`." The fix is: **give each recurring spatial
shape ONE owner component, and compose that.** A shape with an owner cannot drift,
because there is only one copy of its spacing.

> Before you write `<div class="... flex ... border-b ...">` for a row, or
> `<div class="space-y-1.5"><Label/>…` for a field — **stop.** That shape has an owner.
> Compose the owner.

## The ten owners

Import paths are exact. All live under `apps/web/src/components/`.

### Structure

**PageShell** · `page-shell/index.vue`
The page header + centered `max-w-3xl` column. Props: `title`, `description`,
`variant: 'page' | 'tab'` (`tab` for a bot-detail settings tab, `page` for a standalone
route). Slot: `#actions` (header-right toolbar). Every settings surface starts here.

**SettingsSection** · `settings/section.vue`
A grey section label above a bordered white card. Props: `title`. Slots: `#actions`
(header toolbar, right of the title), default (the rows), `#footer` (an action band —
Save/Cancel or pagination — rendered *inside* the card with a top hairline). A card is a
`SettingsSection`; you never hand-roll `rounded-… border bg-card`.

### Rows (things inside a section card)

**SettingsRow** · `settings/row.vue` — *the workhorse.*
One horizontal row inside a section card. The canonical geometry
(`min-h-[3.75rem] py-3 mx-4 border-b`, inset hairline, `last:border-b-0`) lives here and
nowhere else. Props:
- `label`, `description` — the default body.
- `align: 'center' | 'start'` — `start` top-aligns a tall row (leading avatar + a
  multi-line body).
- `stack: 'never' | 'sm' | 'always'` — `never` (default) is one line; `sm` drops to a
  column below the `sm` breakpoint so a label + control don't cramp; **`always`** is a
  permanent column: a full-width control (Textarea, a schema field) under its label.

Slots: `#leading` (media — icon / avatar / channel mark / load skeleton), `#content`
(a fully custom body replacing label/description — use it when you need a `<Label :for>`
for focus-wiring, badges, or a status line), default (the **trailing control**).

**ExpandableSettingsRow** · `settings/expandable-row.vue`
A settings row whose whole header toggles a collapsible body. Props: `label`,
`description`, `v-model:open`. Slots: `#leading`, `#content`, `#trailing` (default: a
chevron that rotates on open), `#expanded` (the disclosed body — put a `FormStack` here).
Use for "Advanced" disclosures and per-item inline editors where the row **owns** what it
reveals. (If the toggle controls a *sibling* block it doesn't own, see the anti-pattern
below.)

**BackendCard** · `settings/backend-card.vue`
A clickable **horizontal** object-entry card (its root is a real `<button>`). Props:
`name`, `subtitle`, `enabled` (draws a status dot). Slots: `#leading` (icon), `#trailing`
(default: a chevron). For "pick a provider / backend / storage" object rows.

### Fields (vertical form controls)

**FieldStack** · `settings/field-stack.vue`
ONE vertical Label-over-control form field (`space-y-1.5`). Props: `label`, `for` (wires
label→control focus), `help` (a muted `<p>` under the control). Slot: `#label` (custom
label row — e.g. a label with an inline "clear" button), default (the control). This is
**distinct from SettingsRow**: SettingsRow puts the label *beside* the control; FieldStack
stacks it *above*.

> **FieldStack is the single owner for vertical form fields — including validated ones.**
> When a FieldStack sits inside a vee-validate `<FormField>`, it takes over `FormItem`'s
> job: it provides the form-item id (so a `<FormControl>` wrapping the control resolves
> aria-invalid / aria-describedby) and renders the field's error inline. So the pattern for
> a validated field is:
> ```
> <FormField v-slot="{ componentField }" name="x">
>   <FieldStack :label="…" for="x-id">
>     <FormControl><Input id="x-id" v-bind="componentField" /></FormControl>
>   </FieldStack>
> </FormField>
> ```
> Keep the outer `<FormField>` and the `<FormControl>`; drop the old `<FormItem>` + bare
> `<Label>`. A plain field with no validation is just `<FieldStack :label>` around the
> control. **One caveat:** because FieldStack now renders errors inline, any hardcoded
> validation message in the zod schema becomes visible — translate it (zh/en/ja) instead
> of shipping the English default. The older `@memohai/ui` `<FormItem>` (`grid gap-2`) is
> superseded; new code uses FieldStack.


**FormStack** · `settings/form-stack.vue`
A `space-y-4` wrapper for a run of `FieldStack`s. Use it around a contiguous form column
(a dialog body, an `#expanded` panel).

### Readouts & notices

**MetricReadout** · `settings/metric-readout.vue`
One metric tile: caption label + tabular value (or a status dot + label). Props: `label`,
`value`, `sub`, `framed` (default `true` — draws the tile box; `false` for a bare stat on
a surface), `status: 'ok' | 'warn' | 'error'`. Slot: `#value` (custom markup — relative
time, mono). **The caller owns the grid**; MetricReadout is a single cell.

**PersonaTile** · `persona-tile/index.vue`
A **vertical, centered** entity/add tile (`w-52 flex-col items-center`). Props: `name`,
`variant: 'entity' | 'add'`. Slots: `#media`, `#status`, `#name`. This is the vertical
counterpart to the horizontal BackendCard — do not confuse them.

**CalloutBanner** · `callout-banner/index.vue`
A framed warning / destructive notice. Props: `tone: 'warning' | 'destructive'`, `title`,
`description`, `clickable` (whole surface becomes a button with a lead-in chevron). Slots:
`#icon`, default (trailing action).

### Plus one atom you already have

**Empty** (`@memohai/ui`) — the empty state. Fold `py-12`/`py-16` as needed. Loading and
empty states must still draw their frame so nothing reflows.

## Which owner? — a decision map

```
Need a bordered card holding rows?              → SettingsSection
  a row: label + a control on one line?         → SettingsRow
  a row whose header expands a body it owns?    → ExpandableSettingsRow
  a clickable "pick this object" row?           → BackendCard
A stacked form field (label above control)?     → FieldStack (wrap a run in FormStack)
A stat / number tile?                           → MetricReadout (you own the grid)
A vertical, centered entity/add tile?           → PersonaTile
A warning / destructive framed notice?          → CalloutBanner
The page frame itself?                          → PageShell
An empty / loading state?                        → Empty (draw the frame)
```

## When to STAY hand-written (do not force an owner)

Unification is the goal — but only for shapes that are **the same spatial relationship**.
Keep a shape local when its relationship genuinely differs. The tells:

- **Different geometry for a reason.** A denser list item (`min-h-[3.25rem] py-2.5`, a
  compact model/provider list) is *not* a `3.75rem` settings row. A dialog-body field with
  `text-xs` muted labels + `h-8` inputs is a *tighter inline-form language* than a
  settings-page FieldStack. Same code ≠ same relationship.
- **A different surface.** Sidebar rows, file-tree rows, chat message rows live in
  non-settings surfaces with their own row systems. Owners are for the settings/form/list
  surface, not everywhere a `border-b` appears.
- **A genuinely one-off compound block.** OAuth device-flow, a drag-drop upload target, a
  Monaco/JSON editor, a snapshot input, a link-code countdown, the single real data table
  — no owner covers these; hand-write them.
- **A centered placeholder** borrowing a row's `min-height` so the panel doesn't reflow —
  a Spinner/Skeleton block, not a row.
- **A trivial muted `<p>`** no-results line.

If you're unsure whether two shapes share a relationship, judge by **geometry + context**,
not by class-string similarity. When the answer is "same shape, same size, same surface" →
it's a MISS, give it its owner. When it's "looks similar, but denser / different surface /
one-off" → keep it local and say why.

## Traps (learned the hard way)

- **Tailwind scans literal source text.** A runtime `` `sm:${align}` `` or
  `` `min-h-[${x}]` `` class is **never generated**. Any conditional class must enumerate
  the *full literal strings* (see `SettingsRow`'s `rootClass` computed — every combination
  spelled out). Never build a class by string concatenation.
- **Don't nest a `<button>` in a `<button>`.** `BackendCard`'s root is a real button, so
  you cannot drop an interactive `Switch`/menu into its trailing slot. If a row needs a
  trailing interactive control *and* a whole-row click, that's a hand-rolled
  stretched-overlay pattern — keep it local (this is why `bot-acp` stayed local).
- **A toggle that reveals a *sibling* block is not an ExpandableSettingsRow.**
  ExpandableSettingsRow owns its `#expanded` body. If your button toggles a separate
  block below it (e.g. a run of grouped rows with their own dividers), keep the toggle as a
  plain `SettingsRow` with the button in its default slot, and leave the revealed block
  outside.
- **`#footer` is `justify-end`.** It fits Save/Cancel. A `justify-between` pagination strip
  (summary span on the left, pager on the right) does not fit the current `#footer` — keep
  that footer local until the owner grows a variant.
- **App pages don't lint raw colors.** Tokens only (`bg-card` / `text-foreground` /
  `border-border`); for hover/selected use the overlay ladder (`--ui-hover` / `bg-accent`),
  never a gray or a `/10` alpha. The `check-ui-contract.mjs` guard flags app-scope
  `hover:*`/`bg-*` injections — component chrome belongs in the component (mark a sanctioned
  line with a `/* ui-allow-style */` same-line comment inside the owner, never on a page).

## Migration discipline (when converting a hand-rolled surface)

1. **Read `memoh-web/SKILL.md` first**, then this file, then the page.
2. **Inventory every behavior before editing** — every `v-model`, click handler,
   loading/empty branch, validation, i18n key — and re-wire each after. A refactor must not
   regress.
3. **Migrate only genuine owner-shapes.** Judge each shape against the "stay hand-written"
   tells above. Don't blind-unify; don't force a different-relationship shape into an owner
   to look thorough.
4. **Don't change color, type, radius, shadow, or copy** while migrating spacing. Remove
   now-unused imports.
5. **Verify:** `eslint` (0), `vue-tsc` (0 errors), `node scripts/check-ui-contract.mjs`
   (0 *new* violations — pre-existing debt in other files is not yours to fix here), then a
   **human** confirms the rendered page (dark + narrow + `zh` + walk every old interaction).
   The agent's "it should work" is not verification.

## Owner reference

The owner sources are the source of truth — read the component before composing it if a
prop is unclear. The full morphology census and the record of what shipped (and what was
confirmed stays-local) live in `docs/design/spacing/owner-vocabulary-census.md`.
