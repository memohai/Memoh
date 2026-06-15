---
name: memoh-ui-refactor
description: Refactor an apps/web page into the new white-floating-card design language with disciplined @memohai/ui usage, deliberate copy, honest empty states, aligned controls, and restrained motion. Use when redesigning, refactoring, polishing, or building any settings/list/detail page in apps/web (e.g. a not-yet-refactored bot tab, a provider list, a settings surface), or when a page feels "dirty" / off-language and needs to match the refactored Overview / Appearance / About / Web Search pages.
---

# Memoh Web — Page Refactor & Design Language

## Non-negotiables — read this even if you skim the rest

The seven rules that break a page if you miss them. The rest of this file is the *why* and
the *how*; these are the *must*.

1. **Copy the new, never the legacy.** Open a refactored reference page (§ Reference map in
   `reference.md`) and mirror it. Never pattern-match off a dirty / un-refactored page.
2. **A refactor must not regress.** *Before* touching anything, inventory every behavior,
   feature, state, and path the old page has — the new page keeps all of them unless you
   deliberately drop one and say so.
3. **Never hand-write a color — tokens only.** `bg-card` / `text-foreground` / `border-border`,
   etc. Dark mode is purely the result of this, and **nothing lints raw colors in app pages**,
   so one `bg-white` / `#hex` / `text-gray-*` ships visibly broken in dark. For any hover /
   selected / pressed / subtle tint, use the neutral **overlay ladder** (`--ui-hover` /
   `--ui-selected` / `bg-accent`) — never a gray or a `/10` alpha.
4. **Build from the shared shell + primitives.** Centered `max-w-3xl` with gutters;
   `SettingsSection` / `SettingsRow` white cards — one hairline, role-map radius, inset
   dividers, deliberate spacing rhythm, and **no hover-rise** on cards.
5. **Use the real `@memohai/ui` components** (Select / Combobox / Tooltip / icon `Button` /
   `Empty`). Never re-skin one or hand-roll an equivalent.
6. **Earn every word and every block.** Cut copy that doesn't guide; hide blocks that aren't
   actionable; empty *and* loading states must still draw their frame (no layout jump).
7. **You are not done until you verify the *rendered* page:** grep for raw colors → flip to
   **dark** → shrink to **narrow + `zh`** → walk **every old interaction** → `mise run lint`.

---

This skill is the **page-level** companion to the **atom-level** contract in
`packages/ui/AGENTS.md`. That file governs how a single control looks; this file
governs how you compose controls into a page that reads like the already-refactored
surfaces (Overview, Appearance, Profile, About, Web Search) and never like the
legacy ones.

It exists because the refactor kept slowing down: each page re-derived the same
decisions from scratch and re-made the same mistakes. The point of this skill is to
make that experience reusable — so refactoring "the next page" is a procedure, not
a re-invention.

## Prime directive

> **Copy the new language. Never copy the legacy.** When unsure how something should
> look or behave, open a refactored reference page and mirror it — do not pattern-match
> off an un-refactored page (even one you are mid-refactor on).

Two non-negotiable first steps before you touch a page:

1. **Read `packages/ui/AGENTS.md` in full.** It is the law for tokens, radius, borders,
   color, motion, and the "clean vs dirty" rule. This skill assumes it.
2. **Open one refactored reference + the page you're replacing side by side.** See
   `reference.md` § Reference map for which page to copy for each page shape, and the
   dirty→clean table for diagnosing what to strip.

## A refactor is behavior-preserving — interrogate what it breaks

The Prime directive covers the *look*; this covers the *behavior*. Changing how a page looks
must not silently change what it does. The most common refactor failure is not an ugly page —
it's a page that quietly **lost** an affordance that was buried in the old messy layout. Before
and during a refactor, stop and ask what the refactor could break:

1. **What is the user's path here?** (This is the § 1 copy question, upstream of pixels.) Why
   does the user come to this page, what are they trying to do, how do they get in and out?
   The visual exists to serve that path — so derive the path first, then build to it.
2. **Does each remaining control's *interaction logic* need to change — and if you change it,
   is it still complete?** A control is not just its look. It carries behavior: a select that
   filters, an input that debounces, a toggle that triggers auto-save, a context menu, keyboard
   handling, a drag, a hover-to-reveal action, an empty/loading/error branch. When you swap a
   legacy control for a refactored one, **re-wire every behavior it had** — don't just port the
   markup.
3. **Did the refactor drop functionality?** Inventory everything the old page could *do* — every
   button, menu item, edge action, state, shortcut — and confirm the new page can still do all
   of it, or that you **deliberately** removed it and said why. Never lose a capability by
   accident.
4. **Is there a better path?** A refactor is the moment to question whether the old flow was even
   right: a step that can be removed, a dialog that can be inlined, two redundant controls that
   can merge, a shorter route in/out. Improve the path, don't just repaint it.
5. **A new page is all of the above, from zero.** With no old page to inventory, you must derive
   the path, the required behaviors, and the complete feature set from the requirement itself.
   The risk inverts: not "losing" an old behavior, but **never specifying** one you needed —
   so think the full interaction surface (states, edges, empties, exits) up front.

## The design language in one breath

The refactor is **not** new chrome. It is a switch to a calmer language whose body is
defined by a **single hairline stroke + an inherited white surface**, and whose
interaction is read through **color/fill change in place** — never by lifting, scaling,
shadowing, or bordering something "to make it nicer."

What concretely changed, before → after:

- **Floating white cards.** Content lives in `bg-card` cards with **one** `border-border`
  hairline and the shell radius. The section title sits *above* the card as quiet muted
  text. Use the shared `SettingsSection` / `SettingsRow` primitives — do not hand-roll a card.
- **Unified stroke.** One hairline, `border-border`. Never `border-border/50`,
  `border-*/40`, or a structural border on a control body.
- **Unified radius.** Only the role-map scale (card 14 / menu-shell 12 / control 8 /
  badge·tooltip 6). Never a bare `rounded` or an off-scale `rounded-lg` on a control.
- **Unified color.** Black/white/gray is ~90% of the UI (the skeleton). Charcoal is the
  high-emphasis CTA; blue means "selected"; purple is scarce. `success`/`warning`/
  `destructive` are **rationed signals**, not surface decoration — never tint a whole
  card `bg-success/5`.
- **Unified components.** Use the refactored `@memohai/ui` atoms as-is. Do not re-skin
  them or inject classes that fight their contract (the canonical "weird Select" bug).
- **No hover-rise, ever.** Cards and rows do **not** lift / scale-up / grow a shadow on
  hover or press. Press-scale belongs only to buttons and sidebar rail items — never to a
  large content card (a bot card does not shrink when you press it).

### The shell & spacing rhythm

This is the part that most often gets skipped and is the fastest tell of an un-refactored
page. The refactored pages (Appearance / Profile / About) are **not full-bleed** — they all
sit inside the same shell, and nothing ever touches an edge or another element.

- **The shell.** Content is a centered column inside the right pane, not stretched edge to
  edge: `mx-auto max-w-3xl` caps the width (~768px) and centers it, `px-6` keeps a left/right
  gutter so nothing glues to the pane edge, `pt-10` pushes the title down off the top, `pb-12`
  leaves room at the bottom. A page that runs full-width or starts flush against the top is
  immediately off-language. (About is the one exception: being sparse, it centers its group
  vertically with a slight upward bias instead of top-aligning.)
- **Spacing is a hierarchy of gaps, not free-styled margins.** Each level of structure has
  its own consistent breathing room, and you reuse the same rung instead of inventing values:
  - title → content: `mb-6` (Profile uses `mb-8`)
  - card group → card group: `space-y-8` — the big, generous gap that separates sections
  - section label → its card: `space-y-2.5`
  - row → row inside a card: a `border-b` hairline divider + `py-3`, each row `min-h-[3.75rem]`
  - label → its description: `mt-0.5`
  - inside a padded card block: `p-4`/`p-5` with `space-y-4`
- **Text is never glued — to edges, to the top, or to each other.** Every label has air above
  and below it; the title has air under it; cards have air between them. When something feels
  cramped, the fix is almost always "use the next rung of the spacing hierarchy," not a
  one-off margin.

Concrete shell + primitives (exact recipes + the full spacing ladder live in `reference.md`):

- Page shell: `mx-auto max-w-3xl px-6 pt-10 pb-12`, title `mb-6 px-2 text-lg font-semibold`,
  sections stacked with `space-y-8`.
- Card: `SettingsSection` = `overflow-hidden rounded-[var(--radius-menu-shell)] border border-border bg-card`,
  optional title above as `px-2 text-[13px] font-medium text-muted-foreground`.
- Row: `SettingsRow` = label (`text-sm font-medium`) + description (`text-xs text-muted-foreground`)
  on the left, the control on the right, rows split by `border-b border-border last:border-b-0`.

### Dividers — inset inside a card, full-bleed everywhere else

A divider has two different jobs and two different widths; using the wrong one is a tell.

- **Separating rows *inside* one white card → inset.** The hairline must **not** reach the
  card's left/right edges. This is done by putting the border on a horizontally-margined row
  (the `mx-4` on `SettingsRow`), never on the card itself, and dropping it on the last row
  (`last:border-b-0`). An edge-to-edge line would visually slice the rounded card into stacked
  tiles and break the "this is one continuous surface" reading.
- **Structurally splitting a container → full-bleed.** A Dialog header/footer band, a
  section-heading underline, or a standalone `Separator` between blocks divides the *whole*
  container, so the line spans edge to edge while the content keeps its own inner padding.

The test: is this line separating **items within one surface** (inset) or **splitting the
container itself** (full-bleed)? Answer that before you place a divider.

### Dark mode is not a task — it is the absence of hardcoded color

**Read this twice. This is the single most-skipped requirement, and nothing will catch it for
you.** You do **not** "add dark mode" at the end. Dark mode is the *automatic* result of using
only semantic tokens; it breaks the moment you hardcode one raw color. So there is exactly one
rule, applied from the first line: **never write a raw color — use a semantic token.**

- Raw colors that silently break dark mode: `bg-white`, `bg-black`, `text-white`, `text-black`,
  any `*-gray-*` / `*-zinc-*` / `*-slate-*` / `*-neutral-*`, any `#hex`, any `bg-[#…]` /
  `text-[#…]`, any inline `style="color: …"` / `background: …`. Use `bg-card`, `bg-background`,
  `text-foreground`, `text-muted-foreground`, `border-border`, `bg-accent`, etc. instead.
- **For tints and subtle layering, prefer the neutral overlay ladder — it is the dark-safe way
  to add "color."** When you need a hover / selected / pressed shade, or a faint layer to set
  something apart, reach for the interaction-overlay tokens (`--ui-hover` / `--ui-selected` /
  `--ui-pressed`, the `--overlay-*` rungs, or `bg-accent` which maps into them) — **never** a
  solid fill, a hand-mixed gray, or an alpha hack (`bg-black/5`, `hover:bg-gray-100`). The
  overlays are chroma-0 and composite over whatever surface they sit on, so they are the **same
  token in light and dark** (light = a black wash, dark = a white wash) and identical across
  every color scheme — no `dark:` variant, no per-scheme override, and they cannot break the way
  a baked color does. (Full ladder in `packages/ui/AGENTS.md` § Color → Interaction overlay.)
- **A `dark:` override is a smell, not a fix.** Themed tokens auto-switch with **no** `dark:`
  prefix. If you're reaching for `dark:bg-…` to patch a page, it means you started from a raw
  light color — go back and replace the base color with a token; don't band-aid it per-mode.
- **There is no safety net for app pages.** The UI-contract guard (`mise run lint`) only scans
  `packages/ui` — `apps/web` pages are explicitly out of scope, and there is no ESLint rule for
  hardcoded colors. So a raw color in a page is caught by *nothing*; lint passes, and the page
  ships broken in dark. The discipline below is the only defense — treat it as mandatory.
- **Before you finish, do two things, every time:** (1) grep the page for raw colors
  (`bg-white`, `text-black`, `text-gray-`, `bg-gray-`, `#`, `dark:`, inline `style=`); (2)
  actually **flip the app to dark and look at the rendered page**. The only sanctioned `bg-white`
  is a physical knob (Switch / Slider thumb) over a colored track. Canvas content (charts) can't
  read tokens — reuse the token→concrete-color resolve the reference pages already do, re-run on
  theme change.

### Narrow screens reflow, never overflow

A settings page is a centered `max-w-3xl` column, but the pane is resizable and the desktop
window can be narrow. Multi-column grids collapse with responsive prefixes (`grid-cols-1
sm:grid-cols-2`, stat rows `grid-cols-2 sm:grid-cols-4`); same-row control clusters (search +
button) must not break or clip. Always check the narrowest realistic width, not just the wide
default — and remember Chinese copy is wider, so the narrow + `zh` combination is the real worst
case (see § 1).

### Scroll ownership

Know who owns the scroll before you add `overflow-*` anywhere. The desktop shell **locks body
overflow**, so a page that needs to scroll must own its own scroll container (the dev wall does
this with `h-dvh overflow-y-auto`); a settings page instead scrolls inside the section's
existing scroll area. The failure modes are symmetric: a page that forgets to own its scroll is
un-scrollable inside the desktop shell, and a page that adds a stray `overflow-*` creates a
*nested* scroll container (a scrollbar inside a scrollbar) or a surprise horizontal scrollbar.
When a transform nudges content sideways (the list↔detail swap pushes panes ±24px), clip it
with `overflow-x-clip` — not `overflow-x-hidden`, which would turn the element into a vertical
scroll container and steal scrolling from the ancestor. Don't introduce a new scroll container
unless you mean to.

## Component discipline

Pick the right component instead of bending the wrong one. See `reference.md` §
Component picker for the full decision table and the icon/badge/tooltip rules. The
recurring failures to avoid:

- **Choosers:** `Select` (pick one value from a menu) · `Combobox` (searchable, single
  *or* `multiple`) · `SegmentedControl` (a mode/filter, no panels) · `Tabs` (switch panels).
  Do not hand-roll a searchable dropdown when `Combobox` exists; do not inject custom
  classes into a `Select` trigger that fight the field-edge contract.
- **Icon buttons:** `<Button variant="ghost" size="icon">` in a toolbar, `variant="outline"`
  standalone. Icons are **lucide components** (`<Plus/>`), never a typed glyph (`"+"`),
  and never free-sized — let the `size-4` control ladder apply. Never `scale-90` a control
  to "fix" its size.
- **`BadgeCount`:** `destructive` red dot pinned to an icon corner = alert/unread; `default`
  neutral count rides a tab/filter/segment; a flat list row uses a plain muted numeral, no bubble.
- **`Tooltip`:** always the `@memohai/ui` `Tooltip`. A hand-rolled or legacy tooltip is a bug.
- **Empty surfaces:** the `Empty` component (icon + title + description + action), framed.
- **Destructive actions:** a filled `<Button variant="destructive">`, gated behind a
  confirmation (`ConfirmPopover`, or a dialog for heavier deletes) — never a bare one-click
  delete, never a ghost button with manual red text. Group truly dangerous actions in a danger
  card kept at the bottom of the page.
- **Long lists / big dropdowns virtualize.** A list or chooser that can hold hundreds of rows
  (sessions, models, searchable selects) must virtualize, not render every node — otherwise the
  refactor that "looks fine" with 5 rows jank-scrolls with 500. Reuse the existing virtualized
  patterns instead of a plain `v-for` over an unbounded list.

## UX principles — the part that is hard to see

These are judgment rules. They are the difference between "it renders" and "it's good."

### 1. Interrogate every line of copy

Before you write *any* user-facing line, slow down and ask, repeatedly:

- The user already knows the page's **icon** and its **name in the sidebar**. So what do
  they actually not know yet?
- Why did they come to this page? What are they here to *do*?
- Does this line **guide** them — point a direction, reduce a decision — or does it just
  restate the title in more words?
- If I add this sentence, does it mean anything to the user? If I remove it, do they lose
  anything?

Then audit both directions: **what guidance is missing** (a user who lands here is lost),
and **what is redundant** (a label that just narrates the obvious). Cut filler; keep
direction. A page is not better for having more words on it.

**Copy is bilingual, and that is a layout constraint, not just a translation chore.** Every
user-facing string goes through an i18n key with **both** `en` and `zh` written — no hardcoded
text. But the two languages have different shapes: Chinese is denser and wider per glyph,
English runs longer. A row that fits perfectly in English can wrap or overflow in Chinese (and
vice versa), which silently breaks same-row height (§ 4) and narrow-screen reflow. So write and
**eyeball both locales**, and design the layout to survive the longer/wider of the two.

### 2. Don't over-prompt (validation and beyond)

A required field that the user merely touched and moved away from should **not** flash red.
On a page that is a single input plus a select, or a two-field sign-in, nagging "you didn't
fill this in" is absurd — the user can see the two empty boxes. Validate at the moment it
matters (submit), and surface the error usefully then.

Generalize this: the red-required box is just one instance. **Restraint applies to all
external component signals.** Don't make a component shout a state the user did not ask
about and does not benefit from.

### 3. Empty *and loading* states must hold the frame

Before shipping an empty state, ask: **can this page hold the void?** If a bare centered
"No data" line leaves the page looking broken or unbalanced, that is the wrong answer.
Keep the card / list / table **frame** drawn, and put the message *inside* it ("No usage
data for the selected period"). The structure should survive having no rows.
(Anti-example: a page that drops to bare centered muted text. Good: a framed `Empty`, or a
`TableEmpty` row inside the table that still draws.)

The loading state has the same duty, plus one more: **the layout must not jump when data
arrives.** A skeleton should match the *shape* of the final content (Profile's skeleton is a
card of rows the same size as the real rows), and every block that loads async should reserve
its height up front (the reference pages set a `min-h` on each row "so a cold load doesn't make
the block jump"). Never let a section pop in at a different size than its placeholder, and use
`—` for a not-yet-sampled value rather than a faked `0`.

### 4. Same-row controls share a height

Anything sitting on one visual line — a search box next to an "Add" button, a select next
to an action — **must be the same height**. A short search field beside a tall button is a
real bug we shipped before. Build the search with `InputGroup` and the action with `Button`
at the matching size, then verify the heights actually line up.

### 5. No redundant or fighting controls

Two controls that solve the same job and contradict each other is a defect, not a feature.
(Anti-example: a "Time Range" preset select *and* a "Custom Range" date picker coexisting
with no defined relationship — picking one silently fights the other.) Either pick one
model, or make their relationship explicit and visible.

### 6. Motion: never abused, always felt

- **Press-scale only where it fits** — buttons, sidebar rail items. **Never** on a large
  content card.
- **Directional list ↔ detail** uses `useViewSwap` + `SwapTransition`: forward = list slides
  out left while detail slides in from the right; back = the reverse. One view visibly gives
  way to the other — no "fade out, then fade in" double-jump.
- The motion duration palette is fixed (see `packages/ui/AGENTS.md` § Motion). Stay in it.
- The rule: **don't overuse motion, but make every user action perceivable.** A click that
  changes nothing visible feels broken even when it worked.

### 7. Think the whole user path, including the exit

Every entry needs a sane, short exit. Trace the full round-trip before you ship.
(Anti-example: opening a manager from the chat sidebar landed the user in Settings, and
"Back" walked Settings → Settings → Chat — two backs to undo one click. The fix was a
direct return to chat.) If getting *out* takes more steps than getting *in*, the path is wrong.

### 8. Save model & feedback (toast timing)

First decide *whether the page even needs a manual Save button.* Most settings surfaces
don't — a Save button exists to let the user commit a deliberate batch (a real form, a risky
change). A page that is a few toggles and selects should **auto-save** each change instead of
hoarding edits behind a button.

- **Auto-save is silent.** It generally gets **no success toast** — a toast on every settings
  tweak is too loud and reads as nagging. Save quietly, only surface *errors* (and roll the
  optimistic edit back on failure). Profile is the model: edits auto-save, success says
  nothing, failure toasts + rolls back.
- **Manual save can confirm.** When the user explicitly clicks Save, a single success toast is
  fine — they took a deliberate action and deserve the acknowledgement.
- **Toast timing in general:** toasts are for *explicit* user actions (save / delete / create)
  and for *errors that need attention.* Never fire them for ambient, automatic, or background
  changes. One toast per action, not one per keystroke.

### 9. Earn the space — show only what's actionable, and only card it when it's a group

Every block on the page must justify its pixels. Two questions decide whether something belongs
and how it's framed:

- **Does this block need to be here right now?** Prefer **progressive disclosure**: show a block
  only when it's actionable, and let the whole block *disappear* when there's nothing to do — a
  healthy, fully-configured bot does not need a row telling it that it's healthy. (Overview hides
  Platforms once everything is connected, and drops the whole Reminders section when the setup
  list is empty.) An always-present "Status: OK" row is noise; an issue banner that appears only
  when there's an issue is signal.
- **Does this block earn a card, or is a card overkill?** A `SettingsSection` frame is for a
  **group of rows**. Wrapping a single row of metric tiles in a bordered card is card-in-card —
  a big box moated around small boxes, which reads as mostly-empty. When content is a single
  self-contained unit (a tile row, a chart), let it sit directly under its title with no outer
  card. Card the groups; don't card the singletons.

## The review ritual — run it on every finished page

When a page looks done, do **not** stop. **Open it in the running dev app** (`mise run dev`) —
and the component wall (`Cmd/Ctrl+Shift+D`) for any component you're unsure about — and look at
the real thing as a first-time user who has never seen it. Reading the code is not the review;
seeing it rendered is:

- Name everything you see, top to bottom. Is any of it filler? Is any guidance missing?
- How does it sit in the viewport? Is it balanced **left ↔ right**? **top ↔ bottom**?
- Force the **empty** state and the **loading** state. Does the frame still hold?
- Walk **every interaction in the step-2 behavior inventory** — does each one still work? Did
  the refactor quietly drop a feature, a state, a shortcut, or a path?
- Do all **same-row controls** line up in height? Do cards share one stroke, one radius?
- **Dark mode (mandatory, no lint net for app pages):** grep for raw colors (`bg-white`,
  `text-black`, `text-gray-`, `bg-gray-`, `#`, `dark:`, inline `style=`), then **flip the app
  to dark and look** — is anything still glued to a light value?
- Shrink to the **narrowest pane width** (and check `zh`): does anything overflow or clip?
- Are dividers the right kind — **inset** inside cards, **full-bleed** as structural splits?
- Is the **save model** right (auto-save silent vs deliberate manual save), with no toast
  spam on ambient changes?
- Is there any **hover-rise**, any tinted card, any off-scale text, any hand-rolled control?

Then run **`mise run lint`** — the UI-contract guard (`scripts/check-ui-contract.mjs`)
mechanically blocks raw colors, invented shadows, off-scale radius, and structural borders
on controls. A page is not done until it passes.

## Refactor workflow (e.g. an un-refactored bot tab)

1. **Read** `packages/ui/AGENTS.md`. Then open a **reference** page matching the target's
   shape and the **page you're replacing** (see `reference.md` § Reference map).
2. **Inventory behavior & path before touching anything.** Write down what the old page can
   *do* — every control's interaction logic, every feature / state / edge / shortcut — and its
   user path in and out. This list is your regression contract: the new page must satisfy it
   (or you decided to drop an item, on purpose). For a *new* page, derive this list from the
   requirement instead — you have nothing to inventory, so the risk is omission.
3. **Diagnose** the old page against the dirty→clean table in `reference.md`. List its sins
   (tinted fills, hairline-alpha borders, off-scale text, `scale-90`, `shadow-none`, colored
   focus rings, invented dashboards) — these are exactly what "off-language" means.
4. **Rebuild** from the shell down: page shell → `SettingsSection`/`SettingsRow` groups →
   the right `@memohai/ui` atoms, tokens only → **re-wire every behavior from the step-2
   inventory** → copy through the § 1 interrogation → empty states that hold the frame →
   aligned same-row heights → only the motion that fits.
5. **Review ritual** above + `mise run lint`.
6. Keep code comments about **why** (the reference pages do this well); never narrate the
   change itself, and never name an external product in a comment.

## Comments & code style

The refactored pages carry short comments explaining *why* a block exists, why it's hidden
in some states, why there's no card-in-card, why a Badge instead of a loose dot, why `—`
instead of a faked `0`. Match that: comments justify a non-obvious decision, they do not
restate the code. Follow `apps/web/AGENTS.md` (semantic tokens only, lucide icons, i18n keys,
vee-validate + Zod, SDK for data).
