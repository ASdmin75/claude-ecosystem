import { useQuery } from '@tanstack/react-query'
import { api } from '../api/client'

const statusColor: Record<string, string> = {
  running: 'bg-blue-100 text-blue-700',
  completed: 'bg-green-100 text-green-700',
  failed: 'bg-red-100 text-red-700',
}

export default function ExecutionHistory() {
  const { data: executions, isLoading } = useQuery({
    queryKey: ['executions'],
    queryFn: () => api.listExecutions({ limit: '50' }),
    refetchInterval: 5000,
  })

  if (isLoading) return <p className="text-gray-500">Loading...</p>

  return (
    <div>
      <h2 className="text-xl font-bold mb-4">Execution History</h2>
      <div className="bg-white rounded-lg shadow overflow-hidden">
        <table className="w-full text-sm">
          <thead className="bg-gray-50">
            <tr>
              <th className="text-left px-4 py-2">Task</th>
              <th className="text-left px-4 py-2">Status</th>
              <th className="text-left px-4 py-2">Trigger</th>
              <th className="text-left px-4 py-2">Duration</th>
              <th className="text-left px-4 py-2">Started</th>
            </tr>
          </thead>
          <tbody>
            {executions?.map((e) => (
              <tr key={e.id} className="border-t hover:bg-gray-50">
                <td className="px-4 py-2 font-medium">{e.task_name}</td>
                <td className="px-4 py-2">
                  <span className={`inline-block px-2 py-0.5 rounded text-xs ${statusColor[e.status] || ''}`}>{e.status}</span>
                </td>
                <td className="px-4 py-2 text-gray-500">{e.trigger}</td>
                <td className="px-4 py-2 text-gray-500">{e.duration_ms ? `${(e.duration_ms / 1000).toFixed(1)}s` : '-'}</td>
                <td className="px-4 py-2 text-gray-500">{new Date(e.started_at).toLocaleString()}</td>
              </tr>
            ))}
            {executions?.length === 0 && (
              <tr><td colSpan={5} className="px-4 py-8 text-center text-gray-400">No executions yet.</td></tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  )
}
