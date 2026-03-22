import type { DeleteAnalysis } from '../types'

const typeBadge: Record<string, { bg: string; text: string; label: string }> = {
  task: { bg: 'bg-blue-100 dark:bg-blue-900/40', text: 'text-blue-700 dark:text-blue-300', label: 'Task' },
  pipeline: { bg: 'bg-purple-100 dark:bg-purple-900/40', text: 'text-purple-700 dark:text-purple-300', label: 'Pipeline' },
  subagent: { bg: 'bg-green-100 dark:bg-green-900/40', text: 'text-green-700 dark:text-green-300', label: 'Agent' },
  domain: { bg: 'bg-orange-100 dark:bg-orange-900/40', text: 'text-orange-700 dark:text-orange-300', label: 'Domain' },
}

interface ConfirmModalProps {
  open: boolean
  title: string
  message: string
  details?: DeleteAnalysis | null
  confirmLabel?: string
  onConfirm: () => void
  onCancel: () => void
  loading?: boolean
}

export default function ConfirmModal({ open, title, message, details, confirmLabel = 'Delete', onConfirm, onCancel, loading }: ConfirmModalProps) {
  if (!open) return null

  const blocked = details?.blocked ?? false

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="fixed inset-0 bg-black/50" onClick={onCancel} />
      <div className="relative bg-white dark:bg-gray-800 rounded-lg shadow-xl dark:shadow-gray-950 p-6 max-w-md w-full mx-4">
        <h3 className="text-lg font-semibold mb-2">{title}</h3>
        <p className="text-sm text-gray-600 dark:text-gray-400 mb-4">{message}</p>

        {details && (
          <div className="space-y-3 mb-4">
            {blocked && details.block_reason && (
              <div className="p-3 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded text-sm text-red-700 dark:text-red-300">
                {details.block_reason}
              </div>
            )}

            {details.used_by && details.used_by.length > 0 && (
              <div>
                <p className="text-xs font-medium text-gray-500 dark:text-gray-400 mb-1">Used by:</p>
                <div className="flex flex-wrap gap-1">
                  {details.used_by.map((dep) => {
                    const badge = typeBadge[dep.type] || typeBadge.task
                    return (
                      <span key={`${dep.type}:${dep.name}`} className={`text-xs px-2 py-0.5 rounded ${badge.bg} ${badge.text}`}>
                        {badge.label}: {dep.name}
                      </span>
                    )
                  })}
                </div>
              </div>
            )}

            {details.cascade_items && details.cascade_items.length > 0 && (
              <div>
                <p className="text-xs font-medium text-gray-500 dark:text-gray-400 mb-1">Will also be deleted:</p>
                <div className="flex flex-wrap gap-1">
                  {details.cascade_items.map((dep) => {
                    const badge = typeBadge[dep.type] || typeBadge.task
                    return (
                      <span key={`${dep.type}:${dep.name}`} className={`text-xs px-2 py-0.5 rounded ${badge.bg} ${badge.text}`}>
                        {badge.label}: {dep.name}
                      </span>
                    )
                  })}
                </div>
              </div>
            )}
          </div>
        )}

        <div className="flex justify-end gap-3">
          <button
            onClick={onCancel}
            className="px-4 py-2 text-sm text-gray-600 dark:text-gray-400 hover:text-gray-800 dark:hover:text-gray-200 border border-gray-200 dark:border-gray-600 rounded"
          >
            Cancel
          </button>
          <button
            onClick={onConfirm}
            disabled={blocked || loading}
            className="px-4 py-2 text-sm bg-red-600 text-white rounded hover:bg-red-700 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {loading ? 'Deleting...' : confirmLabel}
          </button>
        </div>
      </div>
    </div>
  )
}
