export interface ConvMeta {
  id: string; title: string; work_dir: string; model?: string
  pinned?: boolean; tags?: string[]
  created_at: string; updated_at: string
}
export interface SavedConv extends ConvMeta {
  messages: { role: string; content: string }[]
  display_items?: any[]
}
export interface PermReq {
  id: string; tool: string; description: string; args: Record<string, unknown>
  workspace: string; operation: string; target: string; impact: string
  sensitive: boolean; scope: string; preview?: string
}
export interface Grant { id: string; workspace: string; tool: string; label: string; scope: string }
export interface ToolInfo { name: string; description: string }

export type MsgRole = 'user' | 'assistant'
export interface ChatMsg { id: number; role: MsgRole; text: string; streaming?: boolean }
export interface ToolEvt { id: number; kind: 'call' | 'result'; tool: string; body: string }
export type Item =
  | { type: 'msg';   data: ChatMsg }
  | { type: 'tool';  data: ToolEvt }
  | { type: 'error'; id: number; text: string }
  | { type: 'status'; id: number; text: string }

export interface ChatEvent { conversation_id: string; run_id: string; seq: number; data: any }


export type PermMode = 'ask' | 'allow' | 'deny'

export interface MCPServer {
  name: string
  command: string
  args: string[]
  env: Record<string, string>
  enabled: boolean
}

export interface Cfg {
  provider: string; base_url: string; api_key: string
  model: string; work_dir: string
  tool_permissions: Record<string, PermMode>
  whisper_model: string
  system_prompt: string
  context_limit: number
  mcp_servers: MCPServer[]
  notifications: boolean
  search_provider: string
  sandbox_mode: string
  tool_calling: string
  backup_retention: number
  context_strategy: string
  max_rounds: number
  max_parallel_tools: number
  tool_timeout_seconds: number
  sandbox_network: boolean
  search_api_key: string
  theme: string
}

export interface ContextUsage { used: number; limit: number; total?: number; estimated?: boolean; omitted?: number; summarised?: number }
export interface UpdateInfo { available: boolean; current: string; latest: string; url: string }
