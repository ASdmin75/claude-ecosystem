import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import Markdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { api } from '../api/client'
import { useState, useEffect } from 'react'
import type { Execution } from '../types'
import ConfirmModal from './ConfirmModal'

const statusColor: Record<string, string> = {
  running: 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300',
  completed: 'bg-green-100 text-green-700 dark:bg-green-900/40 dark:text-green-300',
  failed: 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300',
  cancelled: 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/40 dark:text-yellow-300',
}

export default function ExecutionHistory() {
  const queryClient = useQueryClient()
  const [selected, setSelected] = useState<Execution | null>(null)
  const { data: executions, isLoading } = useQuery({
    queryKey: ['executions'],
    queryFn: () => api.listExecutions({ limit: '50' }),
    // Poll every 5s when there are running executions (fallback for missed SSE events)
    refetchInterval: (query) =>
      query.state.data?.some((e) => e.status === 'running') ? 5000 : false,
  })

  // Sync selected state with fresh query data so status updates in real-time
  useEffect(() => {
    if (selected && executions) {
      const fresh = executions.find((e) => e.id === selected.id)
      if (fresh && fresh.status !== selected.status) {
        setSelected(fresh)
      }
    }
  }, [executions, selected])

  const detailQuery = useQuery({
    queryKey: ['execution', selected?.id],
    queryFn: () => api.getExecution(selected!.id),
    enabled: !!selected,
    refetchInterval: (query) =>
      query.state.data?.status === 'running' ? 3000 : false,
  })

  const cancelMutation = useMutation({
    mutationFn: (id: string) => api.cancelExecution(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['executions'] })
      queryClient.invalidateQueries({ queryKey: ['execution', selected?.id] })
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.deleteExecution(id),
    onSuccess: (_data, deletedId) => {
      if (selected?.id === deletedId) setSelected(null)
      queryClient.invalidateQueries({ queryKey: ['executions'] })
    },
  })

  const [deleteTarget, setDeleteTarget] = useState<string | null>(null)

  const confirmDelete = (id: string, ev?: React.MouseEvent) => {
    ev?.stopPropagation()
    setDeleteTarget(id)
  }

  if (isLoading) return <p className="text-gray-500 dark:text-gray-400">Loading...</p>

  return (
    <div>
      <h2 className="text-xl font-bold mb-4">Execution History</h2>
      <div className="flex gap-4">
        <div className={selected ? 'w-1/2' : 'w-full'}>
          <div className="bg-white dark:bg-gray-800 rounded-lg shadow dark:shadow-gray-950 overflow-hidden">
            <table className="w-full text-sm">
              <thead className="bg-gray-50 dark:bg-gray-700/50">
                <tr>
                  <th className="text-left px-4 py-2">Task</th>
                  <th className="text-left px-4 py-2">Status</th>
                  <th className="text-left px-4 py-2">Trigger</th>
                  <th className="text-left px-4 py-2">Duration</th>
                  <th className="text-left px-4 py-2">Started</th>
                  <th className="px-4 py-2"></th>
                </tr>
              </thead>
              <tbody>
                {executions?.map((e) => (
                  <tr
                    key={e.id}
                    onClick={() => setSelected(e)}
                    className={`border-t border-gray-100 dark:border-gray-700 cursor-pointer hover:bg-gray-50 dark:hover:bg-gray-700/50 ${selected?.id === e.id ? 'bg-blue-50 dark:bg-blue-900/20' : ''}`}
                  >
                    <td className="px-4 py-2 font-medium">
                      {e.pipeline_name && !e.task_name ? (
                        <span className="inline-flex items-center gap-1.5">
                          <span className="text-purple-500 dark:text-purple-400" title="Pipeline">⛓</span>
                          {e.pipeline_name}
                        </span>
                      ) : (
                        <span className="inline-flex items-center gap-1.5">
                          <span className="text-blue-500 dark:text-blue-400" title="Task">▶</span>
                          {e.task_name || '-'}
                        </span>
                      )}
                    </td>
                    <td className="px-4 py-2">
                      <span className={`inline-block px-2 py-0.5 rounded text-xs ${statusColor[e.status] || ''}`}>{e.status}</span>
                    </td>
                    <td className="px-4 py-2 text-gray-500 dark:text-gray-400">{e.trigger}</td>
                    <td className="px-4 py-2 text-gray-500 dark:text-gray-400">{e.duration_ms ? `${(e.duration_ms / 1000).toFixed(1)}s` : '-'}</td>
                    <td className="px-4 py-2 text-gray-500 dark:text-gray-400">{new Date(e.started_at).toLocaleString()}</td>
                    <td className="px-4 py-2 flex gap-1">
                      {e.status === 'running' && (
                        <button
                          onClick={(ev) => { ev.stopPropagation(); cancelMutation.mutate(e.id) }}
                          className="px-2 py-0.5 bg-red-600 text-white text-xs rounded hover:bg-red-700"
                        >
                          Stop
                        </button>
                      )}
                      {e.status !== 'running' && (
                        <button
                          onClick={(ev) => confirmDelete(e.id, ev)}
                          className="px-2 py-0.5 text-gray-400 hover:text-red-500 dark:text-gray-500 dark:hover:text-red-400 text-xs"
                          title="Delete"
                        >
                          ✕
                        </button>
                      )}
                    </td>
                  </tr>
                ))}
                {executions?.length === 0 && (
                  <tr><td colSpan={6} className="px-4 py-8 text-center text-gray-400 dark:text-gray-500">No executions yet.</td></tr>
                )}
              </tbody>
            </table>
          </div>
        </div>

        {selected && (
          <div className="w-1/2 min-w-0">
            <div className="bg-white dark:bg-gray-800 rounded-lg shadow dark:shadow-gray-950 p-4 overflow-hidden">
              <div className="flex justify-between items-start mb-4">
                <div>
                  <h3 className="font-bold text-lg inline-flex items-center gap-2">
                    {selected.pipeline_name && !selected.task_name ? (
                      <>
                        <span className="text-purple-500 dark:text-purple-400">⛓</span>
                        {selected.pipeline_name}
                        <span className="text-xs font-normal text-purple-500 dark:text-purple-400 bg-purple-50 dark:bg-purple-900/30 px-1.5 py-0.5 rounded">pipeline</span>
                      </>
                    ) : (
                      <>
                        <span className="text-blue-500 dark:text-blue-400">▶</span>
                        {selected.task_name || '-'}
                        <span className="text-xs font-normal text-blue-500 dark:text-blue-400 bg-blue-50 dark:bg-blue-900/30 px-1.5 py-0.5 rounded">task</span>
                      </>
                    )}
                  </h3>
                  <p className="text-xs text-gray-400 dark:text-gray-500 mt-1">ID: {selected.id}</p>
                </div>
                <div className="flex items-center gap-2">
                  {(detailQuery.data?.status ?? selected.status) === 'running' && (
                    <button
                      onClick={() => cancelMutation.mutate(selected.id)}
                      disabled={cancelMutation.isPending}
                      className="px-3 py-1 bg-red-600 text-white text-xs rounded hover:bg-red-700 disabled:opacity-50"
                    >
                      {cancelMutation.isPending ? 'Stopping...' : 'Stop'}
                    </button>
                  )}
                  {(detailQuery.data?.status ?? selected.status) !== 'running' && (
                    <button
                      onClick={() => confirmDelete(selected.id)}
                      disabled={deleteMutation.isPending}
                      className="px-3 py-1 text-gray-400 hover:text-red-500 dark:text-gray-500 dark:hover:text-red-400 text-xs border border-gray-200 dark:border-gray-600 rounded hover:border-red-300 dark:hover:border-red-700"
                      title="Delete execution"
                    >
                      {deleteMutation.isPending ? 'Deleting...' : 'Delete'}
                    </button>
                  )}
                  <button onClick={() => setSelected(null)} className="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 text-lg">&times;</button>
                </div>
              </div>

              {cancelMutation.error && (
                <p className="text-sm mb-2 p-2 bg-red-50 dark:bg-red-900/20 text-red-600 dark:text-red-400 rounded">{cancelMutation.error.message}</p>
              )}

              {(() => {
                const detail = detailQuery.data ?? selected
                return (
                  <div className="grid grid-cols-2 gap-2 text-sm mb-4">
                    <div>
                      <span className="text-gray-500 dark:text-gray-400">Status:</span>{' '}
                      <span className={`inline-block px-2 py-0.5 rounded text-xs ${statusColor[detail.status] || ''}`}>{detail.status}</span>
                    </div>
                    <div><span className="text-gray-500 dark:text-gray-400">Trigger:</span> {detail.trigger}</div>
                    <div><span className="text-gray-500 dark:text-gray-400">Duration:</span> {detail.duration_ms ? `${(detail.duration_ms / 1000).toFixed(1)}s` : '-'}</div>
                    <div><span className="text-gray-500 dark:text-gray-400">Model:</span> {detail.model || '-'}</div>
                    <div><span className="text-gray-500 dark:text-gray-400">Cost:</span> {detail.cost_usd ? `$${detail.cost_usd.toFixed(4)}` : '-'}</div>
                    <div><span className="text-gray-500 dark:text-gray-400">Started:</span> {new Date(detail.started_at).toLocaleString()}</div>
                  </div>
                )
              })()}

              {detailQuery.isLoading && <p className="text-gray-400 dark:text-gray-500 text-sm">Loading details...</p>}

              {detailQuery.data?.error && (
                <div className="mb-4">
                  <h4 className="text-sm font-semibold text-red-600 dark:text-red-400 mb-1">Error</h4>
                  <pre className="bg-red-50 dark:bg-red-900/20 text-red-800 dark:text-red-300 text-xs p-3 rounded overflow-y-auto max-h-40 whitespace-pre-wrap break-all">{detailQuery.data.error}</pre>
                </div>
              )}

              {detailQuery.data?.output && (
                <div>
                  <h4 className="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-1">Output</h4>
                  <div className="bg-gray-50 dark:bg-gray-900 text-gray-800 dark:text-gray-200 text-sm p-3 rounded overflow-auto max-h-96 prose prose-sm prose-gray dark:prose-invert max-w-none">
                    <Markdown remarkPlugins={[remarkGfm]}>{detailQuery.data.output}</Markdown>
                  </div>
                </div>
              )}

              {detailQuery.data && !detailQuery.data.output && !detailQuery.data.error && selected.status === 'running' && (
                <p className="text-sm text-blue-500 dark:text-blue-400">Task is still running...</p>
              )}
            </div>
          </div>
        )}
      </div>

      <ConfirmModal
        open={!!deleteTarget}
        title="Delete execution"
        message="This execution record will be permanently deleted."
        onConfirm={() => { if (deleteTarget) deleteMutation.mutate(deleteTarget); setDeleteTarget(null) }}
        onCancel={() => setDeleteTarget(null)}
      />
    </div>
  )
}
