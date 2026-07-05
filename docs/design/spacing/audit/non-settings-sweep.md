# Non-Settings Surface Spacing Sweep

**Goal:** extend the owner-vocabulary spacing system beyond the settings/form/list
language it currently covers, into the non-settings surfaces that were explicitly
scoped OUT of the first pass — so the branch can carry a *complete* spacing baseline
PR, not a settings-only one.

This is an **expansion of the contract**, not a violation of it. The owner skill
today says "owners are for the settings/form surface; sidebar rows, file-tree rows,
chat message rows live in non-settings surfaces with their own row systems" — i.e.
those surfaces were left un-normalized on purpose. This sweep revisits that line:
where a non-settings shape genuinely recurs, it earns its own owner; where it is a
genuinely one-off compound block, it stays local (and we say why).

## Judgment rule (the constitution for this whole sweep)

The Design System here is **new** — built over the last stretch, not inherited. So the
codebase has **no pre-existing unified spec**. That flips the default stance on every
差异 (difference) we find:

> **A difference is assumed to be historical sediment (random hand-writing), NOT
> design intent — until proven otherwise. The burden of proof is on KEEPING the
> difference, not on unifying it.**

Concretely, when a shape differs from an owner:
1. **Default: unify onto the owner.** "Why is this row 44px instead of 60px?" is usually
   answered by "because someone wrote `py-3` with no min-h and copy-pasted it 3×" — that
   is not a decision, it's the absence of one. Unify it.
2. **A difference that can be JUSTIFIED as serving a real need** (e.g. `font-mono` because
   the chip shows a technical identifier, matching the mono language of its sibling
   tool-call values) is **absorbed into the owner via an enhanced API** (Badge grows a
   `mono` prop) — NOT deleted, and NOT left as a hand-written special case.
3. **Do not retreat into LOCAL** to avoid a visible change. A visible height/shape change
   on migration usually *proves* the drift was real (this row didn't match its own
   neighbours). "It would look different" is a reason to unify, not to skip.

This is the standing default for D → A → B. LOCAL is now reserved for genuinely
different *spatial relationships* (a centered empty state is not a row), never for
"a slightly different number that nobody chose."


## Method

Same loop that worked for settings: **audit → define owner → migrate → guard**.
Five parallel read-only Sonnet auditors, each on one surface cluster, each classifying
every recurring shape as:

- **NEW-OWNER-CANDIDATE** — repeats ≥2–3× with no existing owner → propose an owner.
- **MISS** — actually fits an EXISTING owner (esp. PageShell/SettingsSection on a
  small standalone page) but was hand-rolled.
- **LOCAL** — genuinely one-off / genuinely-different relationship → stay hand-written.

## Surface clusters (auditors)

| # | Cluster | Scope |
|---|---------|-------|
| 1 | Chat composer + message rendering | `pages/home/**` — composer, message rows, attachment cards, tool-call panels |
| 2 | Session list + workspace tabs + panels | `pages/home/**` — session list rows, dockview tab bar, panel chrome |
| 3 | App sidebar | `components/sidebar/**`, `components/settings-sidebar/**` |
| 4 | Onboarding wizard + supermarket | `pages/onboarding/**`, `pages/supermarket/**` |
| 5 | Workspace/display panes + small pages | `file-manager`, `monaco-editor`, `pages/{login,oauth,memory,people,usage,about,transcription,voice}` |

## Scope notes

- `pages/dev/**` (20 files) is the dev component wall — a reference surface, NOT
  production; excluded.
- `pages/bots/**` and `pages/web-search/**` are settings-language and were already
  covered by the first pass; excluded here.
- The `packages/ui/src/components/sidebar/**` shadcn-port library finding is already
  captured by the separate architecture audit; auditor 3 covers only the *app* sidebar.

## Findings

### Auditor 5 — workspace/display panes + small standalone pages ✅

**NEW-OWNER-CANDIDATEs**
- **`PanePlaceholder`** (icon/text centered empty state) — **6 occurrences / 6 files**,
  the widest-recurring shape in this cluster. `file-viewer.vue:911,932`,
  `dockview/panel-asset.vue:31`, `dockview/panel-preview.vue:26` (gap/size drift),
  `browser-pane.vue:56`, `dockview/workspace-watermark.vue:3`, `chat-pane.vue:3`.
  Fingerprint `flex h-full flex-col items-center justify-center gap-{2|3}` + icon
  `size-{10|12} opacity-{30|40}` + `<p class="text-xs">`. Drift: gap 2↔3, size 10↔12,
  opacity 30↔40; same i18n string hand-copied across two components. Props:
  `icon?`, `title`, `description`, or `variant: 'icon'|'text'`.
- **`DiffTitleBar`** (editor compare/diff header strip) — 2 occurrences, **byte-identical**
  across components: `file-viewer.vue:821` == `monaco-editor/diff.vue:112`. Not a
  one-off editor internal (it's shared across two components), so the Monaco exemption
  doesn't apply. Props `left`, `right`, slot `#actions`.
- **Dockview round icon button** — 5 occ / 3 files (`group-actions.vue:10`,
  `header-add-actions.vue:21`, `prefix-header-actions.vue:14,34,47`). Verdict: this is
  a `@memohai/ui` Button preset (`size="chrome"` / `variant="chrome-icon"`) problem,
  NOT a page-level spacing owner. Lower priority; freeze the class if it grows again.

**MISS (fits an existing owner, hand-rolled)**
- **PageShell** — `memory/index.vue:61`, `voice/index.vue:132` copy the literal
  `mx-auto max-w-3xl px-6 pt-10 pb-12` instead of `<PageShell>`. `people`/`usage` in the
  same batch already use it correctly → pure oversight. **Zero-risk literal swap.**
- **SettingsSection** — 5 occ / 3 files hand-roll the grey-label + content without the
  card: `people/index.vue:22`, `usage/index.vue:100,193`, `voice/index.vue:141,199`.
  `voice` uses `text-foreground` (color drift) + a hint `<p>` the component lacks →
  **SettingsSection needs an optional `description` prop** before voice can migrate;
  people/usage are pure MISS, migrate directly.
- **SettingsRow** — `transcription/provider-setting.vue:169` hand-rolls a model-list
  row + manual `border-b` (reinvents `last:border-b-0`) while the identity row above it
  already uses `<SettingsRow>`. Sibling files `speech/`, `video/provider-setting.vue`
  do the same (3-file family) — transcription is the cleanest single fix.
- **panel-schedule.vue:5** panel header ~ PageShell (weak, single instance, floating
  panel not a route) → record, don't force.

**LOCAL (verified, stay hand-written)**
- file-tree rows (`file-tree-node.vue:117,200`) — skill-exempt non-settings row system.
- file-viewer ghost buttons (7×, same file) + Monaco internals — skill-exempt.
- panel-browser/display/terminal root wrappers — trivial layout shell.
- `about/index.vue:2` — self-documented deliberate different relationship (sparse,
  upper-middle float); already composes owners elsewhere.
- `login` vs `oauth/mcp-callback` full-screen centered — only 2, inner content differs;
  revisit if a 3rd appears.
- `memory/components/builtin-config.vue` small boxes — one-off, per-hint tone.

### Auditor 4 — onboarding wizard + supermarket ✅

**NEW-OWNER-CANDIDATEs**
- **`OnboardingFooterNav` / `WizardStepFooter`** — footer container 6×, button pairs 13×,
  ALL hand-rolled bare `<button>` (not even `@memohai/ui` Button). `Step2:112,116,122`,
  `Step3:442,446,452,695,702,709,892,896,903`, `Step4:788,792,798,824,828,834`,
  `Step1:215`, `Step5:102`. Drift: width `w-[180px]`↔`min-w-[180px]`↔`w-[240px]` (3 sets),
  `disabled:opacity-50/60` inconsistent, Step1 extra ring. Props: `prevLabel`,`nextLabel`,
  `nextDisabled`,`nextLoading` (built-in Spinner+Transition, itself duplicated in Step3/4),
  slots `#prev`/`#next`.
- **`OnboardingStepFrame`** (the WIZARD FRAME: exit-anim shell + body + title) — 5 Steps.
  Exit shell `transition-all duration-[175ms]` + `scale-[0.88] opacity-0` (Step1-5);
  body `h-[35rem] max-h-[calc(100dvh-7rem)] flex flex-col` (5×, `pt-16`/`pt-24`/none drift);
  title `text-3xl font-semibold mb-{3|6|8}` (3×); scroll body
  `min-h-0 flex-1 overflow-y-auto -mx-2 px-2 -my-1 py-1` (5×, byte-identical). Props
  `title`,`bodyPadding`,`visible`; slots default/`#header`/`#footer` (composes FooterNav).
- **`ChoiceTile`** (grid icon+label 3-choice) — `Step3:401,411,427`,
  `h-16 rounded-lg border ... flex items-center gap-2.5`. Near-zero drift (new code).
  NOT BackendCard (that's vertical-list p-3.5+chevron) nor PersonaTile (centered). Props
  `icon` slot, `label`, `variant:'dashed'|'solid'`. (ACP setup-mode segmented buttons
  `min-h-10` are denser → stay local per skill.)
- **`MarketItemCard`** (`plugin-card.vue`/`skill-card.vue`) — same shape, two hand copies.
  **REAL VISIBLE BUG: hover feedback differs** — skill-card has border/bg hover, plugin-card
  has none; also `overflow-hidden` on icon box differs (clip behavior). Root
  `group flex flex-row items-start gap-3 p-4`. Make root `<Card role="button">` to dodge
  the button-in-button trap; props `leading`,`title`,`homepage`,`description`,`installLabel`,
  slot `#actions`.

**MISS (existing owner, hand-rolled)**
- **CalloutBanner** — 7× hand-rolled warning/notice boxes bypassing the existing owner
  (already used correctly in bot-overview/bot-container). `Step4:479,566,576,584`,
  `Step5:89`, `Step3:868,876`; full destructive replica at `Step3:584-634`. Drift:
  `rounded-md`↔`rounded-lg`, `px-3 py-2`↔`py-2.5`↔`px-4 py-3`, icon inconsistent — no two
  alike. **Requires: relax CalloutBanner `title` to optional** (single-line notices have
  no title) before the 6 single-line ones can migrate; the `:584` destructive card
  migrates directly.

**Duplicate-code debt (0 drift now, but copy-paste = latent debt)**
- `plugin-detail.vue` vs `skill-detail.vue` — **byte-identical** page structure copied
  twice; local `InfoItem` `defineComponent` (`h()`) is a literal copy
  (`plugin-detail:233` == `skill-detail:172`). Extract `InfoItem` + a `MarketDetailHeader`.

**LOCAL (verified, stay hand-written)**
- Step1 intro animations, Step5 celebration cards (1×), ColorSchemeCard (already a comp),
  Step4 OAuth device-flow (skill-exempt), `model-item.vue` dense row (`min-h-[3.25rem]`,
  correctly NOT a SettingsRow).
- Cross-scope note: `Step4Bot.vue:420` avatar+name-field block ≈ `bots/new.vue:40`
  near-verbatim — the "avatar edit row" shape has spilled beyond onboarding; handle when
  `bots/new.vue` is next touched.

### Auditor 2 — session list + workspace tabs + panels ✅

Surface has ZERO owner coverage → almost everything is NEW-OWNER-CANDIDATE.

- **`DockPanelFrame`** (dock panel host shell) — 7×: `dockview/panel-{chat,browser,display,
  terminal,file,preview,asset}.vue:2`. Fingerprint `flex flex-col h-full w-full` + inner
  `flex-1 min-h-0`. Drift: class order flipped (asset), `relative` on 3/7 uncommented,
  `bg-surface-editor` on file/preview/asset. Props `editorSurface?`, slots `#header`/default.
- **`SidebarPanelHeader`** — 4×: `sidebar/panel-sessions.vue:3`, `recents.vue:9`,
  `panel-schedule.vue:47`, `files-pane.vue:8`. Label `text-xs font-[550]
  tracking-[-0.02em] text-muted-foreground/80 pl-[11px]`. **files-pane.vue:3 comment
  self-admits it's the same shape kept in sync by hand across files.** Container geometry
  drifts 4 ways (`pt-1`/`pt-2`/`h-8`/`min-h-`). Props `label`, slot `#trailing`.
- **`CircleIconButton`** — 7×: `prefix-header-actions.vue:14,34,47`, `group-actions.vue:10`,
  `header-add-actions.vue:21`, `sidebar/index.vue:96`, `panel-schedule.vue:56`.
  **⭐ CONVERGES with auditor 5's "dockview round icon button" — verdict same: this is a
  `@memohai/ui` Button `shape="circle"` atom variant, NOT a layout owner.** `size-7` explicit
  on 5, implicit via `icon-sm` on 2 → will fork if `icon-sm` default changes.
- **`ListRow`** (clickable sidebar list row) — 3×: `session-item.vue:6`,
  `bot-switcher.vue:81`, `session-search-dialog.vue:41`. Drift: 3 totally different
  height/radius/padding sets. **bot-switcher must stay bare `<div>` (sortablejs) → owner
  must be class-only (`listRowClass` util) or polymorphic `<ListRow as>`, not a wrapper.**
- **`EmptyStateNotice`** — 3×: `home/index.vue:4`, `chat-pane.vue:3`,
  `dockview/workspace-watermark.vue:2`. **⭐ CONVERGES with auditor 5's `PanePlaceholder`
  (same chat-pane.vue:3, workspace-watermark evidence).** Inner two `<p>` byte-identical.
  Merge these two candidates into ONE owner.
- **`InlineLoadingRow`** — 4×: `recents.vue:86,102` (py-3 vs py-4 in the SAME file),
  `bot-switcher.vue:112`, `chat-pane.vue:40`. `flex justify-center py-N` + LoaderCircle,
  py 2/3/3/4, icon 3.5/4. Props `size?`.
- **`ConfirmDeleteDialog`** — 3×: `recents.vue:114`, `sidebar/panel-schedule.vue:89`,
  `dockview/panel-schedule.vue:38` (last two near-byte-identical). More logic than spacing,
  but same 同形异码 symptom. Props `title`/`description`/`confirmLabel`/`loading`.
- **`SidebarNavButton`** — 3×, **class byte-identical**: `panel-sessions.vue:12,25`,
  `sidebar/index.vue:134`. `h-9 justify-start gap-[9px] px-[11px] text-control ...`.
  Zero drift now (commented-out "experiment" version proves it already changed once).

**LOCAL (verified)**: trivial muted no-results `<p>` (skill-exempt), `panel-schedule.vue`
full-form container (settings-style detail, different from the 6 media panels), SVG active-tab
shapes (`terminal-tab`/`workspace-tab`, genuine one-off geometry), bot-switcher bare div
(sortablejs). **Bug noted (not spacing):** `panel-schedule.vue:22` references a `scoped`
class only defined in `recents.vue` → silently inert.

### Auditor 1 — chat composer + tool-call rendering ✅ (chat's deepest 同形异码 layer)

- **`ToolDetailPanel`** (tool-call detail container w/ empty slot) — **21× empty-state rows**
  across tool-call render components. The single widest shape in chat.
- **`LabeledMonoRow` / `ListRowWithBadge`** (field-display skeleton: label + mono value,
  optional trailing badge) — recurring across `contacts/memory/schedule/email-accounts` tool
  panels. `text-xs` on parent vs self is the main drift axis.
- **`ToolResultPreviewBox`** (result preview) — `max-h` in **5 buckets** + `border`/
  `overflow-x-auto` each file's own.
- **`CollapsibleHeaderRow`** — `py-0.5` vs `py-px`, `group` vs `group/h`, `duration-75`
  present/absent.
- **`ExpandChevron`** (`:open`) — 3 implementations: `tool-call-cluster.vue:10` (single icon
  rotate), `tool-call-group.vue:26` + `tool-call-inline.vue:56` (two-icon swap, v-if order
  flipped), hover opacity 45/50/60.
- **Nested "capsule card" shell** — `tool-call-group.vue:45` (`rounded-md bg-muted px-2.5
  py-1.5`) vs `tool-call-inline.vue:182` (`rounded-md bg-muted px-3 py-2`) mergeable; the
  `bg-card` inGroup variant (`:181`) is commented LOCAL (card-in-card).

**MISS (existing atom, hand-rolled)**
- **`Badge`** — `contacts.vue:17`, `memory.vue:17`, `schedule.vue:16`, `email-accounts.vue:15`
  **byte-identical** `text-caption text-muted-foreground font-mono shrink-0 rounded bg-muted/30
  px-1 py-0.5`. `@memohai/ui` already has `Badge` (`rounded-full` + accent-soft token); these
  hand-write `rounded` + `bg-muted/30` (alpha hack). Return to the atom.

**Cross-cutting (not a shape — flagged for guard-scope check)**
- Raw px instead of semantic type tokens: `exec.vue:2` `text-[12px]`, `output.vue:6`
  `text-[12px]`, `tool-call-inline.vue:4`/`group.vue:16` `text-[0.90625rem]`, `group.vue:45`
  `text-[0.84375rem]` — none land on `--text-caption/body/label`.
  **GUARD-SCOPE VERIFIED (I own the guard): the "may not cover pages" suspicion is WRONG —
  `APP_DIRS = ['apps/web/src']` walks `pages/` recursively; no scope gap.** But there is a
  real *rule-dimension* gap: guard hard-fails `text-[Npx]` (px never scales — the `text-[12px]`
  hits are just grandfathered in the px baseline), and by design does NOT flag `text-[0.9rem]`
  arbitrary-rem, because rem scales with the user font setting. What the auditor wants — "must
  land on a `--text-*` semantic token, not an off-scale rem" — is a NEW guard dimension
  (type-token discipline), NOT a scope fix. **Candidate guard rule, defer to ranking.**
- NOTE: auditor 1 spawned 3 sub-agents (message rows / attachments / trigger blocks) that may
  still be running; if they report, fold their findings before final ranking.

### Auditor 3 — app sidebar (components/sidebar + settings-sidebar) ✅

- **`SidebarPanelHeader`** — 4×: `files-pane.vue:8`, `panel-schedule.vue:48`,
  `panel-sessions.vue:3`, `recents.vue:9`. Label `text-xs font-[550] tracking-[-0.02em]
  text-muted-foreground/80 pl-[11px]`; containers drift `min-h-[1.6875rem]`/`h-8`/none,
  `mt-2`/`pt-2`/`pt-1`. **⭐ SAME candidate as auditor 2's SidebarPanelHeader — confirmed
  by two independent auditors.** Props `label`, slots default/`#actions`.
- **`SidebarNavButton`** — 3×, **byte-identical**: `index.vue:135`, `panel-sessions.vue:12`,
  `panel-sessions.vue:25`. `h-9 justify-start gap-[9px] px-[11px] text-control font-medium
  text-foreground/92`. **⭐ SAME as auditor 2's SidebarNavButton.** Wrap `<Button ghost block>`.
- **`sidebar/menu-row.vue`** (clickable icon/avatar + label + trailing list row) — 2×:
  `bot-switcher.vue:81`, `session-search-dialog.vue:41`. **⭐ SAME as auditor 2's ListRow.**
  Height `py-1.5` vs `h-9`, hover `--bot-row-tint` vs `--ui-hover`. Class-only (bot-switcher
  needs bare `role=menuitem` div for sortablejs).

**MISS (existing atom/owner, hand-rolled)**
- **Button `loading-mode="leading"`** — 8× hand-stuffed spinners bypassing the atom's
  existing `loadingMode` prop: `files-pane.vue:197,230,263,294,325`, `panel-schedule.vue:112`,
  `panel-sessions.vue:~133,~172` (latter two use bare `LoaderCircle animate-spin`, not even
  the Spinner atom). Drift `mr-1`/`mr-1.5`, `size-3`/`size-4`; also `DialogContent`
  `sm:max-w-md`/`sm:max-w-sm` + confirm text `text-xs`/`text-sm` drift. Use the atom's prop.
- **`SidebarGroupLabel variant="nav"`** — `settings-sidebar/index.vue:71,103` byte-identical
  `!important` override `h-6! pl-[14px]! pr-3! font-[475]` → the atom default never fit;
  add a `nav`/`dense` variant, drop the `!` overrides + the paired `SidebarGroupContent pt-0`.

**Round icon button** — same evidence as auditors 2 & 5 (`sidebar/index.vue:96` search,
`panel-schedule.vue:56` new). **⭐ THREE auditors converge → `Button shape="circle"` atom.**

**LOCAL (verified)**: trivial muted no-results `<p>` (3×, py drift ignorable), centered
loading spinner rows (skill-exempt placeholder), files-pane internal toolbar icon buttons
(single-file, inline-fixable), `SidebarGroup` px/pt (intentional geometry by header presence).

_(all 5 auditors in — final dedup ranking below)_

---

## FINAL DEDUP RANKING (cross-auditor merge)

> **2026-07-05 状态注记:这份 ranking 是历史决策记录,不是待办清单。** Tier A 已全部落地,
> Tier B/C/D 大半落地或已裁决 —— 以文末《EXECUTION LOG — Tier A/B passes (2026-07-04/05)》
> 为准;与该日志矛盾的表格行,以日志为最终状态。

The value of running all five in parallel: the same shapes surfaced from *different*
surfaces, and only a global merge reveals which are truly system-wide. Candidates that ≥2
auditors hit independently (⭐) are the strongest owner cases.

### Tier A — new owners, cross-surface, build these first
| Owner | Occ | Auditors | Note |
|---|---|---|---|
| **PanePlaceholder / EmptyStateNotice** (MERGED) | ~9 | 5 + 2 | widest shape; same chat-pane/watermark evidence. Props `icon?`,`title`,`description`. |
| **SidebarPanelHeader** | 4 | 3 + 2 | files-pane comment self-admits hand-sync. Props `label`, `#actions`. |
| **SidebarNavButton** | 3 | 3 + 2 | byte-identical, 0 drift now = lock before it forks. |
| **InlineLoadingRow** | 4 | 2 | py drifts within one file. Props `size?`. |
| **DockPanelFrame** | 7 | 2 | dock host shell. Props `editorSurface?`, `#header`. |

### Tier B — new owners, single-surface but high recurrence
| Owner | Occ | Note |
|---|---|---|
| **OnboardingStepFrame + OnboardingFooterNav** | 5 + 13 | wizard skeleton, 13 bare buttons, width drift ×3. |
| **ToolDetailPanel** (+ tool-call field row / preview box / chevron) | 21+ | chat's densest layer; several sub-shapes. |
| **MarketItemCard** | 2 | **fixes a real visible hover bug** (plugin vs skill card). |
| **ChoiceTile** | 3 | grid icon+label 3-choice; 0 drift (new code, lock early). |
| **sidebar/menu-row (class-only)** | 3 | ListRow; must be class-only for sortablejs. |
| **ConfirmDeleteDialog** | 3 | more logic than spacing; 2 near-identical. |

### Tier C — atom-layer variant gaps (NOT layout owners)
| Fix | Occ | Auditors | Note |
|---|---|---|---|
| **Button `shape="circle"`** | 7+ | 3+2+5 ⭐⭐⭐ | strongest convergence; fix in `@memohai/ui`, not a wrapper. |
| **Button `loading-mode="leading"`** | 8 | 3 | atom prop already exists, just use it. |
| **SidebarGroupLabel `variant="nav"`** | 2 | 3 | drop `!important` overrides. |

### Tier D — pure MISS onto EXISTING owners (no new component, lowest risk)
| Owner | Sites | Risk |
|---|---|---|
| **PageShell** | `memory:61`, `voice:132` | zero — literal swap, fixes visible title misalignment |
| **CalloutBanner** | onboarding 7× (needs `title` optional) | low |
| **SettingsSection** | `people:22`, `usage:100,193`, `voice:141,199` (voice needs `description` prop) | low |
| **SettingsRow** | `transcription/provider-setting:169` | low |
| **Badge** | tool-call `contacts/memory/schedule/email-accounts` 4× | low |

### Tier E — guard / duplicate-code (not spacing shapes)
- **New guard dimension?** type-token discipline (arbitrary-rem `text-[0.9rem]` not landing
  on `--text-*`). Deferred decision — guard currently only catches px, by design.
- **DiffTitleBar** (2×, byte-identical cross-component) + **InfoItem/MarketDetailHeader**
  (plugin-detail == skill-detail byte-identical) — extract-shared-code, low urgency.

### Verdict
- **~5 Tier-A owners + 3 Tier-C atom fixes** give the biggest system-wide payoff.
- **Tier D is the cheapest win** — pure MISS onto owners that already exist, zero new
  components, and PageShell fixes a bug the user can already SEE.
- Tier B is real but heavier (chat/onboarding are big surfaces); stage after A/C/D.


---

## EXECUTION LOG — Tier D pass (2026-07-04)

### Landed
| Change | Sites | Notes |
|---|---|---|
| `memory` → PageShell | `memory/index.vue` | literal swap; fixes visible title misalignment |
| `people`, `usage` → SettingsSection | `people:22`, `usage:100,193` | as planned |
| Badge `font="mono"` variant (cva) | `packages/ui/badge` + 4 tool-call chips | replaces the `bg-muted/30 px-1 py-0.5` hand-rolled chip; opt-in, orthogonal to size/variant |
| **NEW OWNER: `SectionGroup`** (`components/section-group/index.vue`) | `voice` ×2, `web-search` ×2 | see assembly rule below — this REPLACES the Tier-D plan of pushing voice into SettingsSection |
| `video` → PageShell single-group | `video/index.vue` | dropped the redundant `video.providersTitle` second title tier (removed from zh/en/ja locales); hint → PageShell `description`, Add → `#actions` default Button — byte-matches `providers` |

### Plan corrections discovered during execution
- **voice ≠ SettingsSection** (the Tier-D table above is WRONG on this row). Pushing voice
  into SettingsSection muted its foreground group title and inset the Add button 8px —
  user immediately flagged "两套壳子" vs web-search. The `text-foreground text-label`
  group header recurs across voice / web-search / bots / bot-memory / providers model-item:
  cross-page consistency = design intent, a *different tier* from SettingsSection.
- **transcription/provider-setting:169 is NOT a SettingsRow MISS** — it (and speech/video
  model lists) are clickable navigation rows → belongs to a Tier-A list-row owner.
- **onboarding CalloutBanner ×7 was audit over-merging** — no clean MISS there; dropped.

### THE ASSEMBLY RULE — provider gallery pages (the durable outcome)
Two tiers, never mixed:
- **SettingsSection** = MUTED label + bordered card wrapping its body (settings-row tier).
- **SectionGroup** = FOREGROUND `text-label` label + BARE body whose content carries its
  own borders (page-content tier). No card-in-card.

Assembly by group count:
- **Single-group page** (`providers`, `video`): PageShell owns everything — title,
  hint as `description`, Add in `#actions` — body is the BackendCard grid directly.
  NO SectionGroup: wrapping the only group duplicates the page title at a second tier.
- **Multi-group page** (`voice` TTS/STT, `web-search` search/fetch): PageShell page title,
  then `div.space-y-8` stacking one `SectionGroup` per group (group title + hint +
  per-group Add + grid).
- New gallery pages: count the groups first — one group → copy `providers`; several →
  copy `voice`. Never hand-write the header row again.
- Empty states: page-level empty uses the `@memohai/ui` Empty family (providers does);
  in-group empty stays the one-line muted `<p>` — deliberately NOT abstracted (these
  pages effectively never render empty; an owner slot would be dead API).

### Verification
- eslint --fix clean on all touched files; vue-tsc (`apps/web/tsconfig.json`) clean.
- UI contract guard: 9 violations = the pre-existing baseline (sidebar ×6,
  video/provider-setting ×2 incl. count line); zero new.

---

## EXECUTION LOG — Tier A/B passes (2026-07-04/05)

本轮把 ranking 里的候选逐个落地或裁决;这是本文件的最终状态记录。

### 已建 owner(与落点)
| Owner | 落点 | 备注 |
|---|---|---|
| **PanePlaceholder**(=EmptyStateNotice 合并) | `components/pane-placeholder/` | 四档 loading 梯子的"面级占位"档 |
| **InlineLoadingRow** | `components/inline-loading-row/` | 行内左对齐加载行;bots/* ×5 + dockview + chat-pane 模型 popover ×2 采用。bots/* 5 处 class 里保留 `min-h-[3.75rem]`(借 SettingsRow 行高防回流)并以 `ui-allow-shape` 注释豁免 rule-11 |
| **SidebarPanelHeader** | `components/sidebar/panel-header.vue` | files-pane / panel-schedule / panel-sessions;recents 的头是交互 DropdownMenu,留局部 |
| **SidebarNavButton** | `components/sidebar/nav-button.vue` | 3 处 byte-identical 收进 owner;px 常量随 owner 走,清掉了 sidebar/index.vue 的 guard px 超额 |
| **DockPanelFrame** | `pages/home/components/dockview/panel-frame.vue` | 7/7 面板迁移;根恒 relative,`editorSurface` prop 收编辑器底色,`#header` 放面包屑 |
| **ConfirmDeleteDialog** | `components/confirm-delete-dialog/` | recents + 两份 panel-schedule(后两者原本近逐字节复制);统一 sm:max-w-sm、DialogDescription、删除中禁用取消 |
| **StepExitShell / StepFrame / FooterNav / HintBox** | `pages/onboarding/components/` | 向导骨架族;HintBox 是 CalloutBanner ×7 被裁决 over-merge 后的正确落点(见上一节 Plan corrections) |

### 已修 / 已裁决
- **Button `:loading` 采用**:59 文件 spinner→prop 迁移(disabled 语义等价已验证);
  chat-pane 组合器按钮里的 spinner 换 icon 两处(:470/:547)属 C 档长尾,未动。
- **MarketItemCard hover bug**、**Badge font="mono"**、**SidebarGroupLabel compact**、
  **Button shape 轴**:前几轮已落。
- **ListRow / menu-row:裁决不建。** 三处 hover token(`--sidebar-hover`/`--bot-row-tint`/
  `--ui-hover`)、高度、radius 是各自 surface 的设计意图,不是意外漂移;理由已录
  memoh-ui-owners skill。
- **ChoiceTile / ToolDetailPanel 族**:未建。ChoiceTile 3 处零漂移(低风险);tool-call
  族等 chat UI revamp 方向定了再动(与在途 chat PR 地盘重叠)。
- **type-token guard 维度**(`text-[0.9rem]` 不落 `--text-*`):仍是待决策项,未加。

### Verification(本轮)
- vue-tsc 0;eslint --fix 过全部触碰文件。
- guard:rule-11 WARN 0;✗ 仅剩 `video/provider-setting.vue` 1 个预存 file-group。
