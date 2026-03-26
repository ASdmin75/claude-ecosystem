import { useState } from 'react'
import { api, DiagnosisError } from '../api/client'
import type { WizardPlan, ApplyResult, WizardDiagnosis, RecoveryAction, RetryContext, TestRunResult } from '../types'

type WizardState = 'input' | 'preview' | 'editing' | 'result' | 'testing'

export default function Wizard() {
  const [state, setState] = useState<WizardState>('input')
  const [description, setDescription] = useState('')
  const [workDir, setWorkDir] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [plan, setPlan] = useState<WizardPlan | null>(null)
  const [result, setResult] = useState<ApplyResult | null>(null)
  const [diagnosis, setDiagnosis] = useState<WizardDiagnosis | null>(null)
  const [retryContext, setRetryContext] = useState<RetryContext | null>(null)
  const [userHint, setUserHint] = useState('')
  const [editingJSON, setEditingJSON] = useState('')
  const [editValidation, setEditValidation] = useState<{ valid?: boolean; warnings?: string[]; error?: string } | null>(null)
  const [testResult, setTestResult] = useState<TestRunResult | null>(null)
  const [selectedTestTask, setSelectedTestTask] = useState('')
  const [expandedSections, setExpandedSections] = useState<Record<string, boolean>>({
    mcp_servers: true, domains: true, agents: true, tasks: true, pipelines: true,
  })

  const toggleSection = (key: string) =>
    setExpandedSections((s) => ({ ...s, [key]: !s[key] }))

  const handleGenerate = async () => {
    if (!description.trim()) return
    setLoading(true)
    setError('')
    setDiagnosis(null)
    try {
      const ctx = retryContext
        ? { ...retryContext, user_hint: userHint || undefined }
        : undefined
      const p = await api.wizardGenerate(description, workDir || undefined, ctx)
      setPlan(p)
      setDiagnosis(null)
      setRetryContext(null)
      setUserHint('')
      setState('preview')
    } catch (e) {
      if (e instanceof DiagnosisError) {
        setDiagnosis(e.diagnosis)
        setRetryContext({
          previous_error: e.diagnosis.message,
          previous_raw_output: e.diagnosis.details,
        })
      } else {
        setError(e instanceof Error ? e.message : 'Generation failed')
      }
    } finally {
      setLoading(false)
    }
  }

  const handleApply = async () => {
    if (!plan) return
    setLoading(true)
    setError('')
    setDiagnosis(null)
    try {
      const r = await api.wizardApply(plan.id)
      setResult(r)
      setState('result')
    } catch (e) {
      if (e instanceof DiagnosisError) {
        setDiagnosis(e.diagnosis)
      } else {
        setError(e instanceof Error ? e.message : 'Apply failed')
      }
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

  const handleDiagnosisAction = async (action: RecoveryAction) => {
    switch (action.id) {
      case 'retry':
        handleGenerate()
        break
      case 'retry_simplified':
        setDescription(description.split('.')[0])
        setRetryContext(null)
        break
      case 'retry_with_hint':
        // Just show the hint input — user will fill it and click Generate
        break
      case 'auto_fix':
        if (action.patched_plan && plan) {
          try {
            const updated = await api.wizardUpdatePlan(plan.id, action.patched_plan)
            setPlan(updated)
            setDiagnosis(null)
          } catch (e) {
            setError(e instanceof Error ? e.message : 'Failed to apply auto-fix')
          }
        }
        break
      case 'edit_plan':
        if (plan) {
          const { id: _, status: __, raw_output: ___, ...editable } = plan as WizardPlan & { raw_output?: string }
          setEditingJSON(JSON.stringify(editable, null, 2))
          setEditValidation(null)
          setState('editing')
        }
        break
      case 'edit_task':
        // Navigate to tasks page — for now just go back to result
        setState('result')
        break
      case 'retry_test':
        if (selectedTestTask) handleTestRun(selectedTestTask)
        break
    }
  }

  const handleValidate = async () => {
    if (!plan) return
    setLoading(true)
    setEditValidation(null)
    try {
      const res = await api.wizardValidate(plan.id)
      setEditValidation({ valid: res.valid, warnings: res.warnings })
    } catch (e) {
      if (e instanceof DiagnosisError) {
        setEditValidation({ error: e.diagnosis.message })
      } else {
        setEditValidation({ error: e instanceof Error ? e.message : 'Validation failed' })
      }
    } finally {
      setLoading(false)
    }
  }

  const handleSaveEdit = async () => {
    if (!plan) return
    try {
      const parsed = JSON.parse(editingJSON)
      parsed.id = plan.id
      parsed.status = 'draft'
      const updated = await api.wizardUpdatePlan(plan.id, parsed)
      setPlan(updated)
      setDiagnosis(null)
      setEditValidation(null)
      setState('preview')
    } catch (e) {
      setEditValidation({ error: e instanceof Error ? e.message : 'Invalid JSON' })
    }
  }

  const handleTestRun = async (taskName: string) => {
    if (!plan) return
    setSelectedTestTask(taskName)
    setTestResult(null)
    setLoading(true)
    setState('testing')
    try {
      const tr = await api.wizardTestRun(plan.id, taskName)
      setTestResult(tr)
    } catch (e) {
      setTestResult({
        task_name: taskName,
        output: '',
        error: e instanceof Error ? e.message : 'Test run failed',
        duration_ms: 0,
      })
    } finally {
      setLoading(false)
    }
  }

  const reset = () => {
    setState('input')
    setDescription('')
    setWorkDir('')
    setPlan(null)
    setResult(null)
    setError('')
    setDiagnosis(null)
    setRetryContext(null)
    setUserHint('')
    setTestResult(null)
    setEditingJSON('')
    setEditValidation(null)
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

        {diagnosis && (
          <div className="mt-3">
            <DiagnosisPanel diagnosis={diagnosis} onAction={handleDiagnosisAction} />
            {/* User hint input for retry with context */}
            <div className="mt-3">
              <label className="block text-sm text-gray-600 dark:text-gray-400 mb-1">
                Additional context (optional)
              </label>
              <textarea
                value={userHint}
                onChange={(e) => setUserHint(e.target.value)}
                placeholder="Provide more detail about what went wrong or what you need..."
                rows={3}
                className="w-full p-2 border rounded-lg bg-white dark:bg-gray-800 dark:border-gray-700 text-sm focus:ring-2 focus:ring-blue-500 focus:border-blue-500 outline-none"
              />
            </div>
          </div>
        )}

        <button
          onClick={handleGenerate}
          disabled={loading || !description.trim()}
          className="mt-4 px-6 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2"
        >
          {loading && <Spinner />}
          {loading ? 'Generating...' : retryContext ? 'Retry Generation' : 'Generate Plan'}
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

        {diagnosis && (
          <div className="mb-4">
            <DiagnosisPanel diagnosis={diagnosis} onAction={handleDiagnosisAction} />
          </div>
        )}

        {/* MCP Servers */}
        {plan.mcp_servers && plan.mcp_servers.length > 0 && (
          <CollapsibleSection title={`MCP Servers (${plan.mcp_servers.length})`} expanded={expandedSections.mcp_servers} onToggle={() => toggleSection('mcp_servers')}>
            {plan.mcp_servers.map((m) => (
              <div key={m.name} className="p-3 border dark:border-gray-700 rounded-lg mb-2">
                <div className="flex items-center gap-2">
                  <span className="font-medium text-sm">{m.name}</span>
                  <Badge text={m.command} />
                </div>
                {m.env && Object.keys(m.env).length > 0 && (
                  <div className="mt-2 space-y-0.5">
                    {Object.entries(m.env).map(([k, v]) => (
                      <div key={k} className="text-xs text-gray-500">
                        <span className="text-gray-600 dark:text-gray-400">{k}</span>={v}
                      </div>
                    ))}
                  </div>
                )}
              </div>
            ))}
          </CollapsibleSection>
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
                      {i > 0 && <span className="mx-1">&rarr;</span>}
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
            onClick={() => {
              if (plan) {
                const { id: _, status: __, raw_output: ___, ...editable } = plan as WizardPlan & { raw_output?: string }
                setEditingJSON(JSON.stringify(editable, null, 2))
                setEditValidation(null)
                setState('editing')
              }
            }}
            disabled={loading}
            className="px-6 py-2 bg-yellow-500 text-white rounded-lg hover:bg-yellow-600 disabled:opacity-50"
          >
            Edit Plan
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

  // Editing state
  if (state === 'editing' && plan) {
    return (
      <div className="max-w-4xl mx-auto">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-xl font-bold">Edit Plan</h2>
          <span className="px-2 py-1 text-xs bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-400 rounded">
            Editing
          </span>
        </div>

        <p className="text-sm text-gray-500 dark:text-gray-400 mb-3">
          Edit the plan JSON directly. Use Validate to check before saving.
        </p>

        <textarea
          value={editingJSON}
          onChange={(e) => {
            setEditingJSON(e.target.value)
            setEditValidation(null)
          }}
          rows={24}
          className="w-full p-3 border rounded-lg bg-white dark:bg-gray-800 dark:border-gray-700 text-xs font-mono focus:ring-2 focus:ring-blue-500 focus:border-blue-500 outline-none"
          spellCheck={false}
        />

        {editValidation && (
          <div className="mt-3">
            {editValidation.valid && (
              <div className="p-3 bg-green-50 dark:bg-green-900/20 text-green-600 dark:text-green-400 rounded-lg text-sm flex items-center gap-2">
                <span>&#10003;</span> Plan is valid
                {editValidation.warnings && editValidation.warnings.length > 0 && (
                  <span className="text-yellow-600 dark:text-yellow-400 ml-2">
                    ({editValidation.warnings.length} warning{editValidation.warnings.length > 1 ? 's' : ''})
                  </span>
                )}
              </div>
            )}
            {editValidation.warnings && editValidation.warnings.length > 0 && (
              <div className="mt-2 space-y-1">
                {editValidation.warnings.map((w, i) => (
                  <div key={i} className="p-2 bg-yellow-50 dark:bg-yellow-900/20 text-yellow-600 dark:text-yellow-400 rounded text-xs">
                    {w}
                  </div>
                ))}
              </div>
            )}
            {editValidation.error && (
              <div className="p-3 bg-red-50 dark:bg-red-900/20 text-red-600 dark:text-red-400 rounded-lg text-sm">
                {editValidation.error}
              </div>
            )}
          </div>
        )}

        <div className="flex gap-3 mt-4">
          <button
            onClick={handleValidate}
            disabled={loading}
            className="px-6 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50 flex items-center gap-2"
          >
            {loading && <Spinner />}
            Validate
          </button>
          <button
            onClick={handleSaveEdit}
            disabled={loading}
            className="px-6 py-2 bg-green-600 text-white rounded-lg hover:bg-green-700 disabled:opacity-50"
          >
            Save & Preview
          </button>
          <button
            onClick={() => {
              setEditValidation(null)
              setState('preview')
            }}
            disabled={loading}
            className="px-6 py-2 bg-gray-200 dark:bg-gray-700 text-gray-700 dark:text-gray-300 rounded-lg hover:bg-gray-300 dark:hover:bg-gray-600 disabled:opacity-50"
          >
            Cancel
          </button>
        </div>
      </div>
    )
  }

  // Result state
  if (state === 'result' && result) {
    const all = [
      ...(result.mcp_servers_created || []).map((n) => ({ type: 'MCP Server', name: n })),
      ...(result.domains_created || []).map((n) => ({ type: 'Domain', name: n })),
      ...(result.agents_created || []).map((n) => ({ type: 'Agent', name: n })),
      ...(result.tasks_created || []).map((n) => ({ type: 'Task', name: n })),
      ...(result.pipelines_created || []).map((n) => ({ type: 'Pipeline', name: n })),
    ]
    const createdTasks = result.tasks_created || []

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

        {/* Test Run */}
        {createdTasks.length > 0 && (
          <div className="mb-6 p-4 border dark:border-gray-700 rounded-lg">
            <p className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3">
              Test Run — verify a task works correctly
            </p>
            <div className="flex gap-2 items-center">
              <select
                value={selectedTestTask}
                onChange={(e) => setSelectedTestTask(e.target.value)}
                className="flex-1 p-2 border rounded-lg bg-white dark:bg-gray-800 dark:border-gray-700 text-sm"
              >
                <option value="">Select a task...</option>
                {createdTasks.map((t) => (
                  <option key={t} value={t}>{t}</option>
                ))}
              </select>
              <button
                onClick={() => selectedTestTask && handleTestRun(selectedTestTask)}
                disabled={!selectedTestTask || loading}
                className="px-4 py-2 bg-purple-600 text-white rounded-lg hover:bg-purple-700 disabled:opacity-50 text-sm flex items-center gap-2"
              >
                {loading && <Spinner />}
                Test Run
              </button>
            </div>
          </div>
        )}

        <button
          onClick={reset}
          className="px-6 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700"
        >
          Create Another
        </button>
      </div>
    )
  }

  // Testing state
  if (state === 'testing') {
    return (
      <div className="max-w-3xl mx-auto">
        <h2 className="text-xl font-bold mb-4">Test Run: {selectedTestTask}</h2>

        {loading && (
          <div className="flex items-center gap-3 p-4 bg-blue-50 dark:bg-blue-900/20 rounded-lg text-sm text-blue-700 dark:text-blue-400">
            <Spinner />
            Running task... (max 2 min)
          </div>
        )}

        {testResult && !loading && (
          <>
            {/* Success */}
            {!testResult.error && !testResult.soft_failure && (
              <div className="p-4 bg-green-50 dark:bg-green-900/20 rounded-lg mb-4">
                <p className="text-sm font-medium text-green-700 dark:text-green-400 mb-2">
                  Task completed successfully ({(testResult.duration_ms / 1000).toFixed(1)}s
                  {testResult.cost_usd ? `, $${testResult.cost_usd.toFixed(4)}` : ''})
                </p>
                <pre className="mt-2 p-2 bg-green-100/50 dark:bg-green-900/30 rounded text-xs overflow-x-auto whitespace-pre-wrap max-h-64 overflow-y-auto">
                  {testResult.output || '(no output)'}
                </pre>
              </div>
            )}

            {/* Soft failure */}
            {testResult.soft_failure && testResult.diagnosis && (
              <div className="mb-4">
                <div className="p-4 bg-yellow-50 dark:bg-yellow-900/20 rounded-lg mb-3">
                  <p className="text-sm font-medium text-yellow-700 dark:text-yellow-400 mb-2">
                    Task finished but detected a problem ({(testResult.duration_ms / 1000).toFixed(1)}s)
                  </p>
                  <pre className="mt-2 p-2 bg-yellow-100/50 dark:bg-yellow-900/30 rounded text-xs overflow-x-auto whitespace-pre-wrap max-h-48 overflow-y-auto">
                    {testResult.output || '(no output)'}
                  </pre>
                </div>
                <DiagnosisPanel diagnosis={testResult.diagnosis} onAction={handleDiagnosisAction} />
              </div>
            )}

            {/* Hard error */}
            {testResult.error && !testResult.soft_failure && (
              <div className="mb-4">
                <div className="p-4 bg-red-50 dark:bg-red-900/20 rounded-lg mb-3">
                  <p className="text-sm font-medium text-red-700 dark:text-red-400 mb-2">
                    Task failed ({(testResult.duration_ms / 1000).toFixed(1)}s)
                  </p>
                  <p className="text-xs text-red-600 dark:text-red-400">{testResult.error}</p>
                  {testResult.output && (
                    <pre className="mt-2 p-2 bg-red-100/50 dark:bg-red-900/30 rounded text-xs overflow-x-auto whitespace-pre-wrap max-h-48 overflow-y-auto">
                      {testResult.output}
                    </pre>
                  )}
                </div>
                {testResult.diagnosis && (
                  <DiagnosisPanel diagnosis={testResult.diagnosis} onAction={handleDiagnosisAction} />
                )}
              </div>
            )}
          </>
        )}

        <button
          onClick={() => setState('result')}
          disabled={loading}
          className="mt-4 px-6 py-2 bg-gray-200 dark:bg-gray-700 text-gray-700 dark:text-gray-300 rounded-lg hover:bg-gray-300 dark:hover:bg-gray-600 disabled:opacity-50"
        >
          Back to Results
        </button>
      </div>
    )
  }

  return null
}

// --- Reusable components ---

function DiagnosisPanel({ diagnosis, onAction }: {
  diagnosis: WizardDiagnosis
  onAction: (action: RecoveryAction) => void
}) {
  const [showDetails, setShowDetails] = useState(false)

  const categoryLabel: Record<string, string> = {
    empty_output: 'Empty Output',
    json_parse: 'Parse Error',
    timeout: 'Timeout',
    duplicate_name: 'Duplicate Name',
    missing_reference: 'Missing Reference',
    permission_mode: 'Permission Error',
    apply_failed: 'Apply Failed',
    test_soft_failure: 'Soft Failure',
    test_hard_failure: 'Execution Error',
    unknown: 'Error',
  }

  const categoryColor: Record<string, string> = {
    empty_output: 'bg-orange-100 dark:bg-orange-900/30 text-orange-700 dark:text-orange-400',
    json_parse: 'bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-400',
    timeout: 'bg-yellow-100 dark:bg-yellow-900/30 text-yellow-700 dark:text-yellow-400',
    duplicate_name: 'bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-400',
    missing_reference: 'bg-purple-100 dark:bg-purple-900/30 text-purple-700 dark:text-purple-400',
    test_soft_failure: 'bg-yellow-100 dark:bg-yellow-900/30 text-yellow-700 dark:text-yellow-400',
    test_hard_failure: 'bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-400',
  }

  const color = categoryColor[diagnosis.category] || 'bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-400'

  return (
    <div className={`p-4 rounded-lg ${color}`}>
      <div className="flex items-start gap-2">
        <span className="px-2 py-0.5 bg-white/50 dark:bg-black/20 rounded text-xs font-medium">
          {categoryLabel[diagnosis.category] || diagnosis.category}
        </span>
        <p className="text-sm flex-1">{diagnosis.message}</p>
      </div>

      {diagnosis.details && (
        <div className="mt-2">
          <button
            onClick={() => setShowDetails(!showDetails)}
            className="text-xs underline opacity-70 hover:opacity-100"
          >
            {showDetails ? 'Hide details' : 'Show details'}
          </button>
          {showDetails && (
            <pre className="mt-2 p-2 bg-black/10 dark:bg-black/20 rounded text-xs overflow-x-auto whitespace-pre-wrap max-h-48 overflow-y-auto">
              {diagnosis.details}
            </pre>
          )}
        </div>
      )}

      {diagnosis.suggestions.length > 0 && (
        <div className="flex flex-wrap gap-2 mt-3">
          {diagnosis.suggestions.map((s) => (
            <button
              key={s.id}
              onClick={() => onAction(s)}
              className="px-3 py-1.5 bg-white/70 dark:bg-black/20 rounded text-xs font-medium hover:bg-white dark:hover:bg-black/30 transition-colors"
              title={s.description}
            >
              {s.label}
            </button>
          ))}
        </div>
      )}
    </div>
  )
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
        <span className="text-xs">{expanded ? '\u25BC' : '\u25B6'}</span>
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
