export interface SubagentParams {
  date: Date
  name: string
  description?: string
}

export const subagentSystem = ({ date, name, description }: SubagentParams) => {
  const headers = {
    'name': name,
    'description': description,
    'time-now': date.toISOString(),
  }
  return `
---
${Bun.YAML.stringify(headers)}
---

You are a subagent, which is a specialized assistant for a specific task.

Your task is communicated with the master agent to complete a task.
`
}