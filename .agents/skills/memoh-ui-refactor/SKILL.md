---
name: memoh-ui-refactor
description: Refactor an apps/web page into the new white-floating-card design language with disciplined @memohai/ui usage, deliberate copy, honest empty states, aligned controls, and restrained motion. Use when redesigning, refactoring, polishing, or building any settings/list/detail page in apps/web (e.g. a not-yet-refactored bot tab, a provider list, a settings surface), or when a page feels "dirty" / off-language and needs to match the refactored Overview / Appearance / About / Web Search pages.
---

# Memoh Web — Page Refactor & Design Language

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

### 2. Don't over-prompt (validation and beyond)

A required field that the user merely touched and moved away from should **not** flash red.
On a page that is a single input plus a select, or a two-field sign-in, nagging "you didn't
fill this in" is absurd — the user can see the two empty boxes. Validate at the moment it
matters (submit), and surface the error usefully then.

Generalize this: the red-required box is just one instance. **Restraint applies to all
external component signals.** Don't make a component shout a state the user did not ask
about and does not benefit from.

### 3. Empty states must hold the frame

Before shipping an empty state, ask: **can this page hold the void?** If a bare centered
"No data" line leaves the page looking broken or unbalanced, that is the wrong answer.
Keep the card / list / table **frame** drawn, and put the message *inside* it ("No usage
data for the selected period"). The structure should survive having no rows.
(Anti-example: a page that drops to bare centered muted text. Good: a framed `Empty`, or a
`TableEmpty` row inside the table that still draws.)

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

## The review ritual — run it on every finished page

When a page looks done, do **not** stop. Re-render it and look at it as a first-time user
who has never seen it:

- Name everything you see, top to bottom. Is any of it filler? Is any guidance missing?
- How does it sit in the viewport? Is it balanced **left ↔ right**? **top ↔ bottom**?
- Force the **empty** state and the **loading** state. Does the frame still hold?
- Do all **same-row controls** line up in height? Do cards share one stroke, one radius?
- Is there any **hover-rise**, any tinted card, any off-scale text, any hand-rolled control?

Then run **`mise run lint`** — the UI-contract guard (`scripts/check-ui-contract.mjs`)
mechanically blocks raw colors, invented shadows, off-scale radius, and structural borders
on controls. A page is not done until it passes.

## Refactor workflow (e.g. an un-refactored bot tab)

1. **Read** `packages/ui/AGENTS.md`. Then open a **reference** page matching the target's
   shape and the **page you're replacing** (see `reference.md` § Reference map).
2. **Diagnose** the old page against the dirty→clean table in `reference.md`. List its sins
   (tinted fills, hairline-alpha borders, off-scale text, `scale-90`, `shadow-none`, colored
   focus rings, invented dashboards) — these are exactly what "off-language" means.
3. **Rebuild** from the shell down: page shell → `SettingsSection`/`SettingsRow` groups →
   the right `@memohai/ui` atoms, tokens only → copy through the § 1 interrogation → empty
   states that hold the frame → aligned same-row heights → only the motion that fits.
4. **Review ritual** above + `mise run lint`.
5. Keep code comments about **why** (the reference pages do this well); never narrate the
   change itself, and never name an external product in a comment.

## Comments & code style

The refactored pages carry short comments explaining *why* a block exists, why it's hidden
in some states, why there's no card-in-card, why a Badge instead of a loose dot, why `—`
instead of a faked `0`. Match that: comments justify a non-obvious decision, they do not
restate the code. Follow `apps/web/AGENTS.md` (semantic tokens only, lucide icons, i18n keys,
vee-validate + Zod, SDK for data).
