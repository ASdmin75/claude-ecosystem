import { useQuery } from '@tanstack/react-query'
import { api } from '../api/client'

export default function Dashboard() {
  const { data, isLoading } = useQuery({ queryKey: ['dashboard'], queryFn: api.dashboard })

  if (isLoading) return <p className="text-gray-500">Loading...</p>
  if (!data) return null

  const stats = [
    { label: 'Tasks', value: data.total_tasks },
    { label: 'Pipelines', value: data.total_pipelines },
    { label: 'Executions', value: data.total_executions },
    { label: 'Running', value: data.running, color: 'text-blue-600' },
    { label: 'Completed', value: data.completed, color: 'text-green-600' },
    { label: 'Failed', value: data.failed, color: 'text-red-600' },
  ]

  return (
    <div>
      <h2 className="text-xl font-bold mb-4">Dashboard</h2>
      <div className="grid grid-cols-3 gap-4">
        {stats.map((s) => (
          <div key={s.label} className="bg-white p-4 rounded-lg shadow">
            <p className="text-sm text-gray-500">{s.label}</p>
            <p className={`text-2xl font-bold ${s.color || ''}`}>{s.value}</p>
          </div>
        ))}
      </div>
    </div>
  )
}
