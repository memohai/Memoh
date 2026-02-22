export interface Schedule {
  id: string
  name: string
  description: string
  pattern: string
  maxCalls?: number | null
  command: string
}
