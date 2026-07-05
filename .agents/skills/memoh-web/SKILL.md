---
name: memoh-web
description: Primary Web development skill for apps/web — white-floating-card design language, disciplined @memohai/ui usage, deliberate copy, honest empty states, aligned controls, and restrained motion. Never hand-write controls or menus; never leave stray fragments (orphan status labels, misaligned save hints). Compose from @memohai/ui primitives and reuse existing save/feedback patterns from reference pages. Use for any apps/web UI work — new pages, settings/list/detail surfaces, chat components, polish passes — not only legacy page migrations. Read this skill before writing or changing Web frontend code.
---

# Memoh Web — Page Development & Design Language

## Non-negotiables — read this even if you skim the rest

The ten rules that break a page if you miss them. The rest of this file is the *why* and
the *how*; these are the *must*.

1. **Copy the new, never the legacy — and copy the *contract*, not a page's raw classes.** Open
   a refactored reference page (§ Reference map in `reference.md`) and mirror its *structure*.
   Never pattern-match off a dirty / un-refactored page — and never inherit a reference page's
   stray `font-[NNN]` / `text-[Npx]` / `text-foreground/NN`: some references still carry
   pre-contract token debt (`about` is one — it ships off-scale size, arbitrary weight, and
   hand-mixed alpha in one line), so the token law in `packages/ui/AGENTS.md` outranks any
   single page's classes when they disagree.
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
5. **Reuse a component — never hand-write one.** Compose from the real `@memohai/ui` atoms
   (Select / Combobox / Tooltip / icon `Button` / `Empty`) and the existing shared parts; never
   re-skin one, hand-roll an equivalent, or rebuild a control out of raw `<div>`s. **Menus
   included:** dropdown / context / action menus use `DropdownMenu` / `ContextMenu` and their
   `*Item` / `*Separator` / `*Label` slots — never hand-written `<button>` rows, never `<hr>` /
   `border-b` dividers inside a popover. The same rule applies to every other surface: if
   `@memohai/ui` (or an existing app component) already covers it, use it — hand-built markup
   drifts out of the token contract and reads inconsistent page to page. If a layout repeats,
   extract it into one shared component instead of pasting it twice. A genuinely new component
   is a last resort — clear it with the developer *before* building it.
   **The component system is not modelling clay.** `SettingsSection` / `SettingsRow` / the cards
   are not a blank canvas you reshape to taste — each has *one* sanctioned use, and you compose
   *with* them, you do not knead them into a new shape. The tell that you've started treating the
   system as clay: you're putting a hand-rolled `grid` / multi-column layout / freeform `<div>`
   stack **inside** a `SettingsSection` to "arrange things nicer" (a 2-col stat grid jammed into
   the white card is the canonical offence — see `reference.md` dirty→clean). When the shape you
   want isn't one the primitives already give you, that is **not** license to deform a card from
   the inside — it means you've picked the wrong primitive. Stop, find the component whose *one*
   job is that shape (a value-in-a-card is a `SettingsRow`; a label-on-top/number-below readout
   is a **section-level stat tile**, never a grid inside a card), or clear a real new component
   with the developer. Squeezing the clay is always the wrong move, even when it renders fine.
6. **Earn every word and every block.** Cut copy that doesn't guide; hide blocks that aren't
   actionable; empty *and* loading states must still draw their frame (no layout jump).
   **No stray fragments:** every visible piece must sit in a named region (PageShell
   `#actions`, a row's control column, a dialog footer, a toast) — never a lone status line
   floating where it aligns with nothing. If it can be removed, remove it; if it must stay,
   anchor it and reuse an existing pattern from a reference page.
7. **You are not done until you verify the *rendered* page:** grep for raw colors → flip to
   **dark** → shrink to **narrow + `zh`** → walk **every old interaction** → `mise run lint`.
8. **Draw it before you build it, then audit the nesting.** Sketch the page as an ASCII
   wireframe first — and again when it looks done — and read it like space-complexity: no
   card-in-card, no decorative icon stacked in a card, no nesting layer that isn't earning its
   keep. Fewer boxes, shallower depth.
9. **The root surface answers "is it working," not "configure everything" (the 99/1 rule).**
   The first page a user lands on serves the 99% who came to *glance at state* — it must not
   make them carry the visual weight of inputs, button piles, or history lists that only the 1%
   want. But the 1% are not browsing; they **arrive with a purpose** and will hunt for the
   button — so deep/rare operations (limits, snapshots/history, destructive actions) live behind
   a **named entry point** (a one-line summary row + a button) that opens a focused form showing
   the data *and* the action. Never spill them onto the root, and never bury them in an in-card
   "Advanced" disclosure that mixes diagnostics with real operations (§ 12).
10. **A change is not committed until a human verifies it.** Run lint / grep / type checks
    yourself, but the **rendered** result (visual + interaction) must be checked by a human
    before any `commit` / `amend` / `push` — do not amend or squash unverified work into a
    commit on your own say-so. "It should work" from the agent is not verification; the
    human's eyes on the rendered page is. Follow-up tweaks to an already-committed change
    re-verify before re-amending.

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

## Engineering correctness — the dirt the eye can't see

A page can pass every visual rule and still be **wrong**. The most expensive debt isn't an
ugly card — it's behavior that breaks because two modules quietly disagree about a contract.
This is invisible in a screenshot and survives review, so it gets its own pass. Treat it as
part of "clean," not a separate concern.

- **A cross-module assumption must be enforced or eliminated — never just commented.** The
  back-affordance bug is the cautionary tale: `useSyncedQueryParam` switched tabs with
  `router.replace` under a comment claiming "replace won't bury the previous page," while
  `installBackHistory`'s `afterEach` never distinguished replace from push — so replace *did*
  overwrite `previous`, and the back button started reading the bot's raw `bot-<uuid>` slug.
  Both comments looked reasonable; together they were wrong. If module A leans on module B
  behaving a certain way, lock it with a type or a test, or remove the assumption. A comment
  asserting the contract is not enforcement of it.
- **Layout size must never be driven by content.** A `w-fit` sidebar
  (`master-detail-sidebar-layout`) let one too-long back label stretch the whole panel — so a
  bad string became a visibly wider sidebar. Pin widths and let text `truncate` inside a fixed
  box; a locale change, longer data, or an upstream bug must never move the frame.
- **In-page state syncs with `replace`; whatever reads "the previous page" must honor that.**
  Tab/filter swaps are not navigations — they `router.replace`. A history reader that counts
  replace transitions will treat a tab switch as a place to step "back" to.
- **One root cause often wears two faces.** The slug label and the widened sidebar were the
  same bug. When two oddities appear together on the same action, hunt one upstream cause
  before patching each symptom in place.

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
  tiles and break the "this is one continuous surface" reading. **Corollary:** borders go on
  *rows*, never on the invisible wrapper `<div>` you put a `v-if` block in — a wrapper with
  `border-b` that ends up the **last child of the card** doubles its hairline onto the card's own
  bottom stroke (the recurring "fights the stroke" bug). See reference.md § Dividers.
- **Structurally splitting a container → full-bleed.** A Dialog header/footer band, a
  section-heading underline, or a standalone `Separator` between blocks divides the *whole*
  container, so the line spans edge to edge while the content keeps its own inner padding.

The test: is this line separating **items within one surface** (inset) or **splitting the
container itself** (full-bleed)? Answer that before you place a divider.

**A "divider I never drew" is usually a misplaced `#footer`.** If a hairline appears to float
under a single row over an empty strip, you almost certainly put a `SettingsSection #footer`
(Save band) on a *root-page* card — its full-bleed `border-t` plus the lone row's own inset
`border-b` read as a stray line. The line is real chrome in the wrong home: a root page's Save
belongs in `PageShell #actions`, not a card footer (§ 8). Fix the home, not the line.

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

**When a component must adapt to a resizable pane, viewport breakpoints are the wrong tool.**
`sm:` / `md:` watch the *window* — but a dockview / master-detail pane changes width while the
window doesn't, so a `sm:` grid won't react when the same component sits in a narrow vs wide
pane. Reach for a **container query** (`@container`) so the component responds to *its own
container's* width, not the viewport. (Page-level `max-w-3xl` columns still use viewport
prefixes; this is only for components that live inside variable-width panes.)

Pane width is only one of three "bigger" axes; the page must also hold up under **browser zoom**
and a **larger root/OS font**. The defence is the same discipline: lay out with the spacing
ladder and flex/grid gaps (never a margin tuned to one string), size inline-with-text icons in
`em` so they grow with the text while standalone control icons keep the `size-*` rem ladder, cap
width with `max-w-*` + centre so a wide screen never stretches a line, and let any line that can
outgrow its box `truncate`. Full rules + the verify pass (zoom 50→200%, narrow + `zh`, ultra-wide)
live in `reference.md` § Scaling & zoom.

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

**Every page-level scroll container that holds a centered `max-w-3xl` column must reserve the
scrollbar gutter — `[scrollbar-gutter:stable]`.** The shell centers content with `mx-auto`, so
its left/right margins are computed from the pane's *available* width. When a classic
(space-consuming) scrollbar appears, it eats that width and the whole centered column — title,
card edges, everything — shifts sideways. The tell is real and confusing: two sibling tabs look
"only similar," because a long tab scrolls (narrower pane) while a short one doesn't (wider
pane), so the title and card edges land in different spots as you switch between them. A page
that doesn't scroll *today* will the day its content grows — so this is not optional on the
scroller, it's structural. Reserving the gutter keeps the available width constant whether or
not the scrollbar is visible, so every page that shares (or mirrors) the scroller stays aligned.
There are only a handful of these page-level scrollers (the settings section's `router-view`
pane; any master-detail surface that runs its *own* inner scroll pane, e.g. the bot-detail tab
pane) — put the rule on the scroll container itself, never on each page, so all pages it hosts
inherit it for free. Bounded inner scrollers (a tool-call detail body, a dropdown list, a log
pane) are left-aligned and don't need it.

## Component discipline

**Reuse first; build new only with sign-off.** The default is always to *find and reuse* an
existing component, then to *compose* existing atoms — never to hand-write a control out of raw
markup. The most expensive page is the one where the agent quietly re-built from zero what
already existed. Three rules, in order:

1. **Hand-writing a component is forbidden.** A clickable `<div>` that re-implements a Button, a
   bespoke popover list that re-implements a Select, a `<div>`-grid that re-implements a Table —
   all banned. They can't receive the size / token / focus / a11y contract, and they drift. If
   `@memohai/ui` (or an existing app component) has it, use it as-is.
2. **A composition that can repeat must be extracted, not pasted.** Even when every piece is a
   properly reused atom, if the *arrangement* could appear in more than one place (a provider
   row, a card header, an empty tile, a field cluster), lift it into one shared component and
   reuse that. Copy-pasted markup is duplication waiting to drift out of sync — and a reused
   composition dropped into a spot where the same shape recurs is the signal to extract it.
3. **A genuinely new component needs the developer's OK first.** When nothing fits and no
   composition will do, stop and say so — name what's missing and why — get agreement, then
   build it once in the shared layer. Never silently spawn a one-off component mid-page.

Then pick the right component instead of bending the wrong one. See `reference.md` §
Component picker for the full decision table and the icon/badge/tooltip rules. The
recurring failures to avoid:

- **Menus (dropdown / context / overflow / kebab):** `DropdownMenu` or `ContextMenu` as the
  shell; each action is `DropdownMenuItem` / `ContextMenuItem` (or checkbox/radio variants when
  needed); group labels use `*MenuLabel`; splits use `*MenuSeparator` — never a raw `<button>`,
  clickable `<div>`, or `<hr>` / `border-b` / `h-px bg-border` standing in for menu chrome.
  The trigger is `<Button>` / `TextButton` / `DropdownMenuTrigger as-child`, not a bespoke
  clickable span. Submenus use `*MenuSub` + `*MenuSubTrigger` + `*MenuSubContent`. All menu
  surfaces share `lib/menu.ts` (`menuItemClass`, `menuSeparatorClass`) — hand-building rows
  bypasses that contract and is the fastest path to "this menu looks different from every other
  menu."
- **Choosers:** `Select` (pick one value from a menu) · `Combobox` (searchable, single
  *or* `multiple`) · `SegmentedControl` (a mode/filter, no panels) · `Tabs` (switch panels).
  Do not hand-roll a searchable dropdown when `Combobox` exists; do not inject custom
  classes into a `Select` trigger that fight the field-edge contract.
- **Icon buttons:** `<Button variant="ghost" size="icon">` in a toolbar, `variant="outline"`
  standalone. Icons are **lucide components** (`<Plus/>`), never a typed glyph (`"+"`),
  and never free-sized — let the `size-4` control ladder apply. Never `scale-90` a control
  to "fix" its size.
- **Default to no icon — an icon is a cost, not a freebie.** An icon must earn its place by
  carrying meaning — a brand/provider mark, a status, or a clear action glyph on a button. It is
  never free: a boxed icon drags in a surface (and its shadow), one more color, and a "does this
  glyph even fit our language?" judgment call. So a generic lucide glyph dropped beside a title,
  floated atop a "No X" empty block, or **stacked inside a card** is decoration, not signal — it
  reads as cheap chrome and cheapens the page. Ship none by default; when a spot genuinely seems
  to want one, **clear it with the developer before adding it** rather than sprinkling icons on
  your own judgment.
- **`BadgeCount`:** `destructive` red dot pinned to an icon corner = alert/unread; `default`
  neutral count rides a tab/filter/segment; a flat list row uses a plain muted numeral, no bubble.
- **`Tooltip`:** always the `@memohai/ui` `Tooltip`. A hand-rolled or legacy tooltip is a bug.
- **An empty state keeps the populated skeleton — it is the same page with no rows yet.** The
  worst empty-state failure is letting "there's no data" rearrange the page into a *different*
  shape. Keep the exact frame the populated state uses (the same `SettingsSection` card, the same
  grid container) and drop the message *inside* it, so entering an empty page vs a full one never
  jolts the layout. The model is the **Plugins tab**: its empty state is the very white card it
  shows when populated — just `py-12` centered title + description + the one guiding action (an
  outline "+ Add" / "Supermarket" button). Two hard rules ride on top:
  - **`border-dashed` is NOT an empty-state look.** Dashed is reserved for the **"+ Add another"
    tile** that sits *beside real items in an already-populated list/grid*, where adding one more
    is the secondary affordance. A completely-empty surface takes the **solid** frame its
    populated form has — the section card, or a solid-`border` framed block for a standalone grid
    — never a dashed box, and never bare floating muted text. (This refines the older "outermost
    Empty earns a dashed border" guidance: it does not — outermost empties are solid-framed.)
  - **No decorative icon.** An `EmptyMedia variant="icon"` glyph tile, or any big lucide glyph
    stacked above the title, is banned: it is both card-in-card and the icon-abuse below. Just
    title + description + action. (An action *button* keeps its own small action glyph — that is
    not a decorative tile.) This page-type attracts icon abuse — a giant glyph crammed in front
    of a list item or empty block — so default to **none** everywhere except a button's own glyph.
- **Destructive actions:** a filled `<Button variant="destructive">`, gated behind a
  confirmation (`ConfirmPopover`, or a dialog for heavier deletes) — never a bare one-click
  delete, never a ghost button with manual red text. Group truly dangerous actions in a danger
  card kept at the bottom of the page. **Confirm covers interruption, not just deletion:** any
  action that ends running work or severs a live connection — Stop a runtime, Terminate a
  session, Disconnect — earns the same confirm step, because "it stops what it was doing" is a
  consequence the user must opt into. Skip the confirm only for cheap, reversible actions.
- **Long lists / big dropdowns virtualize.** A list or chooser that can hold hundreds of rows
  (sessions, models, searchable selects) must virtualize, not render every node — otherwise the
  refactor that "looks fine" with 5 rows jank-scrolls with 500. Reuse the existing virtualized
  patterns instead of a plain `v-for` over an unbounded list.

## Compose, don't style — the extension boundary

This is the page-layer half of `packages/ui/AGENTS.md` § *Compose, don't style* (read it for
the ownership table + the four override planes). Component discipline above says *which*
component to use; this says how you are allowed to **add to** one — because the moment you
can't, the only exit left is injecting CSS, and injected CSS is the single largest source of
page debt: it fights the component's `::before` fill / field-edge, breaks dark mode, and
nothing lints it on an app page.

**"I want to add something" has exactly five exits — four need no CSS, the fifth is an upgrade:**

| I want to… | The sanctioned exit |
|---|---|
| add content (icon / badge / suffix) | a **slot** |
| change size / density | the **`size` prop** |
| change meaning (emphasis / danger / selected) | the **`variant` prop** |
| change *outer* layout (width / alignment / outer margin) | a **layout-only className** (see the red line) |
| want a look the component doesn't offer | **upgrade the component** (add a `variant`/slot, or extract a pure-style component) — never inject in place |

**The className red line — the outer box is yours, the body is the component's:**

- **Allowed on a component** (it only positions the outer box): `w-full`, `flex-1`, `grid`,
  `gap-*`, outer margin (`mt-*` / `mx-auto`), `max-w-*`.
- **Forbidden on a component** (it reaches into the body and fights `style.css`): `bg-*`,
  `hover:*`, `active:*`, `border-*`, `shadow-*` / `shadow-none!`, `ring-*`, `h-[Npx]`. If you
  just typed one of these onto a `<Button>` / `<Select>` / `<TextButton>`, stop — pick the
  right `variant`/`size`, or upgrade the component. (Canonical offender: the 6×-pasted "add
  provider" button carrying `bg-background border-border hover:bg-accent shadow-none!` on a
  real `<Button>`.)

**The agent workflow — find, reuse, compose, upgrade; never style:**

1. **Find before you write — and do not trust grep.** Ten near-identical controls can wear a
   hundred names, and they all share similar CSS, so "I didn't match it" does **not** mean it
   doesn't exist — the odds it already exists under another name are high. Check the component
   map / `reference.md` § Component picker before assuming nothing fits. Re-deriving an existing
   component is the #1 debt source.
2. **Priority is an order, not a suggestion:** reuse > compose > upgrade > (never) hand-write style.
3. **Copy only a gold-standard reference, never a dirty page.** few-shot copies what it sees;
   some good-looking pages are already off-contract, so their markup is poison — confirm a page
   is clean before mirroring it.
4. **Red lights — STOP and ask, do not improvise:** you need a *new component*, a *new token*,
   to *edit `style.css`*, or an *a11y / RTL* trade-off. Improvising past any of these means
   hand-writing past the boundary — exactly the move this whole contract exists to prevent.

## The debt taxonomy — name it before you decide to fix it

"Is this debt?" stops being a vibe once the failure has a name. Three axes turn the adjectives
*maintainable / reliable / clean* into a checklist; when unsure whether something is worth
flagging, match it here. This is a diagnostic lens, not new rules — most rows already have a
rule elsewhere; the value is being able to **name** the failure (and so decide whether to fix
it now or mark it known).

- **Maintainability** (can we still change it fast & safely tomorrow):
  - *override-plane debt* — one result must hold on all four planes at once (`packages/ui/AGENTS.md`).
  - *duplication debt* — one arrangement pasted N times; change one = change N, and they drift.
  - *source-of-truth debt* — one value hand-copied to two homes, kept in sync "by remembering"
    (a list mirrored in `cva` and an array with a "keep in sync" comment).
  - *discoverability debt* — can't find it → rebuild it. grep is blind here (same job, a hundred
    names, identical-looking CSS), so a missing component map *guarantees* re-derivation.
- **Reliability** (will it break when no one is looking):
  - *coupling / boundary debt* — a cross-module assumption asserted only in a comment, never
    enforced; survives review, fires at runtime.
  - *consistency debt* — one job done N ways, so one of them is always the odd/wrong one
    (the trigger-open philosophy split: Select vs ghost).
- **Cleanliness** (will it rot the more it is used):
  - *extension-contract debt* — no sanctioned exit → injection (the red line above).
  - *rule debt* — a rule written but not mechanized, or whose guard is scoped to the wrong place
    (the contract scans `packages/ui`; the debt lives in `apps/web`).
  - *exemplar debt* — a dirty page becomes the thing the next author (or agent) copies.

The deepest debt is **un-greppable**: it lives at runtime / in injected dependency behavior —
reka's focus management + `pointer-events`, portal/teleport leaving the style context, z-index
stacking, JS↔CSS measured layout, the Tailwind-v4 `translate`/`transform` remap. String search
can't find it and review can't see it, because it is synthesized only when the page runs. The
defense is always the same: push that complexity **down** into components / tokens / lint, so
neither a human nor an agent ever has to reason about it from inside a page.

## Seam-first debugging — find the seam, don't grind the layer

The debt above is mostly un-greppable, so when a visual / interaction bug appears you cannot
search your way out — and the wrong instinct is to **grind the layer**: stack another
`!important`, nudge a z-index, pile on a hover override, tweak a margin until it "looks right."
Every grind adds a chrome layer (= more debt) and usually treats a symptom whose cause lives in
a seam you are not even editing. Front-end conflicts almost never live *inside* the layer you
are touching; they live in the **seam between two layers**. So when stuck, debug the seam, not
the layer. (This is the diagnostic the override-plane map can't pre-list for you: the map names
*known* seams; this is how you handle a seam nobody wrote down yet.)

1. **Stop signal.** If your next move is "add one more CSS rule" to fix a look / interaction —
   stop. That urge is itself the tell that you're grinding a layer.
2. **Name the seam.** Ask what *composes* this final result: your classes + the framework's
   injected `data-*` / behavior (reka) + the browser's cascade + the current runtime state. The
   fight is at the seam between two of these — not in the one file you opened.
3. **Check ownership.** The misbehaving property — where is its home (the ownership table in
   `packages/ui/AGENTS.md`)? If you're changing it somewhere it shouldn't be touched, that's the
   cause; go fix it in its home.
4. **Find a solved twin.** Has this exact seam already been solved elsewhere? (`select-trigger`
   solved the open/hover seam with `pointer-events:auto`.) Copy the solved one — don't invent a
   second, conflicting fix (that is how the trigger-open split was born).
5. **Fix the contract, not the symptom.** Resolve it at the seam's contract (add / unify a
   `variant` / slot / one philosophy), never by caking CSS onto the symptom.
6. **Circuit breaker.** Same bug, second or third "add CSS" attempt still not clean? That is
   conclusive: the cause is a seam, not a missing rule. Stop adding CSS, return to step 2 — and
   if the honest fix is a new component / token / a `style.css` edit, that's a red light
   (§ Compose, don't style — the extension boundary): stop and ask, don't grind on.

## Re-review — two failures a single read of this skill can't prevent

Reading the rules once, up front, is not enough — two failure modes slip past it, and both are
fixed by the agent **re-reviewing itself**, not by adding more rules:

- **Context decay — you forgot the rules you read.** After a long task (many tool calls, a big
  diff) the skill you read at the start has been diluted out of context, and the code you just
  wrote can quietly violate it. So before calling a thing done, **reload the contract — don't
  review from memory**: re-open the ownership table + the className red line + the five exits
  (and run § The review ritual below), then walk your own diff against them — did I inject an
  appearance/interaction class onto a component, hand-write a hover, hand-roll a control? Memory
  is not the source of truth; the rules file is.
- **The loop is the signal — you're patching a wrong direction.** When the same problem runs
  *human flags it → you patch → still wrong*, two or three rounds deep, **stop patching.** The
  loop is conclusive on its own: the cause is not "one more fix," it's a wrong root or a wrong
  direction, and every patch only stacks complexity (= debt) on a bad base. Even if you
  eventually make it "work," a thing patched onto a wrong direction is debt, not a solution.
  Step back and re-review the *whole* thing, not the last edit: did I understand the requirement
  right? Am I fixing the root or a symptom (§ Seam-first debugging)? Is the honest move to
  **throw it out and redo it** rather than patch again? Is this a red light (§ Compose, don't
  style)? The bar is "is the direction right," never "can I eventually make it run."

Same spirit as § Seam-first debugging, scaled up: seam-first stops you grinding one **layer** on
a single bug (a spatial move — look at the seam); this stops you grinding one **direction**
across many rounds (a time/round move — look at the whole). Both say: at a threshold, quit local
patching and re-review the larger frame.

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

Two repeat offenders that survive the cut above because each line *alone* looks fine — they
only read wrong stacked together:

- **Don't repeat the same word down the nesting ladder.** A page title, a section title, and a
  row label are three rungs, and each must add information, not echo the rung above. A page
  titled *Desktop Environment*, a section titled *Desktop*, and a row labelled *Desktop* says
  the word three times and informs once. When a section holds a single control whose label
  already equals the section title, drop the section title — `SettingsSection` renders titleless
  — and let the row carry the name.
- **Name the user's outcome, not the implementation under it.** Copy is about what the user
  gets, never the stack that delivers it. "Give this bot a screen it can see and control" — not
  "Enable the VNC desktop; auto-provisions Debian/Ubuntu/Alpine." Terms like *VNC*, *gstreamer*,
  *namespace*, *CDI*, *provision*, or a raw base-image name are implementation trivia the user
  never asked for; they belong in a diagnostic *Details* surface at most, never in a headline,
  a toggle label, or its description. (This is also the no-foreign-product-name rule: the UI
  speaks the user's language, not the runtime's.)

**Copy is trilingual — en, zh, AND ja — and that is a layout constraint, not just a
translation chore.** Every user-facing string goes through an i18n key with **all three**
locales written (`apps/web/src/i18n/locales/{en,zh,ja}.json`) — no hardcoded text. The
recurring failure is real and embarrassing: agents write en+zh and **forget ja**, or reference
an existing key that was itself added without ja — the Japanese UI then renders the raw key
string (`bots.memory.lastUpdated`) in production. A 2026-07 parity sweep found 14 such keys
referenced by freshly-written templates. So the rule has two halves:
- **Writing a new key:** add it to all three files in the same edit, placeholders (`{count}`,
  `{agent}`) identical across locales. For Japanese, use natural phrasing but keep the product
  terms Japanese users read in English (Bot, Workspace, Skill, MCP…).
- **Referencing an existing key:** existence in en does NOT mean it exists in ja — grep the key
  in all three locale files before using it; if ja is missing, fill it as part of your change.
Before finishing, verify every key your diff references resolves in all three locales.
The two languages' shapes also constrain layout: Chinese is denser and wider per glyph, English
runs longer, Japanese sits between but often longest with particles. A row that fits perfectly
in English can wrap or overflow in Chinese (and vice versa), which silently breaks same-row
height (§ 4) and narrow-screen reflow. So write and **eyeball all locales**, and design the
layout to survive the longest of the three.

**An error message follows a formula: what happened · why · how to fix.** "Email needs an @
symbol" beats "Invalid input"; "We couldn't reach the server — check your connection and retry"
beats "Something went wrong." Never blame the user ("This field is required", not "You entered
nothing"), and never use humor in an error — they're already frustrated.

**One concept, one word — terminology consistency is the copy-layer of consistency debt (§ The
debt taxonomy).** Pick a term and hold it product-wide: *Delete* (never also Remove / Trash),
*Settings* (never also Preferences / Options), *Create* (never also Add / New). Synonyms-for-
variety read as different features and quietly erode trust. And a button label names the
*outcome*, not "OK / Submit / Yes" — *Save changes*, *Create account*, *Delete 5 items* (show
the count).

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

**"Hold the frame" is for a section that is always present** — the page's primary content, a
list that's core to why the page exists. A *conditional, secondary* section does the opposite:
one that only exists to manage things *when there are things to manage* (an *Active sessions*
list, an issue banner) **vanishes entirely** when empty rather than drawing an empty frame
(this is the §9 / §13 "let the block disappear" move, not a contradiction of the rule above).
The test: would a first-time user with nothing set up expect this block to be on the page at
all? If it only makes sense once populated, hide it while empty; if it's part of the page's
skeleton, keep its frame with the message inside.

### 4. Same-row controls share a height

Anything sitting on one visual line — a search box next to an "Add" button, a select next
to an action — **must be the same height**. A short search field beside a tall button is a
real bug we shipped before. Build the search with `InputGroup` and the action with `Button`
at the matching size, then verify the heights actually line up.

**A control-bearing row's height must not swing with its own interaction.** The trap: a
`SettingsRow` whose `:description` flips with the control's *value* — a mode `SegmentedControl`
whose blurb changes per selection. `SettingsRow` sets a `min-h` floor, so a longer variant wraps
to a second line and the whole row — control included — visibly jumps taller as the user toggles.
That is content-driving-layout (§ Engineering correctness) fired by a click. The fix is almost
always **the copy, not the layout**: keep such a per-value blurb short enough to hold one line so
the height is stable — do **not** over-engineer it (splitting the blurb into its own row, or
reserving a fixed two-line height, is usually worse than tightening the words). Scope note: this
is about *interaction-driven* jumps, not every height difference. A genuinely sparse page whose
row is simply taller because its **static** copy runs long is fine — let it grow; don't chase a
uniform row height for its own sake.

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
- **A button that swaps the whole card's structure is a last resort, not a pattern.** An
  "Add member" button that replaces the member list with a picker form (Cancel/Confirm and
  all) reflows the entire card on click — the frame the user was just reading vanishes.
  Prefer an inline control that commits in place — a select trigger in the header that adds
  on pick, a popover off the row — so the card's layout never changes. Caveat: the design
  system does not yet have a settled pattern for "inline add to a list", so until it does,
  treat the button→swap as an anti-pattern to **avoid**, not a template to copy. (The Bot
  Access members card moved off this swap to an inline header select; the old swap form is
  the anti-example.)

### Select triggers

A recurring anti-pattern in settings rows is a select trigger rebuilt as a `<Button>`.

- **A select trigger is not a button.** Use the shared `selectTriggerClass` or the default trigger
  from `SearchableSelectPopover` / `Select` / `Combobox`. Do not drop a `<Button variant="outline">`
  into the trigger slot — buttons carry a press-scale that visibly lurches on wide, full-width
  selects and breaks the field-like select language. This applies to every chooser, not just the
  ones that happen to be narrow today.
- **Press-scale is a narrow, secondary-button signal.** It is dangerous on wide or primary buttons:
  a full-width button that scales down looks like the whole surface is lurching. For wide / primary
  actions, use a color-press (the library's `primary` / `brand` block mode) plus a `Spinner`, not a
  scale-down. Keep scale for small, secondary, non-block controls.

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
- **A manual Save belongs where the page type puts it — copy the right *page shape*, not the
  first footer you find.** The two homes are not interchangeable, and picking by "some page does
  this" instead of "which page *shape* is this" produces a real defect:
  - **Root page (`PageShell`) → Save in the header `#actions`.** The model is `bot-tool-approval`:
    `<Button :disabled="!hasChanges || saving">{{ saveChanges }}</Button>` in `#actions`, disabled
    when synced. If the save-driving state lives in a child component, `defineExpose({ hasChanges,
    saveLoading, save })` and read it off the child `ref` — do **not** hoist all the page logic up.
  - **Detail form (`SettingsShell` / `DetailPane`) → Save in the section's `#footer` band.** That
    footer exists to *close a form card* (`provider-form`, the web-search/email/video detail
    settings). It is a detail-page device.
  - **The failure this prevents (lived):** a Memory *root* page copied the detail form's
    `SettingsSection #footer` Save. On the common `Off` state the card held exactly one
    `SettingsRow` + the footer band, so the row's own inset `border-b` hairline was left stranded
    above an empty strip with a Save floating at its right — read as "a hand-drawn divider and a
    random button in the card." The hairline was never hand-drawn; the *footer was in the wrong
    home*. Moving Save to `#actions` and deleting the footer made both the strip and the "stray
    line" vanish at once (one wrong root, two symptoms — § Engineering correctness). Before you
    place a Save, name the page type first.

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
- **An in-card "expand details" / "Advanced" disclosure is a junk drawer, not progressive
  disclosure.** Hiding a pile of inputs, a snapshot list, and restore/backup buttons behind a
  *Show details* / *Advanced* toggle *inside the root card* does not reduce weight — it just
  defers it onto the same surface, and a label like "Advanced" becomes a drawer that mixes
  read-only diagnostics with real, sometimes destructive operations. Progressive disclosure
  moves a whole concern **off** the root into its own focused surface (a dialog/form named by
  what it does — *Resource limits*, *Snapshots & restore*, *Details*, *Delete*), reached from a
  one-line entry row (§ 12). The in-card expander is reserved for genuinely secondary fields
  *within a form* (the **More options** chevron in the New Task dialog, § 11; the **Advanced**
  toggle on a provider config) — never as the root page's way to stash whole features. **The line
  is *concern*, not field count.** A form's own advanced/optional fields stay inline behind the
  one canonical toggle (§ Advanced disclosure) **even when there are many of them, split into
  titled groups** (auth, network, env, …) — they are still the *same* concern the user is already
  filling in. Field count never promotes them into a dialog. A § 12 named-entry-row → dialog is
  reserved for a genuinely *separate* concern (limits / snapshots / delete — a task the 1% comes
  to *do*). Pushing a form's advanced fields into their own dialog is itself the anti-pattern: it
  splits one task across two surfaces and walls the same form behind a door. (The Network provider
  config briefly opened a dialog for Tailscale's grouped fields; it now uses the inline toggle —
  the exact pattern from the Access rules card.)

### 10. No stray fragments — every visible piece earns a home

A **stray fragment** is UI that renders but doesn't belong to any layout region: a lone
"Saved" / "已保存" label drifting in a corner, a status chip that doesn't share an edge with
the title row or the card below, a one-off hint parked beside nothing. It reads broken even
when the logic is correct, because the eye can't tell what it is attached to.

**While building, run this on every new visible element:**

1. **What region owns this?** Title actions (`PageShell` `#actions`), a `SettingsRow` control
   column, a dialog footer, inline field feedback, or a toast — pick one. If you can't name
   the owner, it's probably stray.
2. **Can it go away entirely?** Most save/sync feedback does not need a persistent label.
   A disabled Save button already says "nothing to commit"; a success toast on explicit Save
   is enough acknowledgement. **Do not show a standing "Saved" state** — it duplicates what
   the quiet synced form already communicates and tends to land misaligned.
3. **Does this capability already have a house pattern?** Before writing any new status UI,
   search refactored pages for the same job (see § 8 Save model, `profile` auto-save,
   `PageShell` + Save in `#actions`). Reuse that model; don't invent a third one for the
   same page type.
4. **If it must stay, compose it — don't free-float it.** Unsaved hints belong in the
   `#actions` cluster next to the Save button (same `flex` row, same baseline), not in a
   separate corner. Loading belongs on the button (`Spinner`) or in the row being fetched
   (`Skeleton`), not as orphan text.

**Save / sync feedback — the reference models (do not invent a fourth):**

| Model | When | Feedback |
|---|---|---|
| **Silent auto-save** | Toggles / selects that commit on change (`profile`) | Nothing on success; `toast.error` + rollback on failure |
| **Manual Save in `#actions`** | A form the user batches (`PageShell` + Save button) | Button disabled when synced; `Spinner` while saving; **one** success toast on click |
| **Explicit unsaved only** | Manual-save pages where drift is easy to miss | Show `common.unsaved` **only while `hasChanges`**, inside `#actions` beside Save — never a parallel "saved" label when synced |

Anti-example: a floating "已保存" that doesn't share the `#actions` right edge with the
cards below — it answers a question the user isn't asking and breaks the column grid.

### 11. Forms follow one standard; controls are sized on purpose

There is one house form, and the **New Task dialog** (`bot-schedule.vue`'s create/edit
`Dialog`) is it. Mirror its anatomy — don't reinvent a form per page (recipe in
`reference.md` § Form):

- **Title:** a plain `DialogTitle` that names the action ("New Task"), nothing more — no
  subtitle restating it.
- **Fields:** one `space-y-4` column; each field is a `Label` + control in a `space-y-1.5`
  group. Optional fields mark the **label** with a quiet `(optional)`, never the placeholder.
- **Group what belongs together** on one aligned row (a name field + its enable toggle), with
  the heights matched (§ 4).
- **Progressive disclosure:** secondary settings live behind a **More options** chevron,
  collapsed by default; rarely-needed power input (a raw cron, an "advanced" mode) sits in that
  zone — not in the user's face.
- **Footer:** a ghost **Cancel** + a single filled primary (Create / Confirm); a destructive
  delete, when present, sits far left. Validate on **submit**, not on blur (§ 2) — gate the
  primary with a `canSubmit` and surface the error only when they try.

**Size controls deliberately — not "all small," not "all large."** The height ladder is
`sm` h-8 · default h-9 · `lg` h-10. **Default (h-9, full height) is the norm**, and it is the
*only* right size for a form footer and any primary action — a footer of squat half-height
`sm` buttons is a tell that the page wasn't thought through. Drop to `sm` only where space is
genuinely tight *and* the action is secondary (a dense toolbar, an inline-in-field button, a
per-row action in a long list); reserve `lg` for a rare, deliberate hero CTA. Pick the rung for
a reason every time; never shrink everything to look "compact" nor inflate everything to look
"important."

### 12. The root surface vs. entry-point depth (the 99/1 rule)

This is the rule that decides **what belongs on the first page at all** — upstream of how it's
framed (§ 9). It came from a workspace page that shoved a resource-limit form, a snapshot
history list, and restore/backup buttons onto the root, so a user who came only to check "is my
bot's runtime alive?" was met with input boxes and a wall of controls.

The two-sided principle, in the developer's own words: *we cannot make 99% of users carry the
visual weight of 1% of users' needs — but we also cannot sacrifice the 1%'s path for the 99%.*

- **The 99% came to glance.** The root surface answers one question — *is it working, and how
  much is it using?* That is the hero content (a status badge + a usage tile row). It must not
  hand a status-checker a form, a button pile, or a history list they didn't ask for.
- **The 1% came with a purpose.** Someone who needs to set a limit, take/restore a snapshot, or
  delete the thing is **not** browsing — they arrive intending that operation and will look for
  its button. So you do **not** protect the 99% by deleting the 1%'s path; you protect it by
  giving the path a **named entry point**: a quiet one-line row (`label` + a read-only summary as
  its description) with a button that opens a **focused form/dialog** containing the inputs, the
  data (e.g. the snapshot list), and the action (e.g. rollback). The summary line doubles as a
  calm read-only glance for the 99% and the discoverable door for the 1%.
- **Name the door by what's behind it.** *Resource limits*, *Snapshots & restore*, *Details*,
  *Delete* — each its own entry → its own focused surface. Do **not** merge them into one
  "Advanced" drawer (§ 9): a drawer that mixes diagnostics with destructive operations hides the
  1%'s path *and* muddies it.
- **Empty state is the same discipline.** When there is nothing to glance at yet, the root is a
  calm invitation + one guiding action — and even *creating* is a deliberate step, so it opens a
  focused create dialog rather than dumping a creation form onto the first page.
- **The test:** for every block on the root, ask *"did the 99% come to see this?"* If no, it is
  not deleted — it is moved behind a named entry point. The first page holds only what answers
  "is it working."

### 13. Live & status surfaces — fresh, quiet, and singular

A surface whose job is to show *live* state — a runtime's resource use, a desktop's screen, a
session list — has its own discipline, distilled from the Workspace and Desktop tabs:

- **Self-refresh; don't hand the user a Refresh button.** A status/preview surface keeps itself
  current on its own — a silent poll while it's on screen (guard on `document.visibilityState`,
  clear the interval on unmount) or a live stream. A standing *Refresh* button is a confession
  the page doesn't stay fresh, and it drags in the cross-app mess of "some refreshes have an
  icon, some don't, some are `sm`." Reserve a manual refresh only for a genuinely expensive,
  explicit re-fetch the user deliberately chooses to pay for.
- **Relative time for "updated", never a ticking absolute stamp.** "Updated just now / 5 min ago"
  answers the only question the user has — *is this current?* — and stays calm. A full
  `Updated 06/16/2026, 20:04:11` re-renders on every sample and reads like a log line. Use the
  shared locale-aware relative-time formatter.
- **One state, one place.** Render any given status/progress exactly once. The Desktop prepare
  flow shipped the same install progress *twice* — a centered card plus a redundant bottom bar
  that blended into the background and grew a stray scrollbar. Two renders of one state drift,
  conflict, and clutter; keep the one that belongs and delete the other.
- **A cover over a media surface is opaque, not translucent.** A black video/screen frame behind
  a `bg-background/95` "preparing" cover bleeds black through the rounded corners. If the cover
  is meant to *hide* the surface while it loads, make it fully opaque (`bg-background`);
  translucency is only for a scrim you intend to see through.
- **Distill backend flags to one human status — let the live surface carry the rest.** Don't
  mirror every readiness boolean (enabled / running / desktop / browser / toolkit) into a flag
  grid sitting next to the very thing it describes; the live view already shows connecting /
  installing / live / can't-reach. Surface at most one distilled human status, and a *problem*
  only when it's actionable (§9: a healthy state says nothing).
- **Enumerate the in-between states; a blank surface is not one of them.** This is the step that's
  easy to skip and the one that bites: a live surface has more states than "loading" and "live" —
  *idle, connecting, connected-but-no-frame-yet, reconnecting, can't-reach.* The Desktop tab
  shipped the happy path and the explicit *prepare* path and left the gaps, so the user sat on a
  blank black box that reads as broken. **List every state first, then give each one a visible,
  centered affordance** (spinner + one line, *over* the surface — not in an edge footer that a
  tall 4:3 frame pushes off-screen, which is how the status hid in the first place). And **gate
  the "live" view on real content, not the transport flag**: a WebRTC connection reports
  `connected` *before* the first frame paints, so switching on the flag still shows blank — hold
  the loading state until a frame actually arrives (`@playing` → a `videoReady` ref, reset on
  teardown). The flag says the pipe is open; only a frame proves something is on screen.

## The review ritual — run it on every finished page

When a page looks done, do **not** stop. **Open it in the running dev app** (`mise run dev`) —
and the component wall (`Cmd/Ctrl+Shift+D`) for any component you're unsure about — and look at
the real thing as a first-time user who has never seen it. Reading the code is not the review;
seeing it rendered is:

- Name everything you see, top to bottom. Is any of it filler? Is any guidance missing?
  Any **stray fragment** (orphan status text, a chip that doesn't share an edge with its
  region)?
- How does it sit in the viewport? Is it balanced **left ↔ right**? **top ↔ bottom**?
- Force the **empty** state and the **loading** state. Does the frame still hold?
- Walk **every interaction in the step-2 behavior inventory** — does each one still work? Did
  the refactor quietly drop a feature, a state, a shortcut, or a path?
- Do all **same-row controls** line up in height? Do cards share one stroke, one radius?
- **Dark mode (mandatory, no lint net for app pages):** grep for raw colors (`bg-white`,
  `text-black`, `text-gray-`, `bg-gray-`, `#`, `dark:`, inline `style=`), then **flip the app
  to dark and look** — is anything still glued to a light value?
- Shrink to the **narrowest pane width** (and check `zh`): does anything overflow or clip?
- **Switch between sibling pages/tabs that share the scroller** — one long (scrolls), one short
  (doesn't). Does the title / card edge stay put, or jump sideways by a scrollbar's width? If it
  jumps, the page-level scroll container is missing `[scrollbar-gutter:stable]`.
- Are dividers the right kind — **inset** inside cards, **full-bleed** as structural splits?
- Is the **save model** right (auto-save silent vs deliberate manual save), with no toast
  spam on ambient changes?
- Is there any **hover-rise**, any tinted card, any off-scale text, any hand-rolled control,
  or any **hand-built menu** (raw button rows / self-drawn dividers inside a popover)?
- **Re-draw the finished page as a wireframe and re-count its complexity** (§ workflow step 4):
  any card-in-card? any icon stacked inside a card? any nesting layer that adds depth but no
  meaning? If the sketch is busier than the page needs to be, flatten it before you ship.
- **Reuse audit:** did you reuse every component you could — or hand-write / duplicate something
  a primitive or a shared composition already covers? Is any brand-new component cleared with
  the developer? Was a repeated arrangement extracted, not pasted?
- **Import audit — a `@memohai/ui` tag you forgot to import silently degrades to a native
  element, and *nothing flags it*.** Vue resolves an unimported PascalCase tag against the DOM:
  `<Progress>` falls back to the native `<progress>` (a chunky grey OS bar, `model-value` inert),
  `<Switch>`/`<Dialog>` to unknown/native elements — eslint and `vue-tsc` both pass, so it only
  shows up in the *rendered* page (the canonical bug: a billing meter that shipped as a fat grey
  block instead of the thin token bar). For every `@memohai/ui` component the template uses,
  confirm it's in the `import { … } from '@memohai/ui'` list — and trust the rendered pixels over
  "I wrote `<Progress>`, so it's the Progress component."
- **Forms & sizing:** do forms match the New Task standard, and is every button sized on purpose
  (default h-9 for primaries / footers, `sm` only where genuinely tight)? No squat `sm` footers.

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
4. **Wireframe before you build, and audit the complexity.** Before writing any markup, sketch
   the target as an ASCII wireframe (template in `reference.md` § Wireframe) — the shell, each
   card group, the rows, the controls — and read it like space-complexity: count the boxes and
   the nesting depth. Kill **card-in-card** (a bordered box moated around small boxes), kill
   **icons stacked inside a card**, kill any layer that isn't earning its keep. The cheapest
   place to delete a needless layer is here, on paper, before it exists in code.
5. **Rebuild** from the shell down, **reusing components, never hand-writing them**: page shell →
   `SettingsSection`/`SettingsRow` groups → the right `@memohai/ui` atoms, tokens only →
   **re-wire every behavior from the step-2 inventory** → copy through the § 1 interrogation →
   empty states that hold the frame → aligned same-row heights → only the motion that fits. If a
   composition repeats, extract it; if you think you need a new component or an icon, get the
   developer's sign-off first.
6. **Review ritual** above + `mise run lint`.
7. Keep code comments about **why** (the reference pages do this well); never narrate the
   change itself, and never name an external product in a comment.

## Comments & code style

The refactored pages carry short comments explaining *why* a block exists, why it's hidden
in some states, why there's no card-in-card, why a Badge instead of a loose dot, why `—`
instead of a faked `0`. Match that: comments justify a non-obvious decision, they do not
restate the code. Follow `apps/web/AGENTS.md` (semantic tokens only, lucide icons, i18n keys,
vee-validate + Zod, SDK for data).
