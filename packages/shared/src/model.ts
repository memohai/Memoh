export enum ModelClientType {
  OPENAI = 'openai',
  ANTHROPIC = 'anthropic',
  GOOGLE = 'google',
}

export enum ModelType {
  CHAT = 'chat',
  EMBEDDING = 'embedding',
}

export interface BaseModel {
  /**
   * @description The unique identifier for the model
   * @example 'gpt-4o'
   */
  modelId: string

  /**
   * @description The base URL for the model
   * @example 'https://api.openai.com/v1'
   */
  baseUrl: string

  /**
   * @description The API key for the model
   * @example 'sk-1234567890'
   */
  apiKey: string

  /**
   * @description The client type for the model
   * @enum {ModelClientType}
   */
  clientType: ModelClientType

  /**
   * @description The display name for the model
   * @example 'GPT 4o'
   */
  name?: string

  /**
   * @description The model type
   * @enum {ModelType}
   * @default {ModelType.CHAT}
   */
  type?: ModelType
}

export interface EmbeddingModel extends BaseModel {
  type?: ModelType.EMBEDDING

  /**
   * @description The dimensions of the embedding
   * @example 1536
   */
  dimensions: number
}

export interface ChatModel extends BaseModel {
  type?: ModelType.CHAT
}

export type Model = EmbeddingModel | ChatModel


// 表格当中model的类型
export interface ModelList {
  apiKey: string,
  baseUrl: string,
  clientType: 'OpenAI' | 'Anthropic' | 'Google',
  modelId: string,
  name: string,
  type: 'chat' | 'embedding',
  id: string,
  defaultChatModel: boolean,
  defaultEmbeddingModel: boolean,
  defaultSummaryModel: boolean
}

export interface ProviderInfo{
  api_key: string;
  base_url: string;
  client_type: string;
  metadata: Record<'additionalProp1',object>;
  name: string;
}

export const clientType = ['openai', 'anthropic', 'google', 'ollama'] as const
