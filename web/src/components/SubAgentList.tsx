import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'
import { useState } from 'react'

export default function SubAgentList() {
  const queryClient = useQueryClient()
  const { data: agents, isLoading } = useQuery({ queryKey: ['subagents'], queryFn: api.listSubAgents })
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState({ name: '', description: '', instructions: '', model: '' })

  const createMutation = useMutation({
    mutationFn: () => api.createSubAgent(form),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['subagents'] })
      setShowForm(false)
      setForm({ name: '', description: '', instructions: '', model: '' })
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (name: string) => api.deleteSubAgent(name),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['subagents'] }),
  })

  if (isLoading) return <p className="text-gray-500">Loading...</p>

  return (
    <div>
      <div className="flex justify-between items-center mb-4">
        <h2 className="text-xl font-bold">Sub-Agents</h2>
        <button onClick={() => setShowForm(!showForm)} className="px-3 py-1 bg-gray-900 text-white text-sm rounded">
          {showForm ? 'Cancel' : 'New Sub-Agent'}
        </button>
      </div>

      {showForm && (
        <form onSubmit={(e) => { e.preventDefault(); createMutation.mutate() }} className="bg-white p-4 rounded-lg shadow mb-4 space-y-3">
          <input placeholder="Name" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} className="w-full px-3 py-2 border rounded text-sm" />
          <input placeholder="Description" value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} className="w-full px-3 py-2 border rounded text-sm" />
          <input placeholder="Model (optional)" value={form.model} onChange={(e) => setForm({ ...form, model: e.target.value })} className="w-full px-3 py-2 border rounded text-sm" />
          <textarea placeholder="Instructions (markdown)" value={form.instructions} onChange={(e) => setForm({ ...form, instructions: e.target.value })} rows={4} className="w-full px-3 py-2 border rounded text-sm" />
          <button type="submit" className="px-3 py-1 bg-gray-900 text-white text-sm rounded">Create</button>
        </form>
      )}

      <div className="space-y-3">
        {agents?.map((a) => (
          <div key={a.name} className="bg-white p-4 rounded-lg shadow flex justify-between items-start">
            <div>
              <h3 className="font-semibold">{a.name}</h3>
              <p className="text-sm text-gray-500">{a.description}</p>
              {a.model && <p className="text-xs text-gray-400 mt-1">Model: {a.model}</p>}
            </div>
            <button onClick={() => deleteMutation.mutate(a.name)} className="text-red-500 text-sm hover:text-red-700">Delete</button>
          </div>
        ))}
        {agents?.length === 0 && <p className="text-gray-400 text-sm">No sub-agents configured.</p>}
      </div>
    </div>
  )
}
