# Memoh Spacing Contract V1

Date: 2026-07-01

Status: authoritative migration contract.

This file is the source of truth for the first Memoh spacing migration. Older files in
`docs/design/spacing/` are evidence and working notes. Use this contract when deciding
what to implement, what to migrate, and what to leave alone.

## Decision

V1 is an owner-driven spacing system, not a large public semantic-token API.

The public implementation surface is:

```txt
primitive spacing scale
PageShell
SettingsSection
SettingsRow
BackendCard
Appearance complex-row recipe
```

The near-term candidate surface is:

```txt
ExpandableSettingsRow
FormStack
FieldStack
FormDialogShell
AddTile
FramedEmpty
SettingsContent or another settings content owner
DenseListSection / ObjectListRow
```

The deferred surface is:

```txt
ChatSurface
Composer
ToolProcess
StatusBanner
MetricReadout
Popover / Toast / Notification
Onboarding / sparse pages
```

Do not add a global semantic token just because a relationship has a name. Relationship
names are used to review and migrate the product. Values should usually be owned by the
component that enforces the relationship.

## Token Budget

### Public Primitive Scale

The primitive scale is allowed in component implementations and local exceptions. It is
not enough to define the design language by itself.

| Primitive | Value | V1 status |
|---|---:|---|
| `0.5` | 2px | keep |
| `1` | 4px | keep |
| `1.5` | 6px | keep |
| `2` | 8px | keep |
| `2.5` | 10px | keep |
| `3` | 12px | keep |
| `3.5` | 14px | keep |
| `4` | 16px | keep |
| `5` | 20px | keep |
| `6` | 24px | keep |
| `8` | 32px | keep |
| `10` | 40px | keep |
| `12` | 48px | keep |
| `16` | 64px | keep, rare |

### Public Semantic Spacing Tokens

V1 should have no public global semantic spacing tokens.

Do not introduce tokens like:

```txt
--spacing-title-to-body
--spacing-card-title-to-body
--spacing-form-field-gap
--spacing-settings-row-padding-y
```

These names are useful, but the page author should not choose them directly. They belong
inside owners such as `PageShell`, `SettingsSection`, `SettingsRow`, and future form or
dialog primitives.

### Owner-Internal Aliases

Owner-internal aliases are allowed only when they prevent drift inside a component
family. They may become CSS variables, constants, or documented recipes later, but page
code should not scatter them.

Allowed P0 aliases if implementation needs them:

```txt
--page-max-width
--page-gutter-x
--page-padding-top
--page-padding-bottom
--page-header-to-body
--page-title-inset-x
--page-action-gap
--settings-section-label-to-card
--settings-section-label-inset-x
--settings-row-inset-x
--settings-row-padding-y
--settings-row-column-gap
--settings-row-min-height
--settings-label-to-description
--backend-card-padding
--backend-card-media-gap
--backend-card-title-to-subtitle
```

Do not add P1/P2 aliases until the owner is implemented and visually verified.

## P0 Owners

P0 owners are allowed for broad migration now.

### PageShell

Use for normal settings/configuration pages and bot detail tabs.

Validated source: real Appearance page and component-wall PageShell sample match exactly
for the measured layout contract.

| Role | Current value | Rule |
|---|---:|---|
| `page.maxWidth` | `max-w-3xl` | Normal settings/config pages only. |
| `page.gutterX` | `px-6` | Standalone page variant. |
| `page.paddingTop` | `pt-10` | Standalone page variant. |
| `page.paddingBottom` | `pb-12` | Standalone page variant. |
| `page.headerToBody` | `mb-6` | Header block to body content. |
| `page.headerMinHeight` | `min-h-9` | Stable title/action baseline. |
| `page.titleInsetX` | `pl-2` | Align title with section/card rhythm. |
| `page.actionGap` | `gap-2` | Header command cluster. |
| `page.titleToDescription` | `mt-0.5` | Optional description below title. |
| `page.tabPaddingTop` | `pt-6` | Bot detail/tab variant. |
| `page.tabPaddingBottom` | `pb-8` | Bot detail/tab variant. |

Migration rule: replace hand-rolled page headers and page gutters with `PageShell`.
Do not migrate chat, onboarding, or theatrical empty/welcome pages into `PageShell`
without a separate owner decision.

### SettingsSection

Use for a section label plus framed settings card.

Validated source: real Appearance page and component-wall SettingsSection sample match
for the measured section spacing.

| Role | Current value | Rule |
|---|---:|---|
| `section.labelToSurface` | `space-y-2.5` | Label/action row to card. |
| `section.labelInsetX` | `px-2` | Section label inset. |
| `section.headerMinHeight` | `min-h-7` | Stable section header/action row. |
| `section.headerGap` | `gap-4` | Label group to actions. |
| `section.bodyGap.page` | `space-y-8` | Page-level section stack in Appearance. |
| `section.bodyGap.detail` | `space-y-6` | Denser detail contexts, verify per page. |

Migration rule: if a page repeats "section title + card chrome", use
`SettingsSection`. Do not duplicate rounded/border/card wrappers locally.

### SettingsRow

Use for label/control rows, switch rows, and summary/action rows inside
`SettingsSection`.

Validated source: real Appearance page and component-wall SettingsRow sample match for
row inset, padding, minimum height, and label/description rhythm.

| Role | Current value | Rule |
|---|---:|---|
| `settings.rowInsetX` | `mx-4` | Row content and divider inset. |
| `settings.rowMinHeight` | `min-h-[3.75rem]` | Default settings row height. |
| `settings.rowPaddingY` | `py-3` | Default settings row vertical rhythm. |
| `settings.rowColumnGap` | `gap-4` | Label/content or summary/action gap. |
| `settings.labelToDescription` | `mt-0.5` | Label to secondary text. |

Migration rule: replace local settings rows with `SettingsRow` before touching values.
Do not use `SettingsRow` for dense object lists, chat items, model rows, or large
free-form form blocks.

### BackendCard

Use for provider/search/MCP/backend entry cards and similar object-entry cards.

Validated source: existing `BackendCard` implementation and multiple configuration
slices. It still needs one direct measurement pass against Provider/Web Search before
mass migration.

| Role | Current value | Rule |
|---|---:|---|
| `backendCard.padding` | `p-3.5` | Card internal padding. |
| `backendCard.mediaGap` | `gap-3` | Leading media to text/trailing content. |
| `backendCard.titleToSubtitle` | `mt-0.5` | Title to supporting line. |
| `backendCard.trailingGap` | owner-local | Action/chevron area, keep component-owned. |

Migration rule: use `BackendCard` for object-entry cards. Do not use it for add tiles
or full empty states.

### Appearance Complex Row Recipe

Appearance has rows that are not simple `SettingsRow`: they contain a label/description
group and a nested grid of controls.

Keep this as a documented recipe for now:

```txt
row inset/divider: mx-4 border-b border-border py-3 last:border-b-0
label to description: mt-0.5
description to nested controls: mt-3
nested controls grid: grid gap-3 sm:grid-cols-2
```

Migration rule: do not extract `SettingsContent` from this alone. This recipe proves a
real pattern, but a reusable content owner must be validated against Provider, MCP, and
Schedule forms first.

## P1 Candidates

P1 candidates are real, but should not be mass-migrated until each owner is implemented
and verified against at least one real page.

| Candidate | Why it exists | First validation target | Migration status |
|---|---|---|---|
| `ExpandableSettingsRow` | Tool Approval rows open into inline editors. | Tool Approval. | design/implement next |
| `FormStack` | Dialog and settings forms need owned field rhythm. | Schedule New Task, Provider dialogs. | design/implement next |
| `FieldStack` | Label/meta/control spacing repeats with value drift. | Schedule New Task, Provider config. | design/implement next |
| `FormDialogShell` | Dialog padding/header/body/footer needs one shell. | Schedule New Task. | reconcile before migration |
| `AddTile` | Dashed add-card beside real entries is recurring. | MCP tools, provider add/import. | design/implement next |
| `FramedEmpty` | Empty states inside cards/frames need owned padding/action gap. | MCP empty, schedule empty. | design/implement next |
| `SettingsContent` or equivalent | Free-form card content is not row-based. | Provider config + MCP connection. | validate before extraction |
| `DenseListSection` / `ObjectListRow` | Provider model rows are denser than settings rows. | Provider models list. | validate before extraction |
| Settings footer owner | Provider/MCP footer actions repeat but ownership differs. | Provider config + MCP connection. | decide owner first |

P1 rule: adopt the relationship, not the current value. Values can be tuned after the
owner exists.

## Deferred Families

These are real spacing families, but they should not block the configuration-surface
migration.

| Family | Status | Rule |
|---|---|---|
| `chat.*` | defer | Separate chat-surface pass. Do not fit chat into settings rhythm. |
| `composer.*` | defer | Composer owns tray/input/button rhythm later. |
| `tool.*` | defer | Tool process/detail logs need their own dense owner. |
| `banner.*` | defer | Needs one more state-heavy page before extraction. |
| `metric.*` | defer | Needs telemetry/readout evidence. |
| `popover.*` / toast / notification | defer | Feedback/overlay pass, not spacing V1. |
| onboarding / sparse pages | boundary | Use as exception audit, not source for settings tokens. |

## Migration Rules

1. Replace hand-written structures with owner components first.
2. Do not add public semantic spacing tokens during migration.
3. Do not merge relationships because they share a pixel value.
4. Do not split relationships only because current code uses slightly different values.
5. If a page matches a P0 owner, migrate it to that owner.
6. If a page matches a P1 candidate, record the owner need and migrate only after the
   owner is implemented.
7. If a page is chat, onboarding, sparse marketing-like flow, toast, notification, or a
   one-off picker, classify it as deferred or exception unless the owner already exists.
8. Every migrated page needs a visual or measurement check against the target owner.

## Same-Frequency / Different-Code

When two places feel like the same relationship but use different classes, keep one
relationship and let the owner choose the value.

Examples:

```txt
space-y-1.5 vs mb-2
=> FieldStack.labelToControl

space-y-5 vs space-y-3 inside settings cards
=> future settings content owner with density variant, not two public tokens

mt-3 vs gap-3 before nested controls
=> local recipe or owner variant, not a global "12px means nested controls" token
```

## Same-Code / Different-Relationship

When two places use the same primitive but mean different things, keep them separate.

Examples:

```txt
gap-3 in BackendCard media rhythm
gap-3 in nested Appearance controls
gap-3 in sidebar identity media

p-4 in dialog body
p-4 in provider toolbar
p-4 in settings content
```

These can share the primitive scale. They should not share a semantic token.

## Subagent Migration Packet

Every spacing migration subagent must read this file first. Older spacing docs are
evidence only.

For each assigned page or component, return:

```txt
1. Current local spacing decisions.
2. P0 owner replacements.
3. P1 owner candidates that should wait.
4. Deferred/exception areas.
5. Files changed.
6. Visual or measurement verification performed.
```

Allowed edits:

```txt
replace hand-rolled PageShell / SettingsSection / SettingsRow / BackendCard patterns
use the Appearance complex-row recipe when the shape matches exactly
remove duplicated local card/header/row wrappers after owner replacement
```

Disallowed edits:

```txt
invent a new public semantic spacing token
invent a new spacing owner without documenting it here
force chat/onboarding/toast/popover into configuration spacing
change unrelated color, typography, radius, shadow, or copy while migrating spacing
```

## Next Execution Order

1. Verify `BackendCard` against real Provider/Web Search surfaces.
2. Implement and verify `FormStack`, `FieldStack`, and `FormDialogShell` against Schedule
   New Task.
3. Implement and verify `AddTile` and `FramedEmpty` against MCP/provider add or empty
   states.
4. Decide `SettingsContent` only after Provider + MCP + Schedule evidence agrees.
5. Send subagents through settings/configuration pages with this V1 packet.
6. After migration, run screenshot/measurement checks and tune owner values if the whole
   product feels too loose or too dense.

## Verification Already Done

`PageShell`, `SettingsSection`, and `SettingsRow` have been measured against the real
Appearance page. The measured component-wall sample matches the real page for the P0
relationships listed above.

See `docs/design/spacing/appearance-pageshell-verification.md`.
