import { describe, expect, it, vi } from 'vitest'
import type { ToolSet } from 'ai'
import { createToolLoopGuardedTools } from './tool-loop'

describe('tool loop guarded tools', () => {
  it('preserves promised async-iterable tool outputs', async () => {
    const onAbortToolCall = vi.fn()
    const streamedChunks = ['chunk-1', 'chunk-2']
    const stream = {
      async *[Symbol.asyncIterator]() {
        for (const chunk of streamedChunks) {
          yield chunk
        }
      },
    }

    const baseTools = {
      streamy: {
        execute: async () => stream,
      },
    } as unknown as ToolSet

    const tools = createToolLoopGuardedTools(baseTools, {
      repeatThreshold: 1,
      warningsBeforeAbort: 1,
      onAbortToolCall,
      warningKey: '__warn',
      warningText: 'loop warning',
    })

    const output = await tools.streamy.execute?.({ value: 'same' } as never, { toolCallId: 't-stream' } as never)

    expect(output).toBe(stream)
    const received: string[] = []
    for await (const chunk of output as AsyncIterable<string>) {
      received.push(chunk)
    }
    expect(received).toEqual(streamedChunks)
    expect(onAbortToolCall).not.toHaveBeenCalled()
  })

  it('defers abort to stream layer when onAbortToolCall is provided', async () => {
    const onAbortToolCall = vi.fn()
    const baseTools = {
      echo: {
        execute: async (input: unknown) => ({ result: input }),
      },
    } as unknown as ToolSet
    const tools = createToolLoopGuardedTools(baseTools, {
      repeatThreshold: 1,
      warningsBeforeAbort: 1,
      onAbortToolCall,
      warningKey: '__warn',
      warningText: 'loop warning',
    })

    await tools.echo.execute?.({ value: 'same' } as never, { toolCallId: 't-1' } as never)
    const warned = await tools.echo.execute?.({ value: 'same' } as never, { toolCallId: 't-1' } as never)
    expect(warned).toMatchObject({
      __warn: {
        marker: 'MEMOH_TOOL_LOOP_WARNING',
      },
    })
    await tools.echo.execute?.({ value: 'same' } as never, { toolCallId: 't-1' } as never)
    const abortedOutput = await tools.echo.execute?.({ value: 'same' } as never, { toolCallId: 't-1' } as never)

    expect(onAbortToolCall).toHaveBeenCalledWith('t-1')
    expect(abortedOutput).toEqual({ result: { value: 'same' } })
  })

  it('reports abort via callback without throwing inside tool execution', async () => {
    const onAbortToolCall = vi.fn()
    const baseTools = {
      echo: {
        execute: async (input: unknown) => ({ result: input }),
      },
    } as unknown as ToolSet
    const tools = createToolLoopGuardedTools(baseTools, {
      repeatThreshold: 1,
      warningsBeforeAbort: 1,
      onAbortToolCall,
      warningKey: '__warn',
      warningText: 'loop warning',
    })

    await tools.echo.execute?.({ value: 'same' } as never, { toolCallId: 't-2' } as never)
    await tools.echo.execute?.({ value: 'same' } as never, { toolCallId: 't-2' } as never)
    await tools.echo.execute?.({ value: 'same' } as never, { toolCallId: 't-2' } as never)
    const abortedOutput = await tools.echo.execute?.({ value: 'same' } as never, { toolCallId: 't-2' } as never)
    expect(onAbortToolCall).toHaveBeenCalledWith('t-2')
    expect(abortedOutput).toEqual({ result: { value: 'same' } })
  })
})
