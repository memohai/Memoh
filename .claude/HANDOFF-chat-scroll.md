# Handoff:Chat 滚动重构(2026-07-08)

写给下一个实现者(零会话上下文可开工)。动代码前必读:本文件全文 +
`~/Documents/temp/grok-reverse/GROK-SCROLL-逆向.md`(参考实现的逆向文档,本文简称"参考实现")。

## 0. 一句话任务

**抄参考实现的滚动机制**(声明式 reserve、~110 行的简单结构),在它之上加少量
Memoh 自己点名的功能。不追求行为和参考实现完全一致——抄的是**实现方式**,
不是逐条行为。历史教训:这个功能已经历三轮重写,每轮都死在"机制自作聪明"上,
不是死在语义上。语义很简单,守住它,机制越笨越好。

## 1. 现场

- 仓库:Memoh,worktree `/Users/qqqqqf/Documents/Memoh/.claude/worktrees/chat-scroll-refactor`
  (分支 `worktree-chat-scroll-refactor`,基于 origin/main f43512e42)。
  **worktree 可能有他人在途改动,只碰下面列出的文件;所有 git 命令显式 `-C` 到 worktree。**
- 涉及文件(全部改动只应落在这四个):
  - `apps/web/src/pages/home/composables/useChatScroll.ts` — 滚动逻辑主体
  - `apps/web/src/pages/home/components/chat-pane.vue` — 模板结构、per-turn 容器、绑定
  - `apps/web/src/store/chat-list.ts` — 只为落"渲染身份修复"(见地雷 #1)
  - `apps/web/src/pages/home/components/message-item.vue` — action bar 已完成,**别动**
- 当前工作区 = "重写之前"的手工恢复态(用户认可手感,带已知闪烁病,见地雷 #1):
  composable 是命令式 reserve + 补偿层的版本,chat-pane 是 per-turn 容器版。
- **Checkpoint 提交 `dac70cb5c`**:上一轮(声明式 + id 键控)的完整实现 +
  store 身份修复 + 旧契约文件。当零件库用:里面的三个修复各自对应一个已确诊的
  bug(见地雷 #1/#2/#5),代码可直接摘;但整体只被用户测过一次,别整包 checkout。
- 历史版本快照:`~/.claude/file-history/977a8f9a-b154-4543-839a-d9f2b15e0e65/`,
  **编辑后**快照、按批次落盘。`e9a6c93563456830@v*` = useChatScroll(13 版),
  `2d1e8226a047d1ed@v*` = chat-pane(9 版),`32bcfa86f77c0c65@v*` = message-item,
  `33535e770fb4d8c3@v*` = 旧 PLAN 文档。任何中间态都能从这里精确找回。

## 2. 业务语义(提炼版 —— 最高优先级,实现不得违反)

### 发送(钉顶)
1. 发送后,新 prompt 以明显 ease-out 的动画(700ms 档)滚到**视口顶部下方 140px**
   处,上一轮的尾巴在上方露出一条可读的"露边";回复在 prompt 下方预留的空白
   (reserve)里流式生长,**视图停住不动**。
2. 会话的第一轮不钉顶(上面没有内容,钉了只会多出死滚动区)。
3. reserve 在钉顶时**测一次、设一次**,之后交给 CSS:流式消耗它、内容长过它就
   自然失效。**输出完成不回收**;只有下一次发送的交接(新入场稳定后旧空白才撤)
   或离开会话才回收。
4. 入场动画不许匀速、不许中途停死、不许零帧瞬移。

### 进入会话
5. 打开历史会话 = 与"刚发送完"**完全相同的几何、同一段计算代码**,但瞬时无动画。
   没有 user 消息的会话(system/subagent/空会话)落底。

### 跟随 / 逃逸
6. 底部跟随只在用户**到达内容末端**后武装;任何向上滚动立即解除;钉住停靠 =
   不跟随。没有"停顿后自动回锁"的定时器(做过,证伪:会把停靠视图拽回底部)。
7. "到底/还有内容"一律按**内容末端**(最后一条消息底边)计算,不按 scrollHeight
   —— reserve 空白不是内容。跳底按钮只在真有内容在下方时出现,跳的目标是内容末端。

### 其它
8. 上滚加载历史:可见内容纹丝不动。
9. 跨 tab(KeepAlive)恢复原位;离开时在底部的,回来重新走入场。reserve 是渲染
   状态,tab 切换后必须还在。
10. Action bar:最新 assistant 常显、历史 hover 显、流式中隐藏(message-item 已
    实现,勿动)。
11. **任何时刻(发送/流式/完成/刷新)消息不允许闪烁重挂。**

## 3. 抄参考实现的什么

逆向文档是权威,这里只列骨架和一个关键洞见:

- 消息列表按"最后一条 user 消息"切分;最后一轮的容器挂**声明式 min-height**
  ——是渲染的一部分,不是事后往 DOM 写的 inline style。
- reserve 是一屏量级的空白,流式内容长在空白**里面**:入场滚动飞行期间
  **scrollHeight 恒定**,这是参考实现的 spacer 不跳、入场不被打断的根本原因。
  我们历史上"原生 smooth 被 Chromium 掐断"的病,根源是 reserve 由 JS 事后测量
  再写、飞行中 scrollHeight 一直在变——自己捅的刀,别再捅。
- MutationObserver 当跟随心跳;30px 底部阈值。
- 它的 Δ 公式(-52 / 180 / 28 那些数)是对它自家 chrome 拟合的,**数字不抄**;
  我们用测量式:`R = clientHeight - below - 140 + promptOffsetInTurn`,
  下限 `promptOffsetInTurn + promptHeight + clientHeight/3`(代码在 checkpoint 里)。

## 4. 与参考实现的刻意差异(用户点名,必须保留)

| # | 我们 | 参考实现 | 为什么 |
|---|------|---------|--------|
| 1 | 进历史会话入场钉顶(同一段代码) | 直接落底 | 用户要"打开即上次发送后的样子" |
| 2 | 输出完成 reserve 不回收 | 生成一结束就撤 | 撤空白会让停靠视图跳变,用户明确否决 |
| 3 | JS 补间入场(quintic ease-out,700ms,`animateScrollTo` 已存在) | 原生 smooth | Chromium 原生曲线平直如匀速,用户嫌机械;且逐帧重读目标的补间天然免疫内容变动 |
| 4 | prepend 用原生 `overflow-anchor: auto` | anchor:none + 手动补偿 | 见地雷 #6:Memoh 是重 markdown,异步回流多,手动补偿追不上 |
| 5 | 每轮一个永久容器(key=开轮消息 id) | 只切最后一轮 | 见地雷 #7:重 markdown 重挂肉眼可见 |

## 5. 地雷录(每颗都炸过,修复代码都在 checkpoint `dac70cb5c`)

1. **store id 换血 = 闪烁的总根源。** `refreshCurrentSession → replaceMessages`
   整表 splice,乐观 id → 服务器 id 全换 → 所有 key 变 → 整列重挂 → 闪烁 +
   reserve 失锚。修复:`adoptRenderIdentity`(在 checkpoint 的 chat-list.ts 里):
   服务器 turn 继承屏上已有 id,服务器 id 挪进 `serverId`(全库服务器调用本就走
   `serverId ?? id`);匹配序:serverId → 乐观 user 逻辑匹配(externalMessageId /
   文本+时间窗)→ 已匹配 user 后紧邻的乐观 assistant 按相邻认领。
   **这是第一步,先落它再谈滚动。**
2. **发送 arm 时刻禁止改任何响应式渲染状态。** 乐观 push 在 `sendMessage` 内部
   几个 `await` 之后;arm 与 push 之间 Vue 必然 flush 一次,此时 turn 数组还是旧的
   ——按"位置"绑定的样式曾在这一拍把整屏空白错插到视口上方(钳位猛拽)。
   结论:reserve 绑定**按 turn id 键控**(Map),arm 时刻零改动,apply 时设新条目,
   settle 时剪旧条目。位置绑定(倒数第一/第二)永久禁止。
3. **reserve 必须声明式**(:style 来自状态)。命令式 inline style 会死于任何重挂,
   历史上为它长出三层补偿(`restorePinReserve` / `appliedPinContainer` / 延迟释放
   定时器),每层又生新 bug。当前工作区还带着这些补偿层——重构时整体删除,勿保留。
4. **"到底"和"武装跟随"是两个谓词。** 到底(跳底按钮隐藏)允许"停在 reserve
   空白里"算到底;武装跟随绝不允许——否则停靠时向下蹭一下触控板,每个流式 chunk
   都把视图往上拽(实测过的抽搐病)。武装容差 = 列底 padding,用几何算:
   `scrollHeight − 最后一轮容器底边`(不要 getComputedStyle:reka viewport 的
   firstElementChild 是内部 wrapper,padding 为 0)。
5. **`overflow-anchor` 保持 auto。** 试过 anchor:none + 手动 scrollTop 补偿:
   prepend 抽搐、入场偏移漂移——一次性补偿追不上 Shiki/KaTeX/图片的异步回流;
   原生 anchoring 持续纠正,且与逐帧补间共存无冲突。
6. **每轮永久容器,绝不重挂前轮 DOM。** 曾用"切两段"方案,发送时前轮被
   re-parent → 整轮重挂 → Shiki 异步重渲高度塌陷 → 硬跳。key = 开轮消息 id,
   发送只追加。
7. **MutationObserver 回调会强制布局**(读 getBoundingClientRect)。任何"中间坏
   DOM 状态"哪怕在 paint 前被修好,也会在这次强制布局里被钳位/anchoring 坐实成
   真实滚动伤害。每次 flush 的 DOM 都必须是合法状态。
8. 补间期间用 `isProgrammaticScroll` 括号 + wheel/touch 取消;补间无完成回调,
   用 `duration+100` 定时器收尾。跳底/跳消息/入场共用同一个 `animateScrollTo`。
9. 入场钉顶的兜底:无 user 消息的会话让 pin 挂起、跟随落底。

## 6. 推荐执行序(小步,每步用户验收)

1. **落地雷 #1 的 store 修复**:`git -C <worktree> checkout dac70cb5c -- apps/web/src/store/chat-list.ts`,
   用户验证"完成时闪烁"消失。这步独立有效,先拿下。
2. 在当前 composable 上做**结构替换**:删三层补偿,换成 id 键控声明式 reserve
   (代码蓝本在 checkpoint 的 useChatScroll.ts:`turnReserves` Map +
   `turnReserveStyle(turnId)` + `pruneReservesTo`;chat-pane 绑
   `:style="turnReserveStyle(turn.id)"`)。行为不变,只换机制。
3. 落谓词拆分(地雷 #4,蓝本同上:`atContentEnd` vs `isNearBottom`)。
4. 冻结结构。之后所有 bug 逐个指、逐个修,**不再重写**。
5. 每个用户点头的绿点,请求落一个 checkpoint commit(`--no-verify`)。

## 7. 验收清单(全绿才算完成;由用户在真实页面验证,不要自己开浏览器代验)

1. 停靠态直接发送:无闪、无拽,700ms ease 入场,prompt 落 140px,露边可见
2. 流式中视图纹丝不动;向下蹭触控板不会触发逐 chunk 上拽
3. 输出完成:无闪烁、spacing 不掉、不出现假的跳底按钮
4. 滚到内容末端 → 跟随武装贴底;向上滚立即逃逸;停在空白深处不武装
5. 进历史会话 = 发送后同几何,瞬时;无 user turn 的会话落底
6. 上滚加载历史:可见内容不动
7. 跨 tab 切换恢复原位;split pane 各 pane 独立
8. 首轮发送不钉顶(落底跟随)
9. Action bar:最新常显 / 历史 hover / 流式隐藏

## 8. 工程规约(违者返工)

- 回复用户用中文;代码注释**不得出现外部产品名**(用"参考实现"指代)。
- `vue-tsc` 必须 `-p tsconfig.app.json`(根 tsconfig 是 `files: []` 空跑)。
  项目有**预存** tsc 错(chat-list.ts 16 个等):动手前先在 HEAD 上记基线,
  只对"新增"负责,别顺手修不属于你的。
- 改动前跑 `eslint` 三件套文件;UI 改动读 `.agents/skills/memoh-web/SKILL.md`。
- 用户说 commit 才 commit,一律 `--no-verify`;git 全部 `-C` 到 worktree。
- "完成"的唯一定义 = 用户在页面上验证通过。
