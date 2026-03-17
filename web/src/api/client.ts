import type { DashboardData, Task, SubAgent, Pipeline, Execution, ExecutionResult, MCPServer, WizardPlan, ApplyResult } from '../types'

const BASE = '/api/v1'

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

  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(body.error || res.statusText)
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

  // Sub-agents
  listSubAgents: () => request<SubAgent[]>('/subagents'),
  getSubAgent: (name: string) => request<SubAgent>(`/subagents/${name}`),
  createSubAgent: (agent: Partial<SubAgent>) =>
    request<SubAgent>('/subagents', { method: 'POST', body: JSON.stringify(agent) }),
  updateSubAgent: (name: string, agent: Partial<SubAgent>) =>
    request<SubAgent>(`/subagents/${name}`, { method: 'PUT', body: JSON.stringify(agent) }),
  deleteSubAgent: (name: string) =>
    request<void>(`/subagents/${name}`, { method: 'DELETE' }),

  // Pipelines
  listPipelines: () => request<Pipeline[]>('/pipelines'),
  createPipeline: (pipeline: Partial<Pipeline>) =>
    request<Pipeline>('/pipelines', { method: 'POST', body: JSON.stringify(pipeline) }),
  updatePipeline: (name: string, pipeline: Partial<Pipeline>) =>
    request<Pipeline>(`/pipelines/${name}`, { method: 'PUT', body: JSON.stringify(pipeline) }),
  deletePipeline: (name: string) =>
    request<void>(`/pipelines/${name}`, { method: 'DELETE' }),
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

  // Wizard
  wizardGenerate: (description: string, workDir?: string) =>
    request<WizardPlan>('/wizard/generate', {
      method: 'POST',
      body: JSON.stringify({ description, work_dir: workDir }),
    }),
  wizardGetPlan: (id: string) => request<WizardPlan>(`/wizard/plans/${id}`),
  wizardUpdatePlan: (id: string, plan: WizardPlan) =>
    request<WizardPlan>(`/wizard/plans/${id}`, {
      method: 'PUT',
      body: JSON.stringify(plan),
    }),
  wizardApply: (id: string) =>
    request<ApplyResult>(`/wizard/plans/${id}/apply`, { method: 'POST' }),
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
