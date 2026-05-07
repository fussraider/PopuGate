import { ref } from 'vue'
import { useAuthStore } from '@/stores/auth'

interface UseWebSocketOptions {
  url: string
  onMessage: (data: any) => void
  onError?: () => void
}

export function useWebSocket(options: UseWebSocketOptions) {
  const { url, onMessage, onError } = options
  const connected = ref(false)

  let ws: WebSocket | null = null
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null
  let backoff = 1000
  let stopped = false

  function connect() {
    stopped = false
    const authStore = useAuthStore()
    
    let wsUrl = url
    if (url.startsWith('/')) {
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      wsUrl = `${protocol}//${window.location.host}${url}`
    }
    
    wsUrl += (wsUrl.includes('?') ? '&' : '?') + `token=${encodeURIComponent(authStore.accessToken || '')}`
    ws = new WebSocket(wsUrl)

    ws.onopen = () => {
      connected.value = true
      backoff = 1000
    }

    ws.onmessage = (event) => {
      try {
        onMessage(JSON.parse(event.data))
      } catch { /* ignore malformed */ }
    }

    ws.onclose = () => {
      connected.value = false
      if (!stopped) scheduleReconnect()
    }

    ws.onerror = () => {
      onError?.()
    }
  }

  function scheduleReconnect() {
    if (stopped) return
    if (reconnectTimer) clearTimeout(reconnectTimer)
    reconnectTimer = setTimeout(() => {
      connect()
      backoff = Math.min(backoff * 2, 30000)
    }, backoff)
  }

  function disconnect() {
    stopped = true
    if (reconnectTimer) clearTimeout(reconnectTimer)
    ws?.close()
    ws = null
    connected.value = false
  }

  return { connected, connect, disconnect }
}
