import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'
import { useState } from 'react'
import type { Pipeline } from '../types'

const emptyForm: Partial<Pipeline> = {
  name: '', mode: 'sequential', steps: [{ task: '' }], max_iterations: 1, stop_signal: '', collector: '',
}

export default function PipelineList() {
  const queryClient = useQueryClient()
  const { data: pipelines, isLoading } = useQuery({ queryKey: ['pipelines'], queryFn: api.listPipelines })
  const [editing, setEditing] = useState<Partial<Pipeline> | null>(null)
  const [isNew, setIsNew] = useState(false)
  const [result, setResult] = useState<string | null>(null)

  const createMutation = useMutation({
    mutationFn: (p: Partial<Pipeline>) => api.createPipeline(p),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['pipelines'] }); close() },
  })

  const updateMutation = useMutation({
    mutationFn: (p: Partial<Pipeline>) => api.updatePipeline(p.name!, p),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['pipelines'] }); close() },
  })

  const deleteMutation = useMutation({
    mutationFn: (name: string) => {
      if (!window.confirm(`Delete pipeline "${name}"?`)) throw new Error('cancelled')
      return api.deletePipeline(name)
    },
    onSuccess: () => {
      setEditing(null)
      setIsNew(false)
      queryClient.invalidateQueries({ queryKey: ['pipelines'] })
    },
  })

  const runMutation = useMutation({
    mutationFn: (name: string) => api.runPipelineAsync(name),
    onSuccess: (data) => setResult(`Pipeline execution started: ${data.execution_id}`),
    onError: (err) => setResult(`Error: ${err.message}`),
  })

  function close() { setEditing(null); setIsNew(false) }

  function startNew() {
    setEditing({ ...emptyForm, steps: [{ task: '' }] })
    setIsNew(true)
  }

  function startEdit(p: Pipeline) {
    setEditing({ ...p, steps: [...p.steps] })
    setIsNew(false)
  }

  function handleSave() {
    if (!editing) return
    if (isNew) createMutation.mutate(editing)
    else updateMutation.mutate(editing)
  }

  const saving = createMutation.isPending || updateMutation.isPending
  const error = createMutation.error || updateMutation.error

  if (isLoading) return <p className="text-gray-500">Loading...</p>

  return (
    <div>
      <div className="flex justify-between items-center mb-4">
        <h2 className="text-xl font-bold">Pipelines</h2>
        <button onClick={editing ? close : startNew} className="px-3 py-1 bg-gray-900 text-white text-sm rounded">
          {editing ? 'Cancel' : 'New Pipeline'}
        </button>
      </div>

      {result && <p className="text-sm mb-4 p-2 bg-gray-100 rounded">{result}</p>}
      {error && <p className="text-sm mb-4 p-2 bg-red-50 text-red-600 rounded">{error.message}</p>}

      <div className="flex gap-4">
        <div className={editing ? 'w-1/2' : 'w-full'}>
          <div className="space-y-3">
            {pipelines?.map((p) => (
              <div
                key={p.name}
                onClick={() => startEdit(p)}
                className={`bg-white p-4 rounded-lg shadow cursor-pointer hover:bg-gray-50 ${editing?.name === p.name && !isNew ? 'ring-2 ring-blue-300' : ''}`}
              >
                <div className="flex justify-between items-start">
                  <div className="flex-1">
                    <div className="flex items-center gap-2">
                      <h3 className="font-semibold">{p.name}</h3>
                      <span className={`text-xs px-1.5 py-0.5 rounded ${p.mode === 'parallel' ? 'bg-purple-100 text-purple-700' : 'bg-blue-100 text-blue-700'}`}>
                        {p.mode}
                      </span>
                    </div>
                    <p className="text-sm text-gray-500 mt-1">
                      Steps: {p.steps.map((s) => s.task).join(' → ')}
                    </p>
                    <div className="flex gap-3 mt-1 text-xs text-gray-400">
                      <span>Max iterations: {p.max_iterations}</span>
                      {p.stop_signal && <span>Stop: "{p.stop_signal}"</span>}
                      {p.collector && <span>Collector: {p.collector}</span>}
                    </div>
                  </div>
                  <div className="flex gap-2">
                    <button onClick={(e) => { e.stopPropagation(); runMutation.mutate(p.name) }} className="px-3 py-1 bg-gray-900 text-white text-sm rounded hover:bg-gray-800">Run</button>
                    <button onClick={(e) => { e.stopPropagation(); startEdit(p) }} className="text-gray-400 hover:text-gray-600 text-sm">Edit</button>
                    <button onClick={(e) => { e.stopPropagation(); deleteMutation.mutate(p.name) }} className="text-red-400 hover:text-red-600 text-sm">Delete</button>
                  </div>
                </div>
              </div>
            ))}
            {pipelines?.length === 0 && !editing && <p className="text-gray-400 text-sm">No pipelines configured.</p>}
          </div>
        </div>

        {editing && (
          <div className="w-1/2">
            <PipelineEditor
              pipeline={editing}
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

function PipelineEditor({ pipeline, isNew, onChange, onSave, onCancel, saving }: {
  pipeline: Partial<Pipeline>
  isNew: boolean
  onChange: (p: Partial<Pipeline>) => void
  onSave: () => void
  onCancel: () => void
  saving: boolean
}) {
  const set = (key: string, value: unknown) => onChange({ ...pipeline, [key]: value })
  const steps = pipeline.steps || []

  function addStep() {
    onChange({ ...pipeline, steps: [...steps, { task: '' }] })
  }

  function removeStep(index: number) {
    const next = steps.filter((_, i) => i !== index)
    onChange({ ...pipeline, steps: next.length > 0 ? next : [{ task: '' }] })
  }

  function updateStep(index: number, task: string) {
    const next = [...steps]
    next[index] = { task }
    onChange({ ...pipeline, steps: next })
  }

  function moveStep(index: number, direction: -1 | 1) {
    const target = index + direction
    if (target < 0 || target >= steps.length) return
    const next = [...steps]
    ;[next[index], next[target]] = [next[target], next[index]]
    onChange({ ...pipeline, steps: next })
  }

  return (
    <div className="bg-white rounded-lg shadow p-4">
      <div className="flex justify-between items-center mb-4">
        <h3 className="font-bold text-lg">{isNew ? 'New Pipeline' : pipeline.name}</h3>
        <button onClick={onCancel} className="text-gray-400 hover:text-gray-600 text-lg">&times;</button>
      </div>

      <form onSubmit={(e) => { e.preventDefault(); onSave() }} className="space-y-3">
        {isNew && (
          <Field label="Name">
            <input
              value={pipeline.name || ''}
              onChange={(e) => set('name', e.target.value)}
              className="w-full px-2 py-1 border rounded text-sm"
              required
            />
          </Field>
        )}

        <Field label="Mode">
          <select
            value={pipeline.mode || 'sequential'}
            onChange={(e) => set('mode', e.target.value)}
            className="w-full px-2 py-1 border rounded text-sm"
          >
            <option value="sequential">Sequential</option>
            <option value="parallel">Parallel</option>
          </select>
        </Field>

        <Field label="Steps">
          <div className="space-y-2">
            {steps.map((step, i) => (
              <div key={i} className="flex items-center gap-2">
                <span className="text-xs text-gray-400 w-5">{i + 1}.</span>
                <input
                  value={step.task}
                  onChange={(e) => updateStep(i, e.target.value)}
                  placeholder="Task name"
                  className="flex-1 px-2 py-1 border rounded text-sm"
                  required
                />
                <button type="button" onClick={() => moveStep(i, -1)} disabled={i === 0} className="text-gray-400 hover:text-gray-600 text-sm disabled:opacity-30">↑</button>
                <button type="button" onClick={() => moveStep(i, 1)} disabled={i === steps.length - 1} className="text-gray-400 hover:text-gray-600 text-sm disabled:opacity-30">↓</button>
                <button type="button" onClick={() => removeStep(i)} className="text-red-400 hover:text-red-600 text-sm">×</button>
              </div>
            ))}
            <button type="button" onClick={addStep} className="text-sm text-blue-500 hover:text-blue-700">+ Add step</button>
          </div>
        </Field>

        <div className="grid grid-cols-2 gap-3">
          <Field label="Max Iterations">
            <input
              type="number"
              min={1}
              value={pipeline.max_iterations || 1}
              onChange={(e) => set('max_iterations', Number(e.target.value) || 1)}
              className="w-full px-2 py-1 border rounded text-sm"
            />
          </Field>

          <Field label="Stop Signal">
            <input
              value={pipeline.stop_signal || ''}
              onChange={(e) => set('stop_signal', e.target.value)}
              placeholder="e.g. LGTM"
              className="w-full px-2 py-1 border rounded text-sm"
            />
          </Field>
        </div>

        <Field label="Collector (task name, parallel mode)">
          <input
            value={pipeline.collector || ''}
            onChange={(e) => set('collector', e.target.value)}
            placeholder="Optional collector task"
            className="w-full px-2 py-1 border rounded text-sm"
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

function Field({ label, children }: { label: React.ReactNode; children: React.ReactNode }) {
  return (
    <div>
      <label className="block text-xs font-medium text-gray-500 mb-1">{label}</label>
      {children}
    </div>
  )
}
