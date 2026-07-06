import { ref, computed } from 'vue'
import { useAuthStore } from '@/stores/auth'

interface UseWebSocketOptions {
  url: string
  onMessage: (data: any) => void
  onError?: () => void
  onOpen?: () => void
  onClose?: () => void
}

export function useWebSocket(options: UseWebSocketOptions) {
  const { url, onMessage, onError, onOpen, onClose } = options
  const connected = ref(false)

  let ws: WebSocket | null = null
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null
  let backoff = 1000
  let stopped = false
  const handleUnload = () => {
    disconnect()
  }

  function connect() {
    stopped = false
    backoff = 1000
    const authStore = useAuthStore()
    
    let wsUrl = url
    if (url.startsWith('/')) {
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      wsUrl = `${protocol}//${window.location.host}${url}`
    }
    
    wsUrl += (wsUrl.includes('?') ? '&' : '?') + `token=${encodeURIComponent(authStore.accessToken || '')}`
    
    window.addEventListener('beforeunload', handleUnload)
    ws = new WebSocket(wsUrl)

    ws.onopen = () => {
      if (stopped) return
      connected.value = true
      backoff = 1000
      onOpen?.()
    }

    ws.onmessage = (event) => {
      if (stopped) return
      try {
        onMessage(JSON.parse(event.data))
      } catch { /* ignore malformed */ }
    }

    ws.onclose = () => {
      if (stopped) return
      connected.value = false
      scheduleReconnect()
      onClose?.()
    }

    ws.onerror = () => {
      if (stopped) return
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
    window.removeEventListener('beforeunload', handleUnload)
    ws?.close()
    ws = null
    connected.value = false
  }

  return { connected, connect, disconnect }
}

export function useSharedWebSocket(options: UseWebSocketOptions) {
  const status = ref<'connected' | 'connecting' | 'disconnected'>('disconnected')
  const connected = computed(() => status.value === 'connected')
  const subscribers = ref(0)

  const ws = useWebSocket({
    url: options.url,
    onMessage: options.onMessage,
    onOpen: () => {
      status.value = 'connected'
      options.onOpen?.()
    },
    onClose: () => {
      if (status.value !== 'disconnected') {
        status.value = 'connecting'
      }
      options.onClose?.()
    },
    onError: () => {
      if (status.value !== 'disconnected') {
        status.value = 'connecting'
      }
      options.onError?.()
    }
  })

  function start() {
    subscribers.value++
    if (subscribers.value === 1) {
      status.value = 'connecting'
      ws.connect()
    }
  }

  function stop() {
    subscribers.value = Math.max(0, subscribers.value - 1)
    if (subscribers.value === 0) {
      status.value = 'disconnected'
      ws.disconnect()
    }
  }

  return {
    status,
    connected,
    start,
    stop,
    subscribers: computed(() => subscribers.value)
  }
}
