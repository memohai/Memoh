# Schedule Dialog And Sidebar Spacing Cartography

Date: 2026-06-30

Status: second pilot slice, not a final spacing contract.

## Slice

- Archetypes:
  - bot tab list page with create/edit form dialog;
  - dense bot-detail settings sidebar with identity card, search, and grouped nav.
- Screenshot/source:
  - `/var/folders/4_/3rjnqff92rx0nnrpp0hbzz5h0000gn/T/codex-clipboard-4cf7ccc6-a275-48fb-a7e2-de9790eaa9b0.png`
- Primary files read:
  - `apps/web/src/pages/bots/components/bot-schedule.vue`
  - `apps/web/src/pages/bots/components/schedule-editor.vue`
  - `apps/web/src/pages/bots/components/schedule-list-item.vue`
  - `apps/web/src/pages/bots/detail.vue`
  - `apps/web/src/components/master-detail-sidebar-layout/index.vue`
  - `apps/web/src/components/settings-sidebar/index.vue`
  - `apps/web/src/components/settings-sidebar/nav-item.vue`
  - `packages/ui/src/components/dialog/DialogContent.vue`
  - `packages/ui/src/components/dialog/DialogScrollContent.vue`
  - `packages/ui/src/components/dialog/DialogHeader.vue`
  - `packages/ui/src/components/dialog/DialogFooter.vue`
- Current owner primitives:
  - `DialogContent`, `DialogScrollContent`, `DialogHeader`, `DialogFooter`
  - `Button`, `Input`, `Textarea`, `Select`, `Switch`, `TimeInput`
  - `MasterDetailSidebarLayout`, `NavItem`
- Current gold-reference status: the New Task dialog is already named by `memoh-web/reference.md` as the house standard for forms. This does not make every value final; it makes the relationships high-confidence evidence.

## Summary

The screenshot exposes two important spacing families that were not fully covered by the Bot Overview pilot:

- form/dialog rhythm: modal padding, content gap, field stack, label-to-control spacing, inline field clusters, advanced/disclosure spacing, and footer action grouping;
- sidebar rhythm: fixed sidebar width, top reserve, header stack, identity card padding, identity-to-search gap, nav item icon/text gap, group gap, and selected-item geometry.

The highest-confidence finding is that form spacing should become a first-class family, probably owned by a `FormStack` / `FieldStack` / `FormDialogShell` set of primitives. The current implementation uses raw composition classes, but the relationship pattern is coherent and repeated inside one dense form.

The second high-confidence finding is that sidebar spacing deserves its own family, separate from page/settings spacing. The left panel is not just a compact settings page. It has persistent navigation duties, a fixed reading column, and different density constraints.

## Slice: Schedule Page

- Archetype: bot tab list page with create action and empty/grid states.
- Files read:
  - `apps/web/src/pages/bots/components/bot-schedule.vue`
  - `apps/web/src/pages/bots/components/schedule-list-item.vue`
- Current owner primitives: mostly page-local; `PageShell` is not yet used here.

### Relationships

| Relationship | Current implementation | Intent | Candidate role | Owner | Relationship confidence | Value confidence | Decision | Notes |
|---|---|---|---|---|---|---|---|---|
| tab page content width | `mx-auto max-w-3xl` in `bot-schedule.vue:2` | keep bot tab content readable and aligned with sibling bot tabs | `page.maxWidth` | `PageShell` | high | medium | adopt relationship | Same shell relationship as Bot Overview. |
| tab page top/bottom padding | `pt-6 pb-8` in `bot-schedule.vue:2` | compensate for bot-detail parent padding | `page.paddingTop.tab`, `page.paddingBottom.tab` | `PageShell` | high | medium | adopt relationship | Confirms this is a bot-tab shell role, not a one-off. |
| page title/action row to body | `header` uses `mb-6` in `bot-schedule.vue:3` | separate title/action row from list or empty frame | `page.headerToBody` | `PageShell` | high | medium | adopt relationship | Same relationship as Bot Overview. |
| title/action horizontal split gap | `gap-4` in `bot-schedule.vue:3`; `PageShell` uses `gap-4` | keep title truncation and actions separated | `page.headerInlineGap` | `PageShell` | high | medium | adopt relationship | Should be owned by PageShell, not each page header. |
| action cluster gap | action group `gap-2` in `bot-schedule.vue:7` | group peer toolbar buttons | `page.actionGap` or `toolbar.actionGap` | `PageShell` actions slot / toolbar primitive | high | medium | adopt relationship | Recurs in PageShell actions and many toolbar rows. |
| page title inset | `px-2` on header in `bot-schedule.vue:3` | align title with card/section content rail | `page.titleInsetX` | `PageShell` | high | medium-low | adopt relationship, migrate owner | Current `px-2` also insets the right action, which PageShell explicitly avoids. |
| loading row inset and gap | `gap-2 px-2 text-xs` in `bot-schedule.vue:41-43` | keep loading state aligned to title rail and compact | `page.loadingInsetX`, `status.inlineGap` | page loading primitive or local status row | medium | low-medium | defer | Needs comparison with other loading states. |
| fully empty outer frame | dashed frame `rounded menu-shell border border-dashed py-16` in `bot-schedule.vue:50-52` | preserve list skeleton when no tasks exist | `empty.outerPaddingY`, `empty.outerFrame` | `FramedEmpty` / `Empty` wrapper | high | low | remove dashed, adopt relationship | `memoh-web` says fully empty outer surfaces should be solid, not dashed. Dashed is for add tiles beside real items. |
| empty icon to title | icon `mb-3 size-8` in `bot-schedule.vue:54` | decorative focal point above empty title | `empty.mediaToTitle` | `Empty` usage wrapper | medium | low | defer/remove | Current guidance defaults to no decorative icon unless deliberately chosen. |
| empty action to title/body | button `mt-4` in `bot-schedule.vue:58-62` | separate primary empty action from message | `empty.actionGap` | `Empty` usage wrapper | high | medium | adopt relationship | Value should be tested against other empty states. |
| populated list card gap | grid `gap-3` in `bot-schedule.vue:70-72` | separate sibling schedule cards | `list.cardGap` | list/grid primitive or page-local list wrapper | high | medium | adopt relationship, value provisional | Matches metric sibling-card gap but should stay semantically separate. |
| schedule card padding | `px-4 py-3.5` in `schedule-list-item.vue:3-4` | compact interactive item body | `list.cardPaddingX`, `list.cardPaddingY` | `ScheduleListItem` or shared list card primitive | medium-high | medium | defer owner | Needs backend/provider list comparison before adopting globally. |
| schedule card title/meta gap | item heading row `gap-2`; description `mt-0.5` in `schedule-list-item.vue:15-32` | bind time metadata to title and description to title row | `list.itemMetaGap`, `list.itemDescriptionGap` | list item primitive | medium | medium | defer | Similar to settings label-description but list owner differs. |
| schedule card action cluster gap | `gap-2` in `schedule-list-item.vue:37-39` | keep menu button and enable switch grouped | `list.itemActionGap` | list item primitive | medium | medium | defer | May generalize across backend cards. |

### Notes

The Schedule page strengthens the existing PageShell conclusion. It hand-rolls the same shell as Bot Overview, but with a header action. The 8px right-edge drift risk documented in `PageShell` is present because the whole `header` has `px-2`; future migration should move this page to `PageShell variant="tab"` so title inset and action alignment are owned in one place.

The empty state is important because it shows a relation that is valid but a value/style decision that is not. The fully empty frame relationship should be adopted, while the dashed frame should be marked as current debt.

## Slice: New Task Dialog

- Archetype: create/edit form dialog with plain fields, compound schedule picker, advanced disclosure, and submit footer.
- Files read:
  - `apps/web/src/pages/bots/components/bot-schedule.vue`
  - `apps/web/src/pages/bots/components/schedule-editor.vue`
  - dialog primitives under `packages/ui/src/components/dialog/`
- Current owner primitives:
  - `DialogScrollContent` owns modal frame spacing: `p-6`, `gap-4`, `max-w-lg`, `my-8`.
  - `DialogHeader` owns title stack gap: `gap-2`.
  - `DialogFooter` owns footer row layout and default `gap-2`.
  - `ScheduleEditor` currently owns the actual form stack.

### Relationships

| Relationship | Current implementation | Intent | Candidate role | Owner | Relationship confidence | Value confidence | Decision | Notes |
|---|---|---|---|---|---|---|---|---|
| dialog body max width | `DialogScrollContent class="sm:max-w-lg"` in `bot-schedule.vue:89`; base has `max-w-lg` | keep dense form readable while allowing fields to breathe | `dialog.formMaxWidth` | `DialogScrollContent` or `FormDialogShell` | high | medium | adopt relationship | This may become a form dialog size variant rather than a spacing token. |
| dialog outer padding | base `p-6` in `DialogScrollContent.vue:34` | create modal content inset from rounded edge | `dialog.padding` | `DialogScrollContent` | high | medium-high | adopt relationship | Primitive already owns it. |
| dialog content stack gap | base `gap-4` in `DialogScrollContent.vue:34` and `DialogContent.vue:50` | separate header, body, footer siblings | `dialog.contentGap` | `DialogContent` / `DialogScrollContent` | high | medium-high | adopt relationship | Same rung as form field stack. |
| dialog header title gap | `DialogHeader` `gap-2` in `DialogHeader.vue:13` | support title plus description without custom margin | `dialog.headerGap` | `DialogHeader` | high | medium | adopt relationship | Already primitive-owned. |
| form field stack | `form class="space-y-4"` in `schedule-editor.vue:2-4` | vertical rhythm between form fields/sections | `form.fieldGap` | new `FormStack` or `ScheduleEditor` until extracted | high | medium-high | adopt relationship | This is the central form spacing role. |
| label to control | field wrappers `space-y-1.5` in `schedule-editor.vue:7`, `31`, `43`, `154-157` | bind label and control tightly | `form.labelToControl` | new `FieldStack` | high | medium-high | adopt relationship | Strong candidate for shared primitive. |
| optional marker to label | optional span `ml-1` in `schedule-editor.vue:31-35` | make optional marker read as part of label, not separate text | `form.labelMetaGap` | `FieldLabel` or `FieldStack` | high | medium | adopt relationship | Recurs likely in provider/model forms. |
| first-row field to switch | top row `flex items-end gap-3` in `schedule-editor.vue:6` | pair primary name field with enable switch without treating switch as another full field | `form.inlineFieldGap` | `FormStack` / `InlineFieldRow` | high | medium | adopt relationship, test responsive | Current row may need narrow-screen behavior validation. |
| switch label to control | switch cluster `h-9 items-center gap-2` in `schedule-editor.vue:17` | align switch label to input height and keep switch attached | `form.switchInlineGap` | `SwitchField` | high | medium | adopt relationship | Should probably be owned by a SwitchField composition primitive. |
| schedule section label to picker controls | schedule block `space-y-3` in `schedule-editor.vue:56` | give compound picker more room than simple label/control fields | `form.compoundFieldGap` | `FormStack` / schedule picker | high | medium | adopt relationship | Distinct from `form.labelToControl`. |
| schedule picker inline controls | controls row `flex items-center gap-2 flex-wrap` in `schedule-editor.vue:61` | compose select, numeric inputs, time input, and text hints | `form.inlineControlGap` | compound field primitive or schedule picker | high | medium | adopt relationship | Could be general form role, but owner may be compound fields. |
| schedule picker wrapping | `flex-wrap` in `schedule-editor.vue:61` | prevent control overflow in narrow dialog | `form.inlineControlsWrap` | compound field primitive | high | medium | adopt relationship | This is a layout behavior, not just gap. |
| schedule mode select width | `SelectTrigger class="w-36 shrink-0"` in `schedule-editor.vue:63` | stable first control in compound picker | picker-local control size | schedule picker | high | medium | component-local | Width is size/local geometry, not semantic spacing. |
| numeric input widths | `w-20`, `w-16`, `w-24` in `schedule-editor.vue:83`, `96`, `124`, `209` | size numeric controls to expected digits | picker-local control size | schedule picker / field component | high | medium | component-local | Do not put into spacing taxonomy. |
| textarea height | `min-h-[4.5rem]` in `schedule-editor.vue:50` | preserve enough command editing area | form control size | `Textarea` variant or page-local | medium-high | medium | component-local/defer | Size, not spacing. Could become textarea size variant. |
| weekly day grid gap | `grid-cols-7 gap-1` in `schedule-editor.vue:136-139` | dense day chooser grid | `schedule.weekdayGap` | schedule picker | high | medium | component-local | Schedule-specific geometry. |
| weekday button size | `h-9` in `schedule-editor.vue:144` | align weekday touch height with controls | schedule picker geometry | schedule picker | high | medium | component-local | Size, not spacing. |
| advanced/disclosure trigger gap | trigger `gap-1.5` in `schedule-editor.vue:180-184` | bind chevron and label in a subtle disclosure row | `form.disclosureIconGap` | `DisclosureButton` / `TextButton` | medium-high | medium | defer | There may already be a shared disclosure pattern elsewhere. |
| disclosure content top gap | expanded content `mt-3` in `schedule-editor.vue:197-199` | separate hidden advanced control from trigger | `form.disclosureContentGap` | `DisclosureField` | high | medium | adopt relationship | Useful beyond schedule. |
| advanced row label/control spread | `justify-between gap-3` in `schedule-editor.vue:199` | put low-frequency option label and compact value input on one row | `form.optionRowGap` | `OptionRow` / `SettingsRow`-like form primitive | medium | medium | defer | Needs more dialog forms. |
| submit error to footer flow | error `<p>` participates in `space-y-4` at `schedule-editor.vue:217` | keep error in the form stack before actions | `form.errorToFooter` by stack membership | `FormStack` | medium-high | medium | defer | Might be covered by generic `form.fieldGap`. |
| footer to form flow | footer is last child in `space-y-4` at `schedule-editor.vue:224` | actions follow the same field stack rhythm | `form.footerTopGap` | `FormStack` / `DialogFooter` | high | medium | adopt relationship | Current implementation gets this from `space-y-4`, not a named prop. |
| footer action gap | `DialogFooter class="gap-2"` and inner right group `gap-2` in `schedule-editor.vue:224-244` | keep peer actions attached and predictable | `form.footerActionGap` / `dialog.footerGap` | `DialogFooter` | high | medium-high | adopt relationship | Primitive already has default `gap-2`; custom right group repeats it. |
| destructive action to primary actions | footer `sm:justify-between`, left delete slot `flex-1`, right group `gap-2` | separate destructive edit action from cancel/save pair | `form.footerSplitGap` or `dialog.footerSplit` | `FormDialogShell` | high | medium | adopt relationship | Important for edit dialogs, not create-only dialogs. |

### Patterns To Extract

| Primitive | Owns roles | Current examples | Why extract | Priority |
|---|---|---|---|---|
| `FormStack` | `form.fieldGap`, `form.footerTopGap` | `ScheduleEditor` form root `space-y-4` | Most forms should not restate stack rhythm. | high |
| `FieldStack` | `form.labelToControl`, label/meta layout | name, description, command, advanced pattern fields | Label/control spacing is a central form decision. | high |
| `InlineFieldRow` | `form.inlineFieldGap`, wrap behavior if needed | name field + enable switch row | Prevent every dialog from inventing top-row pair spacing. | medium |
| `SwitchField` | `form.switchInlineGap`, switch cluster height/alignment | Enable Task cluster | Switch plus label is common enough to own. | medium |
| `CompoundField` | `form.compoundFieldGap`, `form.inlineControlGap`, wrapping | schedule picker row | Compound controls need more spacing than simple fields. | medium |
| `DisclosureField` | `form.disclosureIconGap`, `form.disclosureContentGap` | More options | Advanced sections should open with one rhythm. | medium |
| `FormDialogShell` | dialog size variant, body stack, split footer | New Task dialog, edit mode delete action | Gives create/edit dialogs one anatomy instead of hand-written `Dialog` + form + footer. | medium-high |

### Exceptions And Local Geometry

| Item | Current implementation | Reason | Decision |
|---|---|---|---|
| Schedule mode select width | `w-36` | fixed readable control width inside schedule picker | component-local |
| Numeric/time input widths | `w-20`, `w-16`, `w-24` | input sizing depends on expected digits and format | component-local |
| Textarea minimum height | `min-h-[4.5rem]` | control size, not surrounding spacing | component-local/defer to Textarea size variants |
| Weekday grid | `grid-cols-7 gap-1`, `h-9` buttons | schedule-specific picker geometry | component-local |
| Raw button styling in weekday buttons | manual `button` with `bg-primary`, `hover:bg-accent`, `rounded-md` | UI contract issue adjacent to spacing | outside spacing scope, but should be normalized if picker is refactored |
| Destructive ghost overrides | `text-destructive hover:bg-destructive/10` on delete button | interaction chrome override smell | outside spacing scope, but should be revisited under UI contract |

## Slice: Bot Detail Sidebar

- Archetype: persistent navigation sidebar with bot identity, search, and grouped bot settings tabs.
- Files read:
  - `apps/web/src/pages/bots/detail.vue`
  - `apps/web/src/components/master-detail-sidebar-layout/index.vue`
  - `apps/web/src/components/settings-sidebar/index.vue`
  - `apps/web/src/components/settings-sidebar/nav-item.vue`
- Current owner primitives:
  - `MasterDetailSidebarLayout` owns width, flush/nested frame, scroll area, mobile sheet.
  - `NavItem` owns nav item button spacing.
  - Bot identity header is page-local.

### Relationships

| Relationship | Current implementation | Intent | Candidate role | Owner | Relationship confidence | Value confidence | Decision | Notes |
|---|---|---|---|---|---|---|---|---|
| sidebar fixed width | desktop `w-60!`; web `w-48 lg:w-52 xl:w-60` in `master-detail-sidebar-layout.vue:10-12`; settings sidebar desktop `15rem`, web 200-360 in `settings-sidebar.vue:6-10`, `203-211` | keep navigation column readable and prevent content-sized width drift | `sidebar.width`, `sidebar.widthMin`, `sidebar.widthMax` | `MasterDetailSidebarLayout` / `SettingsSidebar` | high | medium | adopt relationship | Width is size/layout, but it is core to sidebar spacing rhythm. |
| sidebar right divider | flush layout `border-r border-sidebar-border` in `master-detail-sidebar-layout.vue:21-25` | separate persistent nav from detail pane | `sidebar.divider` | sidebar layout primitive | high | medium-high | adopt relationship | Not spacing, but defines panel boundary. |
| mac traffic reserve | `h-12` reserve in `detail.vue:28-35`; `settings-sidebar.vue:15-29` | avoid macOS window controls while keeping nav balanced | `sidebar.trafficReserveHeight` | sidebar layout primitive | high in desktop | medium | component-local/adopt as layout role | Desktop-specific sidebar geometry. |
| web top padding to back row | `pt-[18px]` in `detail.vue:34-35` and `settings-sidebar.vue:28-29` | keep back affordance away from top edge | `sidebar.headerTopPadding` | sidebar header primitive | high | medium | adopt relationship | Value is already explained in code comments. |
| sidebar header horizontal inset | `px-4` in `detail.vue:34`; `px-[16px]` in settings sidebar `SidebarHeader` and groups | create consistent nav rail inside sidebar | `sidebar.gutterX` | sidebar layout/header primitive | high | medium-high | adopt relationship | Strong candidate. |
| back row to identity card | identity card `mt-3` after `NavItem` in `detail.vue:37-51` | make back and identity read as a header block without crowding | `sidebar.backToIdentityGap` | bot detail sidebar header | high | medium | adopt relationship | Specific to bot-detail header, but likely recurring for entity sidebars. |
| identity card padding | card `p-3` in `detail.vue:51` | give avatar/name/status a compact surface | `sidebar.identityCardPadding` | `IdentityCard` / sidebar header primitive | high | medium | adopt relationship, value provisional | Current screenshot confirms it reads as an anchor. |
| identity avatar to info gap | identity card `gap-3` in `detail.vue:51` | separate avatar and text block | `sidebar.identityMediaGap` | `IdentityCard` | high | medium | adopt relationship | Could be shared with list item media gaps only if owner matches. |
| name to edit icon gap | name row `gap-1` in `detail.vue:79` | keep edit affordance close to editable name | `sidebar.identityNameActionGap` | `IdentityCard` | medium | medium | component-local | Micro interaction geometry. |
| name to status gap | status row `mt-1` in `detail.vue:118` | subordinate status under entity name | `sidebar.identityStatusGap` | `IdentityCard` | high | medium | adopt relationship | Similar to row label-description but sidebar identity owner differs. |
| status dot to status label | status row `gap-1.5` in `detail.vue:118` | bind health dot and label | `status.inlineGap` | status primitive | medium-high | medium | component-local/defer | Same micro cluster as overview. |
| identity to search gap | search wrapper `mt-3` in `detail.vue:147-148` | separate identity header from local nav filtering | `sidebar.identityToSearchGap` | sidebar header primitive | high | medium | adopt relationship | Visible in screenshot and core to sidebar rhythm. |
| search icon/input inset | search icon `left-2.5`, input `pl-8 pr-8 h-8 text-xs` in `detail.vue:148-159` | compact search field with leading/trailing affordances | search field geometry | `InputGroup` or sidebar search primitive | high | medium | component-local/defer | Better owned by an InputGroup variant than spacing role. |
| header block to nav content | header ends at `pb-3`; content wrappers add nested padding (`px-2 pb-2` inside content plus scroll `p-2`) | separate header controls from nav groups | `sidebar.headerToNavGap` | sidebar layout primitive | medium | low | defer | Current nested `p-2` plus inner `px-2` may be accidental. Needs rendered measurement. |
| nav scroll content padding | scroll area inner `p-2 flex flex-col gap-1` in `master-detail-sidebar-layout.vue:36-39`; content `px-2 pb-2` in `detail.vue:176` | keep nav items off panel edges and separate slot content | `sidebar.navPadding` | `MasterDetailSidebarLayout` | medium | low | defer | Double-padding risk; needs visual and code cleanup review. |
| nav item icon/text gap | `NavItem` `gap-2.5` in `nav-item.vue:4` | make icon and label read as one row | `sidebar.navItemIconGap` | `NavItem` | high | medium-high | adopt relationship | Stable nav role. |
| nav item horizontal padding | `pl-3.5 pr-3` in `nav-item.vue:4` | align icon and label inside selectable row | `sidebar.navItemPaddingX` | `NavItem` | high | medium | adopt relationship | Current off-scale values may be optical; keep value provisional. |
| nav item stack gap | `SidebarMenu class="gap-1"` in `detail.vue:183` and settings sidebar groups | separate clickable nav rows in a dense menu | `sidebar.navItemGap` | `SidebarMenu` / sidebar primitive | high | medium-high | adopt relationship | Very stable. |
| nav group to group gap | group wrapper `idx > 0 ? 'mt-4'` in `detail.vue:181`; settings sidebar group `pt-4` | separate unrelated nav clusters | `sidebar.navGroupGap` | sidebar group primitive | high | medium-high | adopt relationship | Strong sidebar-specific role. |
| active nav item geometry | active class `bg-sidebar-accent`; screenshot selected row appears as full-width rounded button | `sidebar.navItemActiveFill`, plus button radius/height from Button | `sidebar.navItemActive` | `NavItem` and Button | high | medium | adopt relationship, not spacing token | Include in sidebar contract though not spacing-only. |
| detail pane gutter around tab component | wrapper `px-6 pt-4 pb-4` in `detail.vue:232-240`; child tab pages add `pt-6 pb-8` | detail pane gives side gutter; tab pages complete vertical rhythm | `page.gutterX`, `page.paddingTop.tab`, `page.paddingBottom.tab` | detail layout + `PageShell` | high | medium | adopt relationship | Important because tab pages omit their own `px-6`. |

### Patterns To Extract

| Primitive | Owns roles | Current examples | Why extract | Priority |
|---|---|---|---|---|
| `BotDetailSidebarHeader` | `sidebar.headerTopPadding`, `sidebar.gutterX`, `sidebar.backToIdentityGap`, `sidebar.identityToSearchGap` | `detail.vue:28-170` | The header anatomy is complex and should not remain a pile of local margins. | high |
| `SidebarIdentityCard` | `sidebar.identityCardPadding`, `sidebar.identityMediaGap`, `sidebar.identityStatusGap` | bot card in sidebar header | Entity sidebars will likely recur. | medium-high |
| `SidebarSearch` | search field height, icon inset, clear affordance | `detail.vue:147-169` | Sidebar search should not hand-roll icon/input insets. | medium |
| `NavItem` contract docs | `sidebar.navItemIconGap`, `sidebar.navItemPaddingX`, active fill | `settings-sidebar/nav-item.vue` | Already exists; should be documented as the owner of sidebar nav spacing. | high |
| `SidebarGroup` spacing contract | `sidebar.navItemGap`, `sidebar.navGroupGap` | bot detail and global settings sidebar | Group rhythm should be explicit across sidebars. | high |

### Exceptions And Local Geometry

| Item | Current implementation | Reason | Decision |
|---|---|---|---|
| macOS traffic-light reserve | `h-12`, `pt-[18px]` | platform shell geometry | sidebar layout role, desktop-specific |
| Avatar size | `size-12` | identity component size, not spacing | component-local |
| Name edit input height/padding | `h-7 px-2 pr-6` | inline editing geometry | component-local |
| Search field icon offsets | `left-2.5`, `pl-8`, `pr-8`, `h-8` | input composition geometry | component-local or `SidebarSearch` |
| Nav item optical padding | `pl-3.5 pr-3` | optical alignment inside sidebar button | adopt as `NavItem` contract only, not primitive scale token |

## Candidate Role Matrix

This matrix extends the first pilot. It intentionally separates relationship confidence from value confidence.

| Candidate role | Overview | Schedule page | Dialog | Sidebar | Backend list | Chat | Relationship confidence | Value confidence | Decision | Owner |
|---|---:|---:|---:|---:|---:|---:|---|---|---|---|
| `page.maxWidth` | yes | yes | no | no | likely | no | high | medium | adopt relationship | `PageShell` |
| `page.paddingTop.tab` | yes | yes | no | no | no | no | high | medium | adopt relationship | `PageShell` |
| `page.paddingBottom.tab` | yes | yes | no | no | no | no | high | medium | adopt relationship | `PageShell` |
| `page.headerToBody` | yes | yes | no | no | likely | no | high | medium | adopt relationship | `PageShell` |
| `page.titleInsetX` | yes | yes | no | no | likely | no | high | medium | adopt relationship | `PageShell` |
| `page.actionGap` | maybe | yes | no | no | likely | no | high | medium | adopt relationship | `PageShell` / toolbar |
| `empty.outerPaddingY` | no | yes | no | no | likely | no | high | medium-low | adopt relationship | `FramedEmpty` |
| `empty.outerFrame` | no | yes | no | no | likely | no | high | low | adopt relationship, remove dashed here | `FramedEmpty` |
| `empty.actionGap` | no | yes | no | no | likely | no | high | medium | adopt relationship | `Empty` wrapper |
| `list.cardGap` | no | yes | no | no | likely | no | high | medium | adopt relationship, tune later | list/grid primitive |
| `dialog.padding` | no | no | yes | no | no | no | high | medium-high | adopt relationship | `DialogContent` / `DialogScrollContent` |
| `dialog.contentGap` | no | no | yes | no | no | no | high | medium-high | adopt relationship | `DialogContent` / `DialogScrollContent` |
| `dialog.headerGap` | no | no | yes | no | no | no | high | medium | adopt relationship | `DialogHeader` |
| `form.fieldGap` | no | no | yes | no | likely | no | high | medium-high | adopt relationship | `FormStack` |
| `form.labelToControl` | maybe | no | yes | no | likely | no | high | medium-high | adopt relationship | `FieldStack` |
| `form.labelMetaGap` | no | no | yes | no | likely | no | high | medium | adopt relationship | `FieldLabel` / `FieldStack` |
| `form.inlineFieldGap` | no | no | yes | no | likely | no | high | medium | adopt relationship | `InlineFieldRow` |
| `form.switchInlineGap` | no | no | yes | no | likely | no | high | medium | adopt relationship | `SwitchField` |
| `form.compoundFieldGap` | no | no | yes | no | maybe | no | high | medium | adopt relationship | `CompoundField` |
| `form.inlineControlGap` | no | no | yes | no | maybe | no | high | medium | adopt relationship | `CompoundField` |
| `form.disclosureContentGap` | no | no | yes | no | likely | no | high | medium | adopt relationship | `DisclosureField` |
| `form.footerActionGap` | no | no | yes | no | likely | no | high | medium-high | adopt relationship | `DialogFooter` / `FormDialogShell` |
| `sidebar.width` | no | no | no | yes | no | no | high | medium | adopt relationship | sidebar layout |
| `sidebar.gutterX` | no | no | no | yes | no | no | high | medium-high | adopt relationship | sidebar layout |
| `sidebar.headerTopPadding` | no | no | no | yes | no | no | high | medium | adopt relationship | sidebar header |
| `sidebar.backToIdentityGap` | no | no | no | yes | no | no | high | medium | adopt relationship | bot detail sidebar header |
| `sidebar.identityCardPadding` | no | no | no | yes | no | no | high | medium | adopt relationship, tune later | `SidebarIdentityCard` |
| `sidebar.identityMediaGap` | no | no | no | yes | no | no | high | medium | adopt relationship | `SidebarIdentityCard` |
| `sidebar.identityToSearchGap` | no | no | no | yes | no | no | high | medium | adopt relationship | sidebar header |
| `sidebar.navItemIconGap` | no | no | no | yes | no | no | high | medium-high | adopt relationship | `NavItem` |
| `sidebar.navItemGap` | no | no | no | yes | no | no | high | medium-high | adopt relationship | `SidebarMenu` |
| `sidebar.navGroupGap` | no | no | no | yes | no | no | high | medium-high | adopt relationship | `SidebarGroup` |

## What This Changes In The Method

The first slice suggested page, section, settings row, banner, and metric families. This second slice adds two families:

1. **Form** should be promoted early. It is high-frequency and high-risk because every dialog invites local `space-y-*`, `gap-*`, and footer decisions.
2. **Sidebar** should be promoted early, but kept separate from page/settings roles. It is a navigation system, not a settings card system.

This also confirms the "relationship first, value second" rule. Examples:

- `empty.outerFrame`: relationship confidence is high, current dashed value/style confidence is low.
- `form.fieldGap`: relationship and value confidence are both relatively high because the dialog is already the form reference.
- `sidebar.navItemPaddingX`: relationship is high, exact value is probably optical and should live inside `NavItem`, not in a global scale.

## Proposed Next Slice Batch

For the 5 to 10 page expansion, prioritize these because they test different relationship families:

1. Backend/provider list plus add/edit dialog: validates list, empty/add tile, form, and backend-detail spacing.
2. Chat message stream plus tool-call detail: validates chat-specific roles and prevents settings roles from leaking into chat.
3. Global settings sidebar or another entity sidebar: validates whether bot-detail sidebar roles generalize.
4. A settings page with complex row variants, for example Tool Approval or MCP: tests `SettingsRow` extensions.
5. Onboarding/About/launcher style surface: marks exceptions and prevents over-normalizing sparse pages.

## First Actionable Contract Draft

Do not migrate broadly yet. The first contract can already be drafted around these owners:

- `PageShell`: page width, tab padding, title/body gap, title/action alignment, action gap.
- `DialogContent` / `FormDialogShell`: dialog padding, dialog content gap, form field stack, footer gap.
- `FieldStack`: label/control and optional label meta rhythm.
- `CompoundField`: inline form control gap and wrapping.
- `NavItem` / sidebar primitives: nav row icon gap, row padding, item gap, group gap, sidebar gutter.
- `FramedEmpty`: outer empty frame, padding, action gap, with dashed explicitly forbidden for fully empty state.

These are narrow enough to avoid token soup, but broad enough to cover the screenshot and the first pilot.

## Open Questions

- Should `ScheduleEditor` be the first extraction target for `FormStack` / `FieldStack`, or should we first audit two more dialogs?
- Should `FormDialogShell` replace the older `apps/web/src/components/form-dialog-shell` component, or should that component be refactored to become the new owner?
- Should sidebar roles be documented under `MasterDetailSidebarLayout`, `SettingsSidebar`, or a shared `SidebarLayout` contract?
- Does bot-detail sidebar currently double-apply padding through `MasterDetailSidebarLayout` scroll content plus local `px-2` wrappers?
- Should the Schedule fully-empty state be corrected now, or wait until `FramedEmpty` is defined?
- Should schedule picker weekday buttons be refactored away from manual button styling when spacing primitives are introduced?
