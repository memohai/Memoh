import { createOpenAI } from '@ai-sdk/openai'
import { createAnthropic } from '@ai-sdk/anthropic'
import { createGoogleGenerativeAI } from '@ai-sdk/google'
import { ClientType, HeliconeConfig, ModelConfig } from './types'

const DEFAULT_HELICONE_GATEWAY = 'https://gateway.helicone.ai'

export interface HeliconeContext {
  botId?: string
  channel?: string
}

// Self-hosted Helicone requires path-based routing to recognize gateway requests.
// We combine it with Helicone-Target-Url to override the default target per provider.
const HELICONE_PROVIDER_PATHS: Record<string, string> = {
  [ClientType.OpenAIResponses]: '/v1/gateway/oai/v1',
  [ClientType.OpenAICompletions]: '/v1/gateway/oai/v1',
  [ClientType.AnthropicMessages]: '/v1/gateway/anthropic',
  [ClientType.GoogleGenerativeAI]: '/v1/gateway/google',
}

const resolveHelicone = (
  helicone: HeliconeConfig,
  clientType: ClientType | string,
  originalBaseURL: string,
  context?: HeliconeContext,
): { baseURL: string; headers: Record<string, string> } => {
  const headers: Record<string, string> = {
    'Helicone-Auth': `Bearer ${helicone.apiKey}`,
    'Helicone-Target-Url': originalBaseURL,
  }
  if (context?.botId) headers['Helicone-Property-BotId'] = context.botId
  if (context?.channel) headers['Helicone-Property-Channel'] = context.channel

  const customBase = helicone.baseUrl.trim()
  if (customBase) {
    const providerPath = HELICONE_PROVIDER_PATHS[clientType] ?? HELICONE_PROVIDER_PATHS[ClientType.OpenAICompletions]
    return { baseURL: customBase.replace(/\/+$/, '') + providerPath, headers }
  }

  return { baseURL: DEFAULT_HELICONE_GATEWAY, headers }
}

export const createModel = (
  model: ModelConfig,
  helicone?: HeliconeConfig,
  context?: HeliconeContext,
) => {
  const apiKey = model.apiKey.trim()
  let baseURL = model.baseUrl.trim()
  const modelId = model.modelId.trim()

  let headers: Record<string, string> | undefined
  if (helicone?.enabled && helicone.apiKey) {
    const resolved = resolveHelicone(helicone, model.clientType, baseURL, context)
    baseURL = resolved.baseURL
    headers = resolved.headers
  }

  switch (model.clientType) {
    case ClientType.OpenAIResponses:
      return createOpenAI({ apiKey, baseURL, headers })(modelId)
    case ClientType.OpenAICompletions:
      return createOpenAI({ apiKey, baseURL, headers }).chat(modelId)
    case ClientType.AnthropicMessages:
      return createAnthropic({ apiKey, baseURL, headers })(modelId)
    case ClientType.GoogleGenerativeAI:
      return createGoogleGenerativeAI({ apiKey, baseURL, headers })(modelId)
    default:
      return createOpenAI({ apiKey, baseURL, headers }).chat(modelId)
  }
}
