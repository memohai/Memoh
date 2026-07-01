# Spacing Research Status

Date: 2026-07-01

Status: evidence note. The authoritative migration contract is
`docs/design/spacing/spacing-contract-v1.md`.

## Current Completion

Approximate research completion: **85% after the MCP + Tool Approval slice**.

This is not measured by file count. It is measured by semantic saturation: whether new slices keep producing new relationship families, or mostly fall into existing families with clearer owners.

## Completed Evidence

| Slice | Document | What It Proved | Completion Impact |
|---|---|---|---|
| Bot Overview | `docs/design/spacing/bot-overview-cartography.md` | `PageShell`, `SettingsSection`, `SettingsRow`, banner, metric/readout, settings content block | first stable settings-page evidence |
| Schedule dialog + bot sidebar | `docs/design/spacing/schedule-dialog-sidebar-cartography.md` | `form.*`, `dialog.*`, `sidebar.*`, empty-state debt, page action rhythm | form and sidebar became first-class families |
| Provider workflow | `docs/design/spacing/provider-cartography.md` | `BackendCard`, `DetailPane`, `SettingsShell`, settings-row forms, dense list rows, toolbar, FormDialogShell reconciliation | moved from role ideas to owner candidates |
| Chat message stream + tool detail | `docs/design/spacing/chat-tool-cartography.md` | `chat.*`, `composer.*`, `tool.*`, markdown rhythm, process/detail wrappers | prevents overfitting the contract to settings/configuration surfaces |
| MCP + Tool Approval | `docs/design/spacing/mcp-tool-approval-cartography.md` | `page.tab*`, `BackendCard`, `gallery.*`, `settings.contentStack`, expandable settings rows, settings footer actions | confirms the first-contract shape is stable enough to draft |
| Token granularity + compression | `docs/design/spacing/token-granularity-review.md`, `docs/design/spacing/spacing-contract-compression.md` | most relationship names should become component responsibilities, not public tokens | prevents token soup before implementation |
| P0 component wall | `apps/web/src/pages/dev/components/sections/SectionSpacing.vue` | primitive scale + `PageShell`, `SettingsSection`, `SettingsRow`, Appearance complex-row evidence, `BackendCard` now have a live reference surface | turns the first contract shape into inspectable UI instead of prose only |
| Expansion plan | `docs/design/spacing/next-slice-expansion-plan.md` | prioritized the remaining 5-10 slices and marked onboarding as boundary sample | prevents infinite collection |

## What Is Ready To Draft

These can already be drafted in the first contract with relationship confidence high and value confidence medium or better:

- `page.*` roles owned by `PageShell`.
- `section.*` roles owned by `SettingsSection` or section-like primitives.
- `settings.row*` roles owned by `SettingsRow`.
- `settings.expandedRow*` roles owned by an expandable settings row primitive.
- `dialog.*` container roles owned by `DialogContent`, `DialogScrollContent`, `DialogHeader`, `DialogFooter`.
- `form.*` roles owned by future `FormStack`, `FieldStack`, `FormDialogShell`.
- `sidebar.*` roles owned by `MasterDetailSidebarLayout`, `SettingsSidebar`, `NavItem`, and sidebar header primitives.
- `empty.*` frame/action roles owned by `FramedEmpty` / `Empty` wrappers.
- `toolbar.*` command grouping roles.
- `chat.*` rail/message rhythm roles owned by `ChatSurface` and `MessageItem`.
- `composer.*` input-tray roles owned by `Composer`.
- `tool.*` process row/detail roles owned by `ToolProcessRow`, `ToolProcessGroup`, and `ToolProcessDetail`.

These are close, but no longer block a v0 contract:

- `detail.*`: Providers and MCP agree, but implementation should decide whether `DetailPane` and `SettingsShell` merge or stay paired.
- `settings.content*`: Appearance proves complex row structure, but not a generic content primitive. Validate against Provider/MCP before promotion.
- `list.row*` dense roles: Provider model rows are enough for v0 relationship naming, but need another list before value tuning.
- `settings.footer*`: Provider and MCP agree on action rhythm, but footer ownership differs.
- `banner.*` and `metric.*`: need one more state/metric-heavy page.
- `popover.*`: ConfirmPopover has a clear local rhythm, but can wait for a popover/menu pass.

These should remain deferred or exception-bound for now:

- `flow.*` / `onboarding.*`
- `sparse.*`
- chat welcome-state theatrical spacing, such as upper-middle viewport offsets
- one-off picker geometry
- numeric input widths, select widths, textarea heights, avatar/icon sizes
- micro gaps inside status dots, badges, and metadata clusters unless a component owns them

## Stop Condition

Stop first-pass research after these remaining slices:

1. Bots grid / global sidebar follow-up.
2. Onboarding boundary audit.

After those, stop if at least 80% of new relationships map to existing families:

- `page`
- `section`
- `settings`
- `form`
- `dialog`
- `sidebar`
- `detail`
- `toolbar`
- `gallery/list`
- `empty`
- `chat/tool`
- `flow/sparse exception`

If a new slice only changes values but not relationship families or owners, it is not a reason to keep collecting. It becomes migration evidence.

## First Contract Shape

The first spacing contract should be a small owner-driven system:

```txt
PageShell
  page.maxWidth
  page.gutterX
  page.paddingTop/page.paddingBottom
  page.headerToBody
  page.titleInsetX
  page.actionGap

SettingsSection / SettingsRow
  section.labelToSurface
  section.stackGap
  section.actionGap
  settings.rowInsetX
  settings.rowMinHeight
  settings.rowPaddingY
  settings.rowColumnGap
  settings.labelToDescription
  settings.contentPadding
  settings.contentStackGap
  settings.expandedRowGap

FormStack / FieldStack / FormDialogShell
  dialog.padding
  dialog.contentGap
  dialog.formMaxWidth
  form.fieldGap
  form.labelToControl
  form.labelMetaGap
  form.footerTopGap
  form.footerActionGap

Sidebar primitives
  sidebar.gutterX
  sidebar.headerTopPadding
  sidebar.identityToSearchGap
  sidebar.navItemIconGap
  sidebar.navItemGap
  sidebar.navGroupGap

List/Gallery primitives
  gallery.cardGap
  backendCard.padding
  backendCard.mediaGap
  addTile.minHeight
  list.toolbarPadding
  list.rowPaddingY.dense
  list.rowActionGap

Empty
  empty.outerFrame
  empty.outerPaddingY
  empty.inCardPaddingY
  empty.actionGap

ChatSurface / Composer / ToolProcess
  chat.threadMaxWidth
  chat.threadGutterX
  chat.turnGap
  chat.messageAvatarGap
  chat.userBubblePadding
  composer.padding
  composer.stackGap
  composer.controlGap
  tool.rowGap
  tool.groupBodyGap
  tool.detailPadding
  tool.detailStackGap
```

Values can be provisional in version 1. The important part is that every adopted relationship has an owner.

## Suggested Next Action

Do **spacing contract v0** next. MCP + Tool Approval already supplied the confidence needed to stop broad collection.

Recommended path:

1. Review the P0 spacing wall in `/dev/components#spacing`.
2. Use Bots grid / global sidebar follow-up as validation, not a blocker.
3. Use onboarding only as a boundary audit before migration.
4. Promote P1 owners only when a page needs them: `ExpandableSettingsRow`, `FormStack`, `FieldStack`, `AddTile`, `FramedEmpty`.
5. Start migration by replacing hand-rolled structures with owners, not by adding public semantic tokens.
