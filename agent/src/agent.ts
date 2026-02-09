import { generateText, ImagePart, LanguageModelUsage, ModelMessage, stepCountIs, streamText, UserModelMessage } from 'ai'
import { AgentInput, AgentParams, AgentSkill, allActions, HTTPMCPConnection, MCPConnection, Schedule, StdioMCPConnection } from './types'
import { system, schedule, user, subagentSystem } from './prompts'
import { AuthFetcher } from './index'
import { createModel } from './model'
import { AgentAction } from './types/action'
import { getTools } from './tools'
import {
  extractAttachmentsFromText,
  stripAttachmentsFromMessages,
  dedupeAttachments,
  AttachmentsStreamExtractor,
} from './utils/attachments'
import type { ContainerFileAttachment } from './types/attachment'
import { getMCPTools } from './tools/mcp'

export const createAgent = ({
  model: modelConfig,
  activeContextTime = 24 * 60,
  brave,
  language = 'Same as the user input',
  allowedActions = allActions,
  channels = [],
  mcpConnections = [],
  skills = [],
  currentChannel = 'Unknown Channel',
  identity = {
    botId: '',
    sessionId: '',
    containerId: '',
    contactId: '',
    contactName: '',
  },
  auth,
}: AgentParams, fetch: AuthFetcher) => {
  const model = createModel(modelConfig)
  const enabledSkills: AgentSkill[] = []

  const enableSkill = (skill: string) => {
    const agentSkill = skills.find(s => s.name === skill)
    if (agentSkill) {
      enabledSkills.push(agentSkill)
    }
  }

  const getEnabledSkills = () => {
    return enabledSkills.map(skill => skill.name)
  }

  const getDefaultMCPConnections = (): MCPConnection[] => {
    const fs: HTTPMCPConnection = {
      type: 'http',
      name: 'fs',
      url: `${auth.baseUrl}/bots/${identity.botId}/container/fs`,
      headers: {
        'Authorization': `Bearer ${auth.bearer}`,
      },
    }
    const mcpFetch: StdioMCPConnection = {
      type: 'stdio',
      name: 'mcp-fetch',
      command: 'npx',
      args: ['fetch-mcp'],
      env: {},
    }
    return [fs, mcpFetch]
  }
  
  const generateSystemPrompt = () => {
    return system({
      date: new Date(),
      language,
      maxContextLoadTime: activeContextTime,
      channels,
      skills,
      enabledSkills,
    })
  }

  const getAgentTools = async () => {
    const tools = getTools(allowedActions, {
      fetch,
      model: modelConfig,
      brave,
      identity,
      enableSkill,
    })
    const defaultMCPConnections = getDefaultMCPConnections()
    const { tools: mcpTools, close: closeMCP } = await getMCPTools([
      ...defaultMCPConnections,
      ...mcpConnections,
    ], {
      botId: identity.botId,
      auth,
      fetch,
    })
    Object.assign(tools, mcpTools)
    return {
      tools,
      close: closeMCP,
    }
  }

  const generateUserPrompt = (input: AgentInput) => {
    const images = input.attachments.filter(attachment => attachment.type === 'image')
    const files = input.attachments.filter((a): a is ContainerFileAttachment => a.type === 'file')
    const text = user(input.query, {
      contactId: identity.contactId,
      contactName: identity.contactName,
      channel: currentChannel,
      date: new Date(),
      attachments: files,
    })
    const userMessage: UserModelMessage = {
      role: 'user',
      content: [
        { type: 'text', text },
        ...images.map(image => ({ type: 'image', image: image.base64 }) as ImagePart),
      ]
    }
    return userMessage
  }

  const ask = async (input: AgentInput) => {
    const userPrompt = generateUserPrompt(input)
    const messages = [...input.messages, userPrompt]
    input.skills.forEach(skill => enableSkill(skill))
    const systemPrompt = generateSystemPrompt()
    const { tools, close } = await getAgentTools()
    const { response, reasoning, text, usage } = await generateText({
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
    const { cleanedText, attachments: textAttachments } = extractAttachmentsFromText(text)
    const { messages: strippedMessages, attachments: messageAttachments } = stripAttachmentsFromMessages(response.messages)
    const allAttachments = dedupeAttachments([...textAttachments, ...messageAttachments])
    return {
      messages: [userPrompt, ...strippedMessages],
      reasoning: reasoning.map(part => part.text),
      usage,
      text: cleanedText,
      attachments: allAttachments,
      skills: getEnabledSkills(),
    }
  }

  const askAsSubagent = async (params: {
    input: string
    name: string
    description: string
    messages: ModelMessage[]
  }) => {
    const userPrompt: UserModelMessage = {
      role: 'user',
      content: [
        { type: 'text', text: params.input },
      ]
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
    const { response, reasoning, text, usage } = await generateText({
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
    return {
      messages: [userPrompt, ...response.messages],
      reasoning: reasoning.map(part => part.text),
      usage,
      text,
      skills: getEnabledSkills(),
    }
  }

  const triggerSchedule = async (params: {
    schedule: Schedule
    messages: ModelMessage[]
    skills: string[]
  }) => {
    const scheduleMessage: UserModelMessage = {
      role: 'user',
      content: [
        { type: 'text', text: schedule({ schedule: params.schedule, date: new Date() }) },
      ]
    }
    const messages = [...params.messages, scheduleMessage]
    params.skills.forEach(skill => enableSkill(skill))
    const { tools, close } = await getAgentTools()
    const { response, reasoning, text, usage } = await generateText({
      model,
      messages,
      system: generateSystemPrompt(),
      stopWhen: stepCountIs(Infinity),
      onFinish: async () => {
        await close()
      },
      tools,
    })
    return {
      messages: [scheduleMessage, ...response.messages],
      reasoning: reasoning.map(part => part.text),
      usage,
      text,
      skills: getEnabledSkills(),
    }
  }

  async function* stream(input: AgentInput): AsyncGenerator<AgentAction> {
    const userPrompt = generateUserPrompt(input)
    const messages = [...input.messages, userPrompt]
    input.skills.forEach(skill => enableSkill(skill))
    const systemPrompt = generateSystemPrompt()
    const attachmentsExtractor = new AttachmentsStreamExtractor()
    const result: {
      messages: ModelMessage[]
      reasoning: string[]
      usage: LanguageModelUsage | null
    } = {
      messages: [],
      reasoning: [],
      usage: null
    }
    const { tools, close } = await getAgentTools()
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
      onFinish: async ({ usage, reasoning, response }) => {
        await close()
        result.usage = usage as never
        result.reasoning = reasoning.map(part => part.text)
        result.messages = response.messages
      }
    })
    yield {
      type: 'agent_start',
      input,
    }
    for await (const chunk of fullStream) {
      switch (chunk.type) {
        case 'reasoning-start': yield {
          type: 'reasoning_start',
          metadata: chunk
        }; break
        case 'reasoning-delta': yield {
          type: 'reasoning_delta',
          delta: chunk.text
        }; break
        case 'reasoning-end': yield {
          type: 'reasoning_end',
          metadata: chunk
        }; break
        case 'text-start': yield {
          type: 'text_start',
        }; break
        case 'text-delta': {
          const { visibleText, attachments } = attachmentsExtractor.push(chunk.text)
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
        case 'tool-call': yield {
          type: 'tool_call_start',
          toolName: chunk.toolName,
          toolCallId: chunk.toolCallId,
          input: chunk.input,
          metadata: chunk
        }; break
        case 'tool-result': yield {
          type: 'tool_call_end',
          toolName: chunk.toolName,
          toolCallId: chunk.toolCallId,
          input: chunk.input,
          result: chunk.output,
          metadata: chunk
        }; break
        case 'file': yield {
          type: 'image_delta',
          image: chunk.file.base64,
          metadata: chunk
        }
      }
    }

    const { messages: strippedMessages } = stripAttachmentsFromMessages(result.messages)
    yield {
      type: 'agent_end',
      messages: [userPrompt, ...strippedMessages],
      reasoning: result.reasoning,
      usage: result.usage!,
      skills: getEnabledSkills(),
    }
  }

  return {
    stream,
    ask,
    askAsSubagent,
    triggerSchedule,
  }
}
