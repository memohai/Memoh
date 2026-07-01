# Chat And Tool Spacing Cartography

Date: 2026-07-01

Status: fourth pilot slice. This is evidence for the first spacing contract, not a migration plan.

## Slice

- Archetype: conversational flow surface with inline process/log details.
- Primary files read:
  - `apps/web/src/pages/home/components/chat-pane.vue`
  - `apps/web/src/pages/home/components/message-item.vue`
  - `apps/web/src/pages/home/components/message-actions.vue`
  - `apps/web/src/pages/home/components/attachment-block.vue`
  - `apps/web/src/pages/home/components/chat-attachment-card.vue`
  - `apps/web/src/pages/home/components/tool-call-inline.vue`
  - `apps/web/src/pages/home/components/tool-call-group.vue`
  - `apps/web/src/pages/home/components/tool-call-cluster.vue`
  - `apps/web/src/pages/home/components/thinking-block.vue`
  - `apps/web/src/pages/home/components/tool-call-detail-exec.vue`
  - `apps/web/src/pages/home/components/tool-call-detail-edit.vue`
  - `apps/web/src/pages/home/components/tool-call-detail-generic.vue`
  - `apps/web/src/pages/home/components/tool-call-detail-spawn.vue`
  - `apps/web/src/pages/home/components/tool-call-detail-browser.vue`
  - `apps/web/src/pages/home/components/tool-call-detail-output.vue`
  - `apps/web/src/pages/home/components/tool-call-detail-image.vue`
- Current owner primitives:
  - chat pane layout
  - message item
  - composer
  - message actions
  - attachment card
  - tool process row/group/detail components

## Summary

Chat should become its own spacing family. It is not a `PageShell`, even though it has gutters and a max width. It is a flow surface whose primary rhythm is turn-by-turn reading, composer alignment, and inline process expansion.

The most important relationship in this slice is that the message thread and the composer share the same content rail:

```txt
max-w-[840px] mx-auto px-4 sm:px-6 lg:px-10
```

That should be modeled as `ChatSurface` / `Composer` alignment, not as a generic page max width. Tool calls also need their own owner. They are dense process logs inside the chat flow; if we force them into card/list/settings roles, the token system will become muddy.

## Slice: Chat Thread Rail

- Archetype: vertically scrolling conversation stream.
- File read: `apps/web/src/pages/home/components/chat-pane.vue`.
- Current owner: chat pane layout.

### Relationships

| Relationship | Current implementation | Intent | Candidate role | Owner | Relationship confidence | Value confidence | Decision | Notes |
|---|---|---|---|---|---|---|---|---|
| outer horizontal viewport padding | `px-3 sm:px-5 lg:px-8` on the message area | keep scroll layer away from window edges | `chat.viewportGutterX` | chat pane layout | high | medium | adopt relationship | Distinct from the content rail gutter. |
| thread content rail | `max-w-[840px] mx-auto px-4 sm:px-6 lg:px-10` | align messages, greeting, and composer on one readable column | `chat.threadMaxWidth`, `chat.threadGutterX` | `ChatSurface` / chat pane | high | medium-high | adopt | Strongest chat relationship. Do not reuse `page.maxWidth`. |
| thread top padding | `pt-6` | give first message breathing room under top fade/header area | `chat.threadPaddingTop` | chat pane layout | high | medium | adopt relationship | Value can be tuned after visual review. |
| thread bottom padding | `pb-28` | preserve scroll room behind fixed composer | `chat.threadPaddingBottom` | chat pane + composer pair | high | medium | adopt relationship | This belongs to the chat/composer contract, not generic page padding. |
| turn gap | `space-y-6` between message wrappers | create readable message-turn rhythm | `chat.turnGap` | chat pane / message stream | high | medium-high | adopt | This is a first-contract candidate. |
| message highlight inset | wrapper `px-2 -mx-2 scroll-mt-2` | allow hover/highlight states without changing rail alignment | `chat.messageStateInset` | message stream | medium-high | medium | component-owned/defer | Relationship is real, but it is state geometry. |
| read-only empty height | `min-h-75` | center/hold read-only empty copy in the stream | empty/sparse local | read-only state | medium | low | defer | Height is state composition, not core spacing. |

## Slice: Composer

- Archetype: fixed input tray aligned to the chat rail.
- File read: `apps/web/src/pages/home/components/chat-pane.vue`.
- Current owner: composer markup inside chat pane.

### Relationships

| Relationship | Current implementation | Intent | Candidate role | Owner | Relationship confidence | Value confidence | Decision | Notes |
|---|---|---|---|---|---|---|---|---|
| composer rail alignment | same `max-w-[840px] mx-auto px-4 sm:px-6 lg:px-10` as thread | make input feel attached to the conversation column | `composer.threadAlignment` | `Composer` / `ChatSurface` | high | medium-high | adopt | Should not be retyped independently. |
| composer dock padding | normal state `pt-2 pb-8` | separate composer from last content and window bottom | `composer.dockPaddingTop`, `composer.dockPaddingBottom` | composer dock | high | medium | adopt relationship | Bottom value may vary with safe areas later. |
| welcome vertical offset | welcome state `pt-[38dvh]` | stage first-run prompt in upper-middle viewport | `composer.welcomeOffset` / sparse exception | welcome composer state | medium | low | exception/defer | This is expressive composition, not a first-contract spacing role. |
| welcome stack gap | `gap-10` between greeting and composer | separate hero-like greeting from input | `composer.welcomeGap` | welcome composer state | medium | low | exception/defer | Do not let this pollute normal chat rhythm. |
| pending task pill gap | `bottom-full mb-2` | attach background task pill above composer | `composer.statusToInputGap` | composer status layer | medium-high | medium | adopt relationship later | Needs one more status surface before first contract. |
| jump button gap | `bottom-full mb-4` | place scroll-to-bottom control above composer | `composer.jumpToInputGap` | composer action layer | medium | medium | component-owned | Similar but intentionally looser than task pill. |
| composer shell padding | `px-2.5 py-2.5` | compact internal edge around textarea/controls | `composer.paddingX`, `composer.paddingY` | `Composer` | high | medium-high | adopt | Owner is clear. |
| composer internal stack | `flex flex-col gap-1` | stack attachments, textarea, controls | `composer.stackGap` | `Composer` | high | medium | adopt | This is composer-owned, not form field gap. |
| attachment tray gap | attachments `flex flex-wrap gap-2 pb-1.5` | separate attached files/media from textarea | `composer.attachmentGap`, `composer.attachmentToInputGap` | composer attachment tray | high | medium | adopt relationship | Confirms attachment roles belong in chat/composer. |
| control row gap | controls row and clusters use `gap-2`; small button pills use `gap-1` | group model/project/send controls | `composer.controlGap`, `composer.controlIconTextGap` | composer controls | high | medium | adopt relationship | Distinct from toolbar action gaps. |
| model/project pill padding | `h-9 gap-1 rounded-full px-3` | compact command pills inside composer | control geometry | composer controls | medium | medium | component-local/defer | Height and radius are control geometry. |

## Slice: Message Item

- Archetype: one conversation turn, with sender metadata, body, attachments, and actions.
- Files read:
  - `apps/web/src/pages/home/components/message-item.vue`
  - `apps/web/src/pages/home/components/message-actions.vue`
  - `apps/web/src/pages/home/components/attachment-block.vue`
- Current owners: `MessageItem`, `MessageActions`, `AttachmentBlock`.

### Relationships

| Relationship | Current implementation | Intent | Candidate role | Owner | Relationship confidence | Value confidence | Decision | Notes |
|---|---|---|---|---|---|---|---|---|
| avatar to body gap | root `flex gap-3 items-start` | separate identity media from message content | `chat.messageAvatarGap` | `MessageItem` | high | medium | adopt relationship | Not a backend card media gap despite same value. |
| sender name to body | `mb-1` | bind sender label to message body | `chat.senderNameToBody` | `MessageItem` | high | medium | adopt relationship | Small but semantic. |
| user message block gap | user wrapper `flex flex-col gap-2` | stack bubble, attachments, actions | `chat.userBlockGap` | `MessageItem` | high | medium | adopt | Relationship is likely stable. |
| user bubble padding | `px-4 py-3` | readable compact text bubble | `chat.userBubblePaddingX`, `chat.userBubblePaddingY` | `MessageItem` | high | medium | adopt relationship | Current value is reasonable but should be reviewed in the component wall. |
| assistant block gap | `space-y-[0.85rem]` | stack assistant markdown, tools, attachments, errors | `chat.assistantBlockGap` | `MessageItem` | high | low-medium | adopt relationship, tune value | Off-scale value suggests visual tuning happened locally. |
| markdown paragraph rhythm | custom `p + p mt-2`, list `my-1.5`, headings `mt-5 mb-2` | make generated text readable | `chat.markdownRhythm.*` | markdown renderer/message prose | high | medium | adopt as component rhythm, not generic spacing | Should be documented separately from UI layout tokens. |
| error block padding/gap | `flex items-start gap-2 px-3 py-2` | show compact error inside assistant flow | `chat.errorBlockGap`, `chat.errorBlockPadding` | message error primitive | medium-high | medium | defer/adopt later | Needs comparison with global errors. |
| message actions offset | assistant actions `mt-2`; user actions `-mt-1`; action row `h-8 gap-0.5` | reveal actions close to message without changing turn rhythm | `chat.actionsTopGap`, action row local | `MessageActions` | high | medium-low | component-owned | Negative margin should remain component-owned. |
| attachment block gap | `flex flex-wrap gap-2` | separate displayed attachments inside a message | `chat.attachmentGap` | `AttachmentBlock` | high | medium | adopt | Same value as composer tray gap, but owner differs. |

## Slice: Composer Attachment Cards

- Archetype: fixed-size attachment thumbnails inside the composer.
- File read: `apps/web/src/pages/home/components/chat-attachment-card.vue`.
- Current owner: `ChatAttachmentCard`.

### Relationships

| Relationship | Current implementation | Intent | Candidate role | Owner | Relationship confidence | Value confidence | Decision | Notes |
|---|---|---|---|---|---|---|---|---|
| attachment card size | `size-30` | stable thumbnail/file preview footprint | attachment card geometry | `ChatAttachmentCard` | high | medium | component-local | Size, not spacing. |
| attachment card content padding | `p-2.5` | compact file/paste card body | `composer.attachmentCardPadding` | `ChatAttachmentCard` | medium-high | medium | component-owned/defer | Could be adopted if attachment cards recur outside composer. |
| attachment card text stack | file card `gap-0.5`; loading card `gap-2`; pasted card `gap-1` | tune each content state | attachment card local gaps | `ChatAttachmentCard` | medium | medium | local | State-specific values should not enter first contract. |
| remove affordance inset | `right-1 top-1 size-5` | place close button inside thumbnail corner | attachment card local geometry | `ChatAttachmentCard` | high | medium | local | Interaction geometry, not system spacing. |

## Slice: Tool Process Rows And Groups

- Archetype: dense expandable process log inside an assistant turn.
- Files read:
  - `tool-call-inline.vue`
  - `tool-call-group.vue`
  - `tool-call-cluster.vue`
  - `thinking-block.vue`
- Current owners: tool call inline/group/cluster and reasoning block.

### Relationships

| Relationship | Current implementation | Intent | Candidate role | Owner | Relationship confidence | Value confidence | Decision | Notes |
|---|---|---|---|---|---|---|---|---|
| process row icon/text gap | row `gap-1.5` | bind icon/status/name in a dense log line | `tool.rowGap` | `ToolProcessRow` | high | medium-high | adopt | This is the core tool-process rhythm. |
| process row vertical padding | `py-px`; cluster summary `py-0.5` | keep process rows readable but low-profile | `tool.rowPaddingY` | `ToolProcessRow` | high | medium | adopt relationship | Values may split root vs summary. |
| chevron gap | `ml-0.5` | keep disclosure icon attached to row label | `tool.disclosureGap` | `ToolProcessRow` | medium-high | medium | component-owned/defer | Too small for first contract unless a primitive exists. |
| group body top gap | group and thought detail `mt-1` | reveal nested process body close to trigger | `tool.groupBodyTopGap` | `ToolProcessGroup` | high | medium-high | adopt | Shared by thinking and tool groups. |
| group body padding | `px-2.5 py-1.5` in grouped collapsed body | compact nested process card body | `tool.groupBodyPaddingX`, `tool.groupBodyPaddingY` | `ToolProcessGroup` | high | medium | adopt relationship | Root detail cards use a looser padding. |
| group item gap | `space-y-0.5` in grouped collapsed body | stack compact process rows | `tool.groupItemGap` | `ToolProcessGroup` | high | medium | adopt | Distinct from chat turn gap. |
| cluster expanded gap | `mt-1 space-y-1.5` | separate multi-step open details | `tool.clusterBodyTopGap`, `tool.clusterItemGap` | `ToolProcessCluster` | high | medium | adopt relationship | Same values as detail stack but cluster owner differs. |
| approval action row | `mt-1.5 ml-5 flex items-center gap-2` | indent approve/reject actions under tool row | `tool.approvalActionTopGap`, `tool.approvalActionInsetX`, `tool.approvalActionGap` | tool approval row | high | medium | adopt relationship later | Also relevant to Tool Approval page. |

## Slice: Tool Details

- Archetype: result-specific detail blocks inside a process row.
- Files read:
  - `tool-call-detail-exec.vue`
  - `tool-call-detail-edit.vue`
  - `tool-call-detail-generic.vue`
  - `tool-call-detail-spawn.vue`
  - `tool-call-detail-browser.vue`
  - `tool-call-detail-output.vue`
  - `tool-call-detail-image.vue`
- Current owners: individual tool detail components.

### Relationships

| Relationship | Current implementation | Intent | Candidate role | Owner | Relationship confidence | Value confidence | Decision | Notes |
|---|---|---|---|---|---|---|---|---|
| root detail top gap | root detail `mt-1.5`; inline detail `mt-1` | separate row label from expanded content | `tool.detailTopGap.root`, `tool.detailTopGap.inline` | `ToolProcessDetail` | high | medium-high | adopt | Root and nested/grouped variants differ intentionally. |
| root detail padding | root card `px-3 py-2`; group card `px-2.5 py-2` | fit output text into small process cards | `tool.detailPaddingX`, `tool.detailPaddingY` | `ToolProcessDetail` | high | medium | adopt relationship | Should be owned by a detail wrapper, not repeated. |
| detail stack gap | many details use `space-y-1.5` | stack metadata, output, screenshots, results | `tool.detailStackGap` | `ToolProcessDetail` | high | medium-high | adopt | Stable across exec/edit/spawn/browser/image. |
| dense detail stack gap | generic detail `space-y-0.5`; browser metadata `gap-0.5` | compact key/value metadata | `tool.detailDenseStackGap` | `ToolDetailMetadata` | high | medium | adopt relationship | Good candidate inside detail primitive. |
| key/value gap | generic/browser rows `gap-1.5` | bind labels to values | `tool.detailKeyValueGap` | `ToolDetailMetadata` | high | medium | adopt | Tool-owned, not form label/control. |
| metadata wrap gap | exec metadata `gap-x-2 gap-y-0.5` | wrap compact runtime metadata | `tool.detailMetaGapX`, `tool.detailMetaGapY` | exec/detail metadata | medium-high | medium | component-owned/defer | Could be adopted after more tool metadata states. |
| code/output padding | edit pre `0.5rem 0.75rem`; browser preview `px-2 py-1`; output uses `CodeBlock` | readable dense code/output blocks | `tool.detailCodePadding` | code/output detail primitive | medium-high | medium-low | defer | CodeBlock may already own part of this. |
| output max heights | `max-h-72`, `max-h-96`, `max-h-48`, image `max-h-64` | bound long tool outputs | detail size limits | specific detail components | high | medium | local geometry | Size limits should not be spacing tokens. |

## Candidate Role Matrix Update

| Candidate role | Existing families touched | Chat/tool evidence | Relationship confidence | Value confidence | Decision | Owner |
|---|---|---|---|---|---|---|
| `chat.threadMaxWidth` / `chat.threadGutterX` | new chat family | message rail and composer rail share the same width/gutter | high | medium-high | adopt | `ChatSurface` |
| `chat.turnGap` | new chat family | message wrappers use a stable vertical rhythm | high | medium-high | adopt | chat message stream |
| `chat.messageAvatarGap` | new chat family | avatar-to-content relationship in message rows | high | medium | adopt relationship | `MessageItem` |
| `chat.userBubblePadding*` | new chat family | user bubbles have clear readable padding | high | medium | adopt relationship | `MessageItem` |
| `chat.assistantBlockGap` | new chat family | assistant content stack exists but uses an off-scale value | high | low-medium | adopt relationship, tune value | `MessageItem` |
| `chat.markdownRhythm.*` | markdown/prose | prose has its own paragraph/list/heading rhythm | high | medium | adopt as renderer rhythm | markdown renderer |
| `composer.padding*` | new composer family | composer shell has clear internal padding | high | medium-high | adopt | `Composer` |
| `composer.attachmentGap` / `composer.stackGap` | new composer family | attachments, textarea, controls stack inside composer | high | medium | adopt relationship | `Composer` |
| `tool.rowGap` / `tool.rowPaddingY` | new tool family | tool/thinking rows share dense process rhythm | high | medium-high | adopt | `ToolProcessRow` |
| `tool.groupBody*` | new tool family | grouped process body has compact nested card rhythm | high | medium | adopt relationship | `ToolProcessGroup` |
| `tool.detailTopGap` / `tool.detailPadding*` | new tool family | root/group/inline detail wrappers repeat these relationships | high | medium | adopt relationship | `ToolProcessDetail` |
| `tool.detailStackGap` / `tool.detailKeyValueGap` | new tool family | detail internals repeat compact stack and metadata row gaps | high | medium | adopt relationship | `ToolProcessDetail` / metadata primitive |
| attachment card state gaps | composer-local | thumbnail/file/paste cards have state-specific tuning | medium | medium | local/defer | `ChatAttachmentCard` |

## Chat/Tool Conclusions

1. Chat should be a separate first-class family. It has content rails, flow spacing, and composer alignment that should not inherit from page/settings primitives.
2. The thread/composer alignment contract is high confidence: one owner should define the rail width and responsive gutters.
3. Tool calls should be modeled as process rows and details, not as generic cards or list rows.
4. Tool detail spacing is real, but the first contract should adopt wrapper relationships first: row gap, group body gap/padding, detail top gap, detail padding, detail stack gap.
5. Markdown rhythm should be documented as prose rhythm. It should not be mixed with layout spacing tokens.
6. Composer welcome spacing is expressive sparse-state composition. Keep it exception-bound until onboarding/sparse pages are audited.
7. This slice increases semantic coverage materially because it adds the non-settings conversational product surface.

## Open Questions

- Should `ChatSurface` own both thread and composer rail tokens, or should `Composer` consume rail tokens from a parent context?
- Should `chat.markdownRhythm.*` live in spacing docs or in a separate prose/renderer contract?
- Should tool detail wrappers be extracted before values are tokenized, so individual detail files stop repeating `mt-1.5`, `px-3 py-2`, and `space-y-1.5`?
- Should approval action spacing be shared between inline tool approvals and the Tool Approval settings page, or are they different workflows?
- Should the off-scale assistant block gap `space-y-[0.85rem]` be normalized to a primitive rung after visual review?
