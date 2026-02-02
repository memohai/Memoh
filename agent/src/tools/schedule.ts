import { tool } from 'ai'
import { z } from 'zod'
import { AuthFetcher } from '..'

export type ScheduleToolParams = {
  fetch: AuthFetcher
}

const ScheduleSchema = z.object({
  name: z.string(),
  description: z.string(),
  pattern: z.string(),
  max_calls: z.number().nullable().optional(),
  enabled: z.boolean(),
  command: z.string(),
})

export const getScheduleTools = ({ fetch }: ScheduleToolParams) => {
  const listSchedules = tool({
    description: 'List schedules for current user',
    inputSchema: z.object({}),
    execute: async () => {
      const response = await fetch('/schedule', { method: 'GET' })
      return response.json()
    },
  })

  const getSchedule = tool({
    description: 'Get a schedule by id',
    inputSchema: z.object({
      id: z.string().describe('Schedule ID'),
    }),
    execute: async ({ id }) => {
      const response = await fetch(`/schedule/${id}`, { method: 'GET' })
      return response.json()
    },
  })

  const createSchedule = tool({
    description: 'Create a new schedule',
    inputSchema: z.object({
      name: z.string(),
      description: z.string(),
      pattern: z.string(),
      max_calls: z.number().nullable().optional().default(null).describe('Max calls (optional, empty for unlimited)'),
      enabled: z.boolean().optional(),
      command: z.string(),
    }),
    execute: async (payload) => {
      const response = await fetch('/schedule', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      })
      return response.json()
    },
  })

  const updateSchedule = tool({
    description: 'Update an existing schedule',
    inputSchema: ScheduleSchema.partial().extend({
      id: z.string(),
    }),
    execute: async (payload) => {
      const { id, ...body } = payload
      const response = await fetch(`/schedule/${id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      })
      return response.json()
    },
  })

  const deleteSchedule = tool({
    description: 'Delete a schedule',
    inputSchema: z.object({
      id: z.string(),
    }),
    execute: async ({ id }) => {
      const response = await fetch(`/schedule/${id}`, { method: 'DELETE' })
      return response.status === 204 ? { success: true } : response.json()
    },
  })

  return {
    'schedule_list': listSchedules,
    'schedule_get': getSchedule,
    'schedule_create': createSchedule,
    'schedule_update': updateSchedule,
    'schedule_delete': deleteSchedule,
  }
}