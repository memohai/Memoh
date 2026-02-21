import { tool } from 'ai'
import { z } from 'zod'

interface SkillToolParams {
  useSkill: (skill: string) => void
}

export const getSkillTools = ({ useSkill }: SkillToolParams) => {
  const useSkillTool = tool({
    description: 'Use a skill if you think it is relevant to the current task',
    inputSchema: z.object({
      skillName: z.string().describe('The name of the skill to use'),
      reason: z.string().describe('The reason why you think this skill is relevant to the current task'),
    }),
    execute: async ({ skillName, reason }) => {
      useSkill(skillName)
      return {
        success: true,
        skillName,
        reason,
      }
    },
  })

  return {
    'use_skill': useSkillTool,
  }
}