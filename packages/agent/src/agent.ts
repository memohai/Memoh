import {
  generateText,
  ImagePart,
  LanguageModelUsage,
  ModelMessage,
  stepCountIs,
  streamText,
  ToolSet,
  UserModelMessage,
} from 'ai'
import {
  AgentInput,
  AgentParams,
  AgentSkill,
  AgentStreamAction,
  allActions,
  Heartbeat,
  MCPConnection,
  Schedule,
} from './types'
import { ClientType, ModelConfig, ModelInput, hasInputModality } from './types/model'
import { system, schedule, heartbeat, subagentSystem } from './prompts'
import { AuthFetcher } from './types'
import { createModel } from './model'
import {
  extractAttachmentsFromText,
  stripAttachmentsFromMessages,
  dedupeAttachments,
  AttachmentsStreamExtractor,
} from './utils/attachments'
import type { GatewayInputAttachment } from './types/attachment'
import { getMCPTools } from './tools/mcp'
import { getTools } from './tools'
import { buildIdentityHeaders } from './utils/headers'
import { createFS } from './utils'
import { createTextLoopGuard, createTextLoopProbeBuffer } from './sential'
import { createToolLoopGuardedTools } from './tool-loop'

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
    .filter((attachment) =>
      attachment.type === 'image' &&
      (attachment.transport === 'inline_data_url' || attachment.transport === 'public_url') &&
      Boolean(attachment.payload),
    )
    .map((attachment) => ({ type: 'image', image: attachment.payload } as ImagePart))
}

export const createAgent = (
  {
    model: modelConfig,
    activeContextTime = 24 * 60,
    language = 'Same as the user input',
    allowedActions = allActions,
    channels = [],
    skills = [],
    mcpConnections = [],
    currentChannel = 'Unknown Channel',
    identity = {
      botId: '',
      containerId: '',
      channelIdentityId: '',
      displayName: '',
    },
    auth,
    inbox = [],
    loopDetection = { enabled: false },
  }: AgentParams,
  fetch: AuthFetcher,
) => {
  const model = createModel(modelConfig)
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

  const loadSystemFiles = async () => {
    const home = '/data'
    const [identityContent, soulContent, toolsContent] = await Promise.all([
      fs.readText(`${home}/IDENTITY.md`),
      fs.readText(`${home}/SOUL.md`),
      fs.readText(`${home}/TOOLS.md`),
    ]).catch((error) => {
      console.error(error)
      return ['', '', '']
    })
    return { identityContent, soulContent, toolsContent }
  }

  const generateSystemPrompt = async () => {
    const { identityContent, soulContent, toolsContent } =
      await loadSystemFiles()
    return system({
      date: new Date(),
      language,
      maxContextLoadTime: activeContextTime,
      channels,
      currentChannel,
      skills,
      enabledSkills,
      identityContent,
      soulContent,
      toolsContent,
      inbox,
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
    const headers = buildIdentityHeaders(identity, auth)
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
    const tools = getTools(allowedActions, {
      fetch,
      model: modelConfig,
      identity,
      auth,
      enableSkill,
    })
    return {
      tools: { ...mcpTools, ...tools } as ToolSet,
      close: closeMCP,
    }
  }

  const generateUserPrompt = (input: AgentInput) => {
    const supportsImage = hasInputModality(modelConfig, ModelInput.Image)
    const imageParts = supportsImage ? buildNativeImageParts(input.attachments) : []

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
    prepareStep,
  }: {
    messages: ModelMessage[]
    systemPrompt: string
    prepareStep?: () => { system: string }
  }) => {
    const { tools, close } = await getAgentTools()
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
        ...(prepareStep && { prepareStep }),
        ...(loopDetectionEnabled && {
          onStepFinish: ({ text }: { text: string }) => {
            if (shouldAbortForToolLoop) {
              throw new Error(TOOL_LOOP_DETECTED_ABORT_MESSAGE)
            }
            if (inspectTextLoop) {
              inspectTextLoop(text)
            }
          },
        }),
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
      prepareStep: () => {
        return {
          system: systemPrompt,
        }
      },
    })
    const stepUsages = buildStepUsages(steps)
    const { cleanedText, attachments: textAttachments } =
      extractAttachmentsFromText(text)
    const { messages: strippedMessages, attachments: messageAttachments } =
      stripAttachmentsFromMessages(response.messages)
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
    const messages = [...params.messages, userPrompt]
    const { response, reasoning, text, usage, steps } = await runTextGeneration({
      messages,
      systemPrompt: generateSubagentSystemPrompt(),
      prepareStep: () => {
        return {
          system: generateSubagentSystemPrompt(),
        }
      },
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

  async function* stream(input: AgentInput): AsyncGenerator<AgentStreamAction> {
    const userPrompt = generateUserPrompt(input)
    const messages = [...input.messages, userPrompt]
    input.skills.forEach((skill) => enableSkill(skill))
    const systemPrompt = await generateSystemPrompt()
    const attachmentsExtractor = new AttachmentsStreamExtractor()
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
    const { tools, close } = await getAgentTools()
    // Stream path needs deferred abort to keep tool_call_start/tool_call_end event pairing.
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
    let streamError: unknown = null
    try {
      const { fullStream } = streamText({
        model,
        messages,
        system: systemPrompt,
        ...(providerOptions && { providerOptions }),
        stopWhen: stepCountIs(Infinity),
        prepareStep: () => {
          return {
            system: systemPrompt,
          }
        },
        tools: guardedTools,
        onFinish: async ({ usage, reasoning, response, steps }) => {
          await closeTools()
          result.usage = usage as never
          result.reasoning = reasoning.map((part) => part.text)
          result.messages = response.messages
          result.usages = buildStepUsages(steps)
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
            const { visibleText, attachments } = attachmentsExtractor.push(
              chunk.text,
            )
            if (visibleText) {
              if (textLoopProbeBuffer) {
                textLoopProbeBuffer.push(visibleText)
              }
              yield {
                type: 'text_delta',
                delta: visibleText,
              }
            }
            if (attachments.length) {
              yield {
                type: 'attachment_delta',
                attachments,
              }
            }
            break
          }
          case 'text-end': {
            // Flush any remaining buffered content before ending the text stream.
            const remainder = attachmentsExtractor.flushRemainder()
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
            if (remainder.attachments.length) {
              yield {
                type: 'attachment_delta',
                attachments: remainder.attachments,
              }
            }
            yield {
              type: 'text_end',
              metadata: chunk,
            }
            break
          }
          case 'tool-call':
            // Flush any remaining buffered content before ending the text stream.
            const remainder = attachmentsExtractor.flushRemainder()
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
            if (remainder.attachments.length) {
              yield {
                type: 'attachment_delta',
                attachments: remainder.attachments,
              }
            }
            yield {
              type: 'tool_call_start',
              toolName: chunk.toolName,
              toolCallId: chunk.toolCallId,
              input: chunk.input,
              metadata: chunk,
            }
            break
          case 'tool-result':
            // Always emit the terminal tool event first so downstream reducers
            // can close the in-flight tool block before the stream aborts.
            const shouldAbortForToolLoop = toolLoopAbortCallIds.delete(chunk.toolCallId)
            yield {
              type: 'tool_call_end',
              toolName: chunk.toolName,
              toolCallId: chunk.toolCallId,
              input: chunk.input,
              result: chunk.output,
              metadata: chunk,
            }
            if (shouldAbortForToolLoop) {
              throw new Error(TOOL_LOOP_DETECTED_ABORT_MESSAGE)
            }
            break
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
      console.error(error)
      throw error
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
