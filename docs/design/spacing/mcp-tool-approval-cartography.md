# MCP And Tool Approval Spacing Cartography

Date: 2026-07-01

Status: fifth pilot slice. This is the final broad configuration slice before drafting the first spacing contract.

## Slice

- Archetype: bot-detail capability configuration.
- Primary files read:
  - `apps/web/src/pages/bots/detail.vue`
  - `apps/web/src/pages/bots/components/bot-mcp.vue`
  - `apps/web/src/pages/bots/components/mcp-server-detail.vue`
  - `apps/web/src/pages/bots/components/bot-tool-approval.vue`
  - `apps/web/src/components/page-shell/index.vue`
  - `apps/web/src/components/settings/section.vue`
  - `apps/web/src/components/settings/row.vue`
  - `apps/web/src/components/settings/backend-card.vue`
  - `apps/web/src/components/key-value-editor/index.vue`
  - `apps/web/src/components/confirm-popover/index.vue`
- Current owner primitives:
  - `MasterDetailSidebarLayout`
  - bot detail tab container
  - `PageShell`
  - `SettingsSection`
  - `SettingsRow`
  - `BackendCard`
  - `KeyValueEditor`
  - `ConfirmPopover`

## Summary

MCP and Tool Approval mostly confirm the architecture already emerging from Provider, Schedule, and Sidebar. They do not introduce a fundamentally new spacing family. Instead, they strengthen several owner candidates:

- `PageShell variant="tab"` is the right owner for bot-detail tab content.
- `BackendCard` plus `gallery.cardGap` is confirmed by MCP list.
- `SettingsSection` and `SettingsRow` are confirmed by Tool Approval and MCP advanced settings.
- `settings.contentStack.*` is needed for free-form content inside a settings card when a strict `SettingsRow` label/control split is too rigid.
- Detail/edit workflows inside a tab should share `PageShell`/`DetailPane` rhythm instead of hand-rolling `max-w-3xl pt-6 pb-8`.

This slice moves the research from "still collecting" to "ready to draft v0." The remaining Bots grid/sidebar and onboarding checks should validate boundaries, not block the first contract.

## Slice: Bot Detail Container

- Archetype: master-detail bot settings shell with persistent sidebar and tab detail pane.
- File read: `apps/web/src/pages/bots/detail.vue`.
- Current owners: `MasterDetailSidebarLayout`, bot detail view, `PageShell variant="tab"`.

### Relationships

| Relationship | Current implementation | Intent | Candidate role | Owner | Relationship confidence | Value confidence | Decision | Notes |
|---|---|---|---|---|---|---|---|---|
| detail pane outer padding | `px-6 pt-4 pb-4` around the active tab | inset every bot tab from the scroll pane edge | `detail.tabViewportPaddingX`, `detail.tabViewportPaddingY` | bot detail pane | high | medium | adopt relationship | This is distinct from standalone page padding. |
| tab page shell | `PageShell variant="tab"` resolves to `mx-auto max-w-3xl pt-6 pb-8` | normalize tab title/body rhythm inside an already padded pane | `page.tabMaxWidth`, `page.tabPaddingTop`, `page.tabPaddingBottom` | `PageShell` | high | medium-high | adopt | This is already encoded in PageShell. |
| tab body total rhythm | container `pt-4` + `PageShell tab pt-6` reaches the same visual top as page rhythm | align tabs without each tab owning outer gutters | `page.tabTopRhythm` | bot detail + PageShell pair | high | medium | document | Pairing should be explicit so tabs do not drift. |
| sidebar group stack | nav groups use `mt-4`, menus use `gap-1`, header card/search use `mt-3` | persistent bot navigation rhythm | `sidebar.navGroupGap`, `sidebar.navItemGap`, `sidebar.identityToSearchGap` | bot sidebar/header | high | medium | already adopted | Confirms sidebar roles from the Schedule/sidebar slice. |

## Slice: MCP List

- Archetype: backend/server gallery inside a bot tab.
- File read: `apps/web/src/pages/bots/components/bot-mcp.vue`.
- Current owners: `PageShell`, `BackendCard`, add tile, `Empty`.

### Relationships

| Relationship | Current implementation | Intent | Candidate role | Owner | Relationship confidence | Value confidence | Decision | Notes |
|---|---|---|---|---|---|---|---|---|
| tab page shell | `PageShell variant="tab"` | same title/actions/body rhythm as other bot tabs | `page.tab*` roles | `PageShell` | high | medium-high | adopt | Confirms Tool Approval and tab pages. |
| action search width | `w-40 sm:w-56` | compact search beside import/add commands | `toolbar.searchWidth` | PageShell actions / toolbar | high | medium | adopt | Same pattern as Provider with a slightly narrower base. |
| action cluster gap | PageShell action slot `gap-2`; trailing buttons | group search, import, add | `toolbar.controlGap`, `toolbar.actionClusterGap` | PageShell actions | high | medium-high | adopt | Strong cross-slice evidence. |
| gallery grid gap | `grid grid-cols-1 gap-3 sm:grid-cols-2` | peer backend cards in a two-up list | `gallery.cardGap` | gallery/list primitive | high | medium-high | adopt | Confirms Provider. |
| backend card media gap/padding | reused `BackendCard` | server icon, title/subtitle, trailing status | `backendCard.padding`, `backendCard.mediaGap` | `BackendCard` | high | medium-high | adopt | Owner already exists. |
| add tile geometry | `min-h-[4.5rem] gap-2 border-dashed` | secondary add affordance beside real servers | `gallery.addTileMinHeight`, `gallery.addTileContentGap` | `AddTile` / gallery primitive | high | medium | adopt relationship | Dashed is correct here because it is an add tile, not a fully empty state. |
| fully empty frame | `Empty ... border border-border py-16` | empty MCP list as a real surface | `empty.outerFrame`, `empty.outerPaddingY` | `FramedEmpty` | high | medium-high | adopt | Confirms the dashed-frame debt identified in older empty states. |
| import dialog content frame | `DialogContent p-0`, header/footer `p-4`, editor body `p-4` | large editor dialog with fixed header/footer | `dialog.editorPadding`, `dialog.headerPadding`, `dialog.footerPadding` | editor/import dialog shell | medium-high | medium | adopt relationship later | Similar to import model dialog; needs a dialog subtype. |

## Slice: MCP Detail

- Archetype: selected backend configuration form with sections, advanced content, status, tools, and footer actions.
- File read: `apps/web/src/pages/bots/components/mcp-server-detail.vue`.
- Current owners: `SettingsSection`, `SettingsRow`, local content blocks, `KeyValueEditor`, footer action row.

### Relationships

| Relationship | Current implementation | Intent | Candidate role | Owner | Relationship confidence | Value confidence | Decision | Notes |
|---|---|---|---|---|---|---|---|---|
| detail section stack | root `space-y-6` | separate connection, advanced, tools, footer actions | `detail.sectionStackGap` | detail body / `SettingsShell`-like convention | high | medium-high | adopt | Confirms Provider detail. |
| connection card content padding | first section `space-y-5 p-4` | free-form form fields inside a settings card | `settings.contentPadding`, `settings.contentStackGap` | settings content block | high | medium | adopt | This is the missing sibling to `SettingsRow`. |
| name row inline gap | `flex items-end gap-3` | name field plus enabled/delete controls | `settings.contentInlineGap` / local field row | settings content block | medium-high | medium | adopt relationship later | It is a content form row, not a two-column `SettingsRow`. |
| enabled/delete cluster gap | delete/on-off row `gap-3`; label/switch `gap-2` | group destructive and enable controls | `settings.actionClusterGap`, `settings.switchInlineGap` | settings content/action cluster | high | medium | adopt relationship | Matches provider entity header action cluster. |
| field stack gap | repeated `Field` blocks inside `space-y-5` | vertical form rhythm in settings-card content | `settings.contentFieldGap` or `form.fieldGap.settingsContent` | settings content block / `FieldStack` | high | medium | adopt relationship | Must not collapse dialog forms and settings rows into one value blindly. |
| status block divider/gap | `space-y-2 border-t pt-4` | separate probe result from editable fields | `settings.contentDividerGap`, `settings.statusStackGap` | settings content status block | medium-high | medium | defer/adopt later | Status pattern should compare with health pages. |
| advanced collapsed row | `SettingsRow` with expand button | summary row for optional advanced content | settings row roles | `SettingsRow` | high | medium-high | adopt | Confirms `SettingsRow` expansion usage. |
| advanced expanded body | `space-y-5 p-4` | advanced free-form content inside the same section | `settings.contentPadding`, `settings.contentStackGap` | settings content block | high | medium | adopt | Same as connection card. |
| key-value editor rows | `flex flex-col gap-2`, row `gap-2` | dense key/value list inside advanced settings | `keyValue.rowGap`, `keyValue.columnGap` | `KeyValueEditor` | high | medium | component-owned | Component-owned, not global. |
| OAuth nested block | `space-y-3 border-t pt-4`, inner form fields | conditional auth config inside advanced content | `settings.conditionalBlockGap` | settings content block | medium-high | medium | defer | Needs OAuth/plugin comparison. |
| discovered tools padding/gap | section content `p-4`; badges `flex flex-wrap gap-1.5` | read-only tool chips | `settings.contentPadding`, `metadata.chipGap` | settings content / chip list | high | medium | adopt relationship for padding, chip gap local | Chip gap is metadata/component-owned. |
| footer action row | `flex flex-wrap items-center justify-end gap-2` | export/probe/save commands after detail sections | `settings.footerActionGap` / `detail.footerActionGap` | detail footer primitive | high | medium-high | adopt | Confirms Provider footer action rhythm, but footer location differs. |

## Slice: Tool Approval

- Archetype: bot-tab policy editor with one status row and expandable per-tool rows.
- File read: `apps/web/src/pages/bots/components/bot-tool-approval.vue`.
- Current owners: `PageShell`, `SettingsSection`, hand-rolled settings rows, inline editor blocks.

### Relationships

| Relationship | Current implementation | Intent | Candidate role | Owner | Relationship confidence | Value confidence | Decision | Notes |
|---|---|---|---|---|---|---|---|---|
| tab page shell with description | `PageShell variant="tab" :description` | title, intro, save action, body | `page.tab*`, `page.titleToDescription` | `PageShell` | high | medium-high | adopt | Confirms description handling in PageShell. |
| page body section stack | body `space-y-8` | separate status card and rules card | `section.stackGap` | page body / PageShell child convention | high | medium | adopt | Same broad setting-page rhythm as Overview. |
| status row | hand-rolled `mx-4 min-h-[3.75rem] gap-4 py-3 border-b` | one-setting row with switch | settings row roles | `SettingsRow` | high | medium-high | migrate to `SettingsRow` or row variant | This is exactly the SettingsRow contract. |
| section header action | `SettingsSection` actions reset button | title + local action | `section.actionGap`, `section.headerMinHeight` | `SettingsSection` | high | medium-high | adopt | Confirms action slot. |
| policy summary row | repeated `mx-4 min-h-[3.75rem] gap-4 py-3 border-b` | expandable object/config row | `settings.row*` | `SettingsRow` or `ExpandableSettingsRow` | high | medium-high | adopt | Should likely become an expandable row primitive. |
| expanded row body | `mx-4 space-y-4 border-b py-4` | inline editor attached to a policy row | `settings.expandedRowTopGap`, `settings.expandedRowPaddingY`, `settings.expandedRowGap` | `ExpandableSettingsRow` | high | medium | adopt | This is strong evidence for an expanded settings row contract. |
| field label/control gap | editor fields `space-y-1.5` | label + segmented/textarea control | `form.labelToControl` inside expanded settings row | `FieldStack` | high | medium | adopt | Same label/control rhythm as dialog form; owner context differs. |
| textarea min height | `min-h-24` | multiline policy entry area | field geometry | textarea/control | high | medium | local | Size, not spacing. |

## Slice: Confirm Popover

- Archetype: compact anchored confirmation surface.
- File read: `apps/web/src/components/confirm-popover/index.vue`.
- Current owner: `ConfirmPopover`.

### Relationships

| Relationship | Current implementation | Intent | Candidate role | Owner | Relationship confidence | Value confidence | Decision | Notes |
|---|---|---|---|---|---|---|---|---|
| popover body stack | `space-y-3` inside inherited `p-4` popover content | compact question/message/actions stack | `popover.contentGap` | `ConfirmPopover` / PopoverContent | high | medium | component-owned/adopt later | This is not settings spacing. |
| icon/question gap | `flex items-start gap-2` | attach optional icon to question | `popover.mediaGap` | `ConfirmPopover` | medium-high | medium | component-owned | Use component owner, not global `gap-2`. |
| action row gap | `gap-2 pt-1` | group cancel/confirm | `popover.footerActionGap`, `popover.footerTopPadding` | `ConfirmPopover` | high | medium | component-owned/adopt later | Useful later for popover contract. |

## Candidate Role Matrix Update

| Candidate role | Previous evidence | MCP/Tool Approval evidence | Relationship confidence | Value confidence | Decision | Owner |
|---|---:|---:|---|---|---|---|
| `page.tab*` | Bot overview and PageShell | MCP list + Tool Approval both use `PageShell variant="tab"` | high | medium-high | adopt | `PageShell` |
| `gallery.cardGap` | Provider | MCP list repeats `gap-3 sm:grid-cols-2` | high | medium-high | adopt | gallery/list primitive |
| `backendCard.*` | Provider | MCP list reuses `BackendCard` | high | medium-high | adopt | `BackendCard` |
| `gallery.addTile*` | Schedule/Provider idea | MCP add tile is clear beside real cards | high | medium | adopt relationship | `AddTile` |
| `empty.outerFrame` | Schedule/Provider | MCP empty uses solid frame correctly | high | medium-high | adopt | `FramedEmpty` |
| `detail.sectionStackGap` | Provider | MCP detail root `space-y-6` | high | medium-high | adopt | detail body / `SettingsShell` |
| `settings.contentPadding` | Bot overview/Provider OAuth | MCP connection/advanced/tools content use `p-4` | high | medium-high | adopt | settings content block |
| `settings.contentStackGap` | Provider OAuth | MCP connection/advanced use `space-y-5` | high | medium | adopt | settings content block |
| `settings.row*` | Overview/Provider | Tool Approval hand-rolls exact row anatomy | high | high | adopt | `SettingsRow` / expandable row |
| `settings.expandedRow*` | Overview hypothesis | Tool Approval open editor confirms it | high | medium | adopt | `ExpandableSettingsRow` |
| `settings.footerActionGap` | Provider | MCP footer row uses `gap-2` | high | medium-high | adopt | settings/detail footer primitive |
| `toolbar.searchWidth` | Provider | MCP action search repeats responsive width | high | medium | adopt | toolbar/action slot |
| `keyValue.*` | none | KeyValueEditor stable component-local gaps | high | medium | component-owned | `KeyValueEditor` |
| `popover.*` | none | ConfirmPopover stable compact surface | medium-high | medium | defer to popover contract | `ConfirmPopover` |

## MCP/Tool Approval Conclusions

1. MCP confirms `BackendCard`, `gallery.cardGap`, `AddTile`, and solid `FramedEmpty` as first-contract candidates.
2. Tool Approval confirms that `SettingsRow` is not merely visual preference. It is a real contract: row inset, min height, column gap, border, and vertical padding are repeated by hand.
3. MCP detail proves we need a `settings.contentStack` sibling to `SettingsRow`: not every settings-card child is a row; some cards contain free-form field stacks, status blocks, and chip lists.
4. `PageShell variant="tab"` should be part of the contract. It is a different environment from standalone pages because the bot detail pane already supplies outer padding.
5. `KeyValueEditor` should own its internal row/column gaps. Do not promote those values globally just because MCP uses them.
6. This slice does not add a new top-level family. It mostly validates the existing contract shape, which means first-pass research can stop soon.

## Open Questions

- Should `SettingsSection` grow an explicit content slot/class for `settings.contentPadding` and `settings.contentStackGap`, so MCP detail stops hand-rolling `p-4 space-y-5`?
- Should `SettingsRow` gain an expandable variant for Tool Approval-style inline editors?
- Should MCP detail setup view use `DetailPane` / `PageShell` instead of a local `section mx-auto max-w-3xl pt-6 pb-8`?
- Should import/editor dialogs become a named dialog subtype, separate from normal form dialogs?
- Should `ConfirmPopover` enter the first spacing contract or wait for a popover/menu pass?
