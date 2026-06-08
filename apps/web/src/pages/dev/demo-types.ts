export interface DemoBlock {
  id: number
  kind: 'thinking' | 'text' | 'tool'
  content?: string
  toolName?: string
  status?: 'running' | 'completed'
  output?: string
}

export interface DemoTurn {
  id: string
  serverId?: string
  role: 'user' | 'assistant'
  text?: string
  blocks?: DemoBlock[]
  ts: string
}

export interface DemoSession {
  id: string
  title: string
  updated_at?: string
}
