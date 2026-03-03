import { AuthFetcher } from '../types'
import { AgentAction, AgentAuthContext, IdentityContext, ModelConfig } from '../types'
import { ToolSet } from 'ai'
import { getWebTools } from './web'
import { getSubagentTools } from './subagent'
import { getSkillTools } from './skill'
import { getBrowserTools } from './browser'

export interface ToolsParams {
  fetch: AuthFetcher
  model: ModelConfig
  identity: IdentityContext
  auth: AgentAuthContext
  enableSkill: (skill: string) => void
}

export const getTools = (
  actions: AgentAction[],
  { fetch, model, identity, auth, enableSkill }: ToolsParams
) => {
  const tools: ToolSet = {}
  if (actions.includes(AgentAction.Web)) {
    const webTools = getWebTools()
    Object.assign(tools, webTools)
    const browserTools = getBrowserTools({ fetch, identity })
    Object.assign(tools, browserTools)
  }
  if (actions.includes(AgentAction.Subagent)) {
    const subagentTools = getSubagentTools({ fetch, model, identity, auth })
    Object.assign(tools, subagentTools)
  }
  if (actions.includes(AgentAction.Skill)) {
    const skillTools = getSkillTools({ useSkill: enableSkill })
    Object.assign(tools, skillTools)
  }
    return tools
}

export * from './web'
export * from './subagent'
export * from './skill'
export * from './mcp'
export * from './browser'
