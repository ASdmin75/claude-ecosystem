import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { api } from '../api/client'
import { useState } from 'react'
import type { Task } from '../types'

const emptyTask: Task = {
  name: '', prompt: '', work_dir: '.',
}

export default function TaskList() {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const { data: tasks, isLoading } = useQuery({ queryKey: ['tasks'], queryFn: api.listTasks })
  const [running, setRunning] = useState<string | null>(null)
  const [result, setResult] = useState<string | null>(null)
  const [editing, setEditing] = useState<Task | null>(null)
  const [isNew, setIsNew] = useState(false)

  const runMutation = useMutation({
    mutationFn: (name: string) => api.runTaskAsync(name),
    onSuccess: (data) => {
      setRunning(null)
      setResult(`Execution started: ${data.execution_id}`)
      queryClient.invalidateQueries({ queryKey: ['executions'] })
    },
    onError: (err) => { setRunning(null); setResult(`Error: ${err.message}`) },
  })

  const createMutation = useMutation({
    mutationFn: (task: Task) => api.createTask(task),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['tasks'] })
      close()
    },
  })

  const updateMutation = useMutation({
    mutationFn: (task: Task) => api.updateTask(task.name, task),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['tasks'] })
      close()
    },
  })

  function close() { setEditing(null); setIsNew(false) }

  function startNew() {
    setEditing({ ...emptyTask })
    setIsNew(true)
  }

  function handleSave() {
    if (!editing) return
    if (isNew) createMutation.mutate(editing)
    else updateMutation.mutate(editing)
  }

  const saving = createMutation.isPending || updateMutation.isPending
  const saveError = createMutation.error || updateMutation.error

  if (isLoading) return <p className="text-gray-500">Loading...</p>

  return (
    <div>
      <div className="flex justify-between items-center mb-4">
        <h2 className="text-xl font-bold">Tasks</h2>
        <button onClick={editing ? close : startNew} className="px-3 py-1 bg-gray-900 text-white text-sm rounded">
          {editing ? 'Cancel' : 'New Task'}
        </button>
      </div>
      {result && (
        <div className="text-sm mb-4 p-2 bg-gray-100 rounded flex justify-between items-center">
          <span>{result}</span>
          <button onClick={() => navigate('/executions')} className="text-blue-600 hover:underline text-xs ml-2">View Executions</button>
        </div>
      )}
      {saveError && (
        <p className="text-sm mb-4 p-2 bg-red-50 text-red-600 rounded">Save failed: {saveError.message}</p>
      )}

      <div className="flex gap-4">
        <div className={editing ? 'w-1/2' : 'w-full'}>
          <div className="space-y-3">
            {tasks?.map((t) => (
              <div key={t.name} className={`bg-white p-4 rounded-lg shadow flex justify-between items-start ${editing?.name === t.name && !isNew ? 'ring-2 ring-blue-300' : ''}`}>
                <div
                  className="flex-1 cursor-pointer"
                  onClick={() => { setEditing({ ...t }); setIsNew(false) }}
                >
                  <h3 className="font-semibold">{t.name}</h3>
                  <p className="text-sm text-gray-500 mt-1">
                    {t.schedule && <span className="mr-3">Schedule: {t.schedule}</span>}
                    {t.tags?.map((tag) => (
                      <span key={tag} className="inline-block bg-gray-100 text-gray-600 text-xs px-2 py-0.5 rounded mr-1">{tag}</span>
                    ))}
                  </p>
                </div>
                <div className="flex gap-2">
                  <button
                    onClick={() => { setEditing({ ...t }); setIsNew(false) }}
                    className="px-3 py-1 bg-gray-200 text-gray-700 text-sm rounded hover:bg-gray-300"
                  >
                    Edit
                  </button>
                  <button
                    onClick={() => { setRunning(t.name); runMutation.mutate(t.name) }}
                    disabled={running === t.name}
                    className="px-3 py-1 bg-gray-900 text-white text-sm rounded hover:bg-gray-800 disabled:opacity-50"
                  >
                    {running === t.name ? 'Running...' : 'Run'}
                  </button>
                </div>
              </div>
            ))}
          </div>
        </div>

        {editing && (
          <div className="w-1/2">
            <TaskEditor
              task={editing}
              isNew={isNew}
              onChange={setEditing}
              onSave={handleSave}
              onCancel={close}
              saving={saving}
            />
          </div>
        )}
      </div>
    </div>
  )
}

function TaskEditor({ task, isNew, onChange, onSave, onCancel, saving }: {
  task: Task
  isNew: boolean
  onChange: (t: Task) => void
  onSave: () => void
  onCancel: () => void
  saving: boolean
}) {
  const set = <K extends keyof Task>(key: K, value: Task[K]) =>
    onChange({ ...task, [key]: value })

  return (
    <div className="bg-white rounded-lg shadow p-4">
      <div className="flex justify-between items-center mb-4">
        <h3 className="font-bold text-lg">{isNew ? 'New Task' : task.name}</h3>
        <button onClick={onCancel} className="text-gray-400 hover:text-gray-600 text-lg">&times;</button>
      </div>

      <form onSubmit={(e) => { e.preventDefault(); onSave() }} className="space-y-3">
        {isNew && (
          <Field label="Name">
            <input
              value={task.name}
              onChange={(e) => set('name', e.target.value)}
              className="w-full px-2 py-1 border rounded text-sm"
              required
            />
          </Field>
        )}

        <Field label="Prompt">
          <textarea
            value={task.prompt}
            onChange={(e) => set('prompt', e.target.value)}
            rows={5}
            className="w-full px-2 py-1 border rounded text-sm font-mono"
            required
          />
        </Field>

        <Field label="Working Directory">
          <input
            value={task.work_dir}
            onChange={(e) => set('work_dir', e.target.value)}
            className="w-full px-2 py-1 border rounded text-sm"
          />
        </Field>

        <div className="grid grid-cols-2 gap-3">
          <Field label="Schedule (cron)">
            <input
              value={task.schedule || ''}
              onChange={(e) => set('schedule', e.target.value)}
              placeholder="0 9 * * 1-5"
              className="w-full px-2 py-1 border rounded text-sm"
            />
          </Field>

          <Field label="Timeout">
            <input
              value={task.timeout || ''}
              onChange={(e) => set('timeout', e.target.value)}
              placeholder="30m"
              className="w-full px-2 py-1 border rounded text-sm"
            />
          </Field>

          <Field label="Model">
            <input
              value={task.model || ''}
              onChange={(e) => set('model', e.target.value)}
              placeholder="claude-sonnet-4-6"
              className="w-full px-2 py-1 border rounded text-sm"
            />
          </Field>

          <Field label="Output Format">
            <select
              value={task.output_format || ''}
              onChange={(e) => set('output_format', e.target.value)}
              className="w-full px-2 py-1 border rounded text-sm"
            >
              <option value="">default</option>
              <option value="text">text</option>
              <option value="json">json</option>
              <option value="stream-json">stream-json</option>
            </select>
          </Field>

          <Field label="Permission Mode">
            <select
              value={task.permission_mode || ''}
              onChange={(e) => set('permission_mode', e.target.value)}
              className="w-full px-2 py-1 border rounded text-sm"
            >
              <option value="">default</option>
              <option value="default">default</option>
              <option value="acceptEdits">acceptEdits</option>
              <option value="dontAsk">dontAsk</option>
              <option value="bypassPermissions">bypassPermissions</option>
              <option value="plan">plan (read-only)</option>
              <option value="auto">auto</option>
            </select>
          </Field>

          <Field label="Max Turns">
            <input
              type="number"
              value={task.max_turns || ''}
              onChange={(e) => set('max_turns', e.target.value ? Number(e.target.value) : 0)}
              className="w-full px-2 py-1 border rounded text-sm"
            />
          </Field>

          <Field label="Max Budget (USD)">
            <input
              type="number"
              step="0.01"
              value={task.max_budget_usd || ''}
              onChange={(e) => set('max_budget_usd', e.target.value ? Number(e.target.value) : 0)}
              className="w-full px-2 py-1 border rounded text-sm"
            />
          </Field>
        </div>

        <Field label="Tags (comma-separated)">
          <input
            value={(task.tags || []).join(', ')}
            onChange={(e) => set('tags', e.target.value.split(',').map((s) => s.trim()).filter(Boolean))}
            className="w-full px-2 py-1 border rounded text-sm"
          />
        </Field>

        <Field label="Agents (comma-separated)">
          <input
            value={(task.agents || []).join(', ')}
            onChange={(e) => set('agents', e.target.value.split(',').map((s) => s.trim()).filter(Boolean))}
            className="w-full px-2 py-1 border rounded text-sm"
          />
        </Field>

        <Field label="MCP Servers (comma-separated)">
          <input
            value={(task.mcp_servers || []).join(', ')}
            onChange={(e) => set('mcp_servers', e.target.value.split(',').map((s) => s.trim()).filter(Boolean))}
            className="w-full px-2 py-1 border rounded text-sm"
          />
        </Field>

        <Field label="Allowed Tools (comma-separated)">
          <input
            value={(task.allowed_tools || []).join(', ')}
            onChange={(e) => set('allowed_tools', e.target.value.split(',').map((s) => s.trim()).filter(Boolean))}
            className="w-full px-2 py-1 border rounded text-sm"
          />
        </Field>

        <Field label="Append System Prompt">
          <textarea
            value={task.append_system_prompt || ''}
            onChange={(e) => set('append_system_prompt', e.target.value)}
            rows={3}
            className="w-full px-2 py-1 border rounded text-sm font-mono"
          />
        </Field>

        <div className="flex gap-2 pt-2">
          <button
            type="submit"
            disabled={saving}
            className="px-4 py-1.5 bg-gray-900 text-white text-sm rounded hover:bg-gray-800 disabled:opacity-50"
          >
            {saving ? 'Saving...' : isNew ? 'Create' : 'Save'}
          </button>
          <button
            type="button"
            onClick={onCancel}
            className="px-4 py-1.5 bg-gray-100 text-gray-700 text-sm rounded hover:bg-gray-200"
          >
            Cancel
          </button>
        </div>
      </form>
    </div>
  )
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <label className="block text-xs font-medium text-gray-500 mb-1">{label}</label>
      {children}
    </div>
  )
}
