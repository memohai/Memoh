# Spacing Owner — Click-to-Open Surface Audit

Date: 2026-07-01
Method: 40 files that open a form/menu behind a click (Dialog / Popover / Sheet /
DropdownContent) were each read in full by an independent Sonnet subagent, which walked
every interactive surface and classified every row/field/tile/banner shape as **MISS**
(unmigrated owner-shape debt), **LOCAL** (deliberately hand-written), or
**DIFF-RELATIONSHIP** (looks similar, different spatial relationship — must not unify).

Raw evidence: `audit/batch-A-raw.json`, `audit/batch-B-raw.json`.

## Headline

**36 MISS across 40 audited files.** Every one hides behind a click — none was reachable
by the earlier "scan the rows" census, because these forms are wrapped in `<Dialog>` or
vee-validate `<FormField>`, not written as canonical `min-h-[3.75rem]` rows. This
confirms the working thesis: **the risk lives in the forms you have to click to reach.**

The audit also surfaced a real architecture fork (below) that changes how 17 of the 36
must be handled.

## RESOLUTION (all 36 handled)

Shipped in three commits after this audit:

- **Group 1 (19 MISS, no validation)** → `1db444c56`. 10 files migrated onto their owners;
  `bot-mcp` add-tile and `bot-channels` type-picker were re-confirmed at migration time as
  genuinely different geometries (horizontal `min-h-[4.5rem]` tile / flat menu row, not
  PersonaTile / SettingsRow) and left local — recorded, not forced.
- **FieldStack v2** → `445c55aa6`. FieldStack now provides `FORM_ITEM_INJECTION_KEY` and
  renders the field error inline when inside a `<FormField>`, so it owns validated fields
  too. Piloted on `add-provider` (5 fields). Standalone FieldStacks unchanged.
- **Group 2 (17 MISS, vee-validate)** → `445c55aa6` (add-provider) + `8d8c645d3`
  (create-model, add-platform). All `FormItem` fields swapped to FieldStack v2, validation
  preserved; previously-hidden schema messages now render inline and were translated to
  zh/en/ja.

Gate for each: ESLint 0, vue-tsc 0, UI contract 0 new violations (the 9 remaining are the
pre-existing `sidebar/index.vue` px + `video/provider-setting.vue` hover debt, outside this
work). A human render check (dark + narrow + zh, submit an empty required field to confirm
the inline error copy reads correctly) is the remaining non-automated step.

## The field-system fork (decided: FieldStack absorbs validation)

Vertical form fields have **two owners** in the codebase today:

- **`FieldStack`** (`settings/field-stack.vue`) — `space-y-1.5`, no validation. 9 files.
- **`FormItem`** (`@memohai/ui`, vee-validate) — `grid gap-2`, carries validation. 12 files.

They are the same Label-over-control shape with different gaps and no overlap (no file
uses both). **Decision: FieldStack becomes the single owner and grows validation
support; the vee-validate `FormField`/`FormItem` fields migrate onto it afterward.**

Consequence for this audit: the 17 MISS that are vee-validate fields **cannot migrate
until FieldStack can carry a validation/error state** — migrating now would drop the
form's validation behavior. They are split out as "blocked on FieldStack v2" below.

## MISS — group 1: migratable now (no validation involved) — 19

These use no vee-validate; the shape is a plain owner shape. Safe to migrate in a batch.

| File | # | Shapes → owner |
|---|---:|---|
| `pages/transcription/components/provider-setting.vue` | 4 | identity header, form footer, section header, models card → SettingsSection + SettingsRow + #footer/#actions |
| `pages/memory/components/add-memory-provider.vue` | 3 | name + provider fields + run wrapper → FieldStack ×2 + FormStack |
| `pages/speech/components/provider-setting.vue` | 2 | Save action bar + section header → SettingsSection (#footer / title) |
| `pages/video/provider-setting.vue` | 2 | dialog-body Label-over-Input ×2 + section footer → FieldStack + SettingsSection #footer |
| `pages/supermarket/components/install-plugin-dialog.vue` | 1 | "Select Bot" field → FieldStack |
| `pages/supermarket/components/install-skill-dialog.vue` | 1 | bot-select field → FieldStack |
| `pages/bots/components/bot-mcp.vue` | 1 | dashed add-tile beside BackendCard grid → PersonaTile(add) or an add-card |
| `pages/bots/components/bot-channels.vue` | 1 | type-picker row list in Add-Channel dialog → SettingsRow list |
| `pages/bots/components/bot-checks-panel.vue` | 1 | empty state → Empty |
| `pages/profile/components/profile-identity.vue` | 1 | avatar + name/username root row → SettingsRow (#leading) |
| `pages/bots/components/bot-schedule.vue` | 1 | page column + tab rhythm → PageShell(variant=tab) |
| `pages/bots/components/schedule-pattern-builder.vue` | 1 | Label + help over hour-grid → FieldStack |

## MISS — group 2: blocked on FieldStack v2 (vee-validate fields) — 17

High-frequency "add X" dialogs. Real Label-over-control debt, but each field is a
`<FormField>/<FormItem>` with live validation. Migrate only after FieldStack carries a
validation state.

| File | # | What |
|---|---:|---|
| `components/create-model/index.vue` | 7 | Type / Model / Display name / Dimensions / Compatibilities / (2 conditional) — the Add-Model dialog |
| `components/add-provider/index.vue` | 6 | Preset / Name / API key / Base URL / Client type — the Add-Provider dialog |
| `components/add-platform/index.vue` | 4 | the Add-Platform dialog fields |

## Confirmed NOT debt (the audit's other job)

The subagents did **not** blindly unify. High-value DIFF-RELATIONSHIP calls, with the
geometry evidence that makes them a genuinely different relationship — keep local:

- **`bot-import-panel.vue`** — every box (selected-file card, profile toggle card, error
  strips, passphrase unlock, footer) carries its own border/radius/bg and is not inside a
  SettingsSection card, so it has no `mx-4`/`border-b` hairline relationship. Denser
  dialog-body language; not owner shapes.
- **Single quick-input dialogs** (New File / New Folder / Rename across files-pane,
  recents, avatar-edit) — a bare `<Input>` with no `<Label>`, a deliberate lighter
  "name this one thing" convention shared across the codebase, not an unmigrated FieldStack.
- **`model-item.vue`** — `min-h-[3.25rem] py-2.5` is a *denser* compact list item than the
  `min-h-[3.75rem] py-3` settings row. Same skeleton, different relationship.
- **Sidebar toolbar bands** (files-pane header/batch-bar) — `h-7` full-bleed chrome above
  the tree, not a SettingsSection footer (which lives inside a card, py-3 px-4).
- **ContextMenu / DropdownMenu compositions** — correct `@memohai/ui` atom usage as-is.

## needsHumanEye (25 flags)

25 findings were flagged by the auditor as "resemblance is close, confirm on the rendered
page." They are recorded in the raw JSON per file; the load-bearing ones are the
DIFF-RELATIONSHIP cards in `bot-import-panel` (selected-file card, profile toggle card,
footer bar) — a human glance confirms whether they read as owner shapes or as the denser
dialog language the auditor judged them to be.

## Recommended sequencing

1. **Group 1 (19 MISS)** — a Phase-3-style subagent batch, same discipline as before
   (behavior-preserving, verify ESLint/tsc/contract, human render check). No blockers.
2. **FieldStack v2** — add a validation/error state so it can own the vee-validate fields.
   A shared-layer change; verify against `provider-form.vue` (the existing FormField user).
3. **Group 2 (17 MISS)** — migrate the add-X dialogs onto FieldStack v2, preserving every
   validation rule. This is where regressions are most likely; go carefully, one dialog at
   a time, not a wide batch.
