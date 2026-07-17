# ChatView 同步层重构

基线：Fodesu stack 顶层 PR #802，commit `68974f413c418e9125dbc2de704a8ac6668c0744`。
范围：主体是 `apps/web` 前端数据链路（渲染层、滚动层不动）；但 §10/§11 触及一个前后端共用的数据契约（稳定身份 + 顺序号），那部分是前后端一体的，见 §0 与 §10。

## 0. 愿景（先读这一节：要什么、为什么，不含实现）

> 这一节只讲**理念**——不含代码位置、不含迁移步骤。它是全文的总纲，§1 起的所有章节都是它的展开。任何后续实现若与本节冲突，是实现错了，不是愿景错了；若本节被证伪，才是愿景要改。**先分清这两件事，再往下读。**

### 为什么做这件事

一批前端 bug（回答生成完跳到后面、点停止历史全没、刷新/换设备看不到生成中内容、生成完抖一下）看似无关，病根只有一个：**前端"这个会话有哪些消息"这份数据，有好几拨代码在各自往里写，谁后到谁覆盖。** 不逐个修症状，而是把结构改对，让这一类 bug 失去发生条件。

### 这件事的本质：一个前 AI 时代就存在的经典问题

**本地有一份缓存，远端有权威真相；本地必须用（否则没有即时上屏、没有离线、每次白屏等网络），又不能完全信任（远端可能被改写、可能正在变化、可能你手里是旧的）；而对齐的资源有限（不能每次全量拉、不能每条都往返确认）。** 这是分布式缓存、离线优先、版本控制、数据库复制反复解的同一道题。把它当经典问题看，而不是当"运行时特性"的边角修补，是整套设计的出发点。

### 五条愿景（本设计的全部理念，缺一不可）

1. **两个权威，一条移动的边界。** 云端不是一份真相，是两个：已完成的消息以 **Postgres 历史**为权威，正在生成的那一轮以 **runtime 运行态（内存/Redis）**为权威。一轮生成完，权威从运行态**原子移交**给历史。前端要做的，是在"正在生成的那一轮"这条缝上，把两个权威缝成一条连续的时间线。

2. **"运行中"不是布尔，是一条边。** `运行中 → 已完成` 的那个瞬间，是一轮消息的所有权移交时刻——所有 bug 都长在这条边上。把它当"布尔翻转"处理，就是病根；把它当"所有权移交"处理，才对。

3. **一条序，从前到后不变（降熵）。** 一条消息从它在运行态诞生的那一刻起，就带着**稳定的身份**和**固定的顺序号**；这两样一路不变地流到 Postgres。运行态里的它 = 落库后的它，只差一个"是否已落库"。**不在中途转换、不在落库时重排或重新编号**——数据从前到后流的是同一条序。

4. **身份与顺序是两把正交的钥匙，缺一不可。** **身份**（这条是谁、跨"运行中→已落库"是否同一条）解决查重与缝合；**顺序号**（它在时间线的位置、有没有漏）解决排序与缺口检测。只有身份会乱序、会漏；只有顺序认不出跨源的同一条。**必须两者并用**——绝不靠文本相似度猜、绝不靠"最后一条"的位置猜。

5. **前端只有一个写入者、只是一个读端。** 前端不是第二个真相源：它从两个权威**拉取**数据、在本地按确定规则**投影**成屏幕上的样子，此外不生成、不重排、不回写。"这个会话有哪些消息"这份数据，只能有一个入口去写。

### 一句话

**把"会话消息"建成一条从运行态到 Postgres 全程不变的序（稳定身份 + 顺序号），让前端退化为这条序的唯一读端与唯一写入者；"运行中"作为一条所有权移交的边被显式处理，而不是被当作一个布尔。**


## 1. 这次要解决什么

先说立场：**我们不是照着 bug 清单去修 bug，而是拆解这些 bug 本质上共同暴露了什么问题。** 逐个修症状，只会在错误的结构上叠更多补丁；把结构改对，这一类 bug 失去发生条件。这个立场对之后的工作同样成立——遇到成片的 bug，先问它们指向哪个结构性病根。

用户能感知到的四个症状：

- 刚发的问题，回答生成完之后跳到了回答后面
- 点"停止"，整段对话历史直接没了
- 刷新页面 / 换设备，看不到正在生成中的内容
- 回答生成完的那一瞬间，窗口上下抖一下

病根只有一个：**前端"这个会话有哪些消息"这份数据，有四拨代码在各自往里写**——

1. 发送时本地先塞一条占位消息（为了 0ms 上屏）
2. 旧的流式通道代码还在写（服务端启用运行时后已不再发这些事件，死路活代码）
3. 新的运行时推送在写（#798 引入）
4. 生成完之后再拉一次服务器历史，用"文字相同 + 5 秒内"**猜**哪条对应哪条

四个写入者互相不知道对方，谁后到谁覆盖。上面每个症状都是这件事的不同切面。

改造前后对比：

```
现状：四拨人写一本账                     目标：一个入口
                                    
 发送占位 ──┐                          发送占位 ──┐
 旧流事件 ──┤                          运行时推送 ─┤
 运行时推送 ─┼──> 消息列表                终态对账 ──┼──> sync/ ──> 消息列表
 REST刷新 ──┘   (谁后到谁覆盖)            (占位靠id认领) ┘  (唯一写入者)
                                    
 对号方式：文字相同+5秒内 (猜)            对号方式：stream_id+generation (精确)
```

**这次改动只做一件事：把"写消息列表"收成一个入口。** 生成中听运行时推送；生成完对一次服务器历史；本地占位消息用发送时的 id 精确对上号，不再靠猜。四拨写入者变一个，时序问题从"修不完"变成"没有发生条件"。

理想分层（本次的靶子是"同步层"这一格）：

```
服务端真身 ──────────── 唯一真相
    │
运行时中间态(#798) ──── 生成中的内容也有账可查（已建好）
    │
┌───┴──────────────────────────────┐
│ 传输层   只管收发，不懂消息          │
│ 同步层   唯一写入者 ← 本次新建       │
│ 数据层   纯容器，只被同步层写         │
│ 渲染层   照数据画（不动）            │
│ 滚动层   只加空间/改高度（不动）       │
└──────────────────────────────────┘
```

三条规则：

1. **顺序只看服务端给的编号**，客户端永远不猜先后
2. **写消息列表只有一个入口**；占位消息用 id 精确撤除
3. **内容等价 ⇒ 画面恒等**：流中→流完、未落库→落库、小块并大块，文字没变，渲染层不许感知

## 2. 当前代码给出的约束（PR #802 基线事实）

改法必须建立在真实合同上，不是设想上。以下是探查 PR #802 对应 commit `68974f413c418e9125dbc2de704a8ac6668c0744` 确认的事实：

### 2.1 服务端 wire 合同（可依赖）

| 事实 | 位置 |
|---|---|
| bot 级 WS `runtime_subscribe {session_id}` → 先发全量 `runtime_snapshot`（带 epoch/seq），后续 `runtime_delta` | `internal/handlers/local_channel.go` |
| **没有 after_seq 续传**：重连一律重新订阅、拿全量快照 | 同上 |
| delta 四类：自含的全量 CurrentRunView（可跳 seq）/ Run patch / MessageAppends+ProgressAppends / MessageUpserts+ResetMessages | `internal/sessionruntime/types.go` |
| 非自含 delta 严格 seq+1，断号即作废重订阅；`runtime_dropped` → 重订阅 | 同上 |
| 每次 run 有唯一 generation（uuid）；stream_id 可复用；abort/steer 必须带 generation | `internal/sessionruntime/commands.go` |
| REST 补水：`GET /bots/{id}/sessions/{sid}/runtime` | `internal/handlers/local_channel.go` |
| 生成中的用户消息 + 助手输出**只存在于运行时快照**，落库是终态时刻的一次原子动作（fenced PersistRound） | `internal/conversation/flow/resolver_stream.go:483-498` |
| 可见输出前中断 → 整轮（含用户消息）不落库 | `resolver_stream.go:608-615` |
| 运行时启用后服务端**不再发** legacy WS start/message/end/error | `local_channel.go:1167-1169`；运行时恒开 `cmd/agent/app.go:200-206` |

### 2.2 可继承的资产（PR #802 基线已具备、直接搬用）

| 资产 | 位置 | 说明 |
|---|---|---|
| 纯 reducer（applied/ignored/resync 三态，7 种 resync 原因） | `packages/sdk/session-runtime/index.ts`（446 行） | Go 生成的合同 fixtures 在 import 时回放校验，不要重写 |
| 快照→消息列表投影 + 占位消息按 stream_id+generation 认领 | `apps/web/src/store/chat-list.ts` `projectRuntimeSnapshot`（由 #798 引入，PR #802 基线仍在） | 认领逻辑正确，但目前只是"第五个写入者"，要提升为唯一入口的一部分 |
| 运行时测试 ~334 个 + 合同 fixtures | `packages/sdk/session-runtime/` | 迁移基线 |

### 2.3 病灶清单（本次要切除的）

| 病灶 | 位置 |
|---|---|
| legacy WS 流事件消费分支（死路活代码） | `apps/web/src/store/chat/assistant-streams.ts`、`realtime.ts` 相关分支 |
| "文字相同 + 5 秒内"猜号 | `chat-list.normalize.ts` `isSameLogicalTurn`（:140-157）及其 REST 刷新调用方 |
| transcript 的 10 个散落写入方、11 个 fetch 触发点 | `chat-list.ts`（main 2913 行 / #798 约 4500 行）、`chat/transcript.ts` |
| 渲染/滚动层为上游抖动做的赔偿（上游稳定后拆） | `useChatScroll.ts` layout-heal :994-1024、identity 迁移 :1057-1072 |

## 3. 目标结构

先看全景——整个 chat 方向的终态，以及每块的归宿。本次施工范围只是其中 sync/ 一格，但每一块的去向现在就定下来，避免"重构 chat = 只动 sync"的误读：

```
apps/web/src/ 的 chat 方向终态

 ┌─ 传输 ──────────────────────────────────────────┐
 │ WS 客户端 / REST(@memohai/sdk)     [保留，不懂消息] │
 └───────────────┬─────────────────────────────────┘
 ┌─ 同步 ────────▼────────────────── ★ 本次新建 ─────┐
 │ store/chat/sync/                                │
 │ (subscription/runtime-session/projection/       │
 │  settle/intents)                 [唯一写入者]     │
 └───────────────┬─────────────────────────────────┘
 ┌─ 数据 ────────▼──────────────────────────────────┐
 │ transcript.ts      [本次降级为纯容器]              │
 │ assistant-streams / realtime 消费分支 [本次删除]   │
 │ refresh-coordinator [本次删除，settle 取代]        │
 │ view-registry      [瘦身：只管视图/草稿，不再碰写入] │
 └───────────────┬─────────────────────────────────┘
 ┌─ 渲染 ────────▼──────────────────────────────────┐
 │ chat-pane.vue 等    [本次不动；god-file 拆分另一条线]│
 └───────────────┬─────────────────────────────────┘
 ┌─ 滚动 ────────▼──────────────────────────────────┐
 │ useChatScroll      [本次不动；步骤5拆两处赔偿代码]   │
 └──────────────────────────────────────────────────┘
```

本次的靶子：新建 `apps/web/src/store/chat/sync/`，它是消息列表的**唯一写入者**：

```
apps/web/src/store/chat/
├── sync/
│   ├── subscription.ts     # 订阅生命周期：runtime_subscribe / 重订阅 / runtime_dropped
│   ├── runtime-session.ts  # 每会话 reducer 实例持有（复用 @memohai/sdk session-runtime）
│   ├── projection.ts       # 快照 → 消息列表投影；占位按 stream_id+generation 认领
│   ├── settle.ts           # 终态对账：run 结束后拉一次 REST 历史，按 id 替换，不猜
│   └── intents.ts          # 用户意图出口：send / abort(带 generation) / steer
├── transcript.ts           # 降级为纯容器：只暴露给 sync/ 的写接口 + 给渲染的读接口
└── （view-registry / refresh-coordinator 等按迁移步骤逐步瘦身或删除）
```

一条消息的完整旅程（运行期数据流）：

```
用户敲回车
    │
 intents.send ──── 塞占位消息(带 stream_id) ────────────┐
    │                                                 ▼
    │ POST /chat                                  transcript ◄─── 渲染层只读
    ▼                                                 ▲
 服务端开始 run (generation=g1)                         │
    │                                                 │
 WS: runtime_snapshot / runtime_delta                  │
    ▼                                                 │
 subscription ──> runtime-session ──> projection ──────┤
   (订阅/重订阅)    (reducer,纯)      (投影+按 stream_id  │
                                     +generation 认领占位│
                                     ,id 不变)          │
    │                                                 │
 run 到终态 (completed/aborted/errored)                 │
    ▼                                                 │
 settle ── GET 持久化历史 ── 按 id 替换 ──────────────────┘
           (aborted×vanished → 用户输入回填草稿框)
```

职责一句话版，附取舍记录（为什么是这样、什么被否掉了——设计 = 被否掉的选项集合）：

- **subscription**：什么时候听、断了怎么重听。不懂消息内容。
  - *为什么重连是全量重订阅而不是增量续传*：不是我们选的，是服务端合同没有 after_seq。后来者不要好心在客户端补一个续传缓存——服务端不支持，缓存只会制造第二份真相。
- **runtime-session**：把 wire 事件喂给 reducer，产出快照。不碰 store。
  - *为什么 reducer 必须纯、不碰 store*：Go 生成的合同 fixtures 在 import 时回放校验，掺进任何副作用回放就废了。这是双端合同不漂移的机械保证，比 code review 可靠。
- **projection**：快照怎么变成屏幕上那列消息。占位消息在这里被精确认领。
  - *为什么认领在 projection 而不在 intents*：认领的触发条件是"快照到达"，不是"发送动作"。放 intents 就得让发送方等回执，重新引入时序耦合。
  - *为什么认领后 id 不变*：id 变 = key 变 = 重挂载 = 抖动（规则 3）。被否掉的方案是"认领时换成服务端 id"——服务端 id 在 settle 阶段才需要，那时按 id 替换整条即可。
- **settle**：生成完之后，和服务器历史对一次账。id 对 id，对不上就以服务器为准。
  - *为什么是终态一次对账、不是持续 diff*：run 中的真相在运行时快照里，对着一份还没写完的账持续 diff，等于重新引入第二写入者——旧代码的病根就是这个。
- **intents**：用户的动作从这里出去。停止必须带 generation，停错代际的 run 是 bug。
  - *为什么 abort 必须带 generation*：stream_id 可复用，只带 stream_id 可能停掉重试后的新 run。服务端合同要求带，前端没有豁免。

删除清单（连同理由）：

- legacy WS 消费分支 —— 服务端已不发（`local_channel.go:1167-1169`）
- `isSameLogicalTurn` 文本+5s 启发式 —— stream_id+generation 提供精确身份
- transcript 上除 sync/ 之外的所有写入方 —— 违反规则 2

## 4. 依赖规则

```
渲染/滚动 ──读──> transcript <──写── sync/ ──> 传输(WS/REST)
```

- 渲染层、滚动层、任何组件：**只读** transcript，禁止 import `sync/` 内部模块
- `sync/` 之外禁止 import 会写 transcript 的接口
- 用 ESLint `no-restricted-imports` 落成 CI 守卫，随第一步迁移一起加

## 5. 状态契约：一条消息的三个维度

大白话：同样是"停止了"，得分清是"停了但保住了半截回答"还是"停了且整轮都没了"——这是两种不同的画面，以前混在一起就是"点停止历史全没"的来源。

每条消息的状态 = Run 状态 × 呈现来源 × 落库状态，三维独立：

| 维度 | 取值 | 谁决定 |
|---|---|---|
| Run | admitting / running / aborting / completed / aborted / errored / lost | 服务端运行时 |
| 呈现来源 Presence | optimistic（本地占位）/ live（运行时投影）/ settled（REST 对账后） | sync/projection + settle |
| 落库 Persistence | unknown（run 中）/ persisted / vanished（整轮未落库） | settle 对账结果 |

关键组合：

- `aborted × persisted`：停止但保住了已生成内容 → 正常显示，标记中断
- `aborted × vanished`：可见输出前停止，整轮没落库 → 消息移除，**用户输入回填到草稿框**（不是无声消失）
- `optimistic → live`：占位被 stream_id+generation 认领，**id 不变**（规则 3：渲染不感知）
- `live → settled`：文字没变则 key 不变、不重挂载（规则 3 的验收点）

## 6. 数据所有权

| 数据 | 唯一 owner | 其他人 |
|---|---|---|
| 消息列表（transcript） | sync/ | 只读 |
| 每会话运行时快照 | runtime-session（reducer） | projection 消费 |
| 占位消息及其 stream_id | intents 创建，projection 认领销毁 | — |
| WS 连接 / 订阅表 | subscription | — |
| 滚动位置、锚点 | 滚动层 | 不许反向影响以上任何一项 |

## 7. 迁移步骤

每步独立可合并，验收基线 = 现有 ~334 个运行时测试 + 合同 fixtures 全绿。

写入路径的演进（每一步关掉谁、打开谁）：

```
        占位直写   legacy WS   REST覆盖刷新   sync/(runtime+settle)
现状      ✓          ✓(死路)      ✓             —
步骤1     ✓          ✓           ✓             骨架(空) + CI守卫(warn)
步骤2     ✓          ✗(屏蔽)      ✓             ✓ runtime 通道上线
步骤3     ✗(改经     ✗           ✓             ✓ + 占位精确认领
          intents)
步骤4     ✗          ✗           ✗(改为settle)  ✓ 完整体
步骤5     ─────────── 删代码 + 拆滚动层赔偿 ───────────
```

每一步旧路径还在兜底，出问题可单步回退；到步骤 4 才达成"唯一写入者"。

1. **搭骨架 + CI 守卫**：建 `sync/` 空模块，加 ESLint import 规则（先 warn 后 error）。验收：lint 通过，现状无破坏。
2. **接运行时为唯一 live 通道**：subscription + runtime-session + projection 上线；runtime-session 内所有 reducer 喂入过一个串行队列（WS delta 与 REST 补水可能同时到，防交错——AI SDK 同款竞态的教训，见 §8）；同会话内屏蔽 legacy WS 分支的写入。验收：生成中内容经运行时展示；刷新/换设备可见生成中内容。
3. **占位精确认领**：intents 发送时带 stream_id，projection 用 stream_id+generation 认领，废弃文本匹配路径的占位撤除。验收：问答顺序在任何网速下稳定（回归"问号跳后面"用例）。
4. **settle 对账替换 REST 覆盖式刷新**：run 终态后一次对账，按 id 替换；`aborted × vanished` → 草稿回填。删除 `isSameLogicalTurn`。验收：点停止不丢历史；`live → settled` 无重挂载（规则 3 的 DOM 断言）。
5. **清尸 + 拆赔偿**：删 legacy WS 消费分支、多余 fetch 触发点；观察一个迭代后拆 useChatScroll 的 layout-heal / identity 迁移赔偿代码。验收：抖动用例（叶子数变化）不再复现。

## 8. 先例对照

调研了两个开源先例（源码在 `~/Documents/Reverse/ai`、`~/Documents/Reverse/ag-ui`，2026-07-14 版本），结论：**没有现成 example 覆盖我们的完整问题（服务端权威快照 + seq/epoch + 多端 + 落库对账），但有几条纪律被独立印证或值得抄。**

### Vercel AI SDK（ai@7.x / @ai-sdk/react@4.x）

比我们简单一档：无 seq、无快照、无流后对账、官方明确不支持多标签页/多设备；断线恢复只给 transport 接口，服务端存储留给用户。可抄三条：

1. **id 流头一锤定音**：assistant 消息 id 由流开头的 start chunk 确定，流中→流完永不变，零重挂载。印证我们规则 3 / "认领后 id 不变"，落在迁移第 3 步验收。
2. **写入串行化**：所有消息状态写入过一个 SerialJobExecutor 串行队列，防 async 回调交错。抄进迁移第 2 步：runtime-session 给 reducer 喂入加显式串行队列（WS delta 与 REST 补水可能同时到）。
3. **断链抛错不猜**：text-delta 找不到对应 text-start 直接抛错，不静默造 part。与我们"断 seq 即 resync"同一哲学。

反面教材：它的 v7.0.28 修复就是 resume 流与新请求竞争 activeResponse 的竞态——连这么简单的模型，一加"恢复"就撞竞态。这正是我们 abort/steer 必须带 generation 的理由。

### AG-UI Protocol

形似而神不似：有 STATE_SNAPSHOT / STATE_DELTA（JSON Patch）/ MESSAGES_SNAPSHOT 一套快照+增量原语，但无 seq/epoch（排序全靠 SSE 到达顺序）、无乐观认领、无 settle、多端明确留白；`resumable` capability 提到 sequence numbers 但 SDK 未实现（纸面能力）。两点收获：

1. **单写入者 + 纯 reducer pipeline 的独立印证**：其客户端唯一通过 `processApplyEvents` 写状态，`defaultApplyEvents` 是事件流→mutation 流的近似纯 reducer——与我们 §3 目标结构同构。
2. **"终态前快照先追平"**：其 interrupt 合同的 MUST——含 interrupt 的 RUN_FINISHED 之前必须先发 State/MessagesSnapshot，保证客户端保住已生成内容。已并入 §9 遗留问题 1，作为向服务端提的合同建议。

实证警示：STATE_DELTA 在其真实 integration 中基本无人用（LangGraph 集成全发全量快照，Vercel adapter 不 import）——**增量机制必须有真实消费者验证，否则只是协议装饰**。我们前端接 delta 时要用真实会话流量验证 delta 路径，而不是只靠 fixtures。

### LangGraph Agent Protocol（`~/Documents/agent-protocol`，2026-07-14 版本）

三个先例里**最接近我们完整问题**的一个：服务端权威 + seq replay + 多端观察 + 全量/增量分层它都有。它是 §10 三层×双主键模型的活教材，但**借模型不搬协议**（理由见下）。

可抄（正是我们缺的那张总图）：

1. **三层分治，每层对齐策略不同**：`values` channel（全量 state 基线，订阅时首个 replay 事件即当前全量）/ `since` + ring-buffer replay（run 边界增量续传）/ `/threads/{id}/history` 分页（全历史按需）。**印证 §10.4：运行层全量、历史层分页，是行业共识不是我们的特殊需求。**
2. **seq 与 id 双键正交**：event 带 `seq`（monotonic，用于 ordering + replay），message 带 `id` + content-block `index`（用于身份 + 装配）。**独立印证 §10.2「两把主键必须正交」。**
3. **增量与全量兜底并存**：`since` 太老、ring buffer 已滚掉 → 退回 `values` 全量基线。**证伪"增量能取代全量"——二者必须并存**，同时佐证 798 的 resync 不是偷懒。
4. **多端 = 多 connection 观察同一 thread**："multiple connections may observe the same thread concurrently"，与我们"手机+web 统一 client"逐字同构。

不能搬（会尾巴摇狗）：

- **graph-native**：整套绕 `namespaces`（agent 树路径）、`checkpoints`（time-travel/fork）、Pregel `tasks`；Memoh 是线性会话，套图模型等于改执行内核。
- **thread-centric，无 bot/session/ACL/channel**：路由键与权限模型和我们不同。
- **无 fenced 落库对账**：它持久化是 checkpoint store，我们是 Postgres 历史 + fence token + settle/vanished/草稿回填语义。
- 它是 **LangGraph Platform 规范**（Platform 是其 superset）——搬协议 = 往那个生态靠，非中立选型。

结论：借它的**分层心智 + since/values 分工 + seq/id 双键**，落进 §10；不搬它的 CDDL 线格式与图模型。像本节对 AG-UI 的处理——借纪律，不搬协议。

## 9. 遗留问题（需与 Fodesu 对齐）

1. **可见输出前中断的语义**：前端按 `vanished → 草稿回填` 处理；服务端是否要发显式 rollback 事实（而非靠对账发现），可选优化。合同建议（借 AG-UI interrupt 合同的 MUST）：aborted 终态 delta 发出前，先保证自含的 CurrentRunView 快照已发出——"终态前快照先追平"，前端就永远不会在停止瞬间面对残缺状态。
2. **steer_current_run / 队列**：wire 合同已有 Queue 字段，本次前端只透传显示，不做交互，占位待后续。
3. **epoch TTL**：运行时快照的 epoch 过期行为对前端重订阅时机的影响，需确认。

### 9a. 待实现文档 B（执行合同，本愿景文档不展开）

以下属"能开工的施工合同"，不是愿景漏洞。愿景（§0-§10）定的是**要什么、为什么**；这些定的是**怎么一步步做对**。单独立一份《实现》文档承接，避免把愿景淹没在工单里：

- **settle 完整状态机**：REST 失败重试、旧 generation 的 settle 响应丢弃、新 run 已开始时是否允许覆盖、用户已加载旧分页时只合并尾部、`errored/lost/completed × vanished` 的处理、附件/skills 回填、刷新或关 pane 后谁继续 settle。
- **唯一写入者的穷举迁移表**：transcript 的写入口不止 send/legacy WS/runtime/REST——retry/edit/fork、approval、ask-user、background task、ACP、draft promotion、分页加载、ephemeral error 都要逐一对账到新的 sync command + owner + 删除步骤，否则只是把第五个写入者改名为 sync/。
- **多实例 / 多 pane 生命周期**：同 `(bot, session)` 多 pane 的订阅引用计数、是否共用 reducer、隐藏但仍 streaming/pending 时是否保留、draft promotion 迁移 optimistic state、淘汰时如何取消 hydration/settle。
- **滚动部署与回滚协议**：后端 stable_id/seq、SDK、前端不可能绝对同时上线——字段 optional 与否、新旧前后端互连行为、双写/双读窗口、DB 回填、feature flag/kill switch、ESLint warn 何时升 error。
- **可执行验收矩阵**：精确命令与用例——snapshot/delta 重放等价、重复/乱序/断号、WS/REST 交错、settle 失败、retry/edit/fork/abort、A→B→A、多 pane、多 tab、分页、缓存淘汰、DOM identity、滚动像素、真实 Redis 多进程；配套指标（resync reason、settle failure、duplicate stable_id、sequence collision）。

> 其中**消息粒度、身份 vs 渲染 key、发号并发**这三条原属此类，但经查证实为**愿景层**问题，已分别收口进 §10.2a、§10.2b、§10.7——不在此列。

## 10. 数据对齐模型：三层 × 双主键（前后端一体）

> 本节范围**越出**开头"只动 apps/web"的红线，是有意的。前面 1-9 节把"写消息列表收成单入口"讲透了，但**单入口靠什么对齐**这一层一直没展开。展开到底，会发现病根不在前端，在**前后端共用的数据契约缺了两样东西：稳定身份 + 顺序号**。后端可与前端同步改，故本节按前后端一体写。

### 10.1 定位与本质

**先把它放回它所属的问题谱系：这是一个前 AI 时代就存在的经典工程问题，不是 LLM/streaming 带来的新东西。** 一端是本地缓存，一端是权威远端——本地数据你**必须**用（不用就没有 0ms 上屏、没有离线、每次都白屏等网络），但你又**不能完全信任**它（远端可能已被改写、可能正在变化、可能你手里的是旧版本）；而对齐的资源不是无限的（不能每次都全量拉、不能每条都往返确认）。**"必须用不可信的本地 + 有限资源下与权威对齐"**——这是分布式缓存、离线优先 App、版本控制、数据库复制几十年来反复解的同一道题。把它当经典问题看，才不会把它当成"运行时特性"的边角去修补；§8 三个先例本质上都是这道题的不同解法。

具体到我们，这道经典题多了一个**"运行中"布尔**带来的褶皱：**权威端不是一个，是两个。** 所以它不是"流式续传"问题，而是"本地缓存 vs 两个云端权威"的对齐问题：

- **云端不是一份真相，是两个权威，中间有一条会移动的边界。**
  - 边界**之前**的所有 turn → **Postgres 历史**是权威。
  - **正在跑的那一个 turn** → **runtime snapshot**（内存/Redis）是权威，它还没进历史。
  - 那个 turn **跑完的瞬间** → 权威从 runtime **原子移交**给历史（fenced `PersistRound`）。
- 本地要对齐的，不是"和一份云端对齐"，是**把两个权威在"边界那一个 turn"上缝起来**。缝合面只有一个 turn，不是整条历史。
- **"运行中"不是一个布尔，是一条边**：`running → terminal` 的下降沿 = 那个 turn 的所有权移交时刻。§1 四个症状（跳到后面 / 历史全没 / 刷新看不到 / 抖一下）**全部长在这条边上**。把它当布尔翻转处理，就是所有 bug 的老家。

### 10.2 两把主键必须正交并存（本节的核心要求）

查重（身份）和排序（顺序）是**两个正交维度**，任何一个都替代不了另一个：

| 主键 | 管什么 | 缺了它 |
|---|---|---|
| **稳定 ID**（身份） | 这条消息**是谁**；跨"运行中→已落库"是否同一条 | 无法跨两个权威源认出同一条 → **重复 / 内容叠加** |
| **seq**（顺序） | 这条消息在时间线的**位置**；以及"我是否漏了"（gap 检测） | 乱序到达时**顺序错乱**；无法检测缺口 |

三条禁令（与 §3 三规则同级，写死）：

1. **禁止按文本匹配**（现 `isSameLogicalTurn` 的"文字+5s"）——身份必须靠稳定 ID。
2. **禁止只按其一**——只有 ID 不知先后与缺口；只有 seq 认不出跨源同一条。**必须 ID + seq 一起。**
3. **禁止用 `created_at` 排序、禁止"最后一条"位置猜**（现 `reconcilePersistedRuntimeReplacement` 从后往前找一条）——顺序必须靠 seq。

### 10.2a 消息粒度：row 级身份 + 确定性 block 投影（seq/身份挂 row，不挂 block）

要谈"一条序"，先定义序的**单位**。代码里有三层对象，必须分清，否则"stable_id 挂哪、seq 挂哪"无从回答：

| 层 | 是什么 | 例子 | 号 |
|---|---|---|---|
| **turn** | 一问一答 | user 说一句 + assistant 答一轮 | `turn_id` / `turn_position` |
| **row**（DB 行） | 一个角色说的一段 | `role=assistant`（话+工具调用）、`role=tool`（工具结果） | `turn_message_seq`（turn 内 row 递增） |
| **block**（前端渲染单元） | 屏幕上一块 | `reasoning` / `text` / `tool` / `attachments` | **无独立号** |

`role` 取值 `user/assistant/system/tool`（`0001_init.up.sql:515`）；block 取值 `reasoning/text/tool/attachments`（`UIMessage.Type`）。一条 `role=assistant` 的 row 可以被 `ConvertModelMessagesToUIAssistantMessages` **确定性地展开**成多个 block：`[reasoning?, text?, tool*]`，顺序由 row 内容决定（`uimessage_convert.go:96`，`uimessage_test.go:106-127` 印证）。反方向并不是双射：例如一条 `role=tool` 的结果 row 会按 `tool_call_id` 与 assistant row 中的工具调用确定性关联，并聚合进同一个 `tool` block。

**核心性质是投影确定性，不是 row ↔ block 一一对应。** 一个 row 可以展开为多个 block，多条相关 row 也可以聚合为一个 block；因此 sync 层必须在渲染投影之外保留完整的**行级 ledger**，逐行记录 `stable_id / turn_position / turn_message_seq` 及关联键。由此推出整套的粒度规则：

- **block 是 row ledger 的派生视图，不是独立持久化实体 → block 不需要、也不应该自行生成 seq 或 stable_id。** 但投影必须保留 block 对一个或多个源 row 的 provenance，不能因聚合丢掉工具结果 row 的身份与坐标。
- **顺序号 `turn_message_seq` 只挂 row 级；身份 stable_id 只认 row。** 前端按 ledger 中各 row 的 `stable_id + seq` 认身份与位置，再用固定规则展开或聚合为 block；不得从 block 文本、数组位置或渲染形状反推 row 身份。
- 这也修正 §11 措辞：所谓"`Messages[]` 携带 stable_id + seq"，准确说是 **runtime wire 必须携带构成投影的完整行级 ledger；`Messages[]` 若仍是 block 数组，则每个 block 只引用一个或多个 ledger row，而不把多行压成一组伪造坐标。**

两条不可违反的边界（本次 refactor 恰好碰它们）：

1. **Redis → Postgres 是无损搬运，不是转换。** 落库时**不得重排顺序、不得重新发号、不得改写 row 结构**——Redis 里那条 row 的 `stable_id / turn_position / turn_message_seq` 原样写进 Postgres。Redis 是"运行中的账"，Postgres 是"落库的账"，同一条 row 的同一组坐标，只差一个"是否已落库"的布尔（§10.7）。**一旦落库端重排或重发号，两账即不相等，"一条序"断裂。**
2. **前端对 row 只做拉取 + 投影，不回写、不改序。** 前端从两个权威（REST 历史 / WS snapshot）**拉取** row，本地可按固定规则展开或聚合成 blocks **渲染**，但行级 ledger 始终保留；它不改写 row、不生成 row 的号，也不把渲染聚合误当成持久化合并。前端是这套数据的**读端**，不是第二个真相源。

**由此，running 与 settled 数据恒等的条件是**：构成投影的每条源 row，其 stable_id 与 `(turn_position, turn_message_seq)` 从 runtime 诞生到落库**全程不变**，且两端使用同一套确定性投影规则。这样无论是一行展开多块，还是工具调用 row 与工具结果 row 聚合成一块，两侧渲染都相等——这正是 §1 那些 bug（流完跳位、内容叠、刷新看不到）的机制级解法：现状 running 用临时 int `fallbackId`、落库换 UUID，身份在边界断裂；改为全程保留同一份 row ledger，边界即弥合。

### 10.2b 身份与渲染 key 也是两个正交的东西（别塞进一个字段）

和 §10.2「身份 vs 顺序」同源的另一处混淆：**实体身份（stable_id）与渲染 key 是两把不同的钥匙，不能挤在同一个字段里。**

- **stable_id（实体身份）**：这条消息**是谁**。参与跨源缝合——optimistic 占位认领、live 投影、settled 对账，靠它认出"运行中这条"和"落库那条"是同一条 row。它由后端发、落库后不变。
- **render_key（渲染身份）**：屏幕上这块 DOM/组件实例**是谁**。它只服务于渲染稳定（Vue `key`），决定了重挂载与否。规则 3 要求它在 `optimistic → live → settled` 全程**不变**（key 变 = 重挂载 = 抖动）。

**二者正交**：一条消息的 stable_id 可能在占位时还没有（后端没发）、live 时才到——但 render_key 从占位那一刻就必须稳定。若塞进一个字段，就出现 §7 步骤 3 与 §10.2 禁令的那种自相矛盾："认领时换成后端 stable_id"（id 要变）vs"认领后 id 不变"（key 不许变）。拆开就顺了：**占位创建时定 render_key（前端生成、全程不换）；拿到后端 stable_id 后把它记到该 render_key 名下，render_key 不动。** 缝合用 stable_id，渲染用 render_key，各管一段，谁也不用变。

这正是本设计一再复用的同一课——**两个正交概念不能塞进一个字段**：身份 vs 顺序（§10.2）、row vs block（§10.2a）、身份 vs 渲染 key（本节）。遇到"一个字段既…又…"的别扭，先问是不是又把两把钥匙挤一起了。

### 10.3 现状缺口（已探查 PR #802 commit `68974f413c418e9125dbc2de704a8ac6668c0744` + 主库 schema 确认）

**关键结论：seq 不是"没有"，是"存了、用于写、但既没用于排序、也没暴露给前端"。金库有，门没开。**

| # | 缺口 | 证据（file:line） | 影响 |
|---|---|---|---|
| 1 | 历史底层**已有二级 seq** | `bot_history_messages.turn_position BIGINT`（轮序）+ `turn_message_seq BIGINT`（轮内序），retry 时 `turn_message_seq+1` 递增 —— `db/postgres/migrations/0001_init.up.sql`、`db/postgres/queries/messages.sql` | 数据模型不缺，无需从零设计 |
| 2 | 顺序号**读路径已用、预留时机过晚** | 读 `ListMessagesBySession` 已 `ORDER BY turn_position, turn_message_seq, created_at, id`（`messages.sql:1422`）——顺序号存了、加载 context 也按它排；但部分写/对账路径仍落在 `created_at`（`messages.sql:52`），且 PostgreSQL 的 `next_turn_position` 计数器到落库时才分配，运行中拿不到 | 缺口在"发号太晚 + 没贯通到 runtime/前端"。⚠️ 本格原结论"原子号源必须保留"已随 §10.7 修订作废——号源应在 runtime 侧、Postgres 只存，计数器去掉，见 §10.7 |
| 3 | 顺序号**从未进入 SDK 契约** | `grep turn_position\|turn_message_seq packages/sdk/src/types.gen.ts` → 零命中 | 前端拿不到顺序号，被迫退回文本+时间猜 |
| 4 | runtime snapshot **只有快照级 seq，无 message 级** | `sessionruntime/types.go`：`Snapshot.Seq` 有，`CurrentRunView.Messages[]` 单条无 seq；前端 `UIMessage.ID int` 由 `messageIdFromRuntime(msg, fallbackId)` 派生（流内下标） | runtime 侧顺序靠数组下标，与历史 `turn_message_seq` 不同源 |
| 5 | 缝合两端 ID 不同源 | runtime `UIMessage.ID` 是 `int`（临时）；历史 `message_id` 是 `string`（UUID 主键） | 缝合无稳定身份可依，退化成 `:1630` 的位置猜测 |

### 10.4 目标设计：三层 × 双主键

```
                 身份键(稳定ID)     顺序键(seq)          对齐策略
──────────────────────────────────────────────────────────────────────
历史层    stable_msg_id        turn_position         REST 分页 + 游标
(Postgres) (string, 权威)       + turn_message_seq    按需增量，绝不全量
                                                     ↑「给我 418→718」在这层
──────────────────────────────────────────────────────────────────────
 ★缝合(边界 turn)：两层都出现 → stable_msg_id 相等则同一条
                → 按 (turn_position, turn_message_seq) 定位 → 按 id 覆盖
                → 既不按文本、也不只按 id、更不按"最后一条"猜
──────────────────────────────────────────────────────────────────────
运行层    stable_msg_id        snapshot.seq +        WS snapshot(全量基线,有界)
(内存/Redis)(同一个 id!)         per-message seq       + delta(seq+1，断号 resync)
                                                     ↑ 全量在这层对：有界，几十KB
──────────────────────────────────────────────────────────────────────
```

- **每层内部**：seq 管顺序 + gap 检测（seq+1，断号回退 resync/重拉基线）。
- **跨层缝合**：stable_msg_id 管身份——**同一条消息从 runtime 诞生起就带一个稳定 ID，一路不变写入 Postgres**；前端 optimistic / live / settled 三阶段认的是同一个 ID。
- **全量 vs 增量分层**：运行层快照有界（一次回答的中间态，`manager.go:515` 证实每 run 重置为空、只累积当前 run 输出），全量 resync 是对的、无状态、抗一切边界；历史层可能几十天几十 MB，**绝不全量，必须分页 + 游标**。二者不是二选一，是各管一层。
- **跨设备统一**：运行层权威须由 Redis 提到共享存储，单进程 Memory 只能做到同进程统一（见 stack 分析）。

### 10.5 进入一个 session 的统一工作流（任何设备等价）

```
1. REST 拉落盘历史     ← 历史权威(边界之前)，分页 + 游标，带 stable_id + turn seq
2. WS runtime_subscribe → snapshot ← 运行权威(边界那个 turn)，带 stable_id + seq
   ★ 缝合：边界 turn 若同时出现在 1 与 2 → 按 stable_id 去重 → 按 turn seq 定位
3. 跟 delta            ← 与 snapshot 同向(同一权威的增量读)，per-message seq 保序
```

只要 1/2/3 每个 API 走通、缝合按 stable_id + seq，**任何设备（web / 桌面 / 手机）看到的都完整统一**——它们在数据流视角是同一个无状态订阅者。

### 10.6 落地改动清单（前后端一体，四层拉通）

后端可同步改，故列为实施项而非"建议"：

- **DB 层**：历史查询 ORDER BY 改 `turn_position, turn_message_seq`（弃 `created_at` 排序）。
- **SDK 契约层**：REST 历史返回体暴露 `turn_position` / `turn_message_seq` + 稳定 `message_id`；生成 SDK 类型。
- **runtime wire 层**：运行时输出携带完整的行级 ledger：每条源 row 都有**稳定 ID**（替代前端 `int fallbackId`）+ 与历史同源的 per-message seq；`CurrentRunView.Messages[]` 的 block 投影引用对应 ledger row，保证这些身份与坐标落库时沿用不变。
- **前端 sync/ 层**：`projection` 认领与 `settle` 对账改为 `stable_id` + `(turn seq)`；切除 `isSameLogicalTurn`（§2.3）与 `reconcilePersistedRuntimeReplacement:1630` 的位置猜测。

> 施工顺序：契约先行（DB + SDK + wire 三者定死 stable_id 与 seq 语义），前端 sync/ 迁移（§7 步骤 3/4）再据此把认领与对账落到双主键上。前面 1-9 节的分步计划不变，本节是它们共同依赖的**数据契约地基**。

### 10.7 顺序号从哪来：一条序，从前到后（降熵）

**本节主轴是降熵**：让数据结构从前（runtime）到后（Postgres）流的是**同一条序**，而不是运行中一套号、落库一套号、前端再拼第三套。复杂度不是靠加机制解决，是靠**删掉"号会变"这件事**解决。

> ❌ **旧（已作废）**：本节下文一度主张"下一个 turn 是几必须由 PostgreSQL 的 session 级原子计数器 `next_turn_position` 裁决、保留并提升为所有写入路径共用的 reservation primitive、Redis 只存已预留好的 ledger；从已落库 max 推算下一值作废"。
>
> **为什么作废**：它把**发号权威**放回了 Postgres——与 §0 愿景 3「消息在运行态诞生时就带着固定顺序号、一路不变流到 Postgres」相抵触：若号是落库前一刻才由 Postgres 预留，那"运行态诞生即带号"就不成立，运行中拿的是一张空头支票（要等 Postgres 分配才知道自己是几号）。这正是我们一路讨论要消灭的"运行中一套、落库另一套"。作废的具体表述包括下文的"先 admission 再调用 PostgreSQL reservation primitive 预留 turn_position"、"保留 `next_turn_position` 并提升为唯一裁决点"、以及"从已落库 max 推算下一值作废"那句。

**新（现行）：发号在 runtime 侧完成，Postgres 只存落库结果、不参与号源裁决。**

一条序的完整流动：

1. **新 round 起手**：任何新 round 都要加载 old messages 给 AI 拼 context（否则 AI 没有对话历史）。这一加载**每行都带 `turn_position`**（`ListMessagesBySession` 已按 `turn_position, turn_message_seq` 排序），于是 `max(turn_position)` 随 context 到手——新 turn = max+1。**这不是一次"查号"，是拼 context 的副产物；起点来自已落库历史，但发号动作由 runtime 做，不由 Postgres 裁决。**
2. **运行中**：新 turn 的 `turn_position` + 每条消息的 `turn_message_seq` 在 runtime 侧诞生时即写入行级 ledger，同步进 Redis snapshot；每条 row 的稳定 ID 一并生成。
3. **worker 换手**：worker 无状态、不私藏顺序状态。接手的 worker 读共享 snapshot 的 ledger，看到轮内 seq 到 5 就发 6——**换手零丢失，因为号在共享的 snapshot 里，不在 worker 里，也不在 Postgres。**
4. **落库**：ledger 里的身份与坐标**原样**写进 Postgres 行——**不重排、不重发号、不改写结构**（§10.2a 边界 1）。Redis 里的号 = Postgres 里的号，定义相等。
5. **Redis 丢失**：运行态没了（本就该没）；所有已完成 turn 的号早已持久化在 Postgres 行里。下一个 round 加载 context 时读到 `max`，接着发——**没有"恢复逻辑"，因为不需要恢复：持久真相一直在 Postgres 的行里，Redis 只是它运行中的镜像。**

由此确定的两条结论：

- **`bot_sessions.next_turn_position` 计数器去掉，Postgres 只做存储。** 发号 = runtime 侧读已落库历史的 `max(turn_position)+1`，不需要一个独立自增列，Postgres 不参与号源裁决。
- **worker 无状态成立的前提，是"顺序状态不在 worker 里"**——run 内在 Redis snapshot 的 ledger，跨 run 在 Postgres 已落库的行里，worker 只读共享层当前值 +1。

> ⚠️ **并发前提（必须一起成立，否则本节不成立）**：runtime 侧"读 max+1 发号"是读-改-写，只有在**同一 session 同一时刻只有一个发号者**时才安全。"一 session 一 active run"的硬约束覆盖了 Web runtime 这条路径；但 §10.8 要求 IM channel / heartbeat / schedule 也发号时，**这些入口也必须纳入同一个 session 级发号互斥**（同一个并发控制点），否则两个入口并发读同一 max 会重号。这条互斥是这套发号模型的**前置条件**，不是可选项。

### 10.8 两个次级决策（不影响上面的一条序，实施期定）

1. **非 runtime 通道发号（Telegram 等 11 通道 + heartbeat/schedule）**：倾向**让 IM Channel 侧自己发号**——它本就承担"从平台拉消息 → 落库"，发号（`max(turn_position)+1`）在同一入口顺手做，与 runtime 路径共用**同一条发号规则**（读已落库 max +1），不分两套语义。用不用一个 runtime 抽象来统一，是形式问题；关键是**规则统一为一条序**，谁执行都读同一个 max。
   > ⚠️ 本条原写法"必须调用 PostgreSQL reservation primitive、谁都不许从本地历史推算下一值"已随 §10.7 修订作废：号源不在 Postgres，非 runtime 通道与 Web runtime 用**同一规则**（读已落库 max +1），但必须纳入**同一个 session 级发号互斥**（见 §10.7 末尾并发前提），否则并发重号。
2. **二级 seq 是否保留**：`(turn_position, turn_message_seq)` 二级结构保留即可——retry/edit 的作废链（`turn_superseded_by_turn_id`）按 turn 粒度操作更自然。**在数据处理/排序上它天然可拍平成一级**（`ORDER BY turn_position, turn_message_seq` 就是一个全序），前端与对齐无需感知二级，读侧当一条连续序用；二级只在写侧/作废链保留其结构价值。两不冲突。

## 11. 落地需求（说清要什么、为什么，不排工单）

本节写给实现者，但**不是任务清单**——是"要达到什么状态、为什么这样更好"。落点（改哪个文件）只标到影响面，具体拆解交给实现者。核心纪律一条：**契约先行**。§10 的一切都建立在"每条消息带稳定 ID + 顺序号"这个契约上；契约（DB 存什么、wire 发什么、SDK 暴露什么）没定死之前，前端的认领/对账无从落地。所以顺序是**先把契约拉成一条序，再让前端顺着这条序走**，不是反过来。

### 11.1 后端要达到的状态（为什么）

四件事，都服务于同一个目的——**让一条消息的身份与顺序从 runtime 诞生起就固定，一路不变流到 Postgres**：

1. **发号从"落库那刻"提前到 runtime row 诞生之前，在 runtime 侧完成，去掉 PostgreSQL 计数器。** 新 round 加载 context 时顺带到手 `max(turn_position)`，runtime 据此发新 turn 的 `turn_position`，每条 row 诞生时写 `turn_message_seq`——**不经 Postgres 裁决、不落库时才分配**。Postgres 只存落库结果、不参与号源。
   > ⚠️ 本条原写法"继续使用 PostgreSQL 原子 `next_turn_position`、经 reservation primitive 预留"已随 §10.7 修订作废：号源在 runtime 侧，Postgres 只存。发号的并发安全依赖"同一 session 同一时刻只有一个发号者"这一互斥前提（见 §10.7 末尾），不是 Postgres 计数器。
   *为什么更好*：运行态诞生即带号（§0 愿景 3），号随数据天然存在、不需要独立自增状态；Redis 丢失无需恢复逻辑（§10.7）。影响面：`internal/conversation/flow` admission/运行时路径、`db/postgres/queries/messages.sql`（去掉 `next_turn_position` 分配）。
2. **runtime snapshot 带完整的行级 ledger，落库时原样写入。** 现状前端 `UIMessage.ID` 是 `int fallbackId`（流内下标，`chat-list.ts:1162`），runtime 侧对 `turn_position/turn_message_seq` 零引用。要让每条源 row 携带**落库后不变的稳定 ID** + `(turn_position, turn_message_seq)`；`CurrentRunView.Messages[]` 的 block 投影保留对一个或多个源 row 的引用，尤其不能在 tool-call/tool-result 聚合时丢掉结果 row。*为什么更好*：这是"缝合"能成立的唯一前提——前端才能按 row ledger 认出运行中与落库后的同一组源数据，并确定性生成相同 block（§10.2a）。影响面：`internal/sessionruntime/types.go`（`CurrentRunView`/`RuntimeDelta` 结构）、agent 事件到 delta 的转换。
3. **顺序号进入 SDK 契约。** 现状 `turn_position/turn_message_seq` 在 `packages/sdk/src/types.gen.ts` 零命中（§10.3），前端拿不到、被迫文本+时间猜。REST 历史返回体要暴露顺序号 + 稳定 message ID。*为什么更好*：前端有了排序依据，才能弃掉 `isSameLogicalTurn` 的启发式。影响面：handler 返回结构 + swagger + `sdk-generate`。
4. **IM Channel 等非 runtime 通道按同一规则发号（读已落库 max +1），纳入同一 session 级发号互斥。** 它们没有 snapshot，但同样读已落库 `max(turn_position)+1` 发号、落库——与 Web runtime 共用**同一条规则**，不分两套语义。
   > ⚠️ 本条原写法"调用同一 PostgreSQL reservation primitive"已随 §10.7 修订作废：号源不在 Postgres。关键是**规则统一 + 纳入同一个 session 级发号互斥**（避免与 Web runtime / heartbeat / schedule 并发重号），见 §10.7 并发前提。
   *为什么更好*：全通道一条序。影响面：各非 runtime 写入入口。

> 迁移期的现实（点到为止，不展开）：去掉计数器属号源迁移。实现者需自行判断历史数据是否需回填 `turn_position`、迁移期是否短暂双写；发号统一为"读已落库 max +1"，不得回退到 Postgres 计数器或第二号源。本文档不规定部署步骤，只规定终态契约。

### 11.2 前端要达到的状态（为什么）

前端的总目标 §1-§7 已讲透（单写入者、三规则、五步迁移）。§10 只在其上追加一条**硬约束**，需回填进 §7 步骤 3/4：

- **认领与对账的键，从"stream_id+generation"升级为"stable_id + 顺序号"。** §7 步骤 3 原文是"projection 用 stream_id+generation 认领占位"——这在**占位→live** 阶段没错（那时消息还没有稳定 ID，靠发送时的 stream_id 认领）。但 §10.2 要求：一旦消息拿到后端的稳定 ID，之后的**live→settled 缝合**必须按 stable_id 认身份、按顺序号排位，**不得**退回文本或"最后一条"位置猜（现 `reconcilePersistedRuntimeReplacement:1630`）。*为什么更好*：占位阶段用 stream_id、落库阶段用 stable_id，各用其所长，中间靠"占位的 stream_id 被 projection 换成后端 stable_id 且 id 不变"接续——这正是 §5 `optimistic→live→settled` 三态的落地。这不与步骤 3 冲突，是把步骤 3、4 的"认领/对账"精确到双主键。

### 11.3 讨论中确认、但正文尚未收录的结论

以下几点在设计讨论中已达成，补记于此，避免只活在对话里：

- **缝合的精确语义（settle 的核心）**：REST 历史（T0 拉）与 WS snapshot（T1 订阅）之间，边界那个 turn 可能**同时出现在两侧**（它恰好在 T0/T1 之间落了库）。此时：按 **stable_id 判定同一条** → 若同一条，**以 Postgres 落库版为权威**（它是移交后的最终态）→ 按顺序号定位、按 id 覆盖 live 版。**不是拼接、不是取并集，是"同一对象的两个视图叠合，落库版赢"。** 这是 settle 模块要写死的规则，替换现在的位置猜测。
- **worker 换手是设计目标、非现状**：§10.7 的"worker 无状态、换手读 snapshot 续号"描述的是**目标能力**。当前 stack 里 run 绑定 owner 租约（Redis 模式），真正的"换一个 worker 接手正在跑的 run"尚未接线。文档把它作为"顺序号为何必须在共享层"的论据成立，但实现者应知道：**换手能力本身是后续工作，本次只需保证"号在共享层、不在 worker"这个前提不被破坏。**
- **retry/edit 与顺序号的关系**：edit 一条历史消息触发 `turn_superseded_by_turn_id` 作废链——被取代的 turn 不删，标记 superseded，新 turn 取新 `turn_position`。顺序号因此**只增不改**，"一条序"在 retry/edit 下仍成立（旧号作废、新号续尾，而非改写旧号）。二级 seq 的价值正在此：作废按 turn 粒度干净利落。
- **epoch 与顺序号是两个正交的序**：`epoch`（run 世代，§9.3）标识"这是哪一次运行"，`turn_position/turn_message_seq` 标识"消息在会话里的位置"。前端 reducer 同时持有二者：epoch 变 → 换了世代，重新 hydrate；顺序号 → 世代内/跨世代的消息排位。二者不可混用，一个管"哪一轮运行"，一个管"哪一条消息"。
- **Redis 是运行中镜像，Postgres 是持久真相，两者的号定义相等**：这是整套的地基认识（§10.7）。Redis 丢 = 运行态丢（应该的）；已完成 turn 的号安在 Postgres 行里，下个 round 加载 context 时读到 `max` 接着发——**无扫描历史恢复逻辑，因为号本就随数据落在每行上，runtime 只需读 max +1，不经 Postgres 计数器。**（本句原写法"从 PostgreSQL 原子计数器继续"已随 §10.7 修订作废：号源在 runtime 侧，Postgres 只存。）
