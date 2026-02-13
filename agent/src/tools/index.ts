import { AuthFetcher } from '..'
import { AgentAction, AgentAuthContext, BraveConfig, IdentityContext, ModelConfig } from '../types'
import { ToolSet } from 'ai'
import { getWebTools } from './web'
import { getSubagentTools } from './subagent'
import { getSkillTools } from './skill'

export interface ToolsParams {
  fetch: AuthFetcher
  model: ModelConfig
  brave?: BraveConfig
  identity: IdentityContext
  auth: AgentAuthContext
  enableSkill: (skill: string) => void
}

export const getTools = (
  actions: AgentAction[],
  { fetch, model, brave, identity, auth, enableSkill }: ToolsParams
) => {
  const tools: ToolSet = {}
  if (actions.includes(AgentAction.Web) && brave) {
    const webTools = getWebTools({ brave })
    Object.assign(tools, webTools)
  }
  if (actions.includes(AgentAction.Subagent)) {
    const subagentTools = getSubagentTools({ fetch, model, brave, identity, auth })
    Object.assign(tools, subagentTools)
  }
  if (actions.includes(AgentAction.Skill)) {
    const skillTools = getSkillTools({ useSkill: enableSkill })
    Object.assign(tools, skillTools)
  }
    return tools
}