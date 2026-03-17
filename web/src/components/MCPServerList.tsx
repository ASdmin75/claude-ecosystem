import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'
import type { MCPServer } from '../types'

export default function MCPServerList() {
  const queryClient = useQueryClient()

  const { data: servers, isLoading } = useQuery({
    queryKey: ['mcp-servers'],
    queryFn: () => api.listMCPServers(),
    refetchInterval: 10000,
  })

  const startMutation = useMutation({
    mutationFn: (name: string) => api.startMCPServer(name),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['mcp-servers'] }),
  })

  const stopMutation = useMutation({
    mutationFn: (name: string) => api.stopMCPServer(name),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['mcp-servers'] }),
  })

  if (isLoading) return <p className="text-gray-500 dark:text-gray-400">Loading...</p>

  const running = servers?.filter((s) => s.running).length ?? 0
  const total = servers?.length ?? 0

  return (
    <div>
      <h2 className="text-xl font-bold mb-4">MCP Servers</h2>

      <div className="grid grid-cols-3 gap-4 mb-6">
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow dark:shadow-gray-950 p-4">
          <p className="text-sm text-gray-500 dark:text-gray-400">Total Servers</p>
          <p className="text-2xl font-bold">{total}</p>
        </div>
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow dark:shadow-gray-950 p-4">
          <p className="text-sm text-gray-500 dark:text-gray-400">Running</p>
          <p className="text-2xl font-bold text-green-600 dark:text-green-400">{running}</p>
        </div>
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow dark:shadow-gray-950 p-4">
          <p className="text-sm text-gray-500 dark:text-gray-400">Stopped</p>
          <p className="text-2xl font-bold text-gray-500 dark:text-gray-400">{total - running}</p>
        </div>
      </div>

      <div className="bg-white dark:bg-gray-800 rounded-lg shadow dark:shadow-gray-950 overflow-hidden">
        <table className="w-full text-sm">
          <thead className="bg-gray-50 dark:bg-gray-700/50">
            <tr>
              <th className="text-left px-4 py-3">Server</th>
              <th className="text-left px-4 py-3">Status</th>
              <th className="text-left px-4 py-3">PID</th>
              <th className="text-right px-4 py-3">Actions</th>
            </tr>
          </thead>
          <tbody>
            {servers?.map((server: MCPServer) => (
              <tr key={server.name} className="border-t border-gray-100 dark:border-gray-700">
                <td className="px-4 py-3">
                  <div className="flex items-center gap-2">
                    <span className={`inline-block w-2 h-2 rounded-full ${server.running ? 'bg-green-500' : 'bg-gray-400 dark:bg-gray-600'}`} />
                    <span className="font-medium">{server.name}</span>
                  </div>
                </td>
                <td className="px-4 py-3">
                  <span className={`inline-block px-2 py-0.5 rounded text-xs ${
                    server.running
                      ? 'bg-green-100 text-green-700 dark:bg-green-900/40 dark:text-green-300'
                      : 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-400'
                  }`}>
                    {server.running ? 'running' : 'stopped'}
                  </span>
                </td>
                <td className="px-4 py-3 text-gray-500 dark:text-gray-400 font-mono text-xs">
                  {server.pid || '-'}
                </td>
                <td className="px-4 py-3 text-right">
                  {server.running ? (
                    <button
                      onClick={() => stopMutation.mutate(server.name)}
                      disabled={stopMutation.isPending}
                      className="px-3 py-1 bg-red-600 text-white text-xs rounded hover:bg-red-700 disabled:opacity-50"
                    >
                      {stopMutation.isPending ? 'Stopping...' : 'Stop'}
                    </button>
                  ) : (
                    <button
                      onClick={() => startMutation.mutate(server.name)}
                      disabled={startMutation.isPending}
                      className="px-3 py-1 bg-green-600 text-white text-xs rounded hover:bg-green-700 disabled:opacity-50"
                    >
                      {startMutation.isPending ? 'Starting...' : 'Start'}
                    </button>
                  )}
                </td>
              </tr>
            ))}
            {servers?.length === 0 && (
              <tr>
                <td colSpan={4} className="px-4 py-8 text-center text-gray-400 dark:text-gray-500">
                  No MCP servers configured.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {(startMutation.error || stopMutation.error) && (
        <p className="mt-4 text-sm text-red-600 dark:text-red-400 bg-red-50 dark:bg-red-900/20 p-3 rounded">
          {(startMutation.error || stopMutation.error)?.message}
        </p>
      )}
    </div>
  )
}
