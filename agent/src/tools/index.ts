import { AuthFetcher } from '..'
import { AgentAction, BraveConfig, IdentityContext, ModelConfig } from '../types'
import { ToolSet } from 'ai'
import { getWebTools } from './web'
import { getScheduleTools } from './schedule'
import { getMemoryTools } from './memory'
import { getSubagentTools } from './subagent'

export interface ToolsParams {
  fetch: AuthFetcher
  model: ModelConfig
  brave?: BraveConfig
  identity: IdentityContext
}

export const getTools = (
  actions: AgentAction[],
  { fetch, model, brave, identity }: ToolsParams
) => {
  const tools: ToolSet = {}
  if (actions.includes(AgentAction.Web) && brave) {
    const webTools = getWebTools({ brave })
    Object.assign(tools, webTools)
  }
  if (actions.includes(AgentAction.Schedule)) {
    const scheduleTools = getScheduleTools({ fetch })
    Object.assign(tools, scheduleTools)
  }
  if (actions.includes(AgentAction.Memory)) {
    const memoryTools = getMemoryTools({ fetch })
    Object.assign(tools, memoryTools)
  }
  if (actions.includes(AgentAction.Subagent)) {
    const subagentTools = getSubagentTools({ fetch, model, brave, identity })
    Object.assign(tools, subagentTools)
  }
  return tools
}