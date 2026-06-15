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

## Reference status (new vs legacy)

> Confirm / extend this list with the maintainer before treating anything as gospel.

- **Reference (refactored — copy these):** Button, Input, Textarea, Select /
  SelectTrigger, NativeSelect, Checkbox, Switch, NumberField, InputGroup, Field,
  SegmentedControl, Toggle.
- **In progress / upcoming:** Slider, RadioGroup, Select menu surface, Combobox,
  PinInput, InputOTP, TagsInput (mid-refactor — their pre-refactor code is a
  textbook § Dirty patterns exhibit; check it in git, do not copy the in-flight
  state).
- **Legacy (do NOT use as reference):** Badge, Alert (semantic fills) — and any
  component not listed as Reference above. When in doubt, ask; do not
  pattern-match off legacy.

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

## Motion & Tailwind v4 gotchas

- **Tailwind v4 maps `translate-x/y`, `scale`, `rotate` to the standalone CSS
  properties `translate` / `scale` / `rotate` — NOT `transform`.** So a
  `transition: transform …` will NOT animate a `translate-x-*` change (it snaps).
  Transition the **actual** property: `transition: translate …`. (This bit the
  Switch thumb — it jumped because the transition targeted `transform`.) The
  `style.css` interaction blocks already animate `translate` / `scale` directly —
  follow that, do not assume v3 `transform`.
- Duration palette in use: field edge ~70ms · switch ~110ms · button color
  ~150ms · toggle release 160ms / press 40ms · button press-scale 255ms (springy
  `linear(...)`) · segmented thumb 250ms. Keep new motion within this range and
  sync co-moving properties (e.g. switch track color + thumb glide both 110ms).

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
