import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { api } from '../api/client'
import { useState } from 'react'
import type { Task, DeleteAnalysis } from '../types'
import ConfirmModal from './ConfirmModal'

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
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null)
  const [deleteAnalysis, setDeleteAnalysis] = useState<DeleteAnalysis | null>(null)

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

  const deleteMutation = useMutation({
    mutationFn: (name: string) => api.deleteTask(name),
    onSuccess: () => {
      setEditing(null)
      setIsNew(false)
      setDeleteTarget(null)
      setDeleteAnalysis(null)
      queryClient.invalidateQueries({ queryKey: ['tasks'] })
      setResult('Task deleted successfully')
    },
    onError: (err) => { setDeleteTarget(null); setDeleteAnalysis(null); setResult(`Delete failed: ${err.message}`) },
  })

  function requestDelete(name: string) {
    setDeleteTarget(name)
    api.getTaskDeleteInfo(name).then(setDeleteAnalysis).catch(() => setDeleteAnalysis(null))
  }

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

  if (isLoading) return <p className="text-gray-500 dark:text-gray-400">Loading...</p>

  return (
    <div className="flex flex-col h-[calc(100vh-3rem)]">
      <div className="flex justify-between items-center mb-4 shrink-0">
        <h2 className="text-xl font-bold">Tasks</h2>
        <button onClick={editing ? close : startNew} className="px-3 py-1 bg-gray-900 dark:bg-gray-700 text-white text-sm rounded">
          {editing ? 'Cancel' : 'New Task'}
        </button>
      </div>
      {result && (
        <div className="text-sm mb-4 p-2 bg-gray-100 dark:bg-gray-800 rounded flex justify-between items-center shrink-0">
          <span>{result}</span>
          <button onClick={() => navigate('/executions')} className="text-blue-600 dark:text-blue-400 hover:underline text-xs ml-2">View Executions</button>
        </div>
      )}
      {saveError && (
        <p className="text-sm mb-4 p-2 bg-red-50 dark:bg-red-900/20 text-red-600 dark:text-red-400 rounded shrink-0">Save failed: {saveError.message}</p>
      )}

      <div className="flex gap-4 min-h-0 flex-1">
        <div className="w-1/2 overflow-y-auto">
          <div className="space-y-3">
            {tasks?.map((t) => (
              <div key={t.name} className={`bg-white dark:bg-gray-800 p-4 rounded-lg shadow dark:shadow-gray-950 flex justify-between items-start cursor-pointer hover:bg-gray-50 dark:hover:bg-gray-700/50 ${editing?.name === t.name && !isNew ? 'ring-2 ring-blue-300 dark:ring-blue-600' : ''}`}
                onClick={() => { setEditing({ ...t }); setIsNew(false) }}
              >
                <div className="flex-1">
                  <h3 className="font-semibold">{t.name}</h3>
                  <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
                    {t.schedule && <span className="mr-3">Schedule: {t.schedule}</span>}
                    {t.tags?.map((tag) => (
                      <span key={tag} className="inline-block bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-300 text-xs px-2 py-0.5 rounded mr-1">{tag}</span>
                    ))}
                  </p>
                </div>
                <div className="flex gap-2">
                  <button
                    onClick={(e) => { e.stopPropagation(); setRunning(t.name); runMutation.mutate(t.name) }}
                    disabled={running === t.name}
                    className="px-3 py-1 bg-gray-900 dark:bg-gray-700 text-white text-sm rounded hover:bg-gray-800 dark:hover:bg-gray-600 disabled:opacity-50"
                  >
                    {running === t.name ? 'Running...' : 'Run'}
                  </button>
                  <button
                    onClick={(e) => { e.stopPropagation(); setEditing({ ...t }); setIsNew(false) }}
                    className="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 text-sm"
                  >
                    Edit
                  </button>
                  <button
                    onClick={(e) => { e.stopPropagation(); requestDelete(t.name) }}
                    className="text-red-400 hover:text-red-600 dark:hover:text-red-300 text-sm"
                  >
                    Delete
                  </button>
                </div>
              </div>
            ))}
          </div>
        </div>

        <div className="w-1/2 overflow-y-auto">
          {editing ? (
            <TaskEditor
              task={editing}
              isNew={isNew}
              onChange={setEditing}
              onSave={handleSave}
              onCancel={close}
              saving={saving}
            />
          ) : (
            <div className="bg-white dark:bg-gray-800 rounded-lg shadow dark:shadow-gray-950 p-8 text-center text-gray-400 dark:text-gray-500">
              <p className="text-sm">Select a task to view or edit</p>
            </div>
          )}
        </div>
      </div>

      <ConfirmModal
        open={deleteTarget !== null}
        title={`Delete task "${deleteTarget}"?`}
        message="This will remove the task from the configuration. A backup will be created."
        details={deleteAnalysis}
        onConfirm={() => deleteTarget && deleteMutation.mutate(deleteTarget)}
        onCancel={() => { setDeleteTarget(null); setDeleteAnalysis(null) }}
        loading={deleteMutation.isPending}
      />
    </div>
  )
}

function csvToArray(s: string): string[] {
  return s.split(',').map((v) => v.trim()).filter(Boolean)
}

function TaskEditor({ task, isNew, onChange, onSave, onCancel, saving }: {
  task: Task
  isNew: boolean
  onChange: (t: Task) => void
  onSave: () => void
  onCancel: () => void
  saving: boolean
}) {
  const [csvFields, setCsvFields] = useState<Record<string, string>>({})
  const set = <K extends keyof Task>(key: K, value: Task[K]) =>
    onChange({ ...task, [key]: value })

  const csvValue = (key: 'tags' | 'agents' | 'mcp_servers' | 'allowed_tools') =>
    key in csvFields ? csvFields[key] : (task[key] || []).join(', ')

  const csvOnChange = (key: 'tags' | 'agents' | 'mcp_servers' | 'allowed_tools', raw: string) => {
    setCsvFields({ ...csvFields, [key]: raw })
    set(key, csvToArray(raw))
  }

  const inputClass = "w-full px-2 py-1 border border-gray-300 dark:border-gray-600 rounded text-sm bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100"
  const selectClass = inputClass

  return (
    <div className="bg-white dark:bg-gray-800 rounded-lg shadow dark:shadow-gray-950 p-4">
      <div className="flex justify-between items-center mb-4">
        <h3 className="font-bold text-lg">{isNew ? 'New Task' : task.name}</h3>
        <button onClick={onCancel} className="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 text-lg">&times;</button>
      </div>

      <form onSubmit={(e) => { e.preventDefault(); onSave() }} className="space-y-3">
        {isNew && (
          <Field label="Name">
            <input
              value={task.name}
              onChange={(e) => set('name', e.target.value)}
              className={inputClass}
              required
            />
          </Field>
        )}

        <Field label="Prompt">
          <textarea
            value={task.prompt}
            onChange={(e) => set('prompt', e.target.value)}
            rows={5}
            className={`${inputClass} font-mono`}
            required
          />
        </Field>

        <Field label="Working Directory">
          <input
            value={task.work_dir}
            onChange={(e) => set('work_dir', e.target.value)}
            className={inputClass}
          />
        </Field>

        <div className="grid grid-cols-2 gap-3">
          <Field label="Schedule (cron)">
            <input
              value={task.schedule || ''}
              onChange={(e) => set('schedule', e.target.value)}
              placeholder="0 9 * * 1-5"
              className={inputClass}
            />
          </Field>

          <Field label="Timeout">
            <input
              value={task.timeout || ''}
              onChange={(e) => set('timeout', e.target.value)}
              placeholder="30m"
              className={inputClass}
            />
          </Field>

          <Field label="Model">
            <input
              value={task.model || ''}
              onChange={(e) => set('model', e.target.value)}
              placeholder="claude-sonnet-4-6"
              className={inputClass}
            />
          </Field>

          <Field label="Output Format">
            <select
              value={task.output_format || ''}
              onChange={(e) => set('output_format', e.target.value)}
              className={selectClass}
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
              className={selectClass}
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
              className={inputClass}
            />
          </Field>

          <Field label="Max Budget (USD)">
            <input
              type="number"
              step="0.01"
              value={task.max_budget_usd || ''}
              onChange={(e) => set('max_budget_usd', e.target.value ? Number(e.target.value) : 0)}
              className={inputClass}
            />
          </Field>

          <Field label="Concurrency">
            <label className="flex items-center gap-2 text-sm cursor-pointer">
              <input
                type="checkbox"
                checked={task.allow_concurrent !== false}
                onChange={(e) => set('allow_concurrent', e.target.checked ? undefined : false)}
                className="rounded border-gray-300 dark:border-gray-600"
              />
              Allow concurrent runs
            </label>
          </Field>
        </div>

        <Field label="Tags (comma-separated)">
          <input
            value={csvValue('tags')}
            onChange={(e) => csvOnChange('tags', e.target.value)}
            className={inputClass}
          />
        </Field>

        <Field label="Agents (comma-separated)">
          <input
            value={csvValue('agents')}
            onChange={(e) => csvOnChange('agents', e.target.value)}
            className={inputClass}
          />
        </Field>

        <Field label="MCP Servers (comma-separated)">
          <input
            value={csvValue('mcp_servers')}
            onChange={(e) => csvOnChange('mcp_servers', e.target.value)}
            className={inputClass}
          />
        </Field>

        <Field label="Allowed Tools (comma-separated)">
          <input
            value={csvValue('allowed_tools')}
            onChange={(e) => csvOnChange('allowed_tools', e.target.value)}
            className={inputClass}
          />
        </Field>

        <Field label="Append System Prompt">
          <textarea
            value={task.append_system_prompt || ''}
            onChange={(e) => set('append_system_prompt', e.target.value)}
            rows={3}
            className={`${inputClass} font-mono`}
          />
        </Field>

        <div className="flex gap-2 pt-2">
          <button
            type="submit"
            disabled={saving}
            className="px-4 py-1.5 bg-gray-900 dark:bg-gray-700 text-white text-sm rounded hover:bg-gray-800 dark:hover:bg-gray-600 disabled:opacity-50"
          >
            {saving ? 'Saving...' : isNew ? 'Create' : 'Save'}
          </button>
          <button
            type="button"
            onClick={onCancel}
            className="px-4 py-1.5 bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-300 text-sm rounded hover:bg-gray-200 dark:hover:bg-gray-600"
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
      <label className="block text-xs font-medium text-gray-500 dark:text-gray-400 mb-1">{label}</label>
      {children}
    </div>
  )
}
