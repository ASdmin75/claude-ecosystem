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
        queryClient.invalidateQueries({ queryKey: ['executions'] })
        queryClient.invalidateQueries({ queryKey: ['dashboard'] })

        if (data.execution_id) {
          queryClient.invalidateQueries({ queryKey: ['execution', data.execution_id] })
        }

        onEvent?.(evt)
      })
    }

    es.onopen = () => {
      reconnectDelay.current = 1000
    }

    es.onerror = () => {
      es.close()
      esRef.current = null
      // Exponential backoff: 1s, 2s, 4s, 8s, max 30s
      reconnectTimer.current = setTimeout(() => {
        connect()
      }, reconnectDelay.current)
      reconnectDelay.current = Math.min(reconnectDelay.current * 2, 30000)
    }
  }, [queryClient, onEvent])

  useEffect(() => {
    connect()
    return () => {
      esRef.current?.close()
      if (reconnectTimer.current) clearTimeout(reconnectTimer.current)
    }
  }, [connect])
}
