import { useState, useCallback, useRef } from 'react'

interface ToastItem {
  id: number
  message: string
  type: 'success' | 'error' | 'info'
}

const typeStyles: Record<string, string> = {
  success: 'bg-green-600 text-white',
  error: 'bg-red-600 text-white',
  info: 'bg-blue-600 text-white',
}

let nextId = 0

export function useToast() {
  const [toasts, setToasts] = useState<ToastItem[]>([])
  const timers = useRef<Map<number, ReturnType<typeof setTimeout>>>(new Map())

  const addToast = useCallback((message: string, type: ToastItem['type'] = 'info') => {
    const id = ++nextId
    setToasts((prev) => [...prev, { id, message, type }])
    const timer = setTimeout(() => {
      setToasts((prev) => prev.filter((t) => t.id !== id))
      timers.current.delete(id)
    }, 5000)
    timers.current.set(id, timer)
  }, [])

  const removeToast = useCallback((id: number) => {
    setToasts((prev) => prev.filter((t) => t.id !== id))
    const timer = timers.current.get(id)
    if (timer) {
      clearTimeout(timer)
      timers.current.delete(id)
    }
  }, [])

  return { toasts, addToast, removeToast }
}

export function ToastContainer({ toasts, onRemove }: { toasts: ToastItem[]; onRemove: (id: number) => void }) {
  if (toasts.length === 0) return null

  return (
    <div className="fixed bottom-4 right-4 z-50 flex flex-col gap-2 max-w-sm">
      {toasts.map((t) => (
        <div
          key={t.id}
          className={`px-4 py-3 rounded-lg shadow-lg text-sm flex items-center justify-between gap-3 animate-slide-in ${typeStyles[t.type]}`}
        >
          <span>{t.message}</span>
          <button onClick={() => onRemove(t.id)} className="opacity-70 hover:opacity-100 text-lg leading-none">&times;</button>
        </div>
      ))}
    </div>
  )
}
