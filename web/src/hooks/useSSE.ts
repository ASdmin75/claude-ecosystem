import { useEffect, useRef, useCallback } from 'react'
import { useQueryClient } from '@tanstack/react-query'

export interface SSEEvent {
  type: string
  data: Record<string, string>
}

type SSEHandler = (event: SSEEvent) => void

export function useSSE(onEvent?: SSEHandler) {
  const queryClient = useQueryClient()
  const esRef = useRef<EventSource | null>(null)
  const reconnectTimer = useRef<ReturnType<typeof setTimeout>>(undefined)
  const reconnectDelay = useRef(1000)
  const reconnectAttempts = useRef(0)
  const maxReconnectAttempts = 50
  const wasConnected = useRef(false)

  const invalidateAll = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: ['executions'] })
    queryClient.invalidateQueries({ queryKey: ['dashboard'] })
  }, [queryClient])

  const connect = useCallback(() => {
    const token = localStorage.getItem('token')
    if (!token) return

    const es = new EventSource(`/api/v1/events?token=${token}`)
    esRef.current = es

    const eventTypes = [
      'task.started',
      'task.completed',
      'pipeline.started',
      'pipeline.completed',
      'task.cancelled',
    ]

    for (const type of eventTypes) {
      es.addEventListener(type, (e: MessageEvent) => {
        let data: Record<string, string>
        try {
          data = JSON.parse(e.data)
        } catch {
          return
        }

        const evt: SSEEvent = { type, data }

        // Invalidate relevant queries on any event
        invalidateAll()

        if (data.execution_id) {
          queryClient.invalidateQueries({ queryKey: ['execution', data.execution_id] })
        }

        onEvent?.(evt)
      })
    }

    es.onopen = () => {
      // On reconnect, refresh data to catch events missed during disconnection
      if (wasConnected.current) {
        invalidateAll()
      }
      wasConnected.current = true
      reconnectDelay.current = 1000
      reconnectAttempts.current = 0
    }

    es.onerror = () => {
      es.close()
      esRef.current = null
      reconnectAttempts.current++

      if (reconnectAttempts.current >= maxReconnectAttempts) {
        console.error('SSE: max reconnection attempts reached, giving up')
        return
      }

      // Exponential backoff: 1s, 2s, 4s, 8s, max 30s
      reconnectTimer.current = setTimeout(() => {
        connect()
      }, reconnectDelay.current)
      reconnectDelay.current = Math.min(reconnectDelay.current * 2, 30000)
    }
  }, [queryClient, onEvent, invalidateAll])

  useEffect(() => {
    connect()

    // Refresh data when tab becomes visible (SSE events may have been missed)
    const onVisible = () => {
      if (document.visibilityState === 'visible') {
        invalidateAll()
      }
    }
    document.addEventListener('visibilitychange', onVisible)

    return () => {
      esRef.current?.close()
      if (reconnectTimer.current) clearTimeout(reconnectTimer.current)
      document.removeEventListener('visibilitychange', onVisible)
    }
  }, [connect, invalidateAll])
}
