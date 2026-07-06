# Memoh Spacing Role Taxonomy

This is a starting taxonomy for Memoh spacing roles. Treat it as a hypothesis to test through interface slices, not as final law.

## Naming Rules

Name roles as:

```txt
<archetype>.<object>.<relationship-or-property>
```

Examples:

- `page.headerToBody`
- `section.labelToSurface`
- `settings.rowPaddingY`
- `form.labelToControl`
- `chat.turnGap`

Prefer relationship names over value names.

Bad:

```txt
space.24.semantic
gapLarge
card10px
```

Good:

```txt
page.headerToBody
settings.labelToDescription
banner.contentGap
```

## Role Adoption Criteria

Adopt a role only when most of these are true:

- It appears in 3+ places, or is highly likely to recur.
- It describes a relationship, not just a numeric value.
- Changing it would change product rhythm, hierarchy, or density.
- Future page authors should not re-decide it.
- It has an owner primitive or component.
- It can be documented in the component wall or reference docs.

If a value is useful but not semantic, keep it in the primitive scale.

## Confidence Split

Track relationship confidence separately from value confidence.

```txt
relationship confidence: high | medium | low
value confidence: high | medium | low
```

Use this when Memoh has no gold reference page yet:

- high relationship / medium value: adopt the role, tune the value later;
- high relationship / low value: adopt the relationship only if owner is clear, mark value provisional;
- medium relationship / low value: defer or keep local;
- low relationship / any value: do not adopt.

Example:

```txt
Role: page.headerToBody
Current: mb-6
Relationship confidence: high
Value confidence: medium
Decision: adopt relationship; keep value provisional until gold synthesis.
```

## Candidate Roles

### Page

```txt
page.maxWidth
page.gutterX
page.paddingTop
page.paddingBottom
page.headerMinHeight
page.titleInsetX
page.headerToBody
page.actionGap
```

Likely owner: `PageShell`.

### Section

```txt
section.stackGap
section.labelToSurface
section.labelInsetX
section.headerMinHeight
section.actionGap
```

Likely owner: `SettingsSection` or section-like primitives.

### Settings

```txt
settings.rowInsetX
settings.rowMinHeight
settings.rowPaddingY
settings.rowColumnGap
settings.labelToDescription
settings.contentPadding
settings.contentStackGap
settings.contentInlineGap
settings.contentDividerGap
settings.expandedRowGap
settings.expandedRowTopGap
settings.expandedRowPaddingY
settings.footerActionGap
```

Likely owner: `SettingsRow`, settings content block, possible `ExpandableSettingsRow`, and settings footer primitive.

Do not use settings roles for chat, dense tables, onboarding, or launcher pages.

`SettingsRow` is for label/control or summary/action rows. `settings.content*` is for free-form content inside a `SettingsSection` card, such as MCP connection forms, advanced key-value editors, OAuth status blocks, or read-only chip lists.

### Surface

```txt
surface.paddingCompact
surface.paddingDefault
surface.paddingLoose
surface.contentGap
surface.headerToContent
```

Likely owners: `Card`, `Dialog`, `Popover`, `Sheet`, or app-level primitives. Use carefully: this category can become too vague.

### Form

```txt
form.fieldGap
form.labelToControl
form.labelMetaGap
form.groupGap
form.inlineFieldGap
form.inlineControlGap
form.switchInlineGap
form.compoundFieldGap
form.disclosureIconGap
form.disclosureContentGap
form.advancedGap
form.footerGap
form.footerActionGap
```

Likely owners: `FormStack`, `FieldStack`, `InlineFieldRow`, `CompoundField`, `SwitchField`, `DisclosureField`, `FormDialogShell`.

Do not turn form control widths into spacing roles. Select widths, numeric input widths, text area heights, and picker grids are usually component-local geometry or size variants.

### Dialog

```txt
dialog.padding
dialog.contentGap
dialog.headerGap
dialog.footerGap
dialog.formMaxWidth
```

Likely owners: `DialogContent`, `DialogScrollContent`, `DialogHeader`, `DialogFooter`, or `FormDialogShell`.

Dialog roles are container rhythm. Form roles are field rhythm. They often share the same primitive rung, but they should not collapse into one name.

### Sidebar

```txt
sidebar.width
sidebar.widthMin
sidebar.widthMax
sidebar.gutterX
sidebar.headerTopPadding
sidebar.headerToNavGap
sidebar.backToIdentityGap
sidebar.identityCardPadding
sidebar.identityMediaGap
sidebar.identityStatusGap
sidebar.identityToSearchGap
sidebar.navPadding
sidebar.navItemIconGap
sidebar.navItemPaddingX
sidebar.navItemGap
sidebar.navGroupGap
```

Likely owners: `MasterDetailSidebarLayout`, `SettingsSidebar`, `NavItem`, `SidebarGroup`, and possible entity-specific sidebar header primitives.

Keep sidebar roles separate from page/settings roles. A sidebar is persistent navigation with a fixed reading column and denser rhythm; it should not inherit page shell spacing just because both have gutters.

### Backend List

```txt
list.cardGap
list.cardPadding
list.cardMinHeight
list.addTileMinHeight
list.toolbarGap
list.toolbarPadding
list.rowPaddingX
list.rowPaddingY
list.rowActionGap
list.footerPadding
```

Likely owners: `BackendList`, `BackendCard`, `AddTile`.

### Detail Pane

```txt
detail.backToBodyGap
detail.backRowPaddingTop
detail.backRowGutterX
detail.maxWidth.narrow
detail.maxWidth.standard
detail.maxWidth.wide
detail.sectionStackGap
```

Likely owners: `DetailPane`, `SettingsShell`, or `PageShell`.

Detail pane roles describe list-to-detail workflows, not all pages with a back button. Promote only if Providers, Web Search, MCP, or similar backend-detail flows confirm the same rhythm.

### Toolbar

```txt
toolbar.controlGap
toolbar.searchWidth
toolbar.padding
toolbar.toListGap
toolbar.actionClusterGap
```

Likely owners: `PageShell` actions slot, list toolbar primitive, or section toolbar primitive.

Keep toolbar roles separate from form inline-control roles. A toolbar organizes commands; a form field organizes input.

### Gallery / Add Tile

```txt
gallery.cardGap
gallery.cardPadding
gallery.cardMinHeight
gallery.addTileMinHeight
gallery.addTileContentGap
```

Likely owners: `BackendCard`, `BotCard`, `AddTile`, or gallery/list primitives.

Do not merge gallery card roles into metric tile roles just because both often use `gap-3`. A gallery card is navigational; a metric tile is read-only telemetry.

### Empty State

```txt
empty.inCardPaddingY
empty.outerPaddingY
empty.contentGap
empty.actionGap
```

Likely owners: `FramedEmpty`, `Empty` usage wrappers.

Remember the Memoh rule:

- in-card empty: no second frame;
- fully empty outer surface: solid frame;
- dashed frame: only add tile beside existing items.

### Banner

```txt
banner.paddingX
banner.paddingY
banner.contentGap
banner.iconGap
```

Likely owner: `StatusBanner`.

### Metric / Telemetry

```txt
metric.tileGap
metric.tilePaddingX
metric.tilePaddingY
metric.labelToValue
metric.valueToMeta
```

Likely owner: `MetricReadout`.

Do not force metric readouts into `SettingsRow` unless they are actually settings rows.

### Chat

```txt
chat.threadMaxWidth
chat.threadGutterX
chat.viewportGutterX
chat.threadTopPadding
chat.threadBottomPadding
chat.turnGap
chat.messageAvatarGap
chat.senderNameToBody
chat.userBlockGap
chat.userBubblePaddingX
chat.userBubblePaddingY
chat.attachmentGap
chat.assistantBlockGap
chat.actionsTopGap
chat.markdownRhythm.paragraphGap
chat.markdownRhythm.listGap
chat.markdownRhythm.headingGap
```

Likely owners: `ChatSurface`, chat pane layout, `MessageItem`, `MessageActions`, `AttachmentBlock`, and markdown renderer. Keep chat roles separate from settings roles even when values match.

Chat is a flow surface, not a page shell. The message thread and composer may share a rail, but that rail should not become `page.maxWidth`.

### Composer

```txt
composer.threadAlignment
composer.dockPaddingTop
composer.dockPaddingBottom
composer.paddingX
composer.paddingY
composer.stackGap
composer.attachmentGap
composer.attachmentToInputGap
composer.controlGap
composer.controlIconTextGap
composer.statusToInputGap
composer.jumpToInputGap
composer.welcomeOffset
composer.welcomeGap
```

Likely owner: `Composer`, with rail values supplied by `ChatSurface`.

Normal composer spacing can be adopted. Welcome-state offsets and hero-like gaps should remain sparse-state exceptions until onboarding is audited.

### Tool Process

```txt
tool.rowGap
tool.rowPaddingY
tool.disclosureGap
tool.groupBodyTopGap
tool.groupBodyPaddingX
tool.groupBodyPaddingY
tool.groupItemGap
tool.clusterBodyTopGap
tool.clusterItemGap
tool.detailTopGap.root
tool.detailTopGap.inline
tool.detailPaddingX
tool.detailPaddingY
tool.detailStackGap
tool.detailDenseStackGap
tool.detailKeyValueGap
tool.detailMetaGapX
tool.detailMetaGapY
tool.approvalActionTopGap
tool.approvalActionInsetX
tool.approvalActionGap
```

Likely owners: `ToolProcessRow`, `ToolProcessGroup`, `ToolProcessCluster`, `ToolProcessDetail`, and tool-detail metadata primitives.

Tool roles describe a dense process log inside the chat stream. Do not merge them into cards, dense list rows, or settings rows just because they use similar `gap` values.

### Launcher / Onboarding / Sparse Pages

Treat these as special archetypes until repetition appears:

```txt
launcher.tileGap
launcher.tilePadding
flow.viewportPadding
flow.panelMaxWidth
flow.titleToDescription
flow.descriptionToAction
flow.footerGap
flow.stepIndicatorGap
onboarding.stepGap
onboarding.footerGap
sparse.upperMiddleBias
```

Default decision: `defer` or `exception`.

Onboarding has spacing, but it should be a boundary sample before it becomes a source of first-contract tokens. Its theatrical composition, staged motion, fixed panel heights, and display-scale gaps can pollute normal configuration surfaces if promoted too early.

## Component-Wall Expectations

When adopting roles, add live examples to the component wall:

- primitive scale visual ruler;
- semantic role matrix;
- PageShell rhythm;
- SettingsSection and SettingsRow rhythm;
- Form rhythm;
- backend list and add tile rhythm;
- empty-state frames;
- banner rhythm;
- metric readout;
- chat/tool detail rhythm if chat roles are adopted.

The wall should teach spacing intentionally, not accidentally through specimen classes.

## Migration Order

Prefer high-reuse, low-risk migrations:

1. Update docs and component wall.
2. Normalize `PageShell`, `SettingsSection`, `SettingsRow` usage.
3. Extract `FramedEmpty` / `AddTile`.
4. Extract `StatusBanner`.
5. Extract `BackendList` or backend-list helpers.
6. Extract `MetricReadout`.
7. Map chat/tool detail roles separately.

Avoid broad class rewrites before the role matrix is reviewed.
