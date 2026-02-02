export interface Schedule {
  id?: string
  pattern: string
  name: string
  description: string
  command: string
  maxCalls?: number | null
}