import { Routes, Route, Link, useLocation } from 'react-router-dom'
import { useEffect, useState, useCallback } from 'react'
import Dashboard from './components/Dashboard'
import TaskList from './components/TaskList'
import SubAgentList from './components/SubAgentList'
import PipelineList from './components/PipelineList'
import ExecutionHistory from './components/ExecutionHistory'
import MCPServerList from './components/MCPServerList'
import Wizard from './components/Wizard'
import Login from './components/Login'
import { ToastContainer, useToast } from './components/Toast'
import { useSSE, type SSEEvent } from './hooks/useSSE'

const navItems = [
  { path: '/', label: 'Dashboard' },
  { path: '/wizard', label: 'Wizard' },
  { path: '/tasks', label: 'Tasks' },
  { path: '/subagents', label: 'Sub-Agents' },
  { path: '/pipelines', label: 'Pipelines' },
  { path: '/mcp-servers', label: 'MCP Servers' },
  { path: '/executions', label: 'Executions' },
]

function useTheme() {
  const [dark, setDark] = useState(() => localStorage.getItem('theme') === 'dark')

  useEffect(() => {
    document.documentElement.classList.toggle('dark', dark)
    localStorage.setItem('theme', dark ? 'dark' : 'light')
  }, [dark])

  return { dark, toggle: () => setDark((d) => !d) }
}

export default function App() {
  const location = useLocation()
  const token = localStorage.getItem('token')
  const { dark, toggle } = useTheme()
  const { toasts, addToast, removeToast } = useToast()

  const handleSSEEvent = useCallback((event: SSEEvent) => {
    const name = event.data.task || event.data.pipeline || ''
    switch (event.type) {
      case 'task.started':
        addToast(`Task "${name}" started`, 'info')
        break
      case 'task.completed':
        if (event.data.status === 'failed') {
          addToast(`Task "${name}" failed`, 'error')
        } else {
          addToast(`Task "${name}" completed`, 'success')
        }
        break
      case 'pipeline.started':
        addToast(`Pipeline "${name}" started`, 'info')
        break
      case 'pipeline.completed':
        if (event.data.status === 'failed') {
          addToast(`Pipeline "${name}" failed`, 'error')
        } else {
          addToast(`Pipeline "${name}" completed`, 'success')
        }
        break
      case 'task.cancelled':
        addToast(`Task "${name}" cancelled`, 'info')
        break
    }
  }, [addToast])

  useSSE(token ? handleSSEEvent : undefined)

  if (!token && location.pathname !== '/login') {
    return <Login />
  }

  return (
    <div className="min-h-screen flex bg-white dark:bg-gray-950 text-gray-900 dark:text-gray-100">
      <nav className="w-56 bg-gray-900 dark:bg-gray-950 dark:border-r dark:border-gray-800 text-white p-4 space-y-2">
        <h1 className="text-lg font-bold mb-6">Claude Ecosystem</h1>
        {navItems.map((item) => (
          <Link
            key={item.path}
            to={item.path}
            className={`block px-3 py-2 rounded text-sm ${
              location.pathname === item.path
                ? 'bg-gray-700 text-white'
                : 'text-gray-300 hover:bg-gray-800'
            }`}
          >
            {item.label}
          </Link>
        ))}
        <div className="pt-6 space-y-2">
          <button
            onClick={toggle}
            className="block w-full text-left px-3 py-2 rounded text-sm text-gray-400 hover:bg-gray-800"
          >
            {dark ? 'Light Mode' : 'Dark Mode'}
          </button>
          <button
            onClick={() => { localStorage.removeItem('token'); window.location.reload() }}
            className="block w-full text-left px-3 py-2 rounded text-sm text-gray-400 hover:bg-gray-800"
          >
            Logout
          </button>
        </div>
      </nav>
      <main className="flex-1 p-6 bg-gray-50 dark:bg-gray-900">
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/wizard" element={<Wizard />} />
          <Route path="/tasks" element={<TaskList />} />
          <Route path="/subagents" element={<SubAgentList />} />
          <Route path="/pipelines" element={<PipelineList />} />
          <Route path="/mcp-servers" element={<MCPServerList />} />
          <Route path="/executions" element={<ExecutionHistory />} />
          <Route path="/login" element={<Login />} />
        </Routes>
      </main>
      <ToastContainer toasts={toasts} onRemove={removeToast} />
    </div>
  )
}
