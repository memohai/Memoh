import { describe, expect, it } from 'vitest'
import { clusterRailBlocks, computeBgTaskPill, distinctToolNames, latestOutputLine, reconcileById, segmentTurnBlocks, shouldRefreshFromMessageCreated, sortByRecency, upsertById } from './chat-list.utils'

describe('chat-list.utils', () => {
  it('replaces existing item with same id and preserves order', () => {
    const items = [
      { id: 2, content: 'second' },
      { id: 4, content: 'fourth' },
    ]

    expect(upsertById(items, { id: 2, content: 'updated' })).toEqual([
      { id: 2, content: 'updated' },
      { id: 4, content: 'fourth' },
    ])
  })

  it('inserts new item and sorts by id', () => {
    const items = [
      { id: 4, content: 'fourth' },
      { id: 8, content: 'eighth' },
    ]

    expect(upsertById(items, { id: 6, content: 'sixth' })).toEqual([
      { id: 4, content: 'fourth' },
      { id: 6, content: 'sixth' },
      { id: 8, content: 'eighth' },
    ])
  })

  it('updates an existing item in place, preserving array and item identity', () => {
    const original = { id: 2, content: 'second' }
    const items = [original, { id: 4, content: 'fourth' }]

    const result = upsertById(items, { id: 2, content: 'updated' })

    expect(result).toBe(items)
    expect(result[0]).toBe(original)
    expect(original.content).toBe('updated')
  })

  it('drops fields absent from the incoming snapshot when updating in place', () => {
    const original: { id: number; content: string; stale?: boolean } = {
      id: 2,
      content: 'second',
      stale: true,
    }
    const items = [original]

    upsertById(items, { id: 2, content: 'updated' })

    expect(original).toEqual({ id: 2, content: 'updated' })
  })

  it('reconcileById reuses matched items in place and follows incoming order', () => {
    const a = { id: 1, v: 'a' }
    const b = { id: 2, v: 'b' }
    const target = [a, b]

    const result = reconcileById(target, [
      { id: 2, v: 'b2' },
      { id: 1, v: 'a2' },
    ])

    expect(result).toBe(target)
    expect(result[0]).toBe(b)
    expect(result[1]).toBe(a)
    expect(a.v).toBe('a2')
    expect(b.v).toBe('b2')
    expect(result.map(x => x.id)).toEqual([2, 1])
  })

  it('reconcileById drops items absent from incoming and inserts new ones', () => {
    const a = { id: 1, v: 'a' }
    const target = [a, { id: 2, v: 'b' }]

    const result = reconcileById(target, [
      { id: 1, v: 'a' },
      { id: 3, v: 'c' },
    ])

    expect(result[0]).toBe(a)
    expect(result.map(x => x.id)).toEqual([1, 3])
  })

  it('reconcileById matches existing items via a custom key', () => {
    const optimistic = { id: 'client-1', serverId: 'server-1', v: 'old' }
    const target: Array<{ id: string; serverId?: string; v: string }> = [optimistic]

    reconcileById(target, [{ id: 'server-1', v: 'new' }], {
      keyOfExisting: item => item.serverId ?? item.id,
    })

    expect(target[0]).toBe(optimistic)
    expect(optimistic.v).toBe('new')
  })

  it('reconcileById applies a custom merge to matched items', () => {
    const a = { id: 1, items: ['x'] }
    const target = [a]

    reconcileById(target, [{ id: 1, items: ['x', 'y'] }], {
      merge: (cur, inc) => {
        cur.items = inc.items
      },
    })

    expect(target[0]).toBe(a)
    expect(a.items).toEqual(['x', 'y'])
  })

  it('sortByRecency orders by updated_at desc, falls back to created_at, stable on ties', () => {
    const a = { id: 'a', updated_at: '2026-01-01T00:00:00Z' }
    const b = { id: 'b', updated_at: '2026-01-03T00:00:00Z' }
    const c = { id: 'c', created_at: '2026-01-02T00:00:00Z' }
    const d = { id: 'd' }
    const e = { id: 'e', updated_at: '2026-01-03T00:00:00Z' }

    expect(sortByRecency([a, b, c, d, e]).map(x => x.id)).toEqual(['b', 'e', 'c', 'a', 'd'])
  })

  it('sortByRecency does not mutate its input', () => {
    const input = [
      { id: 'a', updated_at: '2026-01-01T00:00:00Z' },
      { id: 'b', updated_at: '2026-01-03T00:00:00Z' },
    ]

    sortByRecency(input)

    expect(input.map(x => x.id)).toEqual(['a', 'b'])
  })

  it('latestOutputLine returns the last non-empty line, trimmed', () => {
    expect(latestOutputLine('alpha\nbeta\ngamma')).toBe('gamma')
    expect(latestOutputLine('alpha\nbeta\n')).toBe('beta')
    expect(latestOutputLine('alpha\n\n')).toBe('alpha')
    expect(latestOutputLine('  hi  ')).toBe('hi')
  })

  it('latestOutputLine collapses carriage-return progress to the current segment', () => {
    expect(latestOutputLine('downloading 10%\rdownloading 80%')).toBe('downloading 80%')
    expect(latestOutputLine('build\nstep 1\r')).toBe('step 1')
  })

  it('latestOutputLine returns empty for empty, whitespace, or missing input', () => {
    expect(latestOutputLine('')).toBe('')
    expect(latestOutputLine(undefined)).toBe('')
    expect(latestOutputLine('\r\n')).toBe('')
    expect(latestOutputLine('   \n  ')).toBe('')
  })

  it('refreshes only for current session message_created events', () => {
    expect(shouldRefreshFromMessageCreated('bot-1', 'session-1', null, {
      type: 'message_created',
      bot_id: 'bot-1',
      message: {
        id: 'm1',
        bot_id: 'bot-1',
        session_id: 'session-1',
        role: 'user',
        content: 'hello',
        created_at: '2026-04-10T10:00:00Z',
      },
    })).toBe(true)

    expect(shouldRefreshFromMessageCreated('bot-1', 'session-1', null, {
      type: 'message_created',
      bot_id: 'bot-1',
      message: {
        id: 'm2',
        bot_id: 'bot-1',
        session_id: 'session-2',
        role: 'user',
        content: 'hello',
        created_at: '2026-04-10T10:00:00Z',
      },
    })).toBe(false)

    expect(shouldRefreshFromMessageCreated('bot-1', 'session-1', null, {
      type: 'session_title_updated',
      bot_id: 'bot-1',
      session_id: 'session-1',
      title: 'new title',
    })).toBe(false)
  })

  it('does not refresh current session while a local stream is active', () => {
    expect(shouldRefreshFromMessageCreated('bot-1', 'session-1', 'session-1', {
      type: 'message_created',
      bot_id: 'bot-1',
      message: {
        id: 'm3',
        bot_id: 'bot-1',
        session_id: 'session-1',
        role: 'user',
        content: 'hello',
        created_at: '2026-04-10T10:00:00Z',
      },
    })).toBe(false)
  })
})

describe('segmentTurnBlocks', () => {
  const b = (id: number, type: string) => ({ id, type })

  it('returns no segments for an empty turn', () => {
    expect(segmentTurnBlocks([])).toEqual([])
  })

  it('wraps a lone process block in a rail segment keyed by its id', () => {
    const tool = b(1, 'tool')
    expect(segmentTurnBlocks([tool])).toEqual([
      { kind: 'rail', key: 'rail:1', blocks: [tool] },
    ])
  })

  it('emits text / error / attachments as standalone flow segments', () => {
    const text = b(1, 'text')
    const error = b(2, 'error')
    const attachments = b(3, 'attachments')
    expect(segmentTurnBlocks([text, error, attachments])).toEqual([
      { kind: 'flow', key: 'flow:1', block: text },
      { kind: 'flow', key: 'flow:2', block: error },
      { kind: 'flow', key: 'flow:3', block: attachments },
    ])
  })

  it('coalesces a maximal run of consecutive process blocks into one rail', () => {
    const reasoning = b(1, 'reasoning')
    const tool1 = b(2, 'tool')
    const tool2 = b(3, 'tool')
    expect(segmentTurnBlocks([reasoning, tool1, tool2])).toEqual([
      { kind: 'rail', key: 'rail:1', blocks: [reasoning, tool1, tool2] },
    ])
  })

  it('splits a process run when a flow block interrupts it', () => {
    const tool1 = b(1, 'tool')
    const text = b(2, 'text')
    const tool2 = b(3, 'tool')
    expect(segmentTurnBlocks([tool1, text, tool2])).toEqual([
      { kind: 'rail', key: 'rail:1', blocks: [tool1] },
      { kind: 'flow', key: 'flow:2', block: text },
      { kind: 'rail', key: 'rail:3', blocks: [tool2] },
    ])
  })

  it('keys each segment by its first block so tail growth never reparents earlier segments', () => {
    const tool1 = b(1, 'tool')
    const text = b(2, 'text')
    const tool2 = b(3, 'tool')
    const reasoning = b(4, 'reasoning')

    const before = segmentTurnBlocks([tool1, text, tool2])
    const after = segmentTurnBlocks([tool1, text, tool2, reasoning])

    expect(after[0]!.key).toBe(before[0]!.key)
    expect(after[1]!.key).toBe(before[1]!.key)
    expect(after[2]!.key).toBe(before[2]!.key)
    expect(after[2]).toEqual({ kind: 'rail', key: 'rail:3', blocks: [tool2, reasoning] })
  })

  it('preserves block object identity inside segments', () => {
    const tool = b(1, 'tool')
    const text = b(2, 'text')
    const result = segmentTurnBlocks([tool, text])
    expect((result[0] as { blocks: unknown[] }).blocks[0]).toBe(tool)
    expect((result[1] as { block: unknown }).block).toBe(text)
  })
})

describe('clusterRailBlocks', () => {
  const tool = (id: number, done: boolean, toolName = 'exec') => ({ id, type: 'tool', toolName, done })
  const reasoning = (id: number) => ({ id, type: 'reasoning' })

  it('returns no items for an empty rail', () => {
    expect(clusterRailBlocks([])).toEqual([])
  })

  it('keeps a single done tool solo (a run of one never folds)', () => {
    const t = tool(1, true)
    expect(clusterRailBlocks([t])).toEqual([
      { kind: 'block', key: 'block:1', block: t },
    ])
  })

  it('folds a run of two or more consecutive done tools into a cluster', () => {
    const t1 = tool(1, true)
    const t2 = tool(2, true)
    const t3 = tool(3, true)
    expect(clusterRailBlocks([t1, t2, t3])).toEqual([
      { kind: 'cluster', key: 'cluster:1', tools: [t1, t2, t3] },
    ])
  })

  it('renders an in-progress tool solo and lets it break a done run', () => {
    const t1 = tool(1, true)
    const t2 = tool(2, true)
    const running = tool(3, false)
    expect(clusterRailBlocks([t1, t2, running])).toEqual([
      { kind: 'cluster', key: 'cluster:1', tools: [t1, t2] },
      { kind: 'block', key: 'block:3', block: running },
    ])
  })

  it('lets a reasoning block break a run and render solo', () => {
    const t1 = tool(1, true)
    const r2 = reasoning(2)
    const t3 = tool(3, true)
    const t4 = tool(4, true)
    expect(clusterRailBlocks([t1, r2, t3, t4])).toEqual([
      { kind: 'block', key: 'block:1', block: t1 },
      { kind: 'block', key: 'block:2', block: r2 },
      { kind: 'cluster', key: 'cluster:3', tools: [t3, t4] },
    ])
  })

  it('folds nothing anywhere while the turn streams (keepOpen) so no tool ever reparents mid-stream', () => {
    const t1 = tool(1, true)
    const t2 = tool(2, true)
    const r3 = reasoning(3)
    const t4 = tool(4, true)
    const t5 = tool(5, true)
    expect(clusterRailBlocks([t1, t2, r3, t4, t5], true)).toEqual([
      { kind: 'block', key: 'block:1', block: t1 },
      { kind: 'block', key: 'block:2', block: t2 },
      { kind: 'block', key: 'block:3', block: r3 },
      { kind: 'block', key: 'block:4', block: t4 },
      { kind: 'block', key: 'block:5', block: t5 },
    ])
  })

  it('preserves tool object identity inside clusters', () => {
    const t1 = tool(1, true)
    const t2 = tool(2, true)
    const result = clusterRailBlocks([t1, t2])
    expect((result[0] as { tools: unknown[] }).tools[0]).toBe(t1)
  })
})

describe('distinctToolNames', () => {
  it('returns tool names in first-seen order without duplicates', () => {
    const tools = [
      { id: 1, toolName: 'exec' },
      { id: 2, toolName: 'exec' },
      { id: 3, toolName: 'edit' },
      { id: 4, toolName: 'exec' },
    ]
    expect(distinctToolNames(tools)).toEqual(['exec', 'edit'])
  })

  it('returns an empty list for no tools', () => {
    expect(distinctToolNames([])).toEqual([])
  })
})

describe('computeBgTaskPill', () => {
  const beacon = (taskId: string, phase: 'active' | 'done', visible: boolean, latestLine = '') =>
    ({ taskId, phase, visible, latestLine })

  it('shows no pill when there are no beacons', () => {
    expect(computeBgTaskPill([])).toBeNull()
  })

  it('shows no pill when the only running task is on screen', () => {
    expect(computeBgTaskPill([beacon('t1', 'active', true, 'step 1')])).toBeNull()
  })

  it('shows a running pill for an off-screen running task', () => {
    expect(computeBgTaskPill([beacon('t1', 'active', false, 'step 7')])).toEqual({
      tone: 'running',
      count: 1,
      latestLine: 'step 7',
    })
  })

  it('counts only off-screen running tasks and uses the latest one for the line', () => {
    expect(computeBgTaskPill([
      beacon('t1', 'active', false, 'first'),
      beacon('t2', 'active', true, 'on screen'),
      beacon('t3', 'active', false, 'latest'),
    ])).toEqual({ tone: 'running', count: 2, latestLine: 'latest' })
  })

  it('shows a done pill when an off-screen task has completed and none are running', () => {
    expect(computeBgTaskPill([beacon('t1', 'done', false)])).toEqual({
      tone: 'done',
      count: 1,
      latestLine: '',
    })
  })

  it('prefers the running tone over done when both are off-screen', () => {
    expect(computeBgTaskPill([
      beacon('t1', 'done', false),
      beacon('t2', 'active', false, 'working'),
    ])).toEqual({ tone: 'running', count: 1, latestLine: 'working' })
  })
})
