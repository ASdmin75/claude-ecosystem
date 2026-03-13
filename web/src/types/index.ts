export interface DashboardData {
  total_tasks: number
  total_pipelines: number
  total_executions: number
  running: number
  completed: number
  failed: number
}

export interface NotifyConfig {
  email?: string[]
  webhook?: string
  trigger?: 'on_success' | 'on_failure' | 'always'
}

export interface Task {
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
  append_system_prompt?: string
  max_turns?: number
  max_budget_usd?: number
  output_format?: string
  permission_mode?: string
  notify?: NotifyConfig
}

export interface SubAgent {
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
  scope: string
}

export interface Pipeline {
  name: string
  mode: string
  steps: { task: string }[]
  max_iterations: number
  stop_signal?: string
  collector?: string
  schedule?: string
}

export interface Execution {
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

export interface ExecutionResult {
  task_name: string
  output: string
  duration: number
  error?: string
  model?: string
  session_id?: string
  cost_usd?: number
}

export interface MCPServer {
  name: string
  running: boolean
  pid?: number
}

// Wizard types
export interface WizardPlan {
  id: string
  description: string
  summary: string
  domains?: DomainPlan[]
  agents?: AgentPlan[]
  tasks?: TaskPlan[]
  pipelines?: PipelinePlan[]
  status: 'draft' | 'applied' | 'discarded'
}

export interface DomainPlan {
  name: string
  description?: string
  data_dir: string
  db?: string
  schema?: string
  domain_doc?: string
  tasks?: string[]
  pipelines?: string[]
  agents?: string[]
  mcp_servers?: string[]
}

export interface AgentPlan {
  name: string
  description: string
  tools?: string[]
  model?: string
  permission_mode?: string
  instructions: string
  scope?: 'user' | 'project'
}

export interface TaskPlan {
  name: string
  prompt: string
  work_dir?: string
  schedule?: string
  tags?: string[]
  model?: string
  timeout?: string
  agents?: string[]
  mcp_servers?: string[]
  allowed_tools?: string[]
  max_turns?: number
  max_budget_usd?: number
  permission_mode?: string
  domain?: string
}

export interface PipelinePlan {
  name: string
  mode?: 'sequential' | 'parallel'
  steps: string[]
  max_iterations?: number
  stop_signal?: string
}

export interface ApplyResult {
  domains_created?: string[]
  agents_created?: string[]
  tasks_created?: string[]
  pipelines_created?: string[]
  errors?: string[]
}
