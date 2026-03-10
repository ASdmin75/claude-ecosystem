import { useState } from 'react'
import { api } from '../api/client'
import type { WizardPlan, ApplyResult } from '../types'

type WizardState = 'input' | 'preview' | 'result'

export default function Wizard() {
  const [state, setState] = useState<WizardState>('input')
  const [description, setDescription] = useState('')
  const [workDir, setWorkDir] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [plan, setPlan] = useState<WizardPlan | null>(null)
  const [result, setResult] = useState<ApplyResult | null>(null)
  const [expandedSections, setExpandedSections] = useState<Record<string, boolean>>({
    domains: true, agents: true, tasks: true, pipelines: true,
  })

  const toggleSection = (key: string) =>
    setExpandedSections((s) => ({ ...s, [key]: !s[key] }))

  const handleGenerate = async () => {
    if (!description.trim()) return
    setLoading(true)
    setError('')
    try {
      const p = await api.wizardGenerate(description, workDir || undefined)
      setPlan(p)
      setState('preview')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Generation failed')
    } finally {
      setLoading(false)
    }
  }

  const handleApply = async () => {
    if (!plan) return
    setLoading(true)
    setError('')
    try {
      const r = await api.wizardApply(plan.id)
      setResult(r)
      setState('result')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Apply failed')
    } finally {
      setLoading(false)
    }
  }

  const handleDiscard = async () => {
    if (!plan) return
    try {
      await api.wizardDiscard(plan.id)
    } catch {
      // ignore
    }
    reset()
  }

  const reset = () => {
    setState('input')
    setDescription('')
    setWorkDir('')
    setPlan(null)
    setResult(null)
    setError('')
  }

  // Input state
  if (state === 'input') {
    return (
      <div className="max-w-3xl mx-auto">
        <h2 className="text-xl font-bold mb-4">Wizard</h2>
        <p className="text-gray-600 dark:text-gray-400 mb-6">
          Describe what you want to automate and Claude will generate the optimal configuration.
        </p>

        <textarea
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="Describe what you want to automate..."
          rows={8}
          className="w-full p-3 border rounded-lg bg-white dark:bg-gray-800 dark:border-gray-700 text-sm font-mono focus:ring-2 focus:ring-blue-500 focus:border-blue-500 outline-none"
        />

        <div className="mt-3">
          <label className="block text-sm text-gray-600 dark:text-gray-400 mb-1">
            Working directory (optional)
          </label>
          <input
            type="text"
            value={workDir}
            onChange={(e) => setWorkDir(e.target.value)}
            placeholder="/path/to/project"
            className="w-full p-2 border rounded-lg bg-white dark:bg-gray-800 dark:border-gray-700 text-sm focus:ring-2 focus:ring-blue-500 focus:border-blue-500 outline-none"
          />
        </div>

        {error && (
          <div className="mt-3 p-3 bg-red-50 dark:bg-red-900/20 text-red-600 dark:text-red-400 rounded-lg text-sm">
            {error}
          </div>
        )}

        <button
          onClick={handleGenerate}
          disabled={loading || !description.trim()}
          className="mt-4 px-6 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2"
        >
          {loading && <Spinner />}
          {loading ? 'Generating...' : 'Generate Plan'}
        </button>
      </div>
    )
  }

  // Preview state
  if (state === 'preview' && plan) {
    return (
      <div className="max-w-4xl mx-auto">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-xl font-bold">Review Plan</h2>
          <span className="px-2 py-1 text-xs bg-yellow-100 dark:bg-yellow-900/30 text-yellow-700 dark:text-yellow-400 rounded">
            Draft
          </span>
        </div>

        {/* Summary */}
        <div className="mb-6 p-4 bg-blue-50 dark:bg-blue-900/20 rounded-lg">
          <p className="text-sm text-gray-700 dark:text-gray-300">{plan.summary}</p>
        </div>

        {error && (
          <div className="mb-4 p-3 bg-red-50 dark:bg-red-900/20 text-red-600 dark:text-red-400 rounded-lg text-sm">
            {error}
          </div>
        )}

        {/* Domains */}
        {plan.domains && plan.domains.length > 0 && (
          <CollapsibleSection title={`Domains (${plan.domains.length})`} expanded={expandedSections.domains} onToggle={() => toggleSection('domains')}>
            {plan.domains.map((d) => (
              <div key={d.name} className="p-3 border dark:border-gray-700 rounded-lg mb-2">
                <div className="font-medium text-sm">{d.name}</div>
                {d.description && <div className="text-xs text-gray-500 mt-1">{d.description}</div>}
                <div className="text-xs text-gray-400 mt-1">data_dir: {d.data_dir} | db: {d.db || 'none'}</div>
                {d.schema && (
                  <pre className="mt-2 p-2 bg-gray-100 dark:bg-gray-800 rounded text-xs overflow-x-auto">{d.schema}</pre>
                )}
              </div>
            ))}
          </CollapsibleSection>
        )}

        {/* Agents */}
        {plan.agents && plan.agents.length > 0 && (
          <CollapsibleSection title={`Agents (${plan.agents.length})`} expanded={expandedSections.agents} onToggle={() => toggleSection('agents')}>
            {plan.agents.map((a) => (
              <div key={a.name} className="p-3 border dark:border-gray-700 rounded-lg mb-2">
                <div className="font-medium text-sm">{a.name}</div>
                <div className="text-xs text-gray-500 mt-1">{a.description}</div>
                {a.tools && a.tools.length > 0 && (
                  <div className="flex flex-wrap gap-1 mt-2">
                    {a.tools.map((t) => <Badge key={t} text={t} />)}
                  </div>
                )}
                <pre className="mt-2 p-2 bg-gray-100 dark:bg-gray-800 rounded text-xs overflow-x-auto whitespace-pre-wrap">{a.instructions}</pre>
              </div>
            ))}
          </CollapsibleSection>
        )}

        {/* Tasks */}
        {plan.tasks && plan.tasks.length > 0 && (
          <CollapsibleSection title={`Tasks (${plan.tasks.length})`} expanded={expandedSections.tasks} onToggle={() => toggleSection('tasks')}>
            {plan.tasks.map((t) => (
              <div key={t.name} className="p-3 border dark:border-gray-700 rounded-lg mb-2">
                <div className="flex items-center gap-2">
                  <span className="font-medium text-sm">{t.name}</span>
                  {t.model && <Badge text={t.model} />}
                  {t.schedule && <Badge text={t.schedule} />}
                  {t.domain && <Badge text={`domain:${t.domain}`} />}
                </div>
                <pre className="mt-2 p-2 bg-gray-100 dark:bg-gray-800 rounded text-xs overflow-x-auto whitespace-pre-wrap">{t.prompt}</pre>
                {t.agents && t.agents.length > 0 && (
                  <div className="flex flex-wrap gap-1 mt-2">
                    {t.agents.map((a) => <Badge key={a} text={`agent:${a}`} />)}
                  </div>
                )}
                {t.mcp_servers && t.mcp_servers.length > 0 && (
                  <div className="flex flex-wrap gap-1 mt-1">
                    {t.mcp_servers.map((m) => <Badge key={m} text={`mcp:${m}`} />)}
                  </div>
                )}
              </div>
            ))}
          </CollapsibleSection>
        )}

        {/* Pipelines */}
        {plan.pipelines && plan.pipelines.length > 0 && (
          <CollapsibleSection title={`Pipelines (${plan.pipelines.length})`} expanded={expandedSections.pipelines} onToggle={() => toggleSection('pipelines')}>
            {plan.pipelines.map((p) => (
              <div key={p.name} className="p-3 border dark:border-gray-700 rounded-lg mb-2">
                <div className="flex items-center gap-2">
                  <span className="font-medium text-sm">{p.name}</span>
                  <Badge text={p.mode || 'sequential'} />
                </div>
                <div className="mt-2 flex items-center gap-1 text-xs text-gray-500">
                  {p.steps.map((s, i) => (
                    <span key={s}>
                      {i > 0 && <span className="mx-1">→</span>}
                      <span className="bg-gray-100 dark:bg-gray-800 px-2 py-0.5 rounded">{s}</span>
                    </span>
                  ))}
                </div>
              </div>
            ))}
          </CollapsibleSection>
        )}

        {/* Actions */}
        <div className="flex gap-3 mt-6">
          <button
            onClick={handleApply}
            disabled={loading}
            className="px-6 py-2 bg-green-600 text-white rounded-lg hover:bg-green-700 disabled:opacity-50 flex items-center gap-2"
          >
            {loading && <Spinner />}
            {loading ? 'Applying...' : 'Apply'}
          </button>
          <button
            onClick={handleDiscard}
            disabled={loading}
            className="px-6 py-2 bg-gray-200 dark:bg-gray-700 text-gray-700 dark:text-gray-300 rounded-lg hover:bg-gray-300 dark:hover:bg-gray-600 disabled:opacity-50"
          >
            Discard
          </button>
        </div>
      </div>
    )
  }

  // Result state
  if (state === 'result' && result) {
    const all = [
      ...(result.domains_created || []).map((n) => ({ type: 'Domain', name: n })),
      ...(result.agents_created || []).map((n) => ({ type: 'Agent', name: n })),
      ...(result.tasks_created || []).map((n) => ({ type: 'Task', name: n })),
      ...(result.pipelines_created || []).map((n) => ({ type: 'Pipeline', name: n })),
    ]

    return (
      <div className="max-w-3xl mx-auto">
        <h2 className="text-xl font-bold mb-4">Plan Applied</h2>

        {result.errors && result.errors.length > 0 && (
          <div className="mb-4 p-3 bg-red-50 dark:bg-red-900/20 text-red-600 dark:text-red-400 rounded-lg text-sm">
            {result.errors.map((e, i) => <div key={i}>{e}</div>)}
          </div>
        )}

        <div className="bg-green-50 dark:bg-green-900/20 rounded-lg p-4 mb-6">
          <p className="text-sm text-green-700 dark:text-green-400 font-medium mb-3">
            Successfully created {all.length} entities:
          </p>
          <div className="space-y-1">
            {all.map((item) => (
              <div key={`${item.type}-${item.name}`} className="flex items-center gap-2 text-sm">
                <span className="px-2 py-0.5 bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-400 rounded text-xs">
                  {item.type}
                </span>
                <span className="text-gray-700 dark:text-gray-300">{item.name}</span>
              </div>
            ))}
          </div>
        </div>

        <button
          onClick={reset}
          className="px-6 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700"
        >
          Create Another
        </button>
      </div>
    )
  }

  return null
}

function CollapsibleSection({ title, expanded, onToggle, children }: {
  title: string
  expanded: boolean
  onToggle: () => void
  children: React.ReactNode
}) {
  return (
    <div className="mb-4">
      <button
        onClick={onToggle}
        className="flex items-center gap-2 text-sm font-medium text-gray-700 dark:text-gray-300 mb-2 hover:text-gray-900 dark:hover:text-white"
      >
        <span className="text-xs">{expanded ? '▼' : '▶'}</span>
        {title}
      </button>
      {expanded && <div>{children}</div>}
    </div>
  )
}

function Badge({ text }: { text: string }) {
  return (
    <span className="px-1.5 py-0.5 bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-400 rounded text-xs">
      {text}
    </span>
  )
}

function Spinner() {
  return (
    <svg className="animate-spin h-4 w-4" viewBox="0 0 24 24">
      <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" fill="none" />
      <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
    </svg>
  )
}
