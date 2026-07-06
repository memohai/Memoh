# @memohai/ui — Design Language Contract

Single source of truth for the **cross-cutting** visual and interaction decisions
in this component library. It exists because one class of bug kept recurring:
**inventing chrome** (stray shadows / borders) and **hand-writing raw values**
instead of pulling from tokens. LLMs also tend to assume Tailwind v3 / older Vite
and reintroduce version bugs (see Motion below).

If you build or refactor a component in `packages/ui`, follow this file. When you
make a **new** cross-cutting decision, write it back here — this is a living doc.

## Enforcement is three layers

| Layer | Where | Role |
|---|---|---|
| **Tokens** | `src/style.css` (`@theme inline` / `:root` / `.dark`) | the ONLY place raw values live |
| **This contract** | `packages/ui/AGENTS.md` | the rules + rationale |
| **Guard** | `scripts/check-ui-contract.mjs` (wired into `mise run lint`) | mechanical block on drift |

## Compose, don't style — the one principle (locality)

LLMs are weak at CSS / visual / spatial reasoning and strong at composing by API. The whole
point of this library is to keep that weakness out of the loop: **style has exactly one home
(locality), and the layer above only composes.** An agent (or a person) expresses intent
through a component's API — `variant="destructive"` — never through raw CSS
(`bg-red-500 hover:bg-red-600`). The component is the translation layer between "what I want"
and "what CSS makes it so."

Corollary: **hand-writing CSS / injecting a class is NOT a normal move here — it is an escape
hatch.** Reaching for it means a contract is missing; the right reaction is to *add the
contract* (a `variant`, a slot, a token, a shared component), not to press the hatch and paint
over the component.

**Ownership — every concern has exactly ONE home:**

| Concern | Its only home | Who must never touch it |
|---|---|---|
| Raw values (color / shadow / radius / size) | tokens (`style.css` `:root` / `.dark` / `@theme`) | any `.vue`, any app page |
| Interaction chrome (hover / press / focus / open) | `style.css @layer components`, keyed off `data-*` | any `.vue`, any app page |
| Layout + resting state (height / padding / radius / rest color) | the component `.vue` + `cva` | app pages don't re-write it |
| Composition (arranging components into a screen) | app pages + shared composition components | — |
| Business / orchestration | page logic | components embed no business assumption |

The test: for any value, *"where does it live?"* must have exactly **one** answer. The same
value in two homes is debt. *How* these homes fight the moment you ignore the boundary is the
next section.

## The cascade has four override planes (read before you "just add a class")

The recurring frustration — *"my CSS does nothing / something else wins / the conflict is
invisible"* — is not because the CSS is messy. It is because a final style here has to hold
on **four independent override planes at once**. Touch one without knowing the others and the
result silently changes. This is the mental model that the rest of this file assumes; an agent
that lacks it keeps re-introducing the same class of bug.

**The stack, bottom → top:**

```
5  app pages (apps/web)         compose components; pass ONLY tokens + variant/size
4  .vue + cva (index.ts)        ONLY layout + resting color (height/padding/radius/rest text)
3  style.css @layer components   ALL interaction chrome (hover/press/focus/open), keyed off data-*
2  reka-ui primitives           inject data-state / aria-expanded; set page pointer-events:none on open
1  Tailwind v4 + tokens          @theme / :root / .dark values; @layer utilities classes
```

**The four planes that decide who wins:**

1. **Layer order — `@layer utilities` ALWAYS beats `@layer components`.** A Tailwind utility
   you put on a component's `class` (`hover:bg-*`, `bg-*`, `transition-colors`) overrides the
   hand-written rule in `style.css`. (This is the Switch stray-`transition-colors` bug and why
   the link variants need `!important`.)
2. **`cn()` = clsx + tailwind-merge silently DROPS classes.** When tailwind-merge thinks two
   utilities conflict it keeps only the last and discards the rest — so a base class can vanish
   (the `text-control` vs `text-background` collision patched in `lib/utils.ts`; the
   `[&_svg:not([class*=size-])]` icon-size gotcha).
3. **reka injects `data-state` / `aria-expanded` and sets page `pointer-events:none` while a
   menu is open.** This is the "the library overrides me" plane: an open trigger goes inert, so
   `:hover` cannot fire unless the component opts back in with `pointer-events:auto` (what
   `select-trigger` does; the ghost button instead stays lit via `[data-state=open]`).
4. **Specificity + source order inside `@layer components`.** Later rules win at equal
   specificity (ButtonGroup overriding the standalone button is this plane).

**The boundary that keeps all four from biting (enforce it on every page):**

- **Interaction chrome is owned by `style.css`, keyed off `[data-button]` / `[data-slot]` /
  `[data-variant]` / `[data-state]`.** A `.vue` file and an app page NEVER hand-write a
  hover/press/open fill.
- **A page passes a component only `variant` / `size` / layout (`w-full`) + semantic tokens.**
  It does **not** inject `bg-*` / `hover:bg-*` / `border-border` / `shadow-none!` into a
  `<Button>` / `<Select>` / `<TextButton>` — those collide with the `::before` fill and the
  field-edge contract (the canonical "weird Select" / "fights the ::before" bug). The tell of
  this debt is a fill/hover/border baked onto a real component's `class`; if you need a
  different look, pick the right `variant`, don't paint over it.
- **One trigger-open philosophy, applied everywhere.** "How a trigger looks while its menu is
  open" is ONE decision (pure-pointer via `pointer-events:auto`, OR stay-lit via
  `[data-state=open]`) — Select, Dropdown, Popover and ghost triggers must not each pick their
  own, or they read as "fighting" (the recurring TextButton-vs-Select hover mismatch).

## Three laws

1. **Everything is a token.** No raw `#hex` / `rgb()` / `oklch()` / `color-mix()`
   / arbitrary `[Npx]` radius in component styles — neither in `.vue` class
   attributes nor in the `@layer components` blocks of `style.css`. Raw values
   live ONLY in token definitions (`:root` / `.dark` / `@theme`). Need a value?
   Reuse a token; if none fits, **add a token, then use it**.
2. **Never invent chrome.** A control is differentiated by **fill / color**, not
   by a stray border or shadow. Do not add a `box-shadow` or `border` "to make it
   look nicer." Borders come from `--border` / `--border-hairline` / the
   field-edge contract; shadows come from the elevation tokens. (The Switch once
   shipped with an invented black hairline AND an invented thumb shadow — both
   were wrong and removed.)
3. **New language only.** The refactored Atoms are the reference. Do **not** copy
   an un-refactored legacy component's styling.

## The one rule — clean vs dirty

Every rule below is one principle: **at rest a control is transparent and
inherits its surface, defined only by a single hairline edge; on interaction
that ONE layer changes in place** — the edge swaps color, or one fill tint
deepens. Nothing stacks.

> **Clean = one layer changing in place. Dirty = many chrome layers stacked on a
> single control** (a baked fill + a structural border + a colored outer ring + a
> shadow + hand-written alpha).

The refactored Atoms (Button / Input / Select / Toggle / Checkbox …) pass this
test; the legacy look (see § Dirty patterns) fails it by adding layers. Two
corollaries that the whole system leans on:

- **Stroke defines the body; color expresses interaction.** A non-emphasis
  control is shaped by its edge (transparent fill, inherited surface), and
  hover/press/focus is read through a color change — never by inventing a border
  or shadow at rest.
- **Black / white / gray is the skeleton (~90%). Blue means "selected". Purple is
  scarce.** The high-emphasis CTA fill is CHARCOAL (`--btn-primary`), NOT brand;
  brand purple (`--brand`) is reserved for `variant="brand"` and a few accents.
  Do not paint the UI purple.

## Unification ≠ homogenization

Unifying the system does **not** mean every control looks the same. A component
MAY have its own personality — a unique shape, elevation, or motion (e.g. the
SegmentedControl's elevated sliding thumb). The rule is only that the personality
must be expressed **through tokens**, never raw values. Distinctive is fine; raw
numbers are not.

### Tabs vs SegmentedControl — distinct LOOK, distinct ROLE

They used to look identical (both an enclosed pill row), which made one of them
look redundant. They are NOT interchangeable, so they are now visually split:

- **SegmentedControl** = the enclosed pill with the sliding thumb. It is a single
  **value/mode/filter selector** (`role="radiogroup"`) — it returns a value and
  owns **no** content. Reach for it for Day/Week/Month, List/Board, view toggles.
- **Tabs** = **underline** section navigation (active = `border-foreground`
  bottom border, idle muted → hover foreground). It owns **tabpanels + the tab
  a11y contract** (roving focus, `aria-controls`, panel show/hide) that the
  SegmentedControl deliberately lacks. Reach for it when switching **panels**.

Do not re-skin Tabs back into a pill — that reintroduces the "why do we have
both?" ambiguity. If you only need to pick a value (no panels), use
SegmentedControl, not Tabs.

### Breadcrumb & Pagination — interaction comes from `[data-button]`, not classes

Pagination controls (first/prev/page/next/last) render the real `<Button>` via
reka `as-child`, so they inherit the full `[data-button]` chrome (hover fill /
press-scale / focus ring / disabled fade). Applying `buttonVariants()` **classes**
to a bare reka primitive only carries layout — the interaction lives on
`[data-button]` in `style.css` and was never reached (that was the "pagination has
no hover" bug). Edges are icon-only ghost buttons; the active page is the `outline`
chip (`aria-current="page"`). Breadcrumb separators are a literal `/` (not a
chevron); each crumb is a `<TextButton>` (see below), so its hover/press is the
single-sourced ghost chrome — there is no breadcrumb-only hover rule.

### "Clickable text with a hover chip" = `<TextButton>` (ghost @ `size="text"`)

The recurring "low-emphasis text you can click, with a padded hover hit-area
bigger than the glyphs" affordance (breadcrumb crumbs, inline dropdown triggers,
compact labels/actions) is NOT a new interaction — it is the ghost `<Button>` at
the compact `text` size. `size="text"` = `h-auto` / `rounded-sm` (6px, the
compact-chip radius — not a control's `rounded-md`, not an invented `rounded-xs`) /
the base `text-control` (14px, the standard button text size) / **`size-3` (12px)
icons** (the text/badge rung of the icon ladder — see § Sizing & icons — one notch
under the cap height so the glyph reads quietly, not as a chunky control icon).
Padding is asymmetric on purpose — tight horizontally (`px-1.5`) with a hair more
vertical room (`py-[5px]`) — so the ghost hover/press chip breathes ever so slightly
above and below the text without becoming a tall pill. Pair it with `variant="ghost"` and the existing
`[data-button][data-variant="ghost"]` hover/press/ring chrome applies unchanged.
(One Tailwind gotcha: the icon-size override is written `[&_svg:not([class*=size-])]`
with an **unquoted** attribute value. `tailwind-merge` strips the base `size-4`, so
the replacement must actually generate CSS — and the escaped-quote form
`[class*=\'size-\']` inside a single-quoted TS string is not extracted by the JIT,
which silently leaves the SVG at its 24px intrinsic size.)
`<TextButton>` is the named, discoverable wrapper that defaults to exactly that
(overridable `variant`/`size`, supports `as` / `as-child`). Reach for it instead
of hand-rolling a hover on text. For an underlined inline link inside running
prose use `variant="link"`; for a control-sized action use a normal `<Button>`.

### "A card that DOES something / goes somewhere" = `<ActionCard>`

The recurring need to offer an **entry point** — a card that, when clicked, takes
the user to a next surface (a slide-in detail, a focused dialog, an external URL)
— is NOT a display card with an `@click` bolted on, and NOT a `<Button>` stretched
to card width. It is its own affordance: `<ActionCard>`. It wears the same
white-card language (`bg-card`, one `border-border` hairline, flat) so it sits
among ordinary cards, but three marks make it read as *action*, not
*information*:

- a **required leading icon** (the `#icon` slot — the caller passes a lucide
  component; the card bakes in no glyph, so it never becomes icon-abuse the card
  can't justify), colored `text-foreground` — the SAME weight as the title, not
  muted, so it doesn't lose contrast under the (deliberately subtle) hover fill,
- a **required trailing forward chevron** (`ChevronRight`, overridable via
  `#trailing` — e.g. an `ArrowUpRight` for an external link), kept muted per the
  system-wide directional-glyph convention,
- the **whole card** reacts to the pointer: hover/press change the fill across the
  entire surface with the bespoke `--action-card-hover` / `--action-card-active`
  pair — NOT the shared overlay ladder. ActionCard is the one control whose hover
  surface is a whole card, and at that area even the ladder's lightest rung read
  too strong on dark, so it gets its own measured-delta wash (defined + justified
  in `style.css` `:root`; ~9 RGB delta on the card fill). At a snappy 15ms.

**Radius is `--radius-menu-shell` (12px), NOT the generic Card family's
`--radius-xl` (14px).** ActionCard is almost always adjacent to `SettingsSection`
rows or the app-layer `BackendCard` (both 12px) — sharing their corner keeps a
mixed list of ActionCards and settings rows reading as one system instead of two
subtly different rounds. It is set as a plain CSS property on
`[data-slot="action-card"]` in `style.css` (mirroring how the Toast shell sets the
same token) rather than a Tailwind `rounded-[…]` class, because the guard
HARD-fails arbitrary-radius Tailwind tokens inside `packages/ui`.

Two things make this contract-clean where a hand-rolled clickable card is not:

1. **The hover fill is a `::before` neutral overlay, not a `bg-*` swap.** `--card`
   (white) ≠ `--background`, so `hover:bg-[var(--ui-hover)]` on the card body would
   *replace* the white fill and bleed the page surface through the hover (the trap
   the `bg-accent/30` hand-hack in the old app-layer `BackendCard` danced around).
   The overlay lives on a `z-index:-1` `::before` (style.css,
   `data-slot="action-card"`), composited **over** `bg-card` and **under** the
   content, so the whole card darkens in light / lightens in dark, scheme-agnostic,
   no `dark:` override. Same mechanism as the ghost button.
2. **No press-scale — this is a large content card.** The "no hover-rise" rule
   forbids a big card lifting/shrinking; the press reads purely through a deeper
   fill (`--ui-pressed`), like a table row. Press-scale stays on small
   buttons/rail items only.

It is **presentation-agnostic**: it only says "this is an action entry" and emits
a click / is a link (`as="a"` + `href` for external). The next surface is the
caller's choice. This is the component the page contract's "named entry point →
focused surface" pattern (SKILL §9/§12) was missing — reach for it instead of
stashing whole features behind an in-card "Advanced" disclosure, or re-skinning a
list row (`Item` / the app-layer `BackendCard`) whose meaning is "one of many
peers", not "one door to somewhere else".

## Reference status (new vs legacy)

> Confirm / extend this list with the maintainer before treating anything as gospel.

- **Reference (refactored — copy these):** Button, Input, Textarea, Select /
  SelectTrigger, NativeSelect, Checkbox, Switch, NumberField, InputGroup, Field,
  SegmentedControl, Toggle, ActionCard.
- **In progress / upcoming:** Slider, RadioGroup, Select menu surface, Combobox,
  PinInput, InputOTP, TagsInput (mid-refactor — their pre-refactor code is a
  textbook § Dirty patterns exhibit; check it in git, do not copy the in-flight
  state).
- **Legacy (do NOT use as reference):** Badge, Alert (semantic fills) — and any
  component not listed as Reference above. When in doubt, ask; do not
  pattern-match off legacy.
- **`components/sidebar/` (do NOT copy its styling):** the entire directory (23
  files) is an unmigrated shadcn-vue import — it hand-rolls its own button
  chrome and a separate focus-ring vocabulary instead of the Button/field-edge
  contracts above, and it is where the invalid `hsl(var(--sidebar-border))`
  shadow shipped (browser silently drops the whole declaration — see the
  `check-ui-contract.mjs` oklch-leftover rule). Production usage is limited to
  `settings-sidebar` (via `master-detail-sidebar-layout` → `settings-section` /
  `bots/detail.vue`). Copy patterns from the Reference list, not from here.

## Color

- Pull from the palette tokens in `style.css`. Never write a literal color in a
  component.
- **Selection blue is `--accent-blue-fill`** (`#2383e2`) — the single blue across
  Checkbox / Switch. Its ramp: `--accent-blue-fill-hover` (deeper) /
  `--accent-blue-fill-active` (deeper still).
- **Interaction overlay ladder** — pure neutral overlays, chroma 0, no hue.
  Composites over the underlying surface, inheriting its warmth automatically.
  Zero hue = zero color tilt across surfaces or rendering environments.
  Light mode = black overlays; dark mode = white overlays.
  - `--overlay-hover-light` (4.5% black / 5.5% white) — wide sidebar rows, menu highlights
  - `--overlay-hover` (7.2% / 9%) — standard controls (Toggle, Select, Checkbox)
  - `--overlay-hover-strong` (11% / 13%) — small icon ghost buttons; Toggle on state
  - `--overlay-active` (13.5% / 15.5%) — pressed state (standard controls)
  - `--overlay-active-strong` (17% / 19%) — pressed (small icons) / on+press
  Semantic aliases keep old names so component code is unchanged:
  `--ui-hover` · `--ui-selected` · `--ui-on` · `--ui-pressed` · `--ui-selected-pressed` ·
  `--btn-ghost-hover` · `--sidebar-hover` all point into this ladder.
  Color-scheme variants need **no hover overrides** — overlays are scheme-agnostic.
- **Accent palette** — 11 hues, each with a 6-role ramp: `--accent-{hue}` (icon/
  text, **state-constant**), `-soft` (rest bg), `-soft-hover`, `-soft-active`,
  `-border`, `-deep`. 3-layer model: a colored item's text/icon **never** changes
  on hover/select — only the background deepens `soft → soft-hover → soft-active`.
- **Charcoal CTA, scarce brand.** The high-emphasis button fill is the charcoal
  ramp (`--btn-primary` → `-hover` → `-active`); `variant="default"`/`"primary"`
  use it. Brand purple (`--brand`) is the SCARCE accent — it ships only via
  `<Button variant="brand">` (rare brand CTAs like chat Send) and a handful of
  reserved tokens (`--sidebar-primary`, capability/event accents). Never make
  purple a default surface or a large fill; the skeleton stays black/white/gray.
- **Destructive is filled, not ghost.** `variant="destructive"` renders a solid
  `bg-destructive text-destructive-foreground` button — a filled red CTA. Never
  use `variant="ghost"` with manual destructive text classes (`text-destructive`,
  `hover:bg-destructive/10`) as a substitute; that is the OLD pre-refactor ghost
  pattern. If you need a low-emphasis destructive affordance (e.g. a secondary
  action in a footer), use `variant="outline"` with the destructive class as a
  deliberate exception, not the default.
- **Selection is one blue, applied as fill.** Checkbox, Switch, Radio (checked
  edge) and the Slider range all use `--accent-blue-fill` — the chosen value
  reads as a solid blue fill, not a border. (Hover differs by control: Checkbox
  deepens, Switch brightens.)

## Interaction model (hover / press / focus)

- **Press-scale lives on a `::before` shell, never on the element box**, so the
  press shrink only moves the background — the label/icon **never** moves or
  blurs. This is THE rule behind "icon must not resize on press." See
  `[data-variant]::before` blocks in `style.css`.
  - primary/default: `scale: 0.96` on press (block/full-width instead flips fill
    to `--btn-primary-active` instantly).
  - secondary/outline & ghost: `scale: 0.974` on press.
  - **Full-width (`data-block`) ghost** drops the scale too — a uniform scale on a
    wide button lurches sideways (each edge travels ~width × 2.6%), so it reads the
    press through a deeper fill (`--ui-pressed`, 40ms) instead, mirroring the
    primary-block rule. Covers the sidebar's wide New Chat / Bot Settings / Settings
    rows.
- **Buttons** reuse `<Button>` — including small in-field icon buttons (clear /
  reveal / steppers) via `variant="ghost"` / `InputGroupButton`. Do **not**
  hand-roll an icon-hover background; that is the canonical bug.
- **Fields engage on FOCUS only, never hover** — a text field is a container you
  click into; hover-darkening reads as twitchy. The field edge is one inset 1px
  hairline whose color swaps per state (never an outer ring):
  `--field-edge-rest` → `--field-edge-solid` (focus) / `--field-edge-engaged`
  (subtle) / `--destructive` (invalid).
- **Focus ring** is `--ring` via `focus-visible:ring-2 ring-ring/<alpha>` (or the
  segmented's 2px-track + 4px-ring inset). Keyboard focus only — not a resting
  border. Controls that keep DOM focus after a mouse action (Select trigger,
  native select, Slider thumb) deliberately ship NO focus style — a ring would
  linger as a "stuck" edge.
  - `--ring` is a MID-GRAY (~0.575L), deliberately soft — keyboard focus should
    read as a quiet halo, never a harsh outline. The loud near-black edge
    (`--field-edge-solid`) is a SEPARATE, rationed treatment that belongs only to
    the field-commit semantic (see § Borders & field edge); do not reach for it
    as a general focus/emphasis ring.

## Highlight vs selection (menus, lists, rows)

Two DIFFERENT states with two different expressions — do not conflate them:

- **Highlight** = the transient pointer / keyboard cursor resting on a row. Shown
  by a background tint (`data-[highlighted]:bg-[color:var(--ui-selected)]`, reka's
  roving focus). It just follows the pointer. Single source: `lib/menu.ts`
  `menuItemClass` — Select / Dropdown / Context / Command all reuse it; no
  per-component highlight rule.
- **Selection** = the actually-chosen value. Shown by an INDICATOR (check / dot),
  **never** by a persistent row background.
- Table rows are the exception that proves it (no indicator slot): they tint
  `hover:--ui-hover` / `data-[state=selected]:--ui-selected`.
- `<Item>` interaction is OPT-IN: hover/press fills arm only when the row is a
  real control (`as="a"` / `as="button"`); selection still goes through an
  indicator, not a background.

## Selected state

A THIRD state, distinct from both of the above: a control that IS its own
persistent choice, with **no separate indicator slot** — the control's own
chrome has to show which one is current (a theme-picker button, the sidebar's
current-session row). Token pair: `--selected-bg` (`--overlay-hover-strong`,
one ladder step above `--ui-hover`/`--sidebar-hover` — an idle selected control
must always read deeper than a merely-hovered one) and `--selected-border`
(`--foreground`, button chrome only). Carrier: the `[data-ui-selected]` attribute,
never a page-injected utility class onto the control box. The `ui-` prefix is
load-bearing, twice over: a bare `data-selected` is reka-ui's own attribute
(emitted natively on CalendarCellTrigger / PaginationListItem / TreeItem — a
global rule on it restyles calendar range days and pagination chips), and
`--ui-selected` the *token* is already taken by the menus' roving highlight
above. Both collisions are real; do not "simplify" the name in either
direction.

- **`[data-button]` (outline/secondary variants)** — `[data-ui-selected]` swaps the
  resting fill/ring on the same `::before` shell that already owns hover/press
  (`style.css`), so which one wins when a selected control is also hovered or
  pressed is decided by the cascade engine, not by two layers of utility
  classes fighting for the box. This replaces the old pattern of injecting
  `border-foreground bg-accent text-foreground` onto a `<Button>` from the
  page — that pattern silently fought the hover chrome for the same pixels;
  data-driven chrome can't.
- **Non-button surfaces** (sidebar rows, list rows not wearing `<Button>`) — a
  standalone `[data-ui-selected]` rule applies `--selected-bg` with no
  ring/border; these rows never had one.
- **vs Highlight / indicator-selection (§ above)** — those are for rows that
  DO have an indicator slot (menu items, Select/Command rows): the highlight
  just follows the pointer, the chosen value shows via a check/dot, never a
  persistent background. `--selected-bg` is for controls with no indicator to
  fall back on. Do not reach for `--ui-selected` here — that name is already
  the roving highlight, a different state.
- **vs Pressed** — `--ui-pressed`/`:active` is transient, gone the instant the
  pointer releases. Selected is durable until the user picks something else. A
  control can be both at once (pressing a currently-selected button); the
  chrome engine's cascade decides which reads through, not a hand-tuned class.
- **vs active-route** — `nav-button.vue`'s `active` prop (the Settings row,
  keyed off `route.path`) and the sidebar's horizontal view-switcher tabs
  (`data-[active=true]`, keyed off in-memory view state) are a SEPARATE,
  not-yet-migrated concept: both still render on the plain `--sidebar-accent`
  token via a hand-written class, not `[data-ui-selected]`. Do not casually fold
  them into `--selected-bg` — the sidebar current-session row was deliberately
  moved one ladder step deeper than `--sidebar-accent` (see `session-item.vue`)
  precisely so "this is the session I'm in" reads stronger than "this is the
  nav section I'm in"; unifying the tokens would either flatten that signal or
  deepen the nav/tab active state without a design call. If route-level or
  view-switcher "active" ever gets its own legislation, that's a follow-up
  decision, not a silent side effect of this one.

## Radius

- Use the scale only: `rounded-2xs/xs/sm/md/lg/xl` = `--radius-*`
  (3 / 4 / 6 / 8 / 10 / 14 px; `--radius` = 10px), plus the menu pair
  `--radius-menu` (8) / `--radius-menu-shell` (12).
- Role map (the values components actually land on):
  - Badge / tag / Tooltip / Kbd → `rounded-sm` (6).
  - Controls (button / field / Select / Toggle) and menu rows → `rounded-md` /
    `rounded-menu` (8).
  - Chromed Popover & menu shell → `rounded-menu-shell` (12).
  - Card / Dialog / Sheet → `rounded-xl` (14). (The sizing bench explored
    card 12 / dialog 16, but shipped components consolidate on 14 — follow the
    component, not the bench.)
  - Avatar → `rounded-full`.
- In-field small controls (InputGroup clear/reveal, NumberField steppers) share
  one tuned radius: `rounded-[calc(var(--radius)-5px)]` (5px) — the only allowed
  arbitrary radius (allowlisted in the guard). Smaller than 8px on purpose so a
  ~24px box does not read as a pill.

## Borders & field edge

Borders come in THREE role families — never cross them (putting a structural
border on a control body is a top cause of the "dirty" look):

- **Control edge** — an alpha hairline that melts into the inherited fill:
  `--field-edge-*` (fields / Select trigger) and `--border-hairline`
  (secondary/outline button, Checkbox, Radio, Slider thumb). 1px, changes in
  place. `--shadow-hairline` is this edge expressed as an inset shadow so it can
  animate with hover.
- **Structural edge** — solid neutral `--border`: containers & dividers (Card,
  Accordion, Table row, Alert, separators). Do NOT put `--border` / `border-input`
  on a control body — it reads heavy and dirty.
- **Floating edge** — `--border-menu` (dropdown / select / chromed popover panels)
  and `--border-menu-elevated` (modal surfaces).

Fields use the `--field-edge-*` contract: the edge changes color IN PLACE
(`rest → solid` on focus / `engaged` subtle / `--destructive` invalid). **Never
stack an outer ring on a field, and never grow the border width on focus** (the
old `focus:border-2` + `z-10`/`relative` reflow fix is the anti-pattern).

**The near-black edge (`--field-edge-solid`, ~0.78 alpha) is a scarce, deliberate
accent — not a generic "make it pop" border.** It is loud on purpose and belongs
to ONE semantic: a FIELD committing to focus (Input / Textarea / Select trigger /
InputGroup / NumberField, plus the field-family PinInput / InputOTP / TagsInput).
That input focus look is an intentional design choice, not a default to scatter.
Everywhere else, reach for the softer rungs first — the subtle
`--field-edge-engaged`, the gray `--ui-*` interactive ladder, or the keyboard
focus ring `--ring` (which is a MID-GRAY ~0.575L, **not** black — see Interaction
model). Using the near-black edge as a hover / selected / emphasis treatment
outside the field-commit semantic needs an explicit, case-by-case reason; when in
doubt, default to gray. (Like purple-scarcity, this is a judgment rule — loud
high-contrast chrome is rationed, not guard-enforced.)

Whether a surface even HAS a border is a CONTRAST decision: a Card / dropdown over
the page gets one; a modal over the dark scrim uses `--border-menu-elevated` =
NO border in light (a white panel + scrim + shadow already separate; a dark
hairline the same darkness as the scrim only muddies it) and a white hairline in
dark. Tooltip carries no border at all — its solid fill is its own edge.

## Elevation / shadow

- **Flat by default.** `@layer utilities { .shadow-sm { box-shadow: none } }`
  zeroes Tailwind's `shadow-sm` on purpose — shadow is a scarce, tokenized
  elevation ladder, not decoration.
- The tokenized ladder (use a token; never write `box-shadow: 0 …` inline or a
  raw `shadow-xs/md/lg` utility):
  - `--shadow-hairline` — the 1px inset edge for secondary/outline buttons (a
    shadow only so it can animate with hover; not a drop shadow).
  - `--shadow-thumb` — the faint lift on the SegmentedControl sliding thumb.
  - `--shadow-dropdown` — floating menus (Dropdown / Select / chromed Popover);
    strong negative spread so the cast sits below the panel and barely bleeds out
    the sides.
  - `--shadow-modal` — the modal layer (Dialog / Sheet / CommandDialog).
- Flat controls and Cards carry NO shadow. Tooltip carries none. A `shadow-none`
  fighting an inherited shadow, or an invented `shadow-xs`, is the dirty tell.

## Layering / z-index — the "z 梯"

- **Five semantic tiers, not eleven ad-hoc numbers.** Before this ladder the
  codebase had 26 files, 45 call sites, and 11 distinct raw values (`z-10`,
  `z-20`, `z-30`, `z-40`, `z-50`, `z-[100]`, `z-[1]`, `z-[2]`, raw
  `z-index: 0/1/9999`) with no relationship to each other — every new floating
  element guessed a number. Tokens are defined in `style.css`'s `@theme
  inline` block; each token's VALUE is whatever number already dominated that
  role pre-ladder, so adopting one is a pure rename, never a re-ranking.

  | Token | Value | Role |
  |---|---|---|
  | `--z-raised` | 10 | Local elevation: sticky top-fades, resize rails, overlay badges |
  | `--z-sticky` | 20 | Sticky bars and hand-rolled popovers/rails within a panel |
  | `--z-panel` | 30 | Panel-level floats: composer, minimap, app-level view switches |
  | `--z-overlay` | 50 | Every Radix floating layer: dialog/sheet/popover/tooltip/menu |
  | `--z-top` | 100 | Lightbox / full-screen top layer |

- **Consume via the Tailwind v4 CSS-variable shorthand**: `z-(--z-overlay)` in
  a class list (same pattern as this codebase's existing
  `origin-(--reka-popover-content-transform-origin)` / `w-(--sidebar-width)`),
  or `z-index: var(--z-overlay);` in raw CSS.
- **There is no "in-between" tier.** Wanting a value between two tiers is a
  design question — pick a tier, don't invent a number. `toast` is the one
  intentional exception: it stays a raw `z-index: 9999` (see
  `.memoh-toaster` in `style.css`) specifically so it wins over the
  `--z-top` lightbox too, which a shared tier value can't guarantee once two
  Teleport-rendered layers tie.
- **Context-internal ordering is NOT on this ladder.** A handful of `0`/`1`/`2`
  values (and dockview's own `--dv-overlay-z-index` fallback) only order an
  element against its OWN children/siblings inside one component's stacking
  context — never against another component's z-index. Forcing one of these
  onto a global tier would claim a rank it doesn't have. Each is marked
  `ui-allow-z` at the call site with a written reason (see
  `[data-segment-thumb]` in `style.css`, `.tabs-thumb` in `TabsList.vue`, the
  tab paint order in `dockview-theme.css` / `workspace-tab.vue`, and the
  focus-ring lift in `button-group/index.ts`) — copy that pattern, don't grep
  a token onto them.
- **Guard**: `check-ui-contract.mjs` HARD-fails a bare `z-10/20/30/40/50` or
  `z-[N]` utility in both `packages/ui` and `apps/web` (no baseline — the
  ladder started this migration at zero and stays there) and WARNs on any raw
  `z-index: N` CSS literal outside `var()`. Mark `ui-allow-z` (with a reason)
  for a genuine context-internal exception; there is no other escape hatch.


## Typography

- Size: the `--text-*` scale only (line-height + tracking baked in):
  `text-caption` 11 · `text-body` 12 (workhorse) · `text-label` 13 ·
  `text-control` 14 · `text-title` 16 · `text-heading` 18 · `text-display` 24.
- Weight is **role-mapped** (single source — do not free-style):
  `font-semibold` → surface/section titles · `font-medium` → labels, button text,
  badges, emphasis · `font-normal` → body, descriptions, field values, placeholder.

## Sizing & icons

- Control height ladder: `sm` h-8 (32) · default h-9 (36) · `lg` h-10 (40);
  icon-only buttons `icon-sm` 32 / `icon` 36 / `icon-lg` 40.
- Icon glyph sizes scale with the control on ONE shared ladder — don't free-set
  icon sizes, pick the rung: default control **16px** (`[&_svg]:size-4`); small
  in-field controls **14px** (`size-3.5`); text-scale affordances (TextButton /
  breadcrumb crumbs) and badges **12px** (`size-3`, one notch under the 14px text so
  the glyph reads quietly rather than as a chunky control icon). TextButton does NOT
  get a bespoke icon standard — it is just the bottom rung of this same ladder.
- **Icons are always components, never literal text glyphs.** Use a
  `lucide-vue-next` component (`<Plus/>`, `<X/>`, `<ChevronDown/>`) — never a typed
  character (`"+"`, `"×"`, `"▾"`, `"✓"`) standing in for an icon. A glyph is just
  text: it can't receive `[&_svg]:size-4` / the 16px control sizing or the 2px
  lucide stroke, so it renders tiny and visually inconsistent inside a 32–40px
  button (this is exactly the "+ looks lost in the hover button" bug). It also
  carries no semantics. The container's `[&_svg]:size-4` / `size-3.5` rules only
  reach real `<svg>` children, which is another reason the component form is the
  contract. (Real text content — labels, the Kbd `/`, `⌘` — stays as text.)
- Icon-only buttons (no visible label) MUST carry an `aria-label` for the action.
- Decorative / status icons use `--accent-{hue}` and are **state-constant** (color
  does not change on hover/select).

## Scale with the font — rem on anything that touches text

The root sets `font-size: var(--memoh-ui-font-size, 1rem)`, so the UI font-size control
and browser zoom resize the whole UI **through rem**. A hardcoded `px` does NOT scale —
so a px value on a property that gates text stops growing while the text around it
grows (clipped controls, cramped rows, same-row controls that stop matching height).

- **Anything that touches text is rem / a token, never px.** Font size = the `--text-*`
  scale (`text-body` / `text-label` / `text-control` / …), never `text-[Npx]`.
  Line-height = rem or unitless, never `leading-[Npx]`. Control height, padding, and the
  gaps between text use the rem spacing scale (`h-9` / `p-4` / `gap-2`) or a rem
  arbitrary value — never `h-[Npx]` / `p-[Npx]` / `gap-[Npx]` (≥5px).
- **px is only for non-text decoration:** 1–4px hairlines / indicator bars, plus
  `border-*` / `ring-*` / `outline-*` / `translate-*` / `inset` offsets, icon `size-*`,
  and blur. Widths / caps (`w-*` / `max-h-*`) are a reflow concern, left to review.
- **JS layout must track rem too.** A virtualizer `estimateSize`, a minimap row stride,
  or a scroll-anchor offset hardcoded in px desyncs when the font scales — derive it
  from the root font size or measure the element (`measureElement`).
- **Enforced by the guard.** `scripts/check-ui-contract.mjs` (run by `mise run lint`)
  HARD-fails text-coupled px across `packages/ui` **and** `apps/web`. Existing app-page
  debt is grandfathered by `scripts/ui-px-baseline.json` — a ratchet: new px is blocked,
  the baseline only shrinks. A genuine exception (e.g. a fixed chart-canvas height) opts
  out with a `ui-allow-px` comment on the same line.

## Disabled

- `opacity-40` everywhere for the disabled state (no muddy gray fill, no color
  swap). Loading buttons are the exception — they hold full color (the spinner is
  the signal).

## Pointer Cursor

- Every interactive control sets `cursor-pointer` (Button, Switch, segmented
  item, …). Disabled flips to `cursor-not-allowed`.

## Dirty patterns — anti-examples (do NOT copy)

A control reads "dirty" when it STACKS chrome instead of changing one layer in
place (see § The one rule). These eight are red lines — each roughly means "you
copied an un-refactored component." The pre-refactor PinInput / InputOTP /
TagsInput were textbook exhibits; their code is in git history (don't copy the
in-flight state either — confirm against the rules above).

1. **Baked fill on a control body** — `bg-background` / `bg-input` /
   `dark:bg-*/30` / `bg-secondary` as a tag fill. Be transparent and inherit the
   surface; a baked fill renders grayer than its `bg-card` parent → "inside ≠
   outside."
2. **Structural border on a control** — `border-input` / `border-border` on a
   field or slot. Use the control-edge family (`--field-edge-*` /
   `--border-hairline`).
3. **Focus that grows or stacks the edge** — `focus:border-2`,
   `focus-within:ring-2`, a colored `border-ring` growth, plus `z-10`/`relative`
   to patch the reflow. Swap the field edge to `--field-edge-solid` IN PLACE.
4. **Invalid via an outer ring** — `aria-invalid:ring-destructive/20`. Swap the
   edge to `--destructive` in place instead.
5. **Invented shadow** — a raw `shadow-xs/md/lg` utility, or a `shadow-none`
   fighting an inherited one. Flat controls carry no shadow; elevation is a token.
6. **Hand-written alpha** — `/20` `/30` `/40` `/80` on fills/rings (the sanctioned
   exception is the `ring-ring/<alpha>` focus ring). Everything else is a token.
7. **Selection / active via `ring-offset-*`** — a background-gap halo (pure
   shadcn). Use an indicator or the `--ui-selected` fill (see § Highlight vs
   selection).
8. **Off-scale / mismatched radius** — a control on `rounded-lg`, a tag not on
   `rounded-sm`, a bare `rounded`. Use the § Radius role map.

The guard WARNs on red lines 2 / 5 / 7 (string-detectable); the existing HARD
checks already cover raw color, arbitrary radius, and invented box-shadow.

## Alpha policy

Red line 6 above ("hand-written alpha is a dirty pattern") existed as prose with
no mechanical teeth: the guard's raw-color regex only catches bracket/hex
literals (`bg-[#fff]`, `text-[rgba(...)]`), not Tailwind's built-in `/NN` suffix
on a semantic color name (`text-foreground/92`, `bg-muted/40`). That blind spot
let hand-picked alpha values accumulate — the same visual role (a faint divider,
a muted label, a soft destructive banner) re-derived independently at each call
site, each with its own guessed percentage, silently drifting apart.

**The rule: transparency is a token, same as color.** If a component or page
needs a translucent tint of a semantic color, it reaches for a `-soft` /
`-border` / `-muted` token (below), never a raw `/NN` suffix. Need a shade this
system doesn't have? Add the token (color-mix off the base semantic color, so
it tracks `.dark` / per-scheme overrides automatically — see the token
definitions in `style.css` for the pattern), then use it. Don't invent the
percentage inline.

**Allowlist — the suffix is legitimate here, not a violation:**

- `ring-ring/<alpha>` — the focus ring (pre-existing exemption, § Interaction model).
- Overlay scrims (`bg-black/50`, lightbox chrome) — a full-bleed dimming layer
  behind a modal/lightbox is its own role, disconnected from any semantic
  surface color; forcing it onto a token family built for tinting a surface
  would be the wrong abstraction. Still hand-written on purpose, not debt.
- The neutral **interaction overlay ladder** (`--overlay-hover`, `--ui-pressed`,
  etc., § Color) — already tokens, already alpha-based; nothing to migrate.

**Soft-tier tokens (first batch — color-mix, not hand-picked, so each is
PIXEL-IDENTICAL to the `/NN` call sites it replaces):**

| Token | Formula | Replaces | Role |
|---|---|---|---|
| `--muted-soft` | `color-mix(in oklab, var(--muted) 40%, transparent)` | `bg-muted/40` | Neutral hint/tip box background (the missing soft rung for `--muted`, parallel to `--success-soft` etc.) |
| `--border-soft` | `color-mix(in oklab, var(--border) 60%, transparent)` | `border-border/60` | De-emphasized divider or card edge — softer than the structural `--border` |
| `--destructive-soft` | `color-mix(in oklab, var(--destructive) 5%, transparent)` | `bg-destructive/5` | Destructive banner/callout background fill (the gap `--success-soft`/`--warning-soft`/`--info-soft` had no destructive counterpart for) |
| `--destructive-border` | `color-mix(in oklab, var(--destructive) 30%, transparent)` | `border-destructive/30` | Destructive banner/callout border, and a destructive badge/status outline |

Each is registered in the `@theme inline` block so it composes as an ordinary
utility: `bg-muted-soft`, `border-border-soft`, `bg-destructive-soft`,
`border-destructive-border`.

(`text-foreground/92` — the sidebar row label — was considered for a fifth
token, but its only LIVE call site is `SidebarNavButton`'s `rowClass`, already
the single owner the header comment documents consolidating 3 prior
duplicates; two apparent siblings in `panel-sessions.vue` sit inside a
`<!-- archived experiment -->` comment block, not rendered markup. One
consumer doesn't earn a global token — the line keeps its `/92` with a same-line
`ui-allow-alpha`, consistent with the `ui-allow-style`/`ui-allow-px` markers
already on it for the same "reviewed, owned exception" reason.)

Not every `/NN` found in the codebase belongs to one of these four yet — some
are single-occurrence (no repeated role to name a token after), some need a
design call on which value is canonical when two or three percentages compete
for the same role, and some are chat-surface files intentionally left alone
pending a larger revamp. Those stay hand-written for now, grandfathered by the
guard's ratchet (below) so they don't block on this pass, but also can't grow.

**Guard**: `scripts/check-ui-contract.mjs` HARD-fails a semantic-color `/NN`
suffix (`bg|text|border|divide|ring|shadow|from|to|via` on
`muted|accent|border|foreground|background|destructive|warning|primary|success|info|card|popover|sidebar*`)
in both `packages/ui` and `apps/web`, except the allowlist above. Existing debt
is grandfathered in `scripts/ui-alpha-baseline.json` — a ratchet, same shape as
the px / app-injection / hand-spun-loader baselines: a file keeps its recorded
count, adding more (or a brand-new file) HARD-fails. Regenerate after a cleanup
pass with `node scripts/check-ui-contract.mjs --write-baseline`.

## Motion & Tailwind v4 gotchas

- **Tailwind v4 maps `translate-x/y`, `scale`, `rotate` to the standalone CSS
  properties `translate` / `scale` / `rotate` — NOT `transform`.** So a
  `transition: transform …` will NOT animate a `translate-x-*` change (it snaps).
  Transition the **actual** property: `transition: translate …`. (This bit the
  Switch thumb — it jumped because the transition targeted `transform`.) The
  `style.css` interaction blocks already animate `translate` / `scale` directly —
  follow that, do not assume v3 `transform`.
- Duration palette in use: field edge ~70ms · switch ~110ms · button color
  ~150ms · toggle release 160ms / press 40ms · accordion reveal 180ms · AutoHeight
  swap 220ms · button press-scale 255ms (springy `linear(...)`) · segmented thumb
  250ms. Keep new motion within this range and sync co-moving properties (e.g.
  switch track color + thumb glide both 110ms).
- **`AutoHeight` — height tween for content swaps.** When a container's content
  changes size in place (a dialog's list↔form view swap, a section revealing
  fields), wrap it in `<AutoHeight>` so the height animates (220ms, house spring
  `cubic-bezier(0.32, 0.72, 0, 1)` — a straight ease reads mechanical at this
  size) instead of hard-cutting. It measures its slot via `ResizeObserver` and
  sets an explicit px height on a `overflow-hidden` root; first paint does not
  animate (reveals as a plain cut, like `SwapTransition`), and
  `prefers-reduced-motion` drops the transition. **It is NOT the scroll
  container** — its clip would trap overflow with no scrollbar; keep the
  `overflow-y-auto` element as a separate ancestor and put `AutoHeight` inside
  it. Popovers/selects inside the slot portal to `<body>`, so they are never
  clipped.
- **`DialogPanel` — the capped focused-dialog shell.** NEVER hand-write
  `max-h-[80dvh] grid-rows-[auto_minmax(0,1fr)] sm:max-w-*` onto a
  `DialogContent` — that recipe string is exactly what this component
  encapsulates (dropping the `minmax(0,1fr)` overflows the cap; keeping the
  corner close in a view-swap dialog lands it ~12px off the title
  centerline). Knobs: `width` (`'2xl'` default · `'3xl'` editor-heavy),
  `grow` (fixed `h-[80dvh]` for bodies with no intrinsic height — editors),
  `view-swap` (header is a `DialogViewHeader`, so the built-in corner close
  is disabled — the pairing is enforced by the prop, not by memory),
  `footer` (adds the third grid row for a `DialogFooter`).
- **`DialogBody` — the scrollable body row of a capped dialog.** Inside a
  `DialogPanel`, the body row is `<DialogBody>`: it owns the `-mr-3 pr-3`
  scroll gutter AND a
  stateful scroll-edge fade — content continuing past the box fades out over
  the last 16px, and each edge only fades while more content exists in that
  direction (top of scroll → no top fade; bottom reached → no bottom fade;
  short content → no fade). The fade distance is a registered `@property`
  `<length>` so its appearance eases (200ms) instead of popping;
  `prefers-reduced-motion` drops the transition. Do NOT hand-write
  `[mask-image:linear-gradient(...)]` scroll fades in dialogs — that mask is
  static (fades even at the scroll ends); use this. Nest `AutoHeight` INSIDE
  it, never the other way (AutoHeight can't scroll, DialogBody can't clip a
  height tween).
- **`DialogViewHeader` — back / title / close on ONE centerline.** For a
  dialog that swaps views (list ↔ drill-in form) or any header where the
  built-in corner close (`top-3`) would sit off the title's centerline. Pair
  it with `DialogPanel view-swap` (which disables the corner close); this
  header renders the close inline (kept `relative`, NEVER `static` — the
  ghost hover
  `::before{inset:0}` needs the button as containing block or it tints the
  whole dialog). `centered` + `showBack` are driven by the CALLER's predicate:
  a centered title must be anchored on both flanks (back chevron, or body
  weight below) — bare empty states flush left. Title is the default slot;
  `backLabel` takes the localized aria-label; emits `back`.



## Framework versions (do not assume older)

- **Tailwind CSS v4** (`@theme` / `@layer` / `@custom-variant`; the v4 transform
  split above). Not v3 — there is no `tailwind.config.js` utility theme.
- **Vite 8**, **Vue 3** `<script setup>`, **reka-ui** primitives (props like
  `as-child`, `data-state`, pointer-driven steppers — not click), **vee-validate**
  for form wiring.
- Utilities layer wins over `@layer components`; when a component-layer rule must
  beat a utility, it uses `!important` deliberately (see link variants). Don't add
  a conflicting Tailwind utility on top of a `style.css`-owned property (e.g. the
  Switch had a stray `transition-colors` that overrode the 80ms track timing).

## Open migration debt

The raw-value debt that predated this contract is now **fully migrated** — the
library has zero raw colors / invented shadows outside token blocks, so guard
rules 5/6/7 run as HARD failures. Tokens added in that pass:
`--accent-blue-foreground`, `--btn-secondary-overlay`, `--segment-overlay-hover`,
`--segment-overlay-active`, `--control-label`, `--control-label-hover`,
`--btn-destructive-hover-bg`, `--btn-destructive-hover-text`.

Remaining (non-blocking) TODOs:

- Toggle `tint` active uses `--brand` (consider an accent token).
- Dark-mode accent palette + elevation currently inherit light values.
- SegmentedControl disabled uses CSS `opacity: 0.5`; the contract says 40 — unify
  to `0.4` if/when the maintainer agrees it's not part of its personality.

## Extending this contract

When you lock a new cross-cutting decision (a color role, a duration, an icon
rule, a shape law): (1) add/identify the token in `style.css`, (2) document it
here, (3) add a guard check in `scripts/check-ui-contract.mjs` if it is
mechanically detectable. A decision that is not written here will be re-invented.
