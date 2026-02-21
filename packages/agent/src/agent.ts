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
  MCPConnection,
  Schedule,
} from './types'
import { ModelInput, hasInputModality } from './types/model'
import { system, schedule, subagentSystem } from './prompts'
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
  }: AgentParams,
  fetch: AuthFetcher,
) => {
  const model = createModel(modelConfig)
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
    const [identityContent, soulContent, toolsContent] = await Promise.all([
      fs.readText('/data/IDENTITY.md'),
      fs.readText('/data/SOUL.md'),
      fs.readText('/data/TOOLS.md'),
    ])
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

  const ask = async (input: AgentInput) => {
    const userPrompt = generateUserPrompt(input)
    const messages = [...input.messages, userPrompt]
    input.skills.forEach((skill) => enableSkill(skill))
    const systemPrompt = await generateSystemPrompt()
    const { tools, close } = await getAgentTools()
    const { response, reasoning, text, usage, steps } = await generateText({
      model,
      messages,
      system: systemPrompt,
      stopWhen: stepCountIs(Infinity),
      prepareStep: () => {
        return {
          system: systemPrompt,
        }
      },
      onFinish: async () => {
        await close()
      },
      tools,
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
    const { tools, close } = await getAgentTools()
    const { response, reasoning, text, usage, steps } = await generateText({
      model,
      messages,
      system: generateSubagentSystemPrompt(),
      stopWhen: stepCountIs(Infinity),
      prepareStep: () => {
        return {
          system: generateSubagentSystemPrompt(),
        }
      },
      onFinish: async () => {
        await close()
      },
      tools,
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
    const { tools, close } = await getAgentTools()
    const { response, reasoning, text, usage, steps } = await generateText({
      model,
      messages,
      system: await generateSystemPrompt(),
      stopWhen: stepCountIs(Infinity),
      onFinish: async () => {
        await close()
      },
      tools,
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
    const { tools, close } = await getAgentTools()
    try {
      const { fullStream } = streamText({
        model,
        messages,
        system: systemPrompt,
        stopWhen: stepCountIs(Infinity),
        prepareStep: () => {
          return {
            system: systemPrompt,
          }
        },
        tools,
        onFinish: async ({ usage, reasoning, response, steps }) => {
          await close()
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
              yield {
                type: 'text_delta',
                delta: remainder.visibleText,
              }
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
            yield {
              type: 'tool_call_start',
              toolName: chunk.toolName,
              toolCallId: chunk.toolCallId,
              input: chunk.input,
              metadata: chunk,
            }
            break
          case 'tool-result':
            yield {
              type: 'tool_call_end',
              toolName: chunk.toolName,
              toolCallId: chunk.toolCallId,
              input: chunk.input,
              result: chunk.output,
              metadata: chunk,
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
      console.error(error)
      throw error
    }
  }

  return {
    stream,
    ask,
    askAsSubagent,
    triggerSchedule,
  }
}
