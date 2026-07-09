# Chat 滚动重构 + spacer 钉顶 —— 实施计划

## 业务语义契约(2026-07-07 与用户确立,优先级最高,实现不得违反)

**A. 发送(pin)**
1. 发送 → 平滑动画(JS 补间,700ms,ease-out)把新 prompt 送到视口顶下 `PIN_TOP_OFFSET_PX`(当前 140);上一条消息露出 sliver
2. reserve 一次性设定,回复流式消耗它;内容超过后自然失效
3. reserve 只在两个时点让位:下一次 pin 交接、离开会话。输出完成 / DOM 重挂 / 切 tab 都不得使其消失或跳变
3b. **交接时机:旧 reserve 在入场动画 settle 之后才释放**——动画开始时清除会让旧空白(用户可能正停在里面)瞬间塌掉,新 prompt 零帧上浮,动画报废;释放时空白已在钉住的 prompt 上方,由原生 anchoring 保持画面不动
4. 钉住 = parked:流式不跟随、tool group 开合不动画面
5. **首轮不钉**(2026-07-08 对齐参考实现):上方无任何内容时发送不撑 reserve,内容本来就在顶部

**B. 进入会话(entry)**
6. 进会话落点 = 与发送后同一段计算代码(pinTarget),只是瞬时无动画
7. 无 user 消息的会话 → 落底
8. tab 切回:恢复离开位置;离开时在底部则重走 entry

**C. 在底 / 跟随 / 逃逸**
9. "在底部" = 最后一条消息末尾在视口内(30px 容差),与 reserve 空白无关
10. 跳底按钮只在"下方还有内容"时出现;点击滚到内容末尾,不是 scrollHeight
11. follow 只在用户主动滚到底(按 9)后武装;上滚立即解除;pin 与 follow 互斥
12. 没有"停顿自动锁回"定时器

**D. 历史加载** 上滚 prepend:可见内容不动(原生 anchoring)、算逃逸
**E. 跳转** 引用/rail:补间 + 高亮 + parked
**F. Action bar** 最新 assistant 常显;历史 hover 显;streaming 隐藏

结构决定(与语义同级):每轮一个永久容器(key=开轮消息 id),发送只追加不迁移;**reserve 是声明式渲染状态**(参考实现同款)——一个 **按 turn id 键控的 Map**,模板对每个 turn 容器绑 `turnReserveStyle(turn.id)` 的 :style min-height,重挂天然携带样式,"无 reserve 的布局"不可能被渲染出来;交接=**apply 时给新 turn 设条目、settle 时剪掉其余条目,arm 时零改动**(乐观 push 落在 sendMessage 若干 await 之后,arm 时改任何渲染状态都会早一拍 flush 到错误的 turn 上——按"位置"绑定曾因此把整屏空白错插到视口上方,已废弃,禁止回退)。**渲染 id 必须稳定**:store 的 replace/merge 前跑 adoptRenderIdentity,服务器 turn 继承屏上已有的 id(服务器 id 留在 serverId),乐观→服务器合并零重挂。**follow 武装 ≠ 到底判断**:按钮用"下方是否还有内容"(reserve 空白算到底),武装只认"到达内容末端"(超出内容末端的容差=列底 padding,不含 reserve 空白——否则停靠空白里蹭一下触控板就武装 follow,每个 chunk 把视图往上拽)。禁止倒退回"命令式往 DOM 元素写 inline style"的实现。

## 背景与决策回顾

- 调研结论:Memoh 当前滚动逻辑(`chat-pane.vue` 2542 行)核心能力齐(贴底/解锁/prepend 保位/跳转/跨 tab 恢复),但焊死在巨石组件里,和 `message-item.vue` 深度耦合(它要 emit `{id, top}` 参与滚动状态机),且缺"新消息钉顶 + spacer"。
- 参考了两套线上实现:
  - **shadcn `MessageScroller`**(anchor 定位派):短内容不够钉顶时退化贴底。
  - **Grok**(逆向,`~/Documents/temp/grok-reverse/GROK-SCROLL-逆向.md`):自研,用 `min-height: calc(100vh + Δ)` 强撑最后一轮的容器,不管长短都钉顶;`Δ = -52 + max(0, turnHeight-180) - 28×窄屏`;在底阈值 30px;流式跟随用 MutationObserver;prepend 用手动补偿(非 CSS anchor)。
- **用户已拍板**:钉顶策略选 **Grok 式 —— 强撑,短回复也钉顶**。
- **Grok 的具体数字不能照抄**(是它自己 UI 尺寸拟合出来的),但**结构**可以抄,而且可以比它更简单:Memoh 可以直接用原生 CSS `min-height: calc(<viewportPx> - <chromePx>)` 让浏览器自己处理"内容不够撑、内容够了就不撑"的逻辑,不需要 Grok 那个 `max(0, turnHeight-180)` 的手动衰减项。

## 本次改动范围

**只做滚动这一层**,不碰消息渲染视觉、不碰 composer、不碰其他已存在的 motion。分四个阶段,建议**分别提交**,阶段一先落地再评审,阶段三风险最高单独看:

1. 抽 `useChatScroll` composable(纯重构,行为不变)
2. 显式化"在底部"判定阈值
3. **Grok 式 spacer 钉顶**(新 feature,本次核心)
4. 跳转 / 开局定位收口(现有能力,只是搬进新边界)

不在本次范围内(会另外确认再做):
- `chat-minimap.vue`(390 行死代码,只被自己的测试引用,无业务代码挂载它)+ 其配套测试文件的删除
- `chat-pane.vue` 里手写的右侧 rail 和 `chat-minimap.ts` 剩余导出(`buildMinimapAnchors`/`activeAnchorIndex`/`tickWidth` 等)的重复消解
- `message-item.vue` 的 `@active` 契约命名优化(仍需保留其数据,但会挪到 composable 里接住)

---

## 阶段一:抽取 `useChatScroll` composable(行为不变的纯重构)

新文件 `apps/web/src/pages/home/composables/useChatScroll.ts` + `useChatScroll.test.ts`(项目已有 `useSubagentList.ts`/`useMediaGallery.ts` 同款目录,遵循 `chat-minimap.ts` 的 DI 可测风格——`animateScrollTo` 的 `now/raf/caf` 注入模式直接复用)。

**从 `chat-pane.vue` 搬进来的状态和函数**(现状态位置见 `chat-pane.vue:2091-2478`):
- `isAutoScroll` / `isInstant` / `lockScroll` / `isInit` 四个标志位 + 驱动它们的两个 `watch`/`watchEffect`(2362-2385 贴底驱动、2228-2241 更名为的 escape/relock)
- `scrollToBottom` / `scrollViewportTo` / `startScrollTween`(2213-2253,`animateScrollTo` 从 `chat-minimap.ts` 移入,原文件里其余仅服务于死代码的导出暂不动)
- `scrollToMessage` + 高亮计时器(2446-2466)
- `ensureOlderLoaded` 的 prepend 抑制逻辑(2408-2438,保留其详尽注释——这段是全文件最成熟的部分,注释解释了为什么不手动补偿)
- `onActivated`/`onDeactivated` 跨 tab 滚动位置恢复(2260-2360),含 `elId` Map 和 `isActiveEl`

**composable 对外接口(草案)**:
```ts
export function useChatScroll(options: {
  scrollEl: Ref<HTMLElement | null>
  contentEl: Ref<HTMLElement | null>       // 原 descEl,内容高度探针
  messages: Ref<ChatMessage[]>
  isActive: Ref<boolean>
  sessionId: Ref<string>                    // 跨会话清空 elId 用
}) {
  return {
    isAutoScroll, isScrolling, isAtBottom,   // 状态
    scrollToBottom, scrollToMessage,          // 主动滚动
    highlightedMessageId,                     // 跳转高亮
    onMessageActive,                          // 替代 isActiveEl,message-item 的 @active 接进来
    ensureOlderLoaded,                        // prepend 入口(实际 fetch 仍由调用方 store 做)
    // 阶段三新增:
    pinLastTurn, lastTurnPinStyle,
  }
}
```

**message-item.vue 的耦合怎么处理**:不改它现在往外 emit `{id, top}` 的行为(改数据契约是另一件事,不在本次范围),但接收方从 `chat-pane.vue` 里的裸函数 `isActiveEl` 换成 composable 暴露的 `onMessageActive`——`chat-pane.vue` 该行 `@active="isActiveEl"` 改成 `@active="onMessageActive"`,一行改动,契约不变。

**验收**:这一阶段结束后,`chat-pane.vue` 应该只剩"调用 composable + 渲染"两件事,行为**必须与改动前逐项一致**(贴底、解锁、prepend、跳转、跨 tab 恢复、rail 联动全部照旧)——这是 skill 里"重构不能回归"的硬要求,我会在实现后过一遍现有交互清单自查,但**最终验证仍需你在跑起来的页面里过一遍**(skill 规则:人验证前不算完成)。

---

## 阶段二:显式化"在底部"阈值

现状:`useScroll(scrollEl, {...})` 没传 `offset`(`chat-pane.vue:2106`),`arrivedState.bottom` 用的是 @vueuse 默认值 **0px**(严格贴底才算数)。这比 Grok 的 30px 更严格,可能是"明明看着到底了,`isAtBottom` 却是 false"这类边缘抖动的来源。

改动:composable 里给 `useScroll` 传 `offset: { bottom: BOTTOM_THRESHOLD_PX }`,常量定义 `const BOTTOM_THRESHOLD_PX = 30`(命名清楚来源:参考 Grok 实测阈值),替代原来隐式的 0。这是唯一从 Grok 直接搬的数字——因为它是一个**行为容差**,不依赖 Grok 自己的布局尺寸,通用性高。

---

## 阶段三:Grok 式 spacer 钉顶(本次核心 feature)

### 设计:比 Grok 更简单,靠原生 CSS 而不是手动衰减公式

Grok 用 JS 算 `Δ = -52 + max(0, turnHeight-180) - 28×窄屏` 再拼进 `min-height: calc(100vh + Δ)`,是因为它需要精确控制。但 CSS 的 `min-height` 本身就有"内容不够就撑到这个高度,内容够了就跟内容走"的语义——**这正是我们要的行为**,不需要 Grok 那个随内容高度手动衰减的项。

Memoh 版本:
```
.last-turn-tail {
  min-height: calc(var(--scroll-viewport-h) - var(--chrome-px));
}
```
- `--scroll-viewport-h`:`useElementBounding(scrollEl).height`(已有现成 composable,`chat-pane.vue` 已经在其他地方用它)
- `--chrome-px`:一个需要**在 Memoh 实际布局里测量**的常量(composer 高度 + 顶部渐变遮罩高度 + 一点视觉余量),**不是 Grok 的 -52/180/-28**——这几个数字我会在实现时用浏览器实测 Memoh 自己的 composer/gutter 尺寸重新定,不会照搬 Grok 的值。

### 应用范围:只包住"最后一轮"

`messages` 是扁平的 turn 数组(user/assistant/system 混排,`chat-list.ts:132-209`),不是预先分组的 request/response 对。做法:找到**最后一条 role === 'user' 的 turn 在数组里的下标**,把从它开始到数组末尾的这段(`messages.slice(lastUserIndex)`)包一层容器 div,只对这个容器应用上面的动态 `min-height`。之前的历史消息不受影响,渲染结构改动很小。

### 触发时机与和贴底逻辑的关系(重要,和现状是反的)

现在 `handleKeydown`(`chat-pane.vue:2499-2504`)发送时是 `isAutoScroll.value = true`(重新贴底)。**钉顶和贴底是互斥的两件事**——如果发送后既钉顶又贴底,贴底会立刻把画面拽回底部,钉顶就白做了。

新流程:
1. 用户发送 → **不**设 `isAutoScroll = true`,改为调用 `pinLastTurn(newUserTurnId)`
2. `pinLastTurn` 内部:关闭 `isAutoScroll`(和 prepend 时 `ensureOlderLoaded` 的思路一致——防止贴底 watch 把画面拽走)→ 给最后一轮容器套上动态 `min-height` → `scrollToMessage(newUserTurnId, { block: 'start' })` 把它滚到视口顶部
3. 助手回复流式增长期间:`min-height` 持续跟着 `--scroll-viewport-h` 变(内容如果自然长过这个高度,`min-height` 天然失效,不需要额外代码)
4. 回复流式结束(`streaming` 变 false)后:**恢复正常贴底行为**——用户如果继续往下发消息,还是走"贴底"而不是"钉顶又钉顶";只有*每一次新发的 user turn*都各自触发一次钉顶(钉的是它自己,不是永久锁定)
5. 用户在这期间手动上滚:走现有的 escape 逻辑(阶段一保留),钉顶的 `min-height` 不受影响(它是布局占位,不是滚动行为本身)

### 需要在实现前用真实浏览器量的东西(不是本计划能纸面定的)

- Memoh chat 区的 composer 实际高度(有 pill/多行两种状态,`composerHeight` 已经是个现成的响应式值,可以直接引用)
- 顶部渐变遮罩 `h-10`(`chat-pane.vue:22`)要不要算进 chrome 偏移
- 窄屏(dockview 面板变窄时)要不要像 Grok 一样有单独修正——**Memoh 的窄屏是容器查询语境(pane 变窄,不是 viewport 变窄)**,这点和 Grok(纯 viewport 断点)不同,需要按 memoh-web skill 的"容器查询而非 viewport 断点"规则来对,不能照抄 Grok 的窗口宽度判断

这几点我会在写代码时用浏览器实测校准(不是纸面猜数字),校准过程你可以在做完后跑起来一起看效果、一起调数字。

---

## 阶段四:跳转 / 开局定位(收口,非新写)

`scrollToMessage`(阶段一已抽入 composable)已经能做"跳转到某条 prompt"。`handleReplyJump` 复用它,不用动。

"第一次进入页面定位到哪"——现状 `onActivated` 的跨 tab 恢复(阶段一已抽入)已经处理了"重新激活这个 tab 恢复到之前位置"这件事;**全新打开一个从未渲染过的会话**目前隐式走"贴底"(`isAutoScroll` 默认 `true`)。是否要仿 shadcn 的 `defaultScrollPosition="last-anchor"`(打开定位到最后一个钉顶锚点而不是纯贴底)——这是个小决策,阶段三落地后我会问你要不要顺手加,不在本计划强制。

---

## 风险与验证

- 阶段一是纯重构,回归风险靠"逐项行为清单自查",但**最终仍需你在跑起来的页面里验证**(memoh-web skill 规则:人验证前不算完成,我不会自己开浏览器截图替代)。
- 阶段三是新行为,里面的具体像素数字(chrome 偏移)第一版大概率需要跟你来回调 1-2 轮才能贴合视觉。
- 不涉及数据库/API/其他包,改动面限定在 `apps/web/src/pages/home/components/{chat-pane,message-item}.vue` + 新增 `composables/useChatScroll.ts`。

## 执行顺序建议

先做阶段一(纯重构,可独立提交、可独立验证),验证过关后再做阶段二+三(合并成一次改动,因为阶段三依赖阶段二的显式阈值),阶段四留到你看完阶段三效果后再定要不要加。
