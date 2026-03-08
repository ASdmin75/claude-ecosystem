import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import Markdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { api } from '../api/client'
import { useState } from 'react'
import type { SubAgent } from '../types'

const emptyForm: Partial<SubAgent> = {
  name: '', description: '', instructions: '', model: '', scope: 'user',
  tools: [], disallowed_tools: [], permission_mode: '', max_turns: 0, mcp_servers: [],
}

export default function SubAgentList() {
  const queryClient = useQueryClient()
  const { data: agents, isLoading } = useQuery({ queryKey: ['subagents'], queryFn: api.listSubAgents })
  const [editing, setEditing] = useState<Partial<SubAgent> | null>(null)
  const [isNew, setIsNew] = useState(false)

  const createMutation = useMutation({
    mutationFn: (agent: Partial<SubAgent>) => api.createSubAgent(agent),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['subagents'] }); close() },
  })

  const updateMutation = useMutation({
    mutationFn: (agent: Partial<SubAgent>) => api.updateSubAgent(agent.name!, agent),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['subagents'] }); close() },
  })

  const deleteMutation = useMutation({
    mutationFn: (name: string) => {
      if (!window.confirm(`Delete sub-agent "${name}"?`)) throw new Error('cancelled')
      return api.deleteSubAgent(name)
    },
    onSuccess: () => {
      setEditing(null)
      setIsNew(false)
      queryClient.invalidateQueries({ queryKey: ['subagents'] })
    },
  })

  function close() { setEditing(null); setIsNew(false) }

  function startNew() {
    setEditing({ ...emptyForm })
    setIsNew(true)
  }

  function startEdit(agent: SubAgent) {
    setEditing({ ...agent })
    setIsNew(false)
  }

  function handleSave() {
    if (!editing) return
    if (isNew) createMutation.mutate(editing)
    else updateMutation.mutate(editing)
  }

  const saving = createMutation.isPending || updateMutation.isPending
  const error = createMutation.error || updateMutation.error

  if (isLoading) return <p className="text-gray-500 dark:text-gray-400">Loading...</p>

  return (
    <div>
      <div className="flex justify-between items-center mb-4">
        <h2 className="text-xl font-bold">Sub-Agents</h2>
        <button onClick={editing ? close : startNew} className="px-3 py-1 bg-gray-900 dark:bg-gray-700 text-white text-sm rounded">
          {editing ? 'Cancel' : 'New Sub-Agent'}
        </button>
      </div>

      {error && <p className="text-sm mb-4 p-2 bg-red-50 dark:bg-red-900/20 text-red-600 dark:text-red-400 rounded">{error.message}</p>}

      <div className="flex gap-4">
        <div className={editing ? 'w-1/2' : 'w-full'}>
          <div className="space-y-3">
            {agents?.map((a) => (
              <div
                key={a.name}
                onClick={() => startEdit(a)}
                className={`bg-white dark:bg-gray-800 p-4 rounded-lg shadow dark:shadow-gray-950 cursor-pointer hover:bg-gray-50 dark:hover:bg-gray-700/50 ${editing?.name === a.name && !isNew ? 'ring-2 ring-blue-300 dark:ring-blue-600' : ''}`}
              >
                <div className="flex justify-between items-start">
                  <div className="flex-1">
                    <div className="flex items-center gap-2">
                      <h3 className="font-semibold">{a.name}</h3>
                      <span className={`text-xs px-1.5 py-0.5 rounded ${a.scope === 'project' ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300' : 'bg-green-100 text-green-700 dark:bg-green-900/40 dark:text-green-300'}`}>
                        {a.scope || 'user'}
                      </span>
                    </div>
                    <div className="text-sm text-gray-500 dark:text-gray-400 prose prose-sm prose-gray dark:prose-invert max-w-none line-clamp-3 overflow-hidden"><Markdown remarkPlugins={[remarkGfm]}>{a.description.replace(/\\n/g, '\n')}</Markdown></div>
                    <div className="flex gap-3 mt-2 text-xs text-gray-400 dark:text-gray-500">
                      {a.model && <span>Model: {a.model}</span>}
                      {a.permission_mode && <span>Mode: {a.permission_mode}</span>}
                      {a.max_turns ? <span>Max turns: {a.max_turns}</span> : null}
                      {a.tools && a.tools.length > 0 && <span>Tools: {a.tools.length}</span>}
                    </div>
                  </div>
                  <div className="flex gap-2">
                    <button onClick={(e) => { e.stopPropagation(); startEdit(a) }} className="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 text-sm">Edit</button>
                    <button onClick={(e) => { e.stopPropagation(); deleteMutation.mutate(a.name) }} className="text-red-400 hover:text-red-600 dark:hover:text-red-300 text-sm">Delete</button>
                  </div>
                </div>
              </div>
            ))}
            {agents?.length === 0 && !editing && <p className="text-gray-400 dark:text-gray-500 text-sm">No sub-agents configured.</p>}
          </div>
        </div>

        {editing && (
          <div className="w-1/2">
            <AgentEditor
              agent={editing}
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

function AgentEditor({ agent, isNew, onChange, onSave, onCancel, saving }: {
  agent: Partial<SubAgent>
  isNew: boolean
  onChange: (a: Partial<SubAgent>) => void
  onSave: () => void
  onCancel: () => void
  saving: boolean
}) {
  const [preview, setPreview] = useState(false)
  const [csvFields, setCsvFields] = useState<Record<string, string>>({})
  const set = (key: string, value: unknown) => onChange({ ...agent, [key]: value })
  const csvToArray = (s: string) => s.split(',').map((v) => v.trim()).filter(Boolean)

  const csvValue = (key: 'tools' | 'disallowed_tools' | 'mcp_servers') =>
    key in csvFields ? csvFields[key] : ((agent[key] as string[]) || []).join(', ')

  const csvOnChange = (key: 'tools' | 'disallowed_tools' | 'mcp_servers', raw: string) => {
    setCsvFields({ ...csvFields, [key]: raw })
    set(key, csvToArray(raw))
  }

  const inputClass = "w-full px-2 py-1 border border-gray-300 dark:border-gray-600 rounded text-sm bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100"
  const selectClass = inputClass

  return (
    <div className="bg-white dark:bg-gray-800 rounded-lg shadow dark:shadow-gray-950 p-4">
      <div className="flex justify-between items-center mb-4">
        <h3 className="font-bold text-lg">{isNew ? 'New Sub-Agent' : agent.name}</h3>
        <button onClick={onCancel} className="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 text-lg">&times;</button>
      </div>

      <form onSubmit={(e) => { e.preventDefault(); onSave() }} className="space-y-3">
        {isNew && (
          <>
            <Field label="Name">
              <input
                value={agent.name || ''}
                onChange={(e) => set('name', e.target.value)}
                className={inputClass}
                required
              />
            </Field>
            <Field label="Scope">
              <div className="flex gap-3">
                <label className="flex items-center gap-1.5 text-sm">
                  <input
                    type="radio"
                    name="scope"
                    value="user"
                    checked={(agent.scope || 'user') === 'user'}
                    onChange={() => set('scope', 'user')}
                  />
                  <span>User</span>
                  <span className="text-xs text-gray-400 dark:text-gray-500">(~/.claude/agents/)</span>
                </label>
                <label className="flex items-center gap-1.5 text-sm">
                  <input
                    type="radio"
                    name="scope"
                    value="project"
                    checked={agent.scope === 'project'}
                    onChange={() => set('scope', 'project')}
                  />
                  <span>Project</span>
                  <span className="text-xs text-gray-400 dark:text-gray-500">(.claude/agents/)</span>
                </label>
              </div>
            </Field>
          </>
        )}

        {!isNew && (
          <div className="text-xs text-gray-400 dark:text-gray-500 mb-2">
            Scope: <span className={`px-1.5 py-0.5 rounded ${agent.scope === 'project' ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300' : 'bg-green-100 text-green-700 dark:bg-green-900/40 dark:text-green-300'}`}>{agent.scope || 'user'}</span>
          </div>
        )}

        <Field label="Description">
          <input
            value={agent.description || ''}
            onChange={(e) => set('description', e.target.value)}
            className={inputClass}
            required
          />
        </Field>

        <Field label={
          <span className="flex items-center gap-2">
            Instructions (markdown)
            <button type="button" onClick={() => setPreview(!preview)} className="text-blue-500 dark:text-blue-400 hover:text-blue-700 dark:hover:text-blue-300 text-xs font-normal">
              {preview ? 'Edit' : 'Preview'}
            </button>
          </span>
        }>
          {preview ? (
            <div className="w-full border border-gray-300 dark:border-gray-600 rounded p-3 text-sm prose prose-sm prose-gray dark:prose-invert max-w-none overflow-auto max-h-80 bg-gray-50 dark:bg-gray-900">
              <Markdown remarkPlugins={[remarkGfm]}>{agent.instructions || '*No instructions*'}</Markdown>
            </div>
          ) : (
            <textarea
              value={agent.instructions || ''}
              onChange={(e) => set('instructions', e.target.value)}
              rows={8}
              className={`${inputClass} font-mono`}
            />
          )}
        </Field>

        <div className="grid grid-cols-2 gap-3">
          <Field label="Model">
            <input
              value={agent.model || ''}
              onChange={(e) => set('model', e.target.value)}
              placeholder="claude-sonnet-4-6"
              className={inputClass}
            />
          </Field>

          <Field label="Permission Mode">
            <select
              value={agent.permission_mode || ''}
              onChange={(e) => set('permission_mode', e.target.value)}
              className={selectClass}
            >
              <option value="">default</option>
              <option value="default">default</option>
              <option value="acceptEdits">acceptEdits</option>
              <option value="dontAsk">dontAsk</option>
              <option value="bypassPermissions">bypassPermissions</option>
              <option value="plan">plan</option>
              <option value="auto">auto</option>
            </select>
          </Field>

          <Field label="Max Turns">
            <input
              type="number"
              value={agent.max_turns || ''}
              onChange={(e) => set('max_turns', e.target.value ? Number(e.target.value) : 0)}
              className={inputClass}
            />
          </Field>
        </div>

        <Field label="Tools (comma-separated)">
          <input
            value={csvValue('tools')}
            onChange={(e) => csvOnChange('tools', e.target.value)}
            placeholder="Read, Edit, Bash, Grep, Glob, Write"
            className={inputClass}
          />
        </Field>

        <Field label="Disallowed Tools (comma-separated)">
          <input
            value={csvValue('disallowed_tools')}
            onChange={(e) => csvOnChange('disallowed_tools', e.target.value)}
            placeholder=""
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

        {agent.file_path && (
          <p className="text-xs text-gray-400 dark:text-gray-500">File: {agent.file_path}</p>
        )}

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

function Field({ label, children }: { label: React.ReactNode; children: React.ReactNode }) {
  return (
    <div>
      <label className="block text-xs font-medium text-gray-500 dark:text-gray-400 mb-1">{label}</label>
      {children}
    </div>
  )
}
