# Spacing Owner Vocabulary — Census Result

Date: 2026-07-01

Status: **authoritative gap map**. Supersedes the speculative P1 lists in
`spacing-contract-v1.md`. Built from a 125-sample morphology census that read the
FULL template structure of 35 settings/config files (not grep). This file answers the
one question the earlier docs never closed: **is the owner vocabulary complete enough
to stop defining and start batch-applying?**

Answer: **~60% complete.** Four load-bearing owners already ship the exact slots
needed. Six new owners must be built before mass migration. Everything else stays local.

## The Phase Model (why this doc exists)

```
Phase 1  Define + build the FULL owner vocabulary   ← WE ARE HERE (6 owners left)
Phase 2  Pilot-verify each owner on ONE real page   ← done for SettingsRow (bot-overview, profile)
Phase 3  Mass-apply via subagent batch (V1 packet)  ← NOT a page-by-page main-thread edit
```

The mistake that produced this doc: after piloting `SettingsRow` on two pages, work
drifted into "migrate the next page, and the next" — applying Phase-3 tactics (page by
page) while still in Phase 1 (vocabulary incomplete). You cannot mass-migrate onto a
half-defined vocabulary: the undefined shapes get hand-rolled again *during* migration.
Finish the dictionary, then replace the book.

## Existing owners — READY for mass migration now

These four already ship the exact API the census assumed was pending. ~54 occurrences
are migratable today.

| Morphology | Owner | Occ | Files | Note |
|---|---|---:|---:|---|
| horizontal 3-seg row | `SettingsRow` | 24 | 15 | `#leading` / `#content` / default trailing all shipped |
| dense object-list row | `SettingsRow` | 11 | 8 | needs one optional `align?: 'center'\|'start'` prop (2-3 items-start cases) |
| section card wrapper | `SettingsSection` | 8 | 7 | `chart-card.vue` is a BYTE-COPY → delete + repoint |
| empty state | `Empty` (existing) | 11 | 8 | only fold `py-12/16` into an Empty padding default |
| page column | `PageShell` | 2 | — | `variant:'tab'\|'page'` already ships the exact column |

## Owner gaps — BUILD these six before mass migration (ranked by leverage)

Each: build once in the shared layer, verify against the first-validation-target page,
then it unblocks its bucket.

### 1. FieldStack (+ FormStack wrapper) — unblocks 13
- **First validation:** `bots/components/settings-acp-detail.vue` (ACP managed-field v-for)
- **API:** `FieldStack { label; for?; help? }`, default slot = the control. Renders
  `<div class="space-y-1.5"><Label :for/> + <slot/> + optional help <p class="text-xs text-muted-foreground"/></div>`.
  `FormStack` = `<div class="space-y-4">` wrapper for a run of FieldStacks.
- **Distinct from SettingsRow** (horizontal label|control). Do NOT force vertical
  Label-over-control clusters into SettingsRow. Highest-leverage gap.

### 2. MetricReadout — unblocks 9
- **First validation:** `bots/components/settings-context-card.vue` (repeats tile ~8x, incl. status-dot variants)
- **API:** `{ label; value?; sub?; framed?=true; status?: 'ok'|'warn'|'error' }`. Caller
  owns the grid. Framed tile = caption + tabular value (or status-dot+label) + optional sub,
  `min-h-[4.375rem]`. `framed:false` for bot-overview usage stats.

### 3. SettingsFooterBar — unblocks 7 (deliver as a SettingsSection `#footer` slot, NOT a standalone component)
- **First validation:** `email/components/provider-setting.vue` (LoadingButton submit)
- **API:** add `#footer` slot to `SettingsSection.vue`, rendered INSIDE the bordered card
  after the default slot: `<div v-if="$slots.footer" class="flex items-center justify-end border-t border-border py-3 px-4"><slot name="footer"/></div>`.
  `justify-between` when content spans (pagination). Trivial build, 6-file reuse, zero new component.

### 4. ExpandableSettingsRow — unblocks 6
- **First validation:** `bots/components/bot-heartbeat.vue` L174-248 (log row: whole header toggles, expands to pre/error/usage panels — richest case)
- **API:** `{ label?; description? }` + `v-model:open` + slots `#leading` / `#content` /
  `#trailing` (default: chevron that rotates 90° on open) / `#expanded` (collapsible body).
  Internally composes `SettingsRow` for the header + a height/grid-rows transition.
  Simpler "Advanced" disclosures (network/channel/context-card) fall out for free.

### 5. PersonaTile (+ AddTile variant) — unblocks 5
- **First validation:** `bots/components/bot-card.vue` L2-49, then `bots/index.vue` create-tile + skeleton
- **API:** `{ name; variant?: 'entity'|'add' }`, slots `#media` (Avatar or plus-circle),
  `#status` (absolute corner badge). Vertical centered `w-52 flex-col items-center rounded border bg-card p-5`.
  `add` variant swaps `bg-card`→`bg-background`. **BackendCard does NOT cover this** — BackendCard is horizontal; this is vertical/centered.

### 6. CalloutBanner — unblocks 4
- **First validation:** `bots/components/bot-container.vue` L1035-1128 (3 stacked warning callouts share one skeleton)
- **API:** `{ tone: 'warning'|'destructive'; title; description?; clickable? }`, slots
  `#icon` (default AlertCircle) + default trailing action slot. Rounded `bg-{tone}-soft`
  framed row, `sm:flex-row` responsive. `clickable` → whole surface is a `<button>` with trailing ChevronRight (bot-overview BannerButton).

## Do NOT build these (rejected pseudo-gaps — keep the vocabulary narrow)

| Tempting owner | Why not | What covers it instead |
|---|---|---|
| `DenseListSection` | non-gap | `SettingsSection` `#actions` slot already carries the header toolbar |
| `FramedEmpty` | non-gap | extend existing `Empty` with `framed?` + action slot |
| `SchemaFieldRow` / `StatusActionRow` | non-gap | a responsive `stack?: 'always'\|'sm'` variant on `SettingsRow` |
| `DescriptionList` / `DefinitionRow` | defer (n=2) | keep local until a 3rd read-only key/value site appears |
| `DataTable` | n=1 | one genuine table (compaction logs) — keep local |

## Stays-local (deliberately hand-rolled, ~34 occ)

- centered placeholders (spinner/skeleton borrowing row min-height so panels don't reflow) — 22 occ
- genuinely one-off compound blocks (OAuth device-flow, link-code countdown, Monaco JSON dialog, snapshot input) — the non-footer part of 12
- trivial muted no-results `<p>` lines
- the single real data table

## Execution Order

1. **Build the six owners**, each verified against its first-validation-target page. Two
   quick wins first (SettingsSection `#footer` slot, SettingsRow `align`/`stack` props are
   prop-adds, not new components), then the four real new components.
2. Only after all six exist + are verified: assemble the **Phase-3 subagent batch** — a
   migration packet per page, applied in parallel by subagents, each returning the
   V1-packet report. NOT a main-thread page-by-page edit.
3. `mise run lint` + visual/measurement check per migrated page.

## Full bucket data

Raw census (125 samples, 6 readers + synthesizer) archived in the task output; the bucket
verdicts above are the durable result. Total: 4 existing + 6 new = the whole vocabulary;
everything else stays local.
