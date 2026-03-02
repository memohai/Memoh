import { ToolExecutionOptions, ToolSet } from 'ai'
import { createToolLoopGuard, type ToolLoopInspectResult } from './sential'

export interface CreateToolLoopGuardedToolsOptions {
  repeatThreshold: number
  warningsBeforeAbort: number
  onAbortToolCall: (toolCallId: string) => void
  warningKey: string
  warningText: string
}

const isRecord = (value: unknown): value is Record<string, unknown> => {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
}

const isAsyncIterable = (value: unknown): value is AsyncIterable<unknown> => {
  return (
    value !== null &&
    typeof value === 'object' &&
    Symbol.asyncIterator in value
  )
}

const injectToolLoopWarning = (
  output: unknown,
  inspectResult: ToolLoopInspectResult,
  warningKey: string,
  warningText: string,
): unknown => {
  // Keep warning payload structured so UI/consumers can detect and render it.
  const warningPayload = {
    marker: 'MEMOH_TOOL_LOOP_WARNING',
    message: warningText,
    fingerprint: inspectResult.hash,
    breachCount: inspectResult.breachCount,
  }
  if (isRecord(output)) {
    return {
      ...output,
      [warningKey]: warningPayload,
    }
  }
  return {
    [warningKey]: warningPayload,
    result: output,
  }
}

export function createToolLoopGuardedTools(
  tools: ToolSet,
  {
    repeatThreshold,
    warningsBeforeAbort,
    onAbortToolCall,
    warningKey,
    warningText,
  }: CreateToolLoopGuardedToolsOptions,
): ToolSet {
  const guard = createToolLoopGuard({
    repeatThreshold,
    warningsBeforeAbort,
  })

  // Wrap each executable tool to inspect (toolName + input) after execution.
  // First breach injects a warning into this tool result; second breach signals abort.
  return Object.fromEntries(
    Object.entries(tools).map(([toolName, toolDefinition]) => {
      const execute = toolDefinition.execute
      if (typeof execute !== 'function') {
        return [toolName, toolDefinition]
      }

      const wrappedTool = {
        ...toolDefinition,
        execute: (
          toolInput: unknown,
          options: ToolExecutionOptions,
        ) => {
          const directOutput = execute(
            toolInput as never,
            options as never,
          ) as unknown

          // Streamed tool outputs are passed through unchanged to preserve streaming semantics.
          if (isAsyncIterable(directOutput)) {
            return directOutput as never
          }

          return (async () => {
            const resolvedOutput = await directOutput

            // Tools may return Promise<AsyncIterable>; keep that stream untouched too.
            if (isAsyncIterable(resolvedOutput)) {
              return resolvedOutput as never
            }

            const inspectResult = guard.inspect({
              toolName,
              input: toolInput,
            })
            if (inspectResult.abort) {
              // Report loop abort to generation layer; it decides when/how to stop.
              onAbortToolCall(options.toolCallId)
              return resolvedOutput as never
            }
            if (inspectResult.warn) {
              return injectToolLoopWarning(
                resolvedOutput,
                inspectResult,
                warningKey,
                warningText,
              ) as never
            }
            return resolvedOutput as never
          })()
        },
      }

      return [toolName, wrappedTool]
    }),
  ) as ToolSet
}
