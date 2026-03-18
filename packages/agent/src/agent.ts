import {
  generateText,
  type ImagePart,
  LanguageModelUsage,
  ModelMessage,
  stepCountIs,
  type StepResult,
  streamText,
  ToolSet,
  UserModelMessage,
  type PrepareStepFunction,
} from 'ai'
import {
  AgentInput,
  AgentParams,
  AgentSkill,
  AgentStreamAction,
  Heartbeat,
  MCPConnection,
  Schedule,
  SystemFile,
} from './types'
import { ClientType, ModelConfig, ModelInput, hasInputModality } from './types/model'
import { system, schedule, heartbeat, subagentSystem } from './prompts'
import { AuthFetcher } from './types'
import { createModel } from './model'
import {
  stripAttachmentsFromMessages,
  dedupeAttachments,
  attachmentsResolver,
} from './utils/attachments'
import type { ContainerFileAttachment } from './types/attachment'
import { reactionsResolver, type ReactionItem } from './utils/reactions'
import { speechResolver, type SpeechItem } from './utils/speech'
import { StreamTagExtractor, extractTagsFromText, type TagEvent } from './utils/tag-extractor'
import { createImagePartFromAttachment } from './utils/image-parts'
import type { GatewayInputAttachment } from './types/attachment'
import { getMCPTools } from './tools/mcp'
import { buildIdentityHeaders } from './utils/headers'
import { createFS } from './utils'
import { createTextLoopGuard, createTextLoopProbeBuffer } from './sential'
import { createToolLoopGuardedTools } from './tool-loop'
import { createPrepareStepWithReadMedia } from './utils/read-media-injector'

const ANTHROPIC_BUDGET: Record<string, number> = { low: 5000, medium: 16000, high: 50000 }
const GOOGLE_BUDGET: Record<string, number> = { low: 5000, medium: 16000, high: 50000 }
const LOOP_DETECTED_ABORT_MESSAGE = 'loop detected, stream aborted'
const LOOP_DETECTED_STREAK_THRESHOLD = 3
const LOOP_DETECTED_MIN_NEW_GRAMS_PER_CHUNK = 8
const LOOP_DETECTED_PROBE_CHARS = 256
const TOOL_LOOP_DETECTED_ABORT_MESSAGE = 'tool loop detected, stream aborted'
const TOOL_LOOP_REPEAT_THRESHOLD = 5
const TOOL_LOOP_WARNINGS_BEFORE_ABORT = 1
const TOOL_LOOP_WARNING_KEY = '__memoh_tool_loop_warning'
const TOOL_LOOP_WARNING_TEXT = '[MEMOH_TOOL_LOOP_WARNING] Repeated identical tool invocation (same tool + arguments) was detected more than 5 times. Stop looping this tool and either summarize current results or change strategy.'

const buildProviderOptions = (config: ModelConfig): Record<string, Record<string, unknown>> | undefined => {
  if (!config.reasoning?.enabled) return undefined
  const effort = config.reasoning.effort ?? 'medium'
  switch (config.clientType) {
    case ClientType.AnthropicMessages:
      return { anthropic: { thinking: { type: 'enabled' as const, budgetTokens: ANTHROPIC_BUDGET[effort] } } }
    case ClientType.OpenAIResponses:
      return { openai: { reasoningEffort: effort, reasoningSummary: 'auto' } }
    case ClientType.OpenAICompletions:
      return { openai: { reasoningEffort: effort } }
    case ClientType.GoogleGenerativeAI:
      return { google: { thinkingConfig: { thinkingBudget: GOOGLE_BUDGET[effort] } } }
    default:
      return undefined
  }
}

const buildStepUsages = (
  steps: { usage: LanguageModelUsage; response: { messages: unknown[] } }[],
): (LanguageModelUsage | null)[] => {
  const usages: (LanguageModelUsage | null)[] = []
  for (const step of steps) {
    for (let i = 0; i < step.response.messages.length; i++) {
      usages.push(i === 0 ? step.usage : null)
    }
  }
  return usages
}

export const buildNativeImageParts = (attachments: GatewayInputAttachment[]): ImagePart[] => {
  return attachments
    .map((attachment) => createImagePartFromAttachment(attachment))
    .filter((attachment): attachment is ImagePart => attachment != null)
}

const rebuildPartialMessages = (steps: StepResult<ToolSet>[]): ModelMessage[] => {
  const messages: ModelMessage[] = []
  for (const step of steps) {
    if (step.response?.messages) {
      messages.push(...(step.response.messages as ModelMessage[]))
    }
  }
  return messages
}

export const createAgent = (
  {
    model: modelConfig,
    activeContextTime = 24 * 60,
    language = 'Same as the user input',
    channels = [],
    skills = [],
    mcpConnections = [],
    currentChannel = 'Unknown Channel',
    identity = {
      botId: '',
      channelIdentityId: '',
      displayName: '',
    },
    auth,
    inbox = [],
    loopDetection = { enabled: false },
    isSubagent = false,
  }: AgentParams,
  fetch: AuthFetcher,
) => {
  const model = createModel(modelConfig)
  const supportsImageInput = hasInputModality(modelConfig, ModelInput.Image)
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const providerOptions = buildProviderOptions(modelConfig) as any
  const loopDetectionEnabled = loopDetection?.enabled === true
  const enabledSkills: AgentSkill[] = []
  const fs = createFS({ fetch, botId: identity.botId })

  const enableSkill = (skill: string) => {
    const agentSkill = skills.find((s) => s.name === skill)
    if (agentSkill) {
      enabledSkills.push(agentSkill)
    }
  }

  const getEnabledSkills = () => {
    return enabledSkills.map((skill) => skill.name)
  }

  const loadSystemFiles = async (): Promise<SystemFile[]> => {
    const home = '/data'
    const pad = (n: number) => n.toString().padStart(2, '0')
    const getDateString = (date: Date) =>
      `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`
    const _today = getDateString(new Date())
    const _yesterday = getDateString(new Date(Date.now() - 24 * 60 * 60 * 1000))
    const files = [
      'IDENTITY.md',
      'SOUL.md',
      'TOOLS.md',
      'MEMORY.md',
      'PROFILES.md',
      `memory/${_today}.md`,
      `memory/${_yesterday}.md`,      
    ]
    const promises = files.map((file) => (async () => ({
      filename: file,
      content: await fs.readText(`${home}/${file}`).catch(() => ''),
    }))())
    return await Promise.all(promises) as SystemFile[]
  }

  const generateSystemPrompt = async () => {
    const files = await loadSystemFiles()
    return system({
      date: new Date(),
      language,
      maxContextLoadTime: activeContextTime,
      channels,
      currentChannel,
      skills,
      enabledSkills,
      inbox,
      supportsImageInput,
      files,
    })
  }

  const getAgentTools = async () => {
    const baseUrl = auth.baseUrl.replace(/\/$/, '')
    const botId = identity.botId.trim()
    if (!baseUrl || !botId) {
      return {
        tools: {},
        close: async () => {},
      }
    }
    const headers = buildIdentityHeaders(identity, auth, { isSubagent })
    const builtins: MCPConnection[] = [
      {
        type: 'http',
        name: 'builtin',
        url: `${baseUrl}/bots/${botId}/tools`,
        headers,
      },
    ]
    const { tools: mcpTools, close: closeMCP } = await getMCPTools(
      [...builtins, ...mcpConnections],
      {
        auth,
        fetch,
        botId,
      },
    )
    return {
      tools: mcpTools as ToolSet,
      close: closeMCP,
    }
  }

  const generateUserPrompt = (input: AgentInput) => {
    const imageParts = supportsImageInput ? buildNativeImageParts(input.attachments) : []

    const userMessage: UserModelMessage = {
      role: 'user',
      content: [{ type: 'text', text: input.query }, ...imageParts],
    }
    return userMessage
  }

  const createNonStreamTextLoopInspector = () => {
    if (!loopDetectionEnabled) {
      return null
    }
    const textLoopGuard = createTextLoopGuard({
      consecutiveHitsToAbort: LOOP_DETECTED_STREAK_THRESHOLD,
      minNewGramsPerChunk: LOOP_DETECTED_MIN_NEW_GRAMS_PER_CHUNK,
    })
    return (text: string) => {
      const result = textLoopGuard.inspect(text)
      if (result.abort) {
        throw new Error(LOOP_DETECTED_ABORT_MESSAGE)
      }
    }
  }

  const buildGuardedTools = (
    tools: ToolSet,
    onAbortToolCall: (toolCallId: string) => void = () => {},
  ): ToolSet => {
    if (!loopDetectionEnabled) {
      return tools
    }
    return createToolLoopGuardedTools(tools, {
      repeatThreshold: TOOL_LOOP_REPEAT_THRESHOLD,
      warningsBeforeAbort: TOOL_LOOP_WARNINGS_BEFORE_ABORT,
      onAbortToolCall,
      warningKey: TOOL_LOOP_WARNING_KEY,
      warningText: TOOL_LOOP_WARNING_TEXT,
    })
  }

  const runTextGeneration = async ({
    messages,
    systemPrompt,
    basePrepareStep,
  }: {
    messages: ModelMessage[]
    systemPrompt: string
    basePrepareStep?: PrepareStepFunction
  }) => {
    const { tools: baseTools, close } = await getAgentTools()
    const { prepareStep, tools: readMediaTools } = createPrepareStepWithReadMedia({
      modelConfig,
      fs,
      systemPrompt,
      basePrepareStep,
    })
    const tools = { ...baseTools, ...readMediaTools }
    let shouldAbortForToolLoop = false
    const guardedTools = buildGuardedTools(tools, () => {
      shouldAbortForToolLoop = true
    })
    const inspectTextLoop = createNonStreamTextLoopInspector()
    let runError: unknown = null
    try {
      return await generateText({
        model,
        messages,
        system: systemPrompt,
        ...(providerOptions && { providerOptions }),
        stopWhen: stepCountIs(Infinity),
        prepareStep,
        onStepFinish: ({ text, toolResults }: { text: string; toolResults: Array<{ toolName: string; result: unknown }> }) => {
          if (loopDetectionEnabled) {
            if (shouldAbortForToolLoop) {
              throw new Error(TOOL_LOOP_DETECTED_ABORT_MESSAGE)
            }
            if (inspectTextLoop) {
              inspectTextLoop(text)
            }
          }
          if (toolResults) {
            for (const tr of toolResults) {
              if (tr.toolName === 'use_skill') {
                const result = tr.result as Record<string, unknown> | undefined
                const skillName = typeof result?.skillName === 'string' ? result.skillName : ''
                if (skillName) {
                  enableSkill(skillName)
                }
              }
            }
          }
        },
        tools: guardedTools,
      })
    } catch (error) {
      runError = error
      throw error
    } finally {
      try {
        await close()
      } catch (closeError) {
        if (runError == null) {
          throw closeError
        }
        console.error(closeError)
      }
    }
  }

  const ask = async (input: AgentInput) => {
    const userPrompt = generateUserPrompt(input)
    const messages = [...input.messages, userPrompt]
    input.skills.forEach((skill) => enableSkill(skill))
    const systemPrompt = await generateSystemPrompt()
    const { response, reasoning, text, usage, steps } = await runTextGeneration({
      messages,
      systemPrompt,
      basePrepareStep: () => ({ system: systemPrompt }),
    })
    const stepUsages = buildStepUsages(steps)
    const tagResolvers = [attachmentsResolver, reactionsResolver, speechResolver]
    const { cleanedText, events } = extractTagsFromText(text, tagResolvers)
    const textAttachments = events
      .filter((e) => e.tag === 'attachments')
      .flatMap((e) => e.data as ContainerFileAttachment[])
    const reactions = events
      .filter((e) => e.tag === 'reactions')
      .flatMap((e) => e.data as ReactionItem[])
    const speeches = events
      .filter((e) => e.tag === 'speech')
      .flatMap((e) => e.data as SpeechItem[])
    const { messages: strippedMessages, attachments: messageAttachments } =
      stripAttachmentsFromMessages(response.messages, [reactionsResolver, speechResolver])
    const allAttachments = dedupeAttachments([
      ...textAttachments,
      ...messageAttachments,
    ])
    return {
      messages: [
        userPrompt,
        ...strippedMessages,
      ],
      usages: [null, ...stepUsages] as (LanguageModelUsage | null)[],
      reasoning: reasoning.map((part) => part.text),
      usage,
      text: cleanedText,
      attachments: allAttachments,
      reactions,
      speeches,
      skills: getEnabledSkills(),
    }
  }

  const askAsSubagent = async (params: {
    input: string;
    name: string;
    description: string;
    messages: ModelMessage[];
  }) => {
    const userPrompt: UserModelMessage = {
      role: 'user',
      content: [{ type: 'text', text: params.input }],
    }
    const generateSubagentSystemPrompt = () => {
      return subagentSystem({
        date: new Date(),
        name: params.name,
        description: params.description,
      })
    }
    const systemPrompt = generateSubagentSystemPrompt()
    const messages = [...params.messages, userPrompt]
    const { response, reasoning, text, usage, steps } = await runTextGeneration({
      messages,
      systemPrompt,
      basePrepareStep: () => ({ system: generateSubagentSystemPrompt() }),
    })
    const stepUsages = buildStepUsages(steps)
    return {
      messages: [userPrompt, ...response.messages],
      usages: [null, ...stepUsages] as (LanguageModelUsage | null)[],
      reasoning: reasoning.map((part) => part.text),
      usage,
      text,
      skills: getEnabledSkills(),
    }
  }

  const triggerSchedule = async (params: {
    schedule: Schedule;
    messages: ModelMessage[];
    skills: string[];
  }) => {
    const scheduleMessage: UserModelMessage = {
      role: 'user',
      content: [
        {
          type: 'text',
          text: schedule({ schedule: params.schedule, date: new Date() }),
        },
      ],
    }
    const messages = [...params.messages, scheduleMessage]
    params.skills.forEach((skill) => enableSkill(skill))
    const { response, reasoning, text, usage, steps } = await runTextGeneration({
      messages,
      systemPrompt: await generateSystemPrompt(),
    })
    const stepUsages = buildStepUsages(steps)
    return {
      messages: [scheduleMessage, ...response.messages],
      usages: [null, ...stepUsages] as (LanguageModelUsage | null)[],
      reasoning: reasoning.map((part) => part.text),
      usage,
      text,
      skills: getEnabledSkills(),
    }
  }

  const triggerHeartbeat = async (params: {
    heartbeat: Heartbeat;
    messages: ModelMessage[];
    skills: string[];
  }) => {
    const heartbeatText = await heartbeat({ interval: params.heartbeat.interval, date: new Date(), fs })
    const heartbeatMessage: UserModelMessage = {
      role: 'user',
      content: [
        {
          type: 'text',
          text: heartbeatText,
        },
      ],
    }
    const messages = [...params.messages, heartbeatMessage]
    params.skills.forEach((skill) => enableSkill(skill))
    const { response, reasoning, text, usage, steps } = await runTextGeneration({
      messages,
      systemPrompt: await generateSystemPrompt(),
    })
    const stepUsages = buildStepUsages(steps)
    return {
      messages: [heartbeatMessage, ...response.messages],
      usages: [null, ...stepUsages] as (LanguageModelUsage | null)[],
      reasoning: reasoning.map((part) => part.text),
      usage,
      text,
      skills: getEnabledSkills(),
    }
  }

  const resolveStreamErrorMessage = (raw: unknown): string => {
    if (raw instanceof Error && raw.message.trim()) {
      return raw.message
    }
    if (typeof raw === 'string' && raw.trim()) {
      return raw
    }
    if (raw && typeof raw === 'object') {
      const candidate = raw as { message?: unknown; error?: unknown }
      if (typeof candidate.message === 'string' && candidate.message.trim()) {
        return candidate.message
      }
      if (typeof candidate.error === 'string' && candidate.error.trim()) {
        return candidate.error
      }
      if (candidate.error instanceof Error && candidate.error.message.trim()) {
        return candidate.error.message
      }
    }
    return 'Model stream failed'
  }

  function* emitTagEvents(events: TagEvent[]): Generator<AgentStreamAction> {
    for (const event of events) {
      switch (event.tag) {
        case 'attachments': {
          const attachments = dedupeAttachments(event.data as ContainerFileAttachment[]) as ContainerFileAttachment[]
          if (attachments.length) {
            yield { type: 'attachment_delta', attachments }
          }
          break
        }
        case 'reactions': {
          const reactions = event.data as ReactionItem[]
          if (reactions.length) {
            yield { type: 'reaction_delta', reactions }
          }
          break
        }
        case 'speech': {
          const speeches = event.data as SpeechItem[]
          if (speeches.length) {
            yield { type: 'speech_delta', speeches }
          }
          break
        }
      }
    }
  }

  async function* stream(input: AgentInput): AsyncGenerator<AgentStreamAction> {
    const userPrompt = generateUserPrompt(input)
    const messages = [...input.messages, userPrompt]
    input.skills.forEach((skill) => enableSkill(skill))
    const systemPrompt = await generateSystemPrompt()
    const tagResolvers = [attachmentsResolver, reactionsResolver, speechResolver]
    const tagExtractor = new StreamTagExtractor(tagResolvers)
    const textLoopGuard = loopDetectionEnabled
      ? createTextLoopGuard({
        consecutiveHitsToAbort: LOOP_DETECTED_STREAK_THRESHOLD,
        minNewGramsPerChunk: LOOP_DETECTED_MIN_NEW_GRAMS_PER_CHUNK,
      })
      : null
    const guardLoopOutput = (text: string) => {
      if (!textLoopGuard) {
        return
      }
      const result = textLoopGuard.inspect(text)
      if (result.abort) {
        throw new Error(LOOP_DETECTED_ABORT_MESSAGE)
      }
    }
    const textLoopProbeBuffer = textLoopGuard
      ? createTextLoopProbeBuffer(
        LOOP_DETECTED_PROBE_CHARS,
        guardLoopOutput,
      )
      : null
    const result: {
      messages: ModelMessage[];
      reasoning: string[];
      usage: LanguageModelUsage | null;
      usages: (LanguageModelUsage | null)[];
    } = {
      messages: [],
      reasoning: [],
      usage: null,
      usages: [],
    }
    const toolLoopAbortCallIds = new Set<string>()
    const { tools: baseTools, close } = await getAgentTools()
    const { prepareStep, tools: readMediaTools } = createPrepareStepWithReadMedia({
      modelConfig,
      fs,
      systemPrompt,
      basePrepareStep: () => ({ system: systemPrompt }),
    })
    const tools = { ...baseTools, ...readMediaTools }
    const guardedTools = buildGuardedTools(tools, (toolCallId) => {
      toolLoopAbortCallIds.add(toolCallId)
    })
    let closePromise: Promise<void> | null = null
    const closeTools = async () => {
      if (!closePromise) {
        closePromise = Promise.resolve().then(() => close())
      }
      await closePromise
    }

    const abortController = new AbortController()
    if (input.signal) {
      if (input.signal.aborted) {
        abortController.abort(input.signal.reason)
      } else {
        input.signal.addEventListener('abort', () => abortController.abort(input.signal!.reason), { once: true })
      }
    }
    const abortedSteps: StepResult<ToolSet>[] = []
    let wasAborted = false
    let streamError: unknown = null
    try {
      const { fullStream } = streamText({
        model,
        messages,
        system: systemPrompt,
        ...(providerOptions && { providerOptions }),
        stopWhen: stepCountIs(Infinity),
        prepareStep,
        tools: guardedTools,
        abortSignal: abortController.signal,
        onFinish: async ({ usage, reasoning, response, steps }) => {
          await closeTools()
          result.usage = usage as never
          result.reasoning = reasoning.map((part) => part.text)
          result.messages = response.messages
          result.usages = buildStepUsages(steps)
        },
        onAbort: ({ steps }) => {
          wasAborted = true
          abortedSteps.push(...steps)
        },
      })
      yield {
        type: 'agent_start',
        input,
      }
      for await (const chunk of fullStream) {
        if (chunk.type === 'error') {
          throw new Error(
            resolveStreamErrorMessage((chunk as { error?: unknown }).error),
          )
        }
        switch (chunk.type) {
          case 'reasoning-start':
            yield {
              type: 'reasoning_start',
              metadata: chunk,
            }
            break
          case 'reasoning-delta':
            yield {
              type: 'reasoning_delta',
              delta: chunk.text,
            }
            break
          case 'reasoning-end':
            yield {
              type: 'reasoning_end',
              metadata: chunk,
            }
            break
          case 'text-start':
            yield {
              type: 'text_start',
            }
            break
          case 'text-delta': {
            const { visibleText, events } = tagExtractor.push(chunk.text)
            if (visibleText) {
              if (textLoopProbeBuffer) {
                textLoopProbeBuffer.push(visibleText)
              }
              yield {
                type: 'text_delta',
                delta: visibleText,
              }
            }
            yield* emitTagEvents(events)
            break
          }
          case 'text-end': {
            const remainder = tagExtractor.flushRemainder()
            if (remainder.visibleText) {
              if (textLoopProbeBuffer) {
                textLoopProbeBuffer.push(remainder.visibleText)
              }
              yield {
                type: 'text_delta',
                delta: remainder.visibleText,
              }
            }
            if (textLoopProbeBuffer) {
              textLoopProbeBuffer.flush()
            }
            yield* emitTagEvents(remainder.events)
            yield {
              type: 'text_end',
              metadata: chunk,
            }
            break
          }
          case 'tool-call': {
            const remainder = tagExtractor.flushRemainder()
            if (remainder.visibleText) {
              if (textLoopProbeBuffer) {
                textLoopProbeBuffer.push(remainder.visibleText)
              }
              yield {
                type: 'text_delta',
                delta: remainder.visibleText,
              }
            }
            if (textLoopProbeBuffer) {
              textLoopProbeBuffer.flush()
            }
            yield* emitTagEvents(remainder.events)
            yield {
              type: 'tool_call_start',
              toolName: chunk.toolName,
              toolCallId: chunk.toolCallId,
              input: chunk.input,
              metadata: chunk,
            }
            break
          }
          case 'tool-result': {
            const shouldAbortForToolLoop = toolLoopAbortCallIds.delete(chunk.toolCallId)
            yield {
              type: 'tool_call_end',
              toolName: chunk.toolName,
              toolCallId: chunk.toolCallId,
              input: chunk.input,
              result: chunk.output,
              metadata: chunk,
            }
            if (chunk.toolName === 'use_skill') {
              const res = chunk.output as Record<string, unknown> | undefined
              const sn = typeof res?.skillName === 'string' ? res.skillName : ''
              if (sn) enableSkill(sn)
            }
            if (shouldAbortForToolLoop) {
              throw new Error(TOOL_LOOP_DETECTED_ABORT_MESSAGE)
            }
            break
          }
          case 'file':
            yield {
              type: 'attachment_delta',
              attachments: [
                {
                  type: 'image',
                  url: `data:${chunk.file.mediaType ?? 'image/png'};base64,${chunk.file.base64}`,
                  mime: chunk.file.mediaType ?? 'image/png',
                },
              ],
            }
        }
      }
      if (textLoopProbeBuffer) {
        textLoopProbeBuffer.flush()
      }
  
      const { messages: strippedMessages } = stripAttachmentsFromMessages(
        result.messages,
        [reactionsResolver, speechResolver],
      )
      yield {
        type: 'agent_end',
        messages: [
          userPrompt,
          ...strippedMessages,
        ],
        usages: [null, ...result.usages],
        reasoning: result.reasoning,
        usage: result.usage!,
        skills: getEnabledSkills(),
      }
    } catch (error) {
      streamError = error

      if (wasAborted || abortController.signal.aborted) {
        const partialMessages = rebuildPartialMessages(abortedSteps)
        const partialUsages = abortedSteps.length > 0
          ? buildStepUsages(abortedSteps as Parameters<typeof buildStepUsages>[0])
          : []
        const partialUsage = abortedSteps.length > 0
          ? abortedSteps[abortedSteps.length - 1].usage
          : null
        const partialReasoning = abortedSteps.flatMap(
          (s) => (s.reasoning ?? []).map((r: { text: string }) => r.text),
        )
        yield {
          type: 'agent_abort',
          messages: [userPrompt, ...partialMessages],
          usages: [null, ...partialUsages],
          reasoning: partialReasoning,
          usage: partialUsage as LanguageModelUsage | null,
          skills: getEnabledSkills(),
        }
      } else {
        console.error(error)
        throw error
      }
    } finally {
      try {
        await closeTools()
      } catch (closeError) {
        if (streamError == null) {
          throw closeError
        }
        console.error(closeError)
      }
    }
  }

  return {
    stream,
    ask,
    askAsSubagent,
    triggerSchedule,
    triggerHeartbeat,
  }
}
