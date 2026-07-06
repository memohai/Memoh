# Memoh Spacing Contract V0

Date: 2026-07-01

Status: superseded by `docs/design/spacing/spacing-contract-v1.md`. Keep this file as
evidence only.

This file was the draft contract for review. Values are provisional where noted. The contract is intentionally owner-driven: roles belong to components or primitives, not to a global pile of named pixels.

Important: this is **not** a token manifest. It names spacing relationships and owners. Only a small subset should become public tokens; most adopted relationships should be enforced by composition primitives or component-owned aliases.

## Why This Exists

Memoh has enough evidence to stop broad spacing discovery and start standardizing. The current issue is not that individual pages use `8px`, `10px`, or `16px`. The issue is that repeated relationships are often re-decided in local markup:

- title to body;
- section label to card;
- setting row inset and height;
- dialog header/body/footer rhythm;
- form field label to control;
- sidebar identity/search/nav rhythm;
- backend-card and add-tile rhythm;
- chat thread/composer/tool-process rhythm.

This contract defines the relationships that page authors should not re-decide.

## Research Basis

Evidence documents:

- `docs/design/spacing/bot-overview-cartography.md`
- `docs/design/spacing/schedule-dialog-sidebar-cartography.md`
- `docs/design/spacing/provider-cartography.md`
- `docs/design/spacing/chat-tool-cartography.md`
- `docs/design/spacing/mcp-tool-approval-cartography.md`
- `docs/design/spacing/research-status.md`

Current confidence: high enough for v0. Bots grid/global sidebar and onboarding should validate boundaries, not block adoption.

## Core Rule

Use the narrowest owner that matches the relationship.

```txt
component-owned role > archetype role > primitive scale > local exception
```

Do not promote a number just because it repeats. Promote a relationship when changing it would affect hierarchy, rhythm, density, or future page composition.

Do not promote every adopted relationship into a public token. A relationship can be:

- public component API;
- component-owned CSS variable or constant;
- internal recipe documented in the component wall;
- primitive scale value;
- local exception.

Create a semantic token only when multiple owners must coordinate the same decision or designers need global tuning control.

## Primitive Scale

The primitive scale is still useful, but it is not the design system by itself.

| Primitive | Value | Typical use |
|---|---:|---|
| `0.5` | 2px | tiny icon/chevron offsets, dense metadata |
| `1` | 4px | label-description ties, micro row gaps |
| `1.5` | 6px | field label/control, tool rows, dense chips |
| `2` | 8px | command gaps, attachment gaps, compact stacks |
| `2.5` | 10px | compact shell padding, section label gap |
| `3` | 12px | row vertical padding, media gaps |
| `3.5` | 14px | compact card padding |
| `4` | 16px | default content padding, dialog padding |
| `5` | 20px | settings content field stack |
| `6` | 24px | page header/body, detail section stack, chat turns |
| `8` | 32px | top-level section stacks |
| `10` | 40px | sparse/hero state gap, usually exception-bound |

Use primitive values directly only inside a component owner or for one-off local geometry. Public page code should prefer primitives/components that already encode the relationship.

For the token-granularity decision behind this rule, see `docs/design/spacing/token-granularity-review.md`.

## Adopted Families

### PageShell

Owner: `PageShell`.

| Role | Current value | Confidence | Notes |
|---|---:|---|---|
| `page.maxWidth` | `max-w-3xl` | high | Normal settings/config pages. Do not reuse for chat. |
| `page.gutterX` | `px-6` | high | Standalone page variant. |
| `page.paddingTop` | `pt-10` | medium-high | Standalone page variant. |
| `page.paddingBottom` | `pb-12` | medium-high | Standalone page variant. |
| `page.headerToBody` | `mb-6` | high | Header block to body content. |
| `page.headerMinHeight` | `min-h-9` | high | Keeps title baseline stable with or without actions. |
| `page.titleInsetX` | `pl-2` | high | Aligns page title to section label/card rhythm. |
| `page.actionGap` | `gap-2` | high | Command cluster in header actions. |
| `page.titleToDescription` | `mt-0.5` | high | Title intro text, used by Tool Approval. |
| `page.tabMaxWidth` | `max-w-3xl` | high | Bot detail tab variant. |
| `page.tabPaddingTop` | `pt-6` | high | Used inside bot detail pane padding. |
| `page.tabPaddingBottom` | `pb-8` | high | Used inside bot detail pane padding. |

Rule: standalone pages and bot tabs can share PageShell, but they must use the right variant. Do not hand-roll headers in page components.

### SettingsSection

Owner: `SettingsSection`.

| Role | Current value | Confidence | Notes |
|---|---:|---|---|
| `section.labelToSurface` | `space-y-2.5` | high | Section label/action row to card. |
| `section.labelInsetX` | `px-2` | high | Aligns label with PageShell title inset. |
| `section.headerMinHeight` | `min-h-7` | high | Stable section action/header row. |
| `section.actionGap` | `gap-4` in header row, action components usually `gap-2` | medium-high | Header label/action separation. |
| `section.stackGap` | `space-y-8` for page body, `space-y-6` for detail body | high relationship, medium value | Context decides density. |

Rule: if content is a section with a title and framed card, use `SettingsSection`. Do not duplicate card chrome locally.

### SettingsRow

Owner: `SettingsRow`; likely future `ExpandableSettingsRow`.

| Role | Current value | Confidence | Notes |
|---|---:|---|---|
| `settings.rowInsetX` | `mx-4` | high | Inset border and row content. |
| `settings.rowMinHeight` | `min-h-[3.75rem]` | high | Default settings row height. |
| `settings.rowPaddingY` | `py-3` | high | Default row vertical rhythm. |
| `settings.rowColumnGap` | `gap-4` | high | Label/content or summary/action gap. |
| `settings.labelToDescription` | `mt-0.5` | high | Label to supporting description. |
| `settings.expandedRowPaddingY` | `py-4` | high relationship, medium value | Tool Approval open editor. |
| `settings.expandedRowGap` | `space-y-4` | high relationship, medium value | Inline editor fields under a row. |

Rule: use `SettingsRow` for label/control rows, switch rows, and summary/action rows. Tool Approval hand-rolled rows should migrate to a row primitive.

### Settings Content Block

Owner: future content slot/helper inside `SettingsSection`, or a small `SettingsContent` primitive.

| Role | Current value | Confidence | Notes |
|---|---:|---|---|
| `settings.contentPadding` | `p-4` | high | MCP connection/advanced/tools and Provider OAuth content. |
| `settings.contentStackGap` | `space-y-5` in MCP, `space-y-3` in compact OAuth | high relationship, medium value | Needs density variants. |
| `settings.contentFieldGap` | `space-y-5` | high relationship, medium value | Free-form fields in settings cards. |
| `settings.contentInlineGap` | `gap-3` | medium-high | Inline name + enable/delete row. |
| `settings.contentDividerTopGap` | `border-t pt-4` | medium-high | Status/OAuth conditional blocks. |
| `settings.footerActionGap` | `gap-2` | high | Provider and MCP footer actions. |

Rule: not every settings-card child should be a `SettingsRow`. Use content-block roles for free-form forms, key-value editors, OAuth blocks, status logs, or chip lists inside a section card.

### Form And Dialog

Owners: `FormStack`, `FieldStack`, `FormDialogShell`, `DialogContent`, `DialogHeader`, `DialogFooter`, `DialogScrollContent`.

| Role | Current value | Confidence | Notes |
|---|---:|---|---|
| `dialog.padding` | `p-4` or shell-owned | high relationship, medium value | Dialog shell should own edge padding. |
| `dialog.formMaxWidth` | New Task wider than legacy `FormDialogShell` | high relationship, low-medium value | Reconcile before migration. |
| `dialog.headerToBody` | `mt-4` in older dialogs; shell gap in New Task | high relationship, medium-low value | Should be shell-owned. |
| `form.fieldGap` | `space-y-4` New Task, `gap-3` older dialogs | high relationship, medium-low value | Adopt role, tune value. |
| `form.labelToControl` | `space-y-1.5` / older `mb-2` | high relationship, medium value | Prefer FieldStack ownership. |
| `form.labelMetaGap` | inline label optional text gap | high relationship, medium value | Used by New Task. |
| `form.inlineControlGap` | `gap-2` | high | Schedule and tool approval controls. |
| `form.footerTopGap` | shell/footer-owned | high relationship, medium-low value | Reconcile shells. |
| `form.footerActionGap` | `gap-2` | high | Dialog footer actions. |

Rule: dialog container rhythm and form field rhythm are related but not the same. Do not force settings-row forms and dialog forms into one token.

### Sidebar

Owners: `MasterDetailSidebarLayout`, `SettingsSidebar`, `NavItem`, bot/sidebar header primitives.

| Role | Current value | Confidence | Notes |
|---|---:|---|---|
| `sidebar.gutterX` | `px-4` header, `px-2` nav content | high | Persistent sidebar density. |
| `sidebar.headerTopPadding` | `pt-[18px]` plus mac reserve variants | medium-high | Desktop shell affects value. |
| `sidebar.backToIdentityGap` | `mt-3` | high | Bot sidebar. |
| `sidebar.identityCardPadding` | `p-3` | high | Bot identity card. |
| `sidebar.identityMediaGap` | `gap-3` | high | Avatar/icon to identity text. |
| `sidebar.identityStatusGap` | `mt-1`, `gap-1.5` | medium-high | Status dot/text row. |
| `sidebar.identityToSearchGap` | `mt-3` | high | Identity card to search. |
| `sidebar.navItemGap` | menu `gap-1` | high | Nav row stack. |
| `sidebar.navGroupGap` | `mt-4` | high | Group separation. |

Rule: sidebar roles are not page roles. Sidebars are fixed reading columns with denser rhythm.

### Gallery, Backend Cards, And Add Tiles

Owners: `BackendCard`, future `AddTile`, future gallery/list primitive.

| Role | Current value | Confidence | Notes |
|---|---:|---|---|
| `gallery.cardGap` | `gap-3` | high | Provider and MCP lists. |
| `backendCard.padding` | `p-3.5` | high | Existing `BackendCard`. |
| `backendCard.mediaGap` | `gap-3` | high | Leading media to body/trailing. |
| `backendCard.titleToSubtitle` | `mt-0.5` | high | Existing `BackendCard`. |
| `gallery.addTileMinHeight` | `min-h-[4.5rem]` in MCP | high relationship, medium value | Add tile beside real items. |
| `gallery.addTileContentGap` | `gap-2` | high relationship, medium value | Icon/text inside add tile. |

Rule: dashed frames are for add tiles beside real items, not fully empty states.

### Dense List

Owner: future dense list/list row primitive.

| Role | Current value | Confidence | Notes |
|---|---:|---|---|
| `list.toolbarPadding` | `p-4` | high | Provider model list. |
| `list.toolbarControlGap` | `gap-2` | high | Search/import/add cluster. |
| `list.rowInsetX` | `mx-4` | high | Inset dividers. |
| `list.rowMinHeight.dense` | `min-h-[3.25rem]` | high relationship, medium value | Provider model row. |
| `list.rowPaddingY.dense` | `py-2.5` | high relationship, medium value | Provider model row. |
| `list.rowColumnGap` | `gap-3` | high | Text block to action cluster. |
| `list.rowActionGap.dense` | `gap-0.5`, switch `mr-1` | medium-high relationship, low-medium value | Tune later. |
| `list.footerPadding` | `px-4 py-3` | high | Pagination/status footer. |

Rule: dense object rows are not `SettingsRow`. They are object/list rows with actions.

### Empty States

Owners: `FramedEmpty`, `Empty` usage wrappers.

| Role | Current value | Confidence | Notes |
|---|---:|---|---|
| `empty.outerFrame` | solid `border border-border` | high | Fully empty outer surfaces. |
| `empty.outerPaddingY` | `py-16` | high relationship, medium-high value | Provider/MCP empty lists. |
| `empty.inCardPaddingY` | `py-10` | high relationship, medium value | Empty state inside an existing card. |
| `empty.actionGap` | `EmptyContent` / button slot | high relationship, medium value | Owned by Empty usage. |

Rule:

- in-card empty: no second frame;
- fully empty outer surface: solid frame;
- dashed frame: only add tile beside existing items.

### Chat Surface

Owners: `ChatSurface`, chat pane layout, `MessageItem`, markdown renderer.

| Role | Current value | Confidence | Notes |
|---|---:|---|---|
| `chat.viewportGutterX` | `px-3 sm:px-5 lg:px-8` | high relationship, medium value | Scroll viewport edge. |
| `chat.threadMaxWidth` | `max-w-[840px]` | high | Chat rail, not page width. |
| `chat.threadGutterX` | `px-4 sm:px-6 lg:px-10` | high | Thread/composer shared rail. |
| `chat.threadPaddingTop` | `pt-6` | high relationship, medium value | First message breathing room. |
| `chat.threadPaddingBottom` | `pb-28` | high relationship, medium value | Fixed composer clearance. |
| `chat.turnGap` | `space-y-6` | high | Message turn rhythm. |
| `chat.messageAvatarGap` | `gap-3` | high | Avatar to message content. |
| `chat.senderNameToBody` | `mb-1` | high | Sender label to body. |
| `chat.userBlockGap` | `gap-2` | high | Bubble/attachments/actions. |
| `chat.userBubblePadding` | `px-4 py-3` | high relationship, medium value | User message bubble. |
| `chat.assistantBlockGap` | `space-y-[0.85rem]` | high relationship, low-medium value | Normalize later. |
| `chat.markdownRhythm.*` | local prose margins | high relationship, medium value | Renderer rhythm, not layout token. |

Rule: chat is a flow surface. Do not use page/settings/list spacing roles for it.

### Composer

Owner: `Composer`, consuming chat rail alignment.

| Role | Current value | Confidence | Notes |
|---|---:|---|---|
| `composer.threadAlignment` | same rail as chat thread | high | Should be shared, not retyped. |
| `composer.dockPaddingTop` | `pt-2` | high relationship, medium value | Dock to content separation. |
| `composer.dockPaddingBottom` | `pb-8` | high relationship, medium value | Window bottom/safe-area clearance. |
| `composer.padding` | `px-2.5 py-2.5` | high | Composer shell edge. |
| `composer.stackGap` | `gap-1` | high | Attachments/input/controls stack. |
| `composer.attachmentGap` | `gap-2` | high | Attachment tray. |
| `composer.attachmentToInputGap` | `pb-1.5` | high relationship, medium value | Attachment tray to textarea. |
| `composer.controlGap` | `gap-2` | high | Control clusters. |
| `composer.controlIconTextGap` | `gap-1` | high relationship, medium value | Small control pills. |

Exception: welcome-state `pt-[38dvh]` and `gap-10` are sparse/hero composition, not normal composer tokens.

### Tool Process

Owners: `ToolProcessRow`, `ToolProcessGroup`, `ToolProcessDetail`, tool-detail metadata primitives.

| Role | Current value | Confidence | Notes |
|---|---:|---|---|
| `tool.rowGap` | `gap-1.5` | high | Icon/status/name in process row. |
| `tool.rowPaddingY` | `py-px` / `py-0.5` | high relationship, medium value | Dense log rows. |
| `tool.groupBodyTopGap` | `mt-1` | high | Trigger to nested body. |
| `tool.groupBodyPadding` | `px-2.5 py-1.5` | high relationship, medium value | Nested process card. |
| `tool.groupItemGap` | `space-y-0.5` | high | Compact nested process rows. |
| `tool.detailTopGap` | `mt-1` inline, `mt-1.5` root/card | high | Row to detail content. |
| `tool.detailPadding` | `px-3 py-2` root, `px-2.5 py-2` grouped | high relationship, medium value | Detail wrapper. |
| `tool.detailStackGap` | `space-y-1.5` | high | Detail internals. |
| `tool.detailDenseStackGap` | `space-y-0.5` | high | Metadata internals. |
| `tool.detailKeyValueGap` | `gap-1.5` | high | Metadata label/value row. |
| `tool.approvalActionGap` | `mt-1.5 ml-5 gap-2` | high relationship, medium value | Inline approval action row. |

Rule: tool process spacing is not card/list/settings spacing. It is dense log rhythm inside chat.

## Provisional / Defer

These should be named in docs as known areas, but should not block v0:

- `detail.*`: relationship is real, but decide whether `DetailPane` and `SettingsShell` merge or stay paired.
- `banner.*`: one more state-heavy page should confirm values.
- `metric.*`: one more telemetry page should confirm values.
- `popover.*`: ConfirmPopover is clear, but a menu/popover pass can own it later.
- `onboarding.*`, `flow.*`, `sparse.*`: boundary/exception until audited.
- component geometry: widths, heights, avatar sizes, icon sizes, select widths, textarea heights, output max heights.
- micro metadata gaps unless the component owner is clear.

## Implementation Direction

Do not start with a giant Tailwind token rewrite. Start by making owners explicit.

1. Document v0 in the component wall.
2. Make `PageShell`, `SettingsSection`, `SettingsRow`, and `BackendCard` the first adopted owners.
3. Add `SettingsContent` and `ExpandableSettingsRow` before changing MCP and Tool Approval classes.
4. Add `FramedEmpty` and `AddTile`.
5. Reconcile `FormDialogShell` with the New Task dialog before broad dialog migration.
6. Keep chat/tool roles separate and migrate them only after configuration surfaces are stable.

## Validation Checklist

Before adopting a new role or changing a value, answer:

- What relationship does this describe?
- Who owns it?
- Is this relationship repeated or likely to recur?
- Would changing it affect hierarchy, reading rhythm, density, or future composition?
- Is the value high-confidence, or only the relationship?
- Does it belong to a component, an archetype, a primitive scale, or a local exception?

If the owner is unclear, do not add a token yet.
