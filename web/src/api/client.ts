const BASE = '/api/v1'

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const token = localStorage.getItem('token')
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(token ? { Authorization: `Bearer ${token}` } : {}),
    ...(options?.headers as Record<string, string> || {}),
  }

  const res = await fetch(`${BASE}${path}`, { ...options, headers })

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
  getTask: (name: string) => request<Task>(`/tasks/${name}`),
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

  // MCP Servers
  listMCPServers: () => request<MCPServer[]>('/mcp-servers'),
  startMCPServer: (name: string) =>
    request<void>(`/mcp-servers/${name}/start`, { method: 'POST' }),
  stopMCPServer: (name: string) =>
    request<void>(`/mcp-servers/${name}/stop`, { method: 'POST' }),
}

// SSE helper
export function streamExecution(id: string, onMessage: (data: string) => void): () => void {
  const token = localStorage.getItem('token')
  const es = new EventSource(`${BASE}/executions/${id}/stream?token=${token}`)
  es.onmessage = (e) => onMessage(e.data)
  es.onerror = () => es.close()
  return () => es.close()
}
