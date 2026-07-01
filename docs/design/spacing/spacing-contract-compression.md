# Spacing Contract Compression

Date: 2026-07-01

Status: superseded by `docs/design/spacing/spacing-contract-v1.md`. Keep this file as
evidence only.

This was the implementation-facing compression of `spacing-contract-v0.md`.

Purpose: prevent the relationship contract from becoming a giant token API. This document classifies each spacing family into:

- `public primitive`: page authors should use a component or layout primitive.
- `internal alias`: a component may own a CSS variable, constant, recipe, or variant.
- `doc-only relationship`: useful for review and migration language, not an implementation token.
- `exception`: intentional local composition.
- `defer`: keep in the ledger, revisit after more evidence.

## Executive Decision

Do not create a public token for every role in `spacing-contract-v0.md`.

For v0, the public API should be mostly components:

```txt
PageShell
SettingsSection
SettingsRow
ExpandableSettingsRow
FormStack
FieldStack
FormDialogShell
BackendCard
AddTile
FramedEmpty
```

The following can wait:

```txt
DenseListSection
ObjectListRow
ChatSurface
Composer
ToolProcess
```

Public semantic spacing tokens should be rare. Most values should be internal to the component that owns the relationship.

## Compression Matrix

| Family / role group | Classification | Public surface | Internal aliases worth allowing | Doc-only / exception notes | Implementation priority |
|---|---|---|---|---|---|
| Primitive scale | public primitive scale | `space.*` values or Tailwind scale convention | none | Values are cheap; meaning comes from owners. | P0 |
| `page.*` | public primitive + internal aliases | `PageShell` variants | `--page-gutter-x`, `--page-header-to-body`, maybe `--page-max-width` | `page.tabTopRhythm` is a paired contract, not a token. | P0 |
| `section.*` | public primitive + internal aliases | `SettingsSection` | `--section-label-to-surface`, `--section-label-inset-x` | `section.actionGap` mostly belongs to header layout/action slot. | P0 |
| `settings.row*` | public primitive + internal aliases | `SettingsRow` | `--settings-row-inset-x`, `--settings-row-padding-y`, `--settings-row-column-gap`, `--settings-row-min-height` | Relationship names stay in docs; page authors should not hand-pick them. | P0 |
| `settings.content*` | candidate primitive / validate first | later content owner, maybe `SettingsContent` | later `--settings-content-padding`, `--settings-content-stack-gap`, `--settings-content-field-gap` | Appearance proves complex rows, not a generic content primitive. Validate against Provider/MCP before promotion. | P1 |
| `settings.expandedRow*` | public primitive + internal aliases | `ExpandableSettingsRow` | `--settings-expanded-row-padding-y`, `--settings-expanded-row-gap` | Tool Approval is the first target. | P1 |
| `settings.footer*` | internal alias / defer owner | `SettingsSection` footer slot or detail footer | `--settings-footer-action-gap`, maybe footer padding | Footer location differs between Provider and MCP; do not expose globally yet. | P1 |
| `form.*` | public primitive + internal aliases | `FormStack`, `FieldStack` | `--form-field-gap`, `--form-label-to-control`, `--form-footer-action-gap` | Values need reconciliation between New Task and old `FormDialogShell`. | P1 |
| `dialog.*` | public primitive + internal aliases | `FormDialogShell`, dialog subtypes | `--dialog-padding`, `--dialog-header-to-body`, `--dialog-footer-gap` | Editor/import dialog may become its own subtype. | P1 |
| `sidebar.*` | internal aliases through sidebar primitives | existing sidebar primitives | maybe `--sidebar-nav-group-gap`, `--sidebar-identity-padding` | Mac traffic-light offset and desktop shell geometry remain local. | P2 |
| `gallery.*` | public primitive + internal aliases | `BackendCard`, `AddTile`, maybe gallery wrapper | `--gallery-card-gap`, `--add-tile-min-height`, `--add-tile-content-gap` | Keep dashed rule in docs: dashed only for add tile beside real items. | P1 |
| `backendCard.*` | public primitive + internal aliases | `BackendCard` | `--backend-card-padding`, `--backend-card-media-gap`, `--backend-card-title-to-subtitle` | Existing component already owns most of this. | P0 |
| `list.row*` dense | defer / future public primitive | later `DenseListSection`, `ObjectListRow` | later row padding/action aliases | Provider model rows are enough for naming, not enough for value hardening. | P2 |
| `empty.*` | public primitive + internal aliases | `FramedEmpty`, `Empty` usage wrapper | `--empty-outer-padding-y`, `--empty-in-card-padding-y` | Rules matter more than tokens. | P1 |
| `banner.*` | defer | later `StatusBanner` | none yet | Needs another state-heavy page. | P3 |
| `metric.*` | defer | later `MetricReadout` | none yet | Needs another telemetry page. | P3 |
| `chat.*` | defer public primitive; internal later | later `ChatSurface`, `MessageItem` | later `--chat-turn-gap`, `--chat-thread-gutter-x` | Keep separate from PageShell; do not migrate in first config pass. | P3 |
| `composer.*` | defer public primitive; internal later | later `Composer` | later `--composer-padding-y`, `--composer-control-gap` | Welcome state is exception, not token. | P3 |
| `tool.*` | defer public primitive; internal later | later `ToolProcess*` | later `--tool-detail-padding`, `--tool-row-gap` | Dense process-log owner, not card/list/settings. | P3 |
| `popover.*` | defer | later popover/menu contract | none yet | Toast/notification/popover should be a feedback-surface pass, not spacing v0. | P3 |
| `onboarding.*`, `flow.*`, `sparse.*` | exception / boundary | none in v0 | none | Use for boundary audit only. | P3 |
| sizes and geometry | exception / component-local | component props/variants | component-local constants | Widths, heights, avatar/icon sizes, textarea heights, output max heights are not spacing roles. | ongoing |

## P0 Implementation Scope

P0 should be small and confidence-heavy:

```txt
PageShell
SettingsSection
SettingsRow
BackendCard
primitive scale docs
```

Why these first:

- They already exist or have obvious homes.
- They cover the proven Appearance settings-page rhythm.
- They reduce hand-written rows, card chrome, and page headers.
- They are low-risk compared with dialog/chat/tool migrations.

## P1 Implementation Scope

P1 should add missing but clearly needed primitives:

```txt
ExpandableSettingsRow
SettingsContent or another settings content owner
FormStack
FieldStack
FormDialogShell reconciliation
AddTile
FramedEmpty
settings/detail footer owner
```

Why second:

- They require API/design decisions.
- They touch forms, dialogs, and empty states where visual values need review.
- They unlock Tool Approval, New Task, Provider add/import/model dialogs, and MCP add tile cleanup.

## P2 Implementation Scope

P2 should validate and extract broader patterns:

```txt
sidebar primitive aliases
DenseListSection
ObjectListRow
DetailPane / SettingsShell pairing
```

Why later:

- Values are plausible but not as hardened.
- More interface slices can validate without blocking v0.

## P3 / Boundary Scope

P3 should not block the first spacing contract:

```txt
ChatSurface
Composer
ToolProcess
StatusBanner
MetricReadout
Popover / Toast / Notification
Onboarding / sparse pages
```

Why later:

- These are real families but not part of the first configuration-surface cleanup.
- Chat/tool should stay separate and should be migrated deliberately.
- Toast/notification/popover belongs to a feedback/overlay pass.
- Onboarding is a boundary sample; it should teach exception handling.

## Public Token Budget

For v0, keep public semantic spacing tokens at or near zero.

Acceptable public-ish tokens only if implementation demands them:

```txt
--page-max-width
--page-tab-max-width
--chat-thread-max-width
--settings-row-min-height
```

Even these can remain component-owned unless another package, Figma export, or cross-component contract needs them.

## Internal Alias Budget

Internal aliases are allowed when they prevent drift inside a component family.

Good candidates:

```txt
--page-gutter-x
--page-header-to-body
--section-label-to-surface
--settings-row-inset-x
--settings-row-padding-y
--settings-row-column-gap
--settings-content-padding
--settings-content-stack-gap
--form-field-gap
--form-label-to-control
--backend-card-padding
--backend-card-media-gap
--empty-outer-padding-y
```

Guardrail: internal aliases should be used by owners, not scattered through page code.

## Same-Frequency / Different-Code Rule

When two values feel like the same rhythm but differ in code, do not create two tokens immediately.

Prefer:

```txt
same relationship + owner variant + value tuning later
```

Examples:

- `space-y-1.5` vs `mb-2` for form label/control -> one `FieldStack` relationship.
- `space-y-5` vs `space-y-3` inside settings content -> one future content-owner family with density variants, after Provider/MCP validation.
- `mt-1.5` vs `mt-1` in tool details -> one `ToolProcessDetail` relationship with root/group variants.

## Same-Code / Different-Frequency Rule

When the same value appears in different relationships, do not merge them.

Examples:

- `gap-3` in backend card, sidebar identity, entity header, list row.
- `gap-2` in toolbar, composer attachments, form inline controls, popover footer.
- `p-4` in settings content, dialog editor body, generic card content.

These may alias the same primitive. They are not the same semantic token.

## Component Wall Implication

The component wall should not present a giant semantic-token table.

It should present:

1. primitive scale ruler;
2. P0 component primitives and the relationships they own;
3. P1 proposed primitives;
4. "same-frequency / different-code" examples;
5. exceptions gallery.

Recommended first component-wall panels:

```txt
Primitive Scale
PageShell
SettingsSection + SettingsRow
Appearance complex row
BackendCard
AddTile + FramedEmpty
FormStack + FieldStack
```

Chat/tool can be a later panel after configuration primitives are stable.

## Migration Guardrail

When migrating a page:

1. Replace hand-rolled structure with a primitive first.
2. Only add an internal alias if the primitive needs one.
3. Do not add a public token to make a one-off migration easier.
4. If a value differs but the rhythm is the same, document it as same-frequency/different-code and tune later.
5. If a value matches but the relationship differs, keep it owner-specific.

The aim is fewer decisions in page code, not more named things to choose from.
