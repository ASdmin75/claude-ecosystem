interface DashboardData {
  total_tasks: number
  total_pipelines: number
  total_executions: number
  running: number
  completed: number
  failed: number
}

interface Task {
  name: string
  prompt: string
  work_dir: string
  schedule?: string
  watch?: { paths: string[]; extensions: string[]; debounce: string }
  tags?: string[]
  model?: string
  timeout?: string
  agents?: string[]
  mcp_servers?: string[]
  allowed_tools?: string[]
  json_schema?: string
  max_turns?: number
  max_budget_usd?: number
  output_format?: string
}

interface SubAgent {
  name: string
  description: string
  tools?: string[]
  disallowed_tools?: string[]
  model?: string
  permission_mode?: string
  max_turns?: number
  mcp_servers?: string[]
  instructions: string
  file_path: string
}

interface Pipeline {
  name: string
  mode: string
  steps: { task: string }[]
  max_iterations: number
  stop_signal?: string
  collector?: string
}

interface Execution {
  id: string
  task_name: string
  pipeline_name?: string
  status: string
  trigger: string
  prompt: string
  output?: string
  error?: string
  model?: string
  cost_usd?: number
  duration_ms?: number
  session_id?: string
  started_at: string
  completed_at?: string
}

interface ExecutionResult {
  task_name: string
  output: string
  duration: number
  error?: string
  model?: string
  session_id?: string
  cost_usd?: number
}

interface MCPServer {
  name: string
  running: boolean
  pid?: number
}
