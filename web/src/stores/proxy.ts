import { defineStore } from 'pinia'
import { ref } from 'vue'
import { proxyApi, healthApi } from '@/api/endpoints'
import { useAuthStore } from '@/stores/auth'
import { useSharedWebSocket } from '@/composables/useWebSocket'
import type { ProxyStatus, HealthStatus } from '@/types/models'

export type ProxyAction = 'start' | 'stop' | 'restart' | 'reload' | 'reloadZeroDowntime'

export const useProxyStore = defineStore('proxy', () => {
  const status = ref<ProxyStatus | null>(null)
  const health = ref<HealthStatus | null>(null)
  const loading = ref(false)
  const activeAction = ref<ProxyAction | null>(null)
  const logs = ref('')
  const isFollowing = ref(false)
  const maxLogs = ref(200)
  let eventSource: EventSource | null = null

  async function loadStatus() {
    try {
      status.value = await proxyApi.status()
    } catch { /* ignore */ }
  }

  async function loadHealth() {
    try {
      health.value = await healthApi.check()
    } catch { /* ignore */ }
  }

  function truncateLogs(text: string, limit: number): string {
    const lines = text.split('\n')
    if (lines.length > limit) {
      return lines.slice(-limit).join('\n')
    }
    return text
  }

  async function loadLogs() {
    try {
      const newLogs = await proxyApi.logs(maxLogs.value.toString())
      logs.value = truncateLogs(newLogs, maxLogs.value)
    } catch { /* ignore */ }
  }

  function stopLogsFollow() {
    if (eventSource) {
      eventSource.close()
      eventSource = null
    }
    isFollowing.value = false
  }

  function startLogsFollow() {
    stopLogsFollow()
    isFollowing.value = true
    
    // Get token from auth store
    const authStore = useAuthStore()
    const token = authStore.accessToken || ''
    
    const url = `/api/v1/proxy/logs?tail=${maxLogs.value}&follow=true&token=${encodeURIComponent(token)}`
    eventSource = new EventSource(url)
    
    eventSource.onmessage = (event) => {
      if (event.data) {
        logs.value = truncateLogs(logs.value + event.data + '\n', maxLogs.value)
      }
    }
    
    eventSource.onerror = () => {
      stopLogsFollow()
    }
  }

  async function start() {
    loading.value = true
    activeAction.value = 'start'
    try {
      await proxyApi.start()
      await refreshState()
    } finally {
      loading.value = false
      activeAction.value = null
    }
  }

  async function stop() {
    loading.value = true
    activeAction.value = 'stop'
    try {
      await proxyApi.stop()
      await refreshState()
    } finally {
      loading.value = false
      activeAction.value = null
    }
  }

  async function restart() {
    loading.value = true
    activeAction.value = 'restart'
    try {
      await proxyApi.restart()
      await refreshState()
    } finally {
      loading.value = false
      activeAction.value = null
    }
  }

  async function reload() {
    loading.value = true
    activeAction.value = 'reload'
    try {
      await proxyApi.reload()
      await refreshState()
    } finally {
      loading.value = false
      activeAction.value = null
    }
  }

  async function reloadZeroDowntime() {
    loading.value = true
    activeAction.value = 'reloadZeroDowntime'
    try {
      await proxyApi.reloadZeroDowntime()
      await refreshState()
    } finally {
      loading.value = false
      activeAction.value = null
    }
  }

  const ws = useSharedWebSocket({
    url: '/api/v1/proxy/status/ws',
    onMessage: (data) => {
      status.value = data
    }
  })

  async function refreshState() {
    await Promise.all([loadStatus(), loadHealth()])
  }

  return { 
    status, health, loading, activeAction, logs, isFollowing, maxLogs, 
    wsConnected: ws.connected, wsStatus: ws.status, 
    loadStatus, loadHealth, loadLogs, start, stop, restart, reload, reloadZeroDowntime, refreshState, 
    startLogsFollow, stopLogsFollow, startStatusStream: ws.start, stopStatusStream: ws.stop 
  }
})
