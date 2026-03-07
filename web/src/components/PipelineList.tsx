import { useQuery, useMutation } from '@tanstack/react-query'
import { api } from '../api/client'
import { useState } from 'react'

export default function PipelineList() {
  const { data: pipelines, isLoading } = useQuery({ queryKey: ['pipelines'], queryFn: api.listPipelines })
  const [result, setResult] = useState<string | null>(null)

  const runMutation = useMutation({
    mutationFn: (name: string) => api.runPipelineAsync(name),
    onSuccess: (data) => setResult(`Pipeline execution started: ${data.execution_id}`),
    onError: (err) => setResult(`Error: ${err.message}`),
  })

  if (isLoading) return <p className="text-gray-500">Loading...</p>

  return (
    <div>
      <h2 className="text-xl font-bold mb-4">Pipelines</h2>
      {result && <p className="text-sm mb-4 p-2 bg-gray-100 rounded">{result}</p>}
      <div className="space-y-3">
        {pipelines?.map((p) => (
          <div key={p.name} className="bg-white p-4 rounded-lg shadow flex justify-between items-start">
            <div>
              <h3 className="font-semibold">{p.name}</h3>
              <p className="text-sm text-gray-500">
                Mode: {p.mode} | Steps: {p.steps.map((s) => s.task).join(' -> ')} | Max: {p.max_iterations}
              </p>
            </div>
            <button onClick={() => runMutation.mutate(p.name)} className="px-3 py-1 bg-gray-900 text-white text-sm rounded hover:bg-gray-800">Run</button>
          </div>
        ))}
      </div>
    </div>
  )
}
