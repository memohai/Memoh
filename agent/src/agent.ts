import { generateText, ImagePart, LanguageModelUsage, ModelMessage, stepCountIs, streamText, UserModelMessage } from 'ai'
import { AgentInput, AgentParams, allActions, Schedule } from './types'
import { system, schedule, user, subagentSystem } from './prompts'
import { AuthFetcher } from './index'
import { createModel } from './model'
import { AgentAction } from './types/action'
import { getTools } from './tools'

export const createAgent = ({
  model: modelConfig,
  activeContextTime = 24 * 60,
  brave,
  language = 'Same as the user input',
  allowedActions = allActions,
  identity,
  platforms = [],
  currentPlatform = 'Unknown Platform',
}: AgentParams, fetch: AuthFetcher) => {
  const model = createModel(modelConfig)
  
  const generateSystemPrompt = () => {
    return system({
      date: new Date(),
      language,
      maxContextLoadTime: activeContextTime,
      platforms,
      skills: [],
      enabledSkills: [],
    })
  }

  const tools = getTools(allowedActions, {
    fetch,
    model: modelConfig,
    brave,
    identity,
  })

  const generateUserPrompt = (input: AgentInput) => {
    const images = input.attachments.filter(attachment => attachment.type === 'image')
    const files = input.attachments.filter(attachment => attachment.type === 'file')
    const text = user(input.query, {
      contactId: identity.contactId,
      contactName: identity.contactName,
      platform: currentPlatform,
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
    const { response, reasoning, text, usage } = await generateText({
      model,
      messages,
      system: generateSystemPrompt(),
      stopWhen: stepCountIs(Infinity),
      prepareStep: () => {
        return {
          system: generateSystemPrompt(),
        }
      },
      tools,
    })
    return {
      messages: [userPrompt, ...response.messages],
      reasoning: reasoning.map(part => part.text),
      usage,
      text,
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
      tools,
    })
    return {
      messages: [userPrompt, ...response.messages],
      reasoning: reasoning.map(part => part.text),
      usage,
      text,
    }
  }

  const triggerSchedule = async (params: {
    schedule: Schedule
    messages: ModelMessage[]
  }) => {
    const scheduleMessage: UserModelMessage = {
      role: 'user',
      content: [
        { type: 'text', text: schedule({ schedule: params.schedule, date: new Date() }) },
      ]
    }
    const messages = [...params.messages, scheduleMessage]
    const { response, reasoning, text, usage } = await generateText({
      model,
      messages,
      system: generateSystemPrompt(),
      stopWhen: stepCountIs(Infinity),
    })
    return {
      messages: [scheduleMessage, ...response.messages],
      reasoning: reasoning.map(part => part.text),
      usage,
      text,
    }
  }

  async function* stream(input: AgentInput): AsyncGenerator<AgentAction> {
    const userPrompt = generateUserPrompt(input)
    const messages = [...input.messages, userPrompt]
    const result: {
      messages: ModelMessage[]
      reasoning: string[]
      usage: LanguageModelUsage | null
    } = {
      messages: [],
      reasoning: [],
      usage: null
    }
    const { fullStream } = streamText({
      model,
      messages,
      system: generateSystemPrompt(),
      stopWhen: stepCountIs(Infinity),
      prepareStep: () => {
        return {
          system: generateSystemPrompt(),
        }
      },
      tools,
      onFinish: ({ usage, reasoning, response }) => {
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
        case 'text-delta': yield {
          type: 'text_delta',
          delta: chunk.text
        }; break
        case 'text-end': yield {
          type: 'text_end',
          metadata: chunk
        }; break
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
    yield {
      type: 'agent_end',
      messages: [userPrompt, ...result.messages],
      skills: [],
      reasoning: result.reasoning,
      usage: result.usage!,
    }
  }

  return {
    stream,
    ask,
    askAsSubagent,
    triggerSchedule,
  }
}