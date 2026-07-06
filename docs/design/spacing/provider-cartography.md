# Provider Spacing Cartography

Date: 2026-07-01

Status: third pilot slice. This is evidence for the first spacing contract, not a migration plan.

## Slice

- Archetype: global backend configuration workflow.
- Screenshot/source:
  - Providers detail screenshot from 2026-06-30 conversation.
- Primary files read:
  - `apps/web/src/pages/providers/index.vue`
  - `apps/web/src/pages/providers/model-setting.vue`
  - `apps/web/src/pages/providers/components/provider-form.vue`
  - `apps/web/src/pages/providers/components/model-list.vue`
  - `apps/web/src/pages/providers/components/model-item.vue`
  - `apps/web/src/components/settings/backend-card.vue`
  - `apps/web/src/components/settings/detail-pane.vue`
  - `apps/web/src/components/settings-shell/index.vue`
  - `apps/web/src/components/add-provider/index.vue`
  - `apps/web/src/components/form-dialog-shell/index.vue`
  - `apps/web/src/components/import-models-dialog/index.vue`
  - `apps/web/src/components/create-model/index.vue`
- Current owner primitives:
  - `PageShell`
  - `BackendCard`
  - `DetailPane`
  - `SettingsShell`
  - `SettingsSection`
  - `SettingsRow`
  - `FormDialogShell`
  - `ModelItem` as a local dense list row

## Summary

Providers is the highest-yield configuration slice so far. It confirms that Memoh's first spacing contract should not be just a bag of semantic tokens. It should be a set of owner primitives:

- `PageShell` owns global list-page rhythm.
- `BackendCard` owns backend/gallery item rhythm.
- `DetailPane` plus `SettingsShell` own list-to-detail rhythm.
- `SettingsSection` and `SettingsRow` own configuration rows, including form rows.
- `FormDialogShell` should be reconciled with the newer New Task dialog rhythm.
- A dense list row primitive is likely needed for model rows.

Provider also proves that some repeated numeric values are not the same role. `gap-3` appears in backend cards, entity headers, onboarding cards, and warning blocks, but those relationships need separate owners.

## Slice: Provider Gallery

- Archetype: global settings list page with search/action header and backend-card grid.
- Files read:
  - `apps/web/src/pages/providers/index.vue`
  - `apps/web/src/components/settings/backend-card.vue`
- Current owner primitives: `PageShell`, `BackendCard`, `InputGroup`, `Button`, `Empty`.

### Relationships

| Relationship | Current implementation | Intent | Candidate role | Owner | Relationship confidence | Value confidence | Decision | Notes |
|---|---|---|---|---|---|---|---|---|
| page shell | `PageShell` in `providers/index.vue:121` | list page title/actions/body rhythm | `page.*` roles | `PageShell` | high | medium-high | adopt | Confirms PageShell is the right owner for global settings list pages. |
| page action search width | `w-44 sm:w-56` in `providers/index.vue:126-129` | keep search compact beside Add action | `toolbar.searchWidth` | PageShell actions or toolbar primitive | high | medium | adopt relationship, tune later | Same idea appears in MCP and model list. |
| page action cluster gap | PageShell action slot owns `gap-2`; children are search + button | group search and Add Provider | `toolbar.controlGap` / `page.actionGap` | `PageShell` actions slot | high | medium | adopt | This is toolbar command spacing, not form spacing. |
| gallery grid gap | `grid grid-cols-1 gap-3 sm:grid-cols-2` in `providers/index.vue:146-149` | separate backend cards as peer navigation tiles | `gallery.cardGap` or `list.cardGap` | backend/gallery list primitive | high | medium | adopt relationship | Same structural pattern as Schedule/MCP list grids. |
| backend card padding | `p-3.5` in `backend-card.vue:4` | compact clickable backend tile body | `gallery.cardPadding` / `backendCard.padding` | `BackendCard` | high | medium | adopt relationship | Owner is clear; exact value can remain component-local. |
| backend card media gap | `gap-3` in `backend-card.vue:4` | separate icon/avatar, text, trailing metadata | `backendCard.mediaGap` | `BackendCard` | high | medium | adopt relationship | Do not merge with generic `gap-3`. |
| backend card title to subtitle | `mt-0.5` in `backend-card.vue:18-21` | bind secondary detail to title | `backendCard.titleToSubtitle` | `BackendCard` | high | medium-high | adopt | Same number as settings label-description, different owner. |
| provider icon size | leading icon wrapper `size-10` in `providers/index.vue:158-171`; BackendCard accepts slot | stable avatar/icon block | backend card media size | `BackendCard` slot convention | medium-high | medium | component-local/defer | Size, not spacing. Could become BackendCard prop. |
| trailing model count gap | `gap-1` in `providers/index.vue:174-180` | bind icon and count as metadata | metadata micro-gap | local metadata/status primitive | medium | medium | component-local/defer | Too small/specific for first contract. |
| empty outer frame | dashed `border-dashed py-16` in `providers/index.vue:189-191` | preserve gallery skeleton when no providers exist | `empty.outerFrame`, `empty.outerPaddingY` | `FramedEmpty` | high | low for dashed | adopt relationship, remove dashed | Same debt as Schedule: fully empty outer surface should be solid. |
| empty icon/title/body/action rhythm | `EmptyHeader`, `EmptyMedia`, `EmptyTitle`, `EmptyDescription`, `EmptyContent` | structured empty message | `empty.contentGap`, `empty.actionGap` | `Empty` usage wrapper | high | medium-low | adopt relationship, value provisional | Decorative icon is not automatically a first-contract pattern. |

## Slice: Provider Detail Pane

- Archetype: list-to-detail workflow with width-matched back row and detail body.
- Files read:
  - `apps/web/src/components/settings/detail-pane.vue`
  - `apps/web/src/components/settings-shell/index.vue`
  - `apps/web/src/pages/providers/model-setting.vue`
- Current owner primitives: `DetailPane`, `SettingsShell`.

### Relationships

| Relationship | Current implementation | Intent | Candidate role | Owner | Relationship confidence | Value confidence | Decision | Notes |
|---|---|---|---|---|---|---|---|---|
| detail back row width | `DetailPane width="narrow"` and `max-w-3xl` in `detail-pane.vue:37-46` | align back row to detail content width | `detail.maxWidth.narrow` | `DetailPane` / `SettingsShell` | high | medium | adopt relationship | Providers validates `detail.*` family. |
| detail back row gutter | `px-4 md:px-6` in `detail-pane.vue:5-7` | match SettingsShell body gutter | `detail.backRowGutterX` | `DetailPane` | high | medium | adopt | Must remain synced with SettingsShell. |
| detail back row top padding | `pt-4 md:pt-6` in `detail-pane.vue:5-7` | place back affordance before detail body | `detail.backRowPaddingTop` | `DetailPane` | high | medium | adopt relationship, value provisional | Different from page top padding. |
| detail body shell | `SettingsShell width="narrow"` in `model-setting.vue:2`; `SettingsShell` uses `px-4 pt-2 pb-10 md:px-6 md:pt-4 md:pb-12` | reusable detail content body | `detail.bodyGutterX`, `detail.bodyPaddingTop`, `detail.bodyPaddingBottom` | `SettingsShell` | high | medium | adopt relationship | This should be documented with DetailPane. |
| detail section stack | `space-y-6` in `model-setting.vue:3` | separate identity header, configuration, model list | `detail.sectionStackGap` | `SettingsShell` child convention | high | medium | adopt relationship | Distinct from page `section.stackGap` (`space-y-8`) because detail is denser. |

### Notes

`DetailPane` and `SettingsShell` form a paired primitive, but their contract is currently split across two files. The first spacing contract should document them together so back row width, body width, and gutters cannot drift.

## Slice: Provider Identity Header

- Archetype: compact entity header above configuration sections.
- File read: `apps/web/src/pages/providers/model-setting.vue`.
- Current owner: page-local `section`.

### Relationships

| Relationship | Current implementation | Intent | Candidate role | Owner | Relationship confidence | Value confidence | Decision | Notes |
|---|---|---|---|---|---|---|---|---|
| entity header frame | `rounded menu-shell border bg-card px-4 py-3` in `model-setting.vue:4` | compact selected-provider summary surface | `entityHeader.paddingX`, `entityHeader.paddingY` | new `EntityHeader` or detail primitive | high | medium | adopt relationship, defer owner | Similar to sidebar identity card but wider/detail-owned. |
| entity media gap | `gap-3` in `model-setting.vue:4` | separate provider icon and name | `entityHeader.mediaGap` | `EntityHeader` | high | medium | adopt relationship | Same intent as backend card media gap, but detail header owner differs. |
| entity action gap | trailing action cluster `gap-2` in `model-setting.vue:23` | group delete and enable switch | `entityHeader.actionGap` | `EntityHeader` | high | medium | adopt relationship | Likely reusable across detail headers. |
| icon size | icon wrapper `size-9` in `model-setting.vue:5` | stable compact identity icon | entity header local size | `EntityHeader` | medium-high | medium | component-local | Size, not spacing. |

## Slice: Provider Configuration Form

- Archetype: settings-card-as-form; rows are configuration fields, footer owns actions.
- Files read:
  - `apps/web/src/pages/providers/components/provider-form.vue`
  - `apps/web/src/components/settings/section.vue`
  - `apps/web/src/components/settings/row.vue`
- Current owner primitives: `SettingsSection`, `SettingsRow`, form controls, footer local block.

### Relationships

| Relationship | Current implementation | Intent | Candidate role | Owner | Relationship confidence | Value confidence | Decision | Notes |
|---|---|---|---|---|---|---|---|---|
| section label to config card | `SettingsSection :title` in `provider-form.vue:3` | label and card grouping | `section.labelToSurface` | `SettingsSection` | high | medium-high | adopt | Confirms first pilot. |
| config row rhythm | repeated `SettingsRow` in `provider-form.vue:12`, `32`, `64`, `85`, `106` | field rows inside one card | settings row roles | `SettingsRow` | high | medium-high | adopt | Strong evidence that form rows can be represented as settings rows in detail pages. |
| row control width | `FormItem class="w-80"` in `provider-form.vue:13`, `33`, `65`, `86` | align input widths in two-column settings rows | `settings.formControlWidth` | `SettingsRow` / form-row convention | high | medium | adopt relationship as size/layout role | Not spacing exactly, but critical to row rhythm. |
| compact select width | prompt cache `SelectTrigger min-w-36` in `provider-form.vue:117-120` | compact option control for short values | row control local size | provider form | medium | medium | component-local/defer | Size. Needs more forms before tokenizing. |
| footer row divider/inset | `mx-4 border-t py-3` in `provider-form.vue:144` | close field rows and separate actions | `settings.footerInsetX`, `settings.footerPaddingY` | `SettingsSection` footer slot or settings footer primitive | high | medium | adopt relationship | This is missing as a shared primitive. |
| footer action gap | footer `gap-2` and inner `gap-2` in `provider-form.vue:144-145` | group test/save actions | `settings.footerActionGap` / `toolbar.actionClusterGap` | settings footer primitive | high | medium-high | adopt relationship | Same command gap as other action clusters. |
| OAuth block section gap | OAuth `SettingsSection class="mt-6"` in `provider-form.vue:209-212` | separate conditional config section from main config | `detail.sectionStackGap` or `settings.conditionalSectionGap` | detail body / SettingsSection stack | medium-high | medium | adopt relationship, naming needs care | Same value as detail stack, but local margin bypasses stack owner. |
| OAuth content padding/gap | `p-4 space-y-3 text-xs` in `provider-form.vue:213` | dense informational config content inside a card | `settings.contentPadding`, `settings.contentGap.compact` | settings content block primitive | high | medium | adopt relationship | Confirms settings content block from Bot Overview. |
| OAuth nested block padding/gap | `rounded-md bg-muted/40 p-3 space-y-2` and `space-y-1` in `provider-form.vue:251-299` | grouped device/account info inside content card | component-local or `infoBlock.*` | OAuth status block | medium | low-medium | defer/component-local | Nested blocks risk becoming token soup. |

### Notes

Provider configuration strongly supports a distinction between two form modes:

- **dialog form rhythm**: `FormStack` / `FieldStack`, as in New Task and Add Provider.
- **settings-row form rhythm**: `SettingsSection` / `SettingsRow`, as in Provider detail.

Do not force these into one `form.fieldGap` model. A settings-row form is a settings card whose right column contains controls.

## Slice: Model List

- Archetype: dense list inside a settings section, with toolbar, rows, pagination, and in-card empty states.
- Files read:
  - `apps/web/src/pages/providers/components/model-list.vue`
  - `apps/web/src/pages/providers/components/model-item.vue`
- Current owner primitives: `SettingsSection`, local `ModelItem`, `InputGroup`, `Pagination`, `Empty`.

### Relationships

| Relationship | Current implementation | Intent | Candidate role | Owner | Relationship confidence | Value confidence | Decision | Notes |
|---|---|---|---|---|---|---|---|---|
| list toolbar padding | toolbar `p-4` in `model-list.vue:7` | give search/actions a padded block inside section card | `list.toolbarPadding` | dense list primitive | high | medium | adopt relationship | Different from page action slot. |
| list toolbar control gap | toolbar `gap-2`; action cluster `gap-2` in `model-list.vue:7`, `21-24` | group search, import, add controls | `toolbar.controlGap`, `toolbar.actionClusterGap` | list toolbar primitive | high | medium-high | adopt | Confirms toolbar roles. |
| toolbar to first row | no divider after toolbar; spacing and row padding separate it | avoid over-ruled card interior | `toolbar.toListGap` by layout | list section primitive | medium-high | medium | adopt relationship, value implicit | Document as "no divider after toolbar" if adopted. |
| model row inset | `mx-4` in `model-item.vue:6` | inset row dividers inside settings card | `list.rowInsetX` or reuse `settings.rowInsetX` | dense list row primitive | high | medium | adopt relationship | Same visual divider rule as SettingsRow, but denser row owner. |
| model row min height | `min-h-[3.25rem]` in `model-item.vue:6` | compact row for many models | `list.rowMinHeight.dense` | dense list row primitive | high | medium | adopt relationship | Distinct from SettingsRow `3.75rem`. |
| model row vertical padding | `py-2.5` in `model-item.vue:6` | dense list rhythm | `list.rowPaddingY.dense` | dense list row primitive | high | medium | adopt relationship | Strong argument for a list row primitive. |
| model row column gap | `gap-3` in `model-item.vue:6` | separate text block and action cluster | `list.rowColumnGap` | dense list row primitive | high | medium | adopt relationship | Distinct from SettingsRow `gap-4`. |
| model title/meta gap | title row `gap-2`; second line `mt-0.5 gap-2` in `model-item.vue:16-50` | compact row metadata rhythm | `list.rowMetaGap`, `list.rowDescriptionGap` | dense list row primitive | high | medium | adopt relationship | Repeats backend card title/subtitle idea with list owner. |
| model action cluster gap | `gap-0.5`, switch `mr-1` in `model-item.vue:66-69` | dense icon action strip with switch leading | `list.rowActionGap.dense` | dense list row primitive | medium-high | low-medium | adopt relationship, tune value later | Value may be too tight; relationship is real. |
| pagination footer padding | `border-t px-4 py-3` in `model-list.vue:52-55` | separate pagination/status footer from rows | `list.footerPaddingX`, `list.footerPaddingY` | dense list primitive | high | medium | adopt relationship | Similar to settings footer but full-width divider. |
| in-card empty padding | `Empty class="px-4 py-10"` in `model-list.vue:90-105` | show empty/no-results inside existing section frame | `empty.inCardPaddingY`, `empty.inCardPaddingX` | `Empty` usage wrapper / dense list | high | medium | adopt relationship | No border here, correctly uses parent section frame. |

### Pattern To Extract

`ModelItem` is a strong candidate for a generic dense list row primitive:

```txt
DenseListSection
  owns: list.toolbarPadding, toolbar.controlGap, list.rowInsetX, list.rowMinHeight.dense,
        list.rowPaddingY.dense, list.rowColumnGap, list.footerPadding, empty.inCardPaddingY
```

This is not the same as `SettingsRow`. Settings rows are configuration rows with a label/control split. Dense list rows are object rows with actions.

## Slice: Add Provider / Model Dialogs

- Archetype: older form dialog shell with create forms.
- Files read:
  - `apps/web/src/components/form-dialog-shell/index.vue`
  - `apps/web/src/components/add-provider/index.vue`
  - `apps/web/src/components/import-models-dialog/index.vue`
  - `apps/web/src/components/create-model/index.vue`
- Current owner primitives: `FormDialogShell`, local `flex flex-col gap-3 mt-4` bodies.

### Relationships

| Relationship | Current implementation | Intent | Candidate role | Owner | Relationship confidence | Value confidence | Decision | Notes |
|---|---|---|---|---|---|---|---|---|
| form dialog max width | default `sm:max-w-106.25` in `form-dialog-shell.vue:67-72` | custom form modal width | `dialog.formMaxWidth` | `FormDialogShell` | high | low-medium | adopt relationship, tune value | Off-scale width suggests this should be normalized. |
| dialog header/body gap | body slot begins with `mt-4` in `add-provider.vue:28-30`, `create-model.vue:20-22`, `import-models-dialog.vue:21-23` | separate header from form body | `dialog.headerToBody` or `form.bodyTopGap` | `FormDialogShell` | high | low-medium | adopt relationship, move owner | Should be shell-owned, not repeated in every body slot. |
| dialog form body stack | `flex flex-col gap-3` in add/model/import bodies | vertical field rhythm | `form.fieldGap` | `FormStack` / `FormDialogShell` | high | medium-low | adopt relationship, reconcile with New Task | New Task uses `space-y-4`; this older shell uses `gap-3`. Values need review. |
| label to control | repeated `Label class="mb-2"` in add/model dialogs | field label/control relationship | `form.labelToControl` | `FieldStack` | high | medium-low | adopt relationship, reconcile value | New Task uses `space-y-1.5`; values differ. |
| switch row in dialog | `FormItem flex ... justify-between rounded-lg border p-3 shadow-sm` in `add-provider.vue:158-177` | binary option row inside dialog | `form.optionRowPadding`, `form.optionRowColumnGap` | `SwitchField` / option row primitive | medium-high | low | defer | Current row has card-like chrome; should be reviewed under UI contract. |
| dialog footer top gap | `DialogFooter class="mt-4"` in `form-dialog-shell.vue:21` | separate body from actions | `form.footerTopGap` | `FormDialogShell` | high | medium-low | adopt relationship | Should align with New Task dialog. |
| footer action gap | `DialogFooter` default `gap-2` | group cancel/submit | `form.footerActionGap` | `DialogFooter` | high | medium-high | adopt | Confirms previous slice. |

### Notes

`FormDialogShell` is the clearest reconciliation task in the spacing system:

- New Task dialog is the current reference for create/edit forms.
- Provider add/model/import dialogs use an older shell with `mt-4`, `gap-3`, `mb-2`, and an off-scale max width.
- The first contract should not pick one value blindly. It should define the owner first: `FormDialogShell` should own body top gap, field stack, footer gap, and width variant.

## Candidate Role Matrix Update

| Candidate role | Overview | Schedule/Dialog | Sidebar | Provider | Relationship confidence | Value confidence | Decision | Owner |
|---|---:|---:|---:|---:|---|---|---|---|
| `page.*` shell roles | yes | yes | no | yes | high | medium-high | adopt | `PageShell` |
| `gallery.cardGap` / `list.cardGap` | maybe | yes | no | yes | high | medium | adopt relationship | gallery/list primitive |
| `backendCard.padding` | no | maybe | no | yes | high | medium | adopt relationship | `BackendCard` |
| `backendCard.mediaGap` | no | maybe | no | yes | high | medium | adopt relationship | `BackendCard` |
| `empty.outerFrame` | no | yes | no | yes | high | low for current dashed examples | adopt relationship, remove dashed debt | `FramedEmpty` |
| `detail.*` roles | no | no | no | yes | high | medium | adopt relationship | `DetailPane` + `SettingsShell` |
| `detail.sectionStackGap` | no | no | no | yes | high | medium | adopt relationship | `SettingsShell` child convention |
| `entityHeader.*` | no | no | related | yes | high | medium | defer owner, adopt relationship | new detail/entity header primitive |
| `settings.*` row roles | yes | no | no | yes | high | medium-high | adopt | `SettingsSection`, `SettingsRow` |
| `settings.formControlWidth` | no | no | no | yes | high | medium | adopt as layout role | `SettingsRow` form variant |
| `settings.footer*` | maybe | no | no | yes | high | medium | adopt relationship | settings footer primitive |
| `settings.contentPadding` / `contentGap` | yes | no | no | yes | high | medium | adopt relationship | settings content block |
| `toolbar.controlGap` | maybe | yes | no | yes | high | medium-high | adopt | toolbar/list/action primitives |
| `list.row*` dense roles | no | no | no | yes | high | medium | adopt relationship | dense list row primitive |
| `empty.inCardPaddingY` | no | no | no | yes | high | medium | adopt | `Empty` usage wrapper / dense list |
| `dialog.formMaxWidth` | no | yes | no | yes | high | low-medium | adopt relationship, tune value | `FormDialogShell` / `DialogScrollContent` |
| `form.fieldGap` | no | yes | no | yes | high | medium-low | adopt relationship, reconcile values | `FormStack` |
| `form.labelToControl` | no | yes | no | yes | high | medium-low | adopt relationship, reconcile values | `FieldStack` |
| `form.footerActionGap` | no | yes | no | yes | high | medium-high | adopt | `DialogFooter` / `FormDialogShell` |

## Provider Conclusions

1. Providers moves the research from "role ideas" to "owner candidates." Many relationships now have obvious owners.
2. `BackendCard` and a future `AddTile` / gallery primitive should enter the first contract if MCP confirms the same backend-list pattern.
3. `DetailPane` and `SettingsShell` should be documented together; they are a paired detail workflow primitive.
4. `SettingsRow` can carry settings-row forms, but dialog forms need `FormStack` / `FieldStack`. Those are different form modes.
5. `ModelItem` shows that Memoh needs a dense list row primitive. It should not be squeezed into `SettingsRow`.
6. Fully empty Provider and Schedule pages both use dashed frames today. That relationship should be normalized to solid `FramedEmpty`; dashed should remain for add tiles beside real items.
7. `FormDialogShell` is older than the New Task dialog reference. Its roles are valid, but its values should be reconciled before migration.

## Open Questions

- Should `BackendCard` own icon/avatar size conventions, or should leading media remain slot-local?
- Should Provider gallery and bot gallery share a generic `GalleryGrid`, or should backend lists remain distinct from bot launchers?
- Should `DetailPane` absorb `SettingsShell`, or should they remain separate but documented as a pair?
- Should a settings footer become a slot on `SettingsSection` so provider footer rows do not hand-roll `mx-4 border-t py-3`?
- Should dense list rows become a shared `SettingsListRow` / `ObjectListRow` primitive?
- Should `FormDialogShell` be rewritten to use the New Task dialog anatomy before any broad form migration?
