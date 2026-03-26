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
  allow_concurrent?: boolean
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
  allow_concurrent?: boolean
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
export interface MCPServerPlan {
  name: string
  command: string
  args?: string[]
  env?: Record<string, string>
}

export interface WizardPlan {
  id: string
  description: string
  summary: string
  mcp_servers?: MCPServerPlan[]
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
  mcp_servers_created?: string[]
  domains_created?: string[]
  agents_created?: string[]
  tasks_created?: string[]
  pipelines_created?: string[]
  errors?: string[]
}

// Wizard troubleshooter types
export interface WizardDiagnosis {
  category: string
  message: string
  details?: string
  suggestions: RecoveryAction[]
}

export interface RecoveryAction {
  id: string
  label: string
  description: string
  patched_plan?: WizardPlan
}

export interface RetryContext {
  previous_error: string
  previous_raw_output?: string
  user_hint?: string
}

export interface TestRunResult {
  task_name: string
  output: string
  error?: string
  soft_failure?: string
  duration_ms: number
  cost_usd?: number
  diagnosis?: WizardDiagnosis
}

// Delete / Backup types
export interface Dependency {
  type: 'task' | 'pipeline' | 'subagent' | 'domain'
  name: string
}

export interface DeleteAnalysis {
  entity: Dependency
  used_by: Dependency[]
  can_delete: boolean
  cascade_items: Dependency[]
  blocked: boolean
  block_reason?: string
}

export interface DeleteResponse {
  backup_id: string
  deleted: string[]
}

export interface BackupEntry {
  id: string
  entity_type: string
  entity_name: string
  action: string
  parent_id: string
  created_at: string
  restored_at?: string
}
