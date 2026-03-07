import { Routes, Route, Link, useLocation } from 'react-router-dom'
import Dashboard from './components/Dashboard'
import TaskList from './components/TaskList'
import SubAgentList from './components/SubAgentList'
import PipelineList from './components/PipelineList'
import ExecutionHistory from './components/ExecutionHistory'
import Login from './components/Login'

const navItems = [
  { path: '/', label: 'Dashboard' },
  { path: '/tasks', label: 'Tasks' },
  { path: '/subagents', label: 'Sub-Agents' },
  { path: '/pipelines', label: 'Pipelines' },
  { path: '/executions', label: 'Executions' },
]

export default function App() {
  const location = useLocation()
  const token = localStorage.getItem('token')

  if (!token && location.pathname !== '/login') {
    return <Login />
  }

  return (
    <div className="min-h-screen flex">
      <nav className="w-56 bg-gray-900 text-white p-4 space-y-2">
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
        <button
          onClick={() => { localStorage.removeItem('token'); window.location.reload() }}
          className="block w-full text-left px-3 py-2 rounded text-sm text-gray-400 hover:bg-gray-800 mt-8"
        >
          Logout
        </button>
      </nav>
      <main className="flex-1 p-6">
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/tasks" element={<TaskList />} />
          <Route path="/subagents" element={<SubAgentList />} />
          <Route path="/pipelines" element={<PipelineList />} />
          <Route path="/executions" element={<ExecutionHistory />} />
          <Route path="/login" element={<Login />} />
        </Routes>
      </main>
    </div>
  )
}
