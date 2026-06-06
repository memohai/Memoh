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

## Unification ≠ homogenization

Unifying the system does **not** mean every control looks the same. A component
MAY have its own personality — a unique shape, elevation, or motion (e.g. the
SegmentedControl's elevated sliding thumb). The rule is only that the personality
must be expressed **through tokens**, never raw values. Distinctive is fine; raw
numbers are not.

## Reference status (new vs legacy)

> Confirm / extend this list with the maintainer before treating anything as gospel.

- **Reference (refactored — copy these):** Button, Input, Textarea, Select /
  SelectTrigger, NativeSelect, Checkbox, Switch, NumberField, InputGroup, Field,
  SegmentedControl, Toggle.
- **In progress / upcoming:** Slider, RadioGroup, Select menu surface, Combobox.
- **Legacy (do NOT use as reference):** Badge — and any component not listed as
  Reference above. When in doubt, ask; do not pattern-match off legacy.

## Color

- Pull from the palette tokens in `style.css`. Never write a literal color in a
  component.
- **Selection blue is `--accent-blue-fill`** (`#2383e2`) — the single blue across
  Checkbox / Switch. Its ramp: `--accent-blue-fill-hover` (deeper) /
  `--accent-blue-fill-active` (deeper still).
- **Interactive surface gray ladder** — the single source for hover/selected/
  press tints (Toggle, Ghost button, SegmentedControl, menu items):
  `--ui-hover` (lightest) · `--ui-selected` · `--ui-pressed` ·
  `--ui-selected-pressed`. Dark mode lightens instead of darkens.
- **Accent palette** — 11 hues, each with a 6-role ramp: `--accent-{hue}` (icon/
  text, **state-constant**), `-soft` (rest bg), `-soft-hover`, `-soft-active`,
  `-border`, `-deep`. 3-layer model: a colored item's text/icon **never** changes
  on hover/select — only the background deepens `soft → soft-hover → soft-active`.

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
  border.

## Radius

- Use the scale only: `rounded-2xs/xs/sm/md/lg/xl` = `--radius-*`
  (3 / 4 / 6 / 8 / 10 / 14 px; `--radius` = 10px).
- Controls (buttons, fields) are `rounded-md` (8px).
- In-field small controls (InputGroup clear/reveal, NumberField steppers) share
  one tuned radius: `rounded-[calc(var(--radius)-5px)]` (5px) — the only allowed
  arbitrary radius (allowlisted in the guard). Smaller than 8px on purpose so a
  ~24px box does not read as a pill.

## Borders & field edge

- Borders only from `--border` / `--border-hairline` (and `--shadow-hairline` =
  the inset 1px chrome for secondary/outline buttons & checkbox).
- Fields use the `--field-edge-*` contract above. Never stack an outer ring on a
  field — the edge changes color in place.

## Elevation / shadow

- Shadows are tokenized so elevation never fragments: `--shadow-hairline`
  (1px inset edge), `--shadow-thumb` (subtle lift for the segmented sliding
  thumb). Use a token; do not write `box-shadow: 0 1px 2px rgb(...)` inline.
- Most controls are flat. Do not add shadow for decoration.

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
- Icon glyph sizes scale with the control: default **16px** (`[&_svg]:size-4`);
  small in-field controls **14px** (`size-3.5`). Don't free-set icon sizes.
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

## Disabled

- `opacity-40` everywhere for the disabled state (no muddy gray fill, no color
  swap). Loading buttons are the exception — they hold full color (the spinner is
  the signal).

## Cursor

- Every interactive control sets `cursor-pointer` (Button, Switch, segmented
  item, …). Disabled flips to `cursor-not-allowed`.

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
