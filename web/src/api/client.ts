import type { DashboardData, Task, SubAgent, Pipeline, Execution, ExecutionResult, MCPServer, WizardPlan, ApplyResult, DeleteAnalysis, DeleteResponse, BackupEntry, WizardDiagnosis, RetryContext, TestRunResult } from '../types'

const BASE = '/api/v1'

// DiagnosisError is thrown when the server returns a 422 with a WizardDiagnosis.
export class DiagnosisError extends Error {
  diagnosis: WizardDiagnosis
  constructor(diagnosis: WizardDiagnosis) {
    super(diagnosis.message)
    this.name = 'DiagnosisError'
    this.diagnosis = diagnosis
  }
}

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const token = localStorage.getItem('token')
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(token ? { Authorization: `Bearer ${token}` } : {}),
    ...(options?.headers as Record<string, string> || {}),
  }

  const res = await fetch(`${BASE}${path}`, { ...options, headers, cache: 'no-store' })

  if (res.status === 401) {
    localStorage.removeItem('token')
    window.location.href = '/login'
    throw new Error('Unauthorized')
  }

  // Structured diagnosis from wizard endpoints
  if (res.status === 422) {
    const body = await res.json().catch(() => null)
    if (body && body.category) {
      throw new DiagnosisError(body as WizardDiagnosis)
    }
  }

  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }))
    // Avoid exposing internal server details to the UI.
    const msg = body.error || res.statusText
    if (res.status >= 500) {
      throw new Error('Internal server error')
    }
    throw new Error(msg)
  }

  return res.json()
}

export const api = {
  // Auth
  login: (username: string, password: string) =>
    request<{ token: string; expires_at: string }>('/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    }),

  // Dashboard
  dashboard: () => request<DashboardData>('/dashboard'),

  // Tasks
  listTasks: () => request<Task[]>('/tasks'),
  createTask: (task: Partial<Task>) =>
    request<Task>('/tasks', { method: 'POST', body: JSON.stringify(task) }),
  getTask: (name: string) => request<Task>(`/tasks/${name}`),
  updateTask: (name: string, task: Partial<Task>) =>
    request<Task>(`/tasks/${name}`, { method: 'PUT', body: JSON.stringify(task) }),
  runTask: (name: string, vars?: Record<string, string>) =>
    request<ExecutionResult>(`/tasks/${name}/run`, {
      method: 'POST',
      body: JSON.stringify({ template_vars: vars }),
    }),
  runTaskAsync: (name: string, vars?: Record<string, string>) =>
    request<{ execution_id: string }>(`/tasks/${name}/run-async`, {
      method: 'POST',
      body: JSON.stringify({ template_vars: vars }),
    }),
  deleteTask: (name: string) =>
    request<DeleteResponse>(`/tasks/${name}`, { method: 'DELETE' }),
  getTaskDeleteInfo: (name: string) =>
    request<DeleteAnalysis>(`/tasks/${name}/delete-info`),

  // Sub-agents
  listSubAgents: () => request<SubAgent[]>('/subagents'),
  getSubAgent: (name: string) => request<SubAgent>(`/subagents/${name}`),
  createSubAgent: (agent: Partial<SubAgent>) =>
    request<SubAgent>('/subagents', { method: 'POST', body: JSON.stringify(agent) }),
  updateSubAgent: (name: string, agent: Partial<SubAgent>) =>
    request<SubAgent>(`/subagents/${name}`, { method: 'PUT', body: JSON.stringify(agent) }),
  deleteSubAgent: (name: string) =>
    request<DeleteResponse>(`/subagents/${name}`, { method: 'DELETE' }),
  getSubAgentDeleteInfo: (name: string) =>
    request<DeleteAnalysis>(`/subagents/${name}/delete-info`),

  // Pipelines
  listPipelines: () => request<Pipeline[]>('/pipelines'),
  createPipeline: (pipeline: Partial<Pipeline>) =>
    request<Pipeline>('/pipelines', { method: 'POST', body: JSON.stringify(pipeline) }),
  updatePipeline: (name: string, pipeline: Partial<Pipeline>) =>
    request<Pipeline>(`/pipelines/${name}`, { method: 'PUT', body: JSON.stringify(pipeline) }),
  deletePipeline: (name: string) =>
    request<DeleteResponse>(`/pipelines/${name}`, { method: 'DELETE' }),
  getPipelineDeleteInfo: (name: string) =>
    request<DeleteAnalysis>(`/pipelines/${name}/delete-info`),
  runPipeline: (name: string) =>
    request<ExecutionResult>(`/pipelines/${name}/run`, { method: 'POST' }),
  runPipelineAsync: (name: string) =>
    request<{ execution_id: string }>(`/pipelines/${name}/run-async`, { method: 'POST' }),

  // Executions
  listExecutions: (params?: Record<string, string>) => {
    const qs = params ? '?' + new URLSearchParams(params).toString() : ''
    return request<Execution[]>(`/executions${qs}`)
  },
  getExecution: (id: string) => request<Execution>(`/executions/${id}`),
  cancelExecution: (id: string) =>
    request<{ status: string }>(`/executions/${id}/cancel`, { method: 'POST' }),
  deleteExecution: (id: string) =>
    request<{ status: string }>(`/executions/${id}`, { method: 'DELETE' }),

  // MCP Servers
  listMCPServers: () => request<MCPServer[]>('/mcp-servers'),
  startMCPServer: (name: string) =>
    request<void>(`/mcp-servers/${name}/start`, { method: 'POST' }),
  stopMCPServer: (name: string) =>
    request<void>(`/mcp-servers/${name}/stop`, { method: 'POST' }),

  // Backups
  listBackups: () => request<BackupEntry[]>('/backups'),
  getBackup: (id: string) => request<BackupEntry>(`/backups/${id}`),
  restoreBackup: (id: string) =>
    request<{ status: string }>(`/backups/${id}/restore`, { method: 'POST' }),

  // Wizard
  wizardGenerate: (description: string, workDir?: string, retryContext?: RetryContext) =>
    request<WizardPlan>('/wizard/generate', {
      method: 'POST',
      body: JSON.stringify({ description, work_dir: workDir, retry_context: retryContext }),
    }),
  wizardGetPlan: (id: string) => request<WizardPlan>(`/wizard/plans/${id}`),
  wizardUpdatePlan: (id: string, plan: WizardPlan) =>
    request<WizardPlan>(`/wizard/plans/${id}`, {
      method: 'PUT',
      body: JSON.stringify(plan),
    }),
  wizardApply: (id: string) =>
    request<ApplyResult>(`/wizard/plans/${id}/apply`, { method: 'POST' }),
  wizardValidate: (id: string) =>
    request<{ valid: boolean; warnings?: string[] }>(`/wizard/plans/${id}/validate`, {
      method: 'POST',
    }),
  wizardTestRun: (id: string, taskName: string) =>
    request<TestRunResult>(`/wizard/plans/${id}/test`, {
      method: 'POST',
      body: JSON.stringify({ task_name: taskName }),
    }),
  wizardDiscard: (id: string) =>
    request<void>(`/wizard/plans/${id}`, { method: 'DELETE' }),
}

// SSE helper for per-execution streaming
export function streamExecution(id: string, onMessage: (data: string) => void): () => void {
  const token = localStorage.getItem('token')
  const es = new EventSource(`${BASE}/executions/${id}/stream?token=${token}`)

  // Listen to named SSE events (task.output, task.completed, pipeline.completed)
  const eventTypes = ['task.output', 'task.completed', 'pipeline.completed']
  for (const type of eventTypes) {
    es.addEventListener(type, (e: MessageEvent) => {
      onMessage(e.data)
      if (type === 'task.completed' || type === 'pipeline.completed') {
        es.close()
      }
    })
  }

  // Fallback for unnamed events
  es.onmessage = (e) => onMessage(e.data)
  es.onerror = () => es.close()
  return () => es.close()
}
