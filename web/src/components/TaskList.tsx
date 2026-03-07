import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'
import { useState } from 'react'

export default function TaskList() {
  const queryClient = useQueryClient()
  const { data: tasks, isLoading } = useQuery({ queryKey: ['tasks'], queryFn: api.listTasks })
  const [running, setRunning] = useState<string | null>(null)
  const [result, setResult] = useState<string | null>(null)

  const runMutation = useMutation({
    mutationFn: (name: string) => api.runTaskAsync(name),
    onSuccess: (data) => {
      setRunning(null)
      setResult(`Execution started: ${data.execution_id}`)
      queryClient.invalidateQueries({ queryKey: ['executions'] })
    },
    onError: (err) => { setRunning(null); setResult(`Error: ${err.message}`) },
  })

  if (isLoading) return <p className="text-gray-500">Loading...</p>

  return (
    <div>
      <h2 className="text-xl font-bold mb-4">Tasks</h2>
      {result && <p className="text-sm mb-4 p-2 bg-gray-100 rounded">{result}</p>}
      <div className="space-y-3">
        {tasks?.map((t) => (
          <div key={t.name} className="bg-white p-4 rounded-lg shadow flex justify-between items-start">
            <div>
              <h3 className="font-semibold">{t.name}</h3>
              <p className="text-sm text-gray-500 mt-1">
                {t.schedule && <span className="mr-3">Schedule: {t.schedule}</span>}
                {t.tags?.map((tag) => (
                  <span key={tag} className="inline-block bg-gray-100 text-gray-600 text-xs px-2 py-0.5 rounded mr-1">{tag}</span>
                ))}
              </p>
            </div>
            <button
              onClick={() => { setRunning(t.name); runMutation.mutate(t.name) }}
              disabled={running === t.name}
              className="px-3 py-1 bg-gray-900 text-white text-sm rounded hover:bg-gray-800 disabled:opacity-50"
            >
              {running === t.name ? 'Running...' : 'Run'}
            </button>
          </div>
        ))}
      </div>
    </div>
  )
}
