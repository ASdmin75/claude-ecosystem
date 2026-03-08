import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import Markdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { api } from '../api/client'
import { useState } from 'react'
import type { Execution } from '../types'

const statusColor: Record<string, string> = {
  running: 'bg-blue-100 text-blue-700',
  completed: 'bg-green-100 text-green-700',
  failed: 'bg-red-100 text-red-700',
  cancelled: 'bg-yellow-100 text-yellow-700',
}

export default function ExecutionHistory() {
  const queryClient = useQueryClient()
  const [selected, setSelected] = useState<Execution | null>(null)
  const { data: executions, isLoading } = useQuery({
    queryKey: ['executions'],
    queryFn: () => api.listExecutions({ limit: '50' }),
    refetchInterval: 5000,
  })

  const detailQuery = useQuery({
    queryKey: ['execution', selected?.id],
    queryFn: () => api.getExecution(selected!.id),
    enabled: !!selected,
    refetchInterval: selected?.status === 'running' ? 3000 : false,
  })

  const cancelMutation = useMutation({
    mutationFn: (id: string) => api.cancelExecution(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['executions'] })
      queryClient.invalidateQueries({ queryKey: ['execution', selected?.id] })
    },
  })

  if (isLoading) return <p className="text-gray-500">Loading...</p>

  return (
    <div>
      <h2 className="text-xl font-bold mb-4">Execution History</h2>
      <div className="flex gap-4">
        <div className={selected ? 'w-1/2' : 'w-full'}>
          <div className="bg-white rounded-lg shadow overflow-hidden">
            <table className="w-full text-sm">
              <thead className="bg-gray-50">
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
                    className={`border-t cursor-pointer hover:bg-gray-50 ${selected?.id === e.id ? 'bg-blue-50' : ''}`}
                  >
                    <td className="px-4 py-2 font-medium">{e.task_name}</td>
                    <td className="px-4 py-2">
                      <span className={`inline-block px-2 py-0.5 rounded text-xs ${statusColor[e.status] || ''}`}>{e.status}</span>
                    </td>
                    <td className="px-4 py-2 text-gray-500">{e.trigger}</td>
                    <td className="px-4 py-2 text-gray-500">{e.duration_ms ? `${(e.duration_ms / 1000).toFixed(1)}s` : '-'}</td>
                    <td className="px-4 py-2 text-gray-500">{new Date(e.started_at).toLocaleString()}</td>
                    <td className="px-4 py-2">
                      {e.status === 'running' && (
                        <button
                          onClick={(ev) => { ev.stopPropagation(); cancelMutation.mutate(e.id) }}
                          className="px-2 py-0.5 bg-red-600 text-white text-xs rounded hover:bg-red-700"
                        >
                          Stop
                        </button>
                      )}
                    </td>
                  </tr>
                ))}
                {executions?.length === 0 && (
                  <tr><td colSpan={6} className="px-4 py-8 text-center text-gray-400">No executions yet.</td></tr>
                )}
              </tbody>
            </table>
          </div>
        </div>

        {selected && (
          <div className="w-1/2 min-w-0">
            <div className="bg-white rounded-lg shadow p-4 overflow-hidden">
              <div className="flex justify-between items-start mb-4">
                <div>
                  <h3 className="font-bold text-lg">{selected.task_name}</h3>
                  <p className="text-xs text-gray-400 mt-1">ID: {selected.id}</p>
                </div>
                <div className="flex items-center gap-2">
                  {(selected.status === 'running' || detailQuery.data?.status === 'running') && (
                    <button
                      onClick={() => cancelMutation.mutate(selected.id)}
                      disabled={cancelMutation.isPending}
                      className="px-3 py-1 bg-red-600 text-white text-xs rounded hover:bg-red-700 disabled:opacity-50"
                    >
                      {cancelMutation.isPending ? 'Stopping...' : 'Stop'}
                    </button>
                  )}
                  <button onClick={() => setSelected(null)} className="text-gray-400 hover:text-gray-600 text-lg">&times;</button>
                </div>
              </div>

              {cancelMutation.error && (
                <p className="text-sm mb-2 p-2 bg-red-50 text-red-600 rounded">{cancelMutation.error.message}</p>
              )}

              <div className="grid grid-cols-2 gap-2 text-sm mb-4">
                <div>
                  <span className="text-gray-500">Status:</span>{' '}
                  <span className={`inline-block px-2 py-0.5 rounded text-xs ${statusColor[selected.status] || ''}`}>{selected.status}</span>
                </div>
                <div><span className="text-gray-500">Trigger:</span> {selected.trigger}</div>
                <div><span className="text-gray-500">Duration:</span> {selected.duration_ms ? `${(selected.duration_ms / 1000).toFixed(1)}s` : '-'}</div>
                <div><span className="text-gray-500">Model:</span> {selected.model || '-'}</div>
                <div><span className="text-gray-500">Cost:</span> {selected.cost_usd ? `$${selected.cost_usd.toFixed(4)}` : '-'}</div>
                <div><span className="text-gray-500">Started:</span> {new Date(selected.started_at).toLocaleString()}</div>
              </div>

              {detailQuery.isLoading && <p className="text-gray-400 text-sm">Loading details...</p>}

              {detailQuery.data?.error && (
                <div className="mb-4">
                  <h4 className="text-sm font-semibold text-red-600 mb-1">Error</h4>
                  <pre className="bg-red-50 text-red-800 text-xs p-3 rounded overflow-y-auto max-h-40 whitespace-pre-wrap break-all">{detailQuery.data.error}</pre>
                </div>
              )}

              {detailQuery.data?.output && (
                <div>
                  <h4 className="text-sm font-semibold text-gray-700 mb-1">Output</h4>
                  <div className="bg-gray-50 text-gray-800 text-sm p-3 rounded overflow-auto max-h-96 prose prose-sm prose-gray max-w-none">
                    <Markdown remarkPlugins={[remarkGfm]}>{detailQuery.data.output}</Markdown>
                  </div>
                </div>
              )}

              {detailQuery.data && !detailQuery.data.output && !detailQuery.data.error && selected.status === 'running' && (
                <p className="text-sm text-blue-500">Task is still running...</p>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
