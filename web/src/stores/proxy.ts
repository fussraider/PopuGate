import { defineStore } from 'pinia'
import { ref } from 'vue'
import { proxyApi, healthApi } from '@/api/endpoints'
import { useAuthStore } from '@/stores/auth'
import type { ProxyStatus, HealthStatus } from '@/types/models'

export const useProxyStore = defineStore('proxy', () => {
  const status = ref<ProxyStatus | null>(null)
  const health = ref<HealthStatus | null>(null)
  const loading = ref(false)
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
    try {
      await proxyApi.start()
      await Promise.all([loadStatus(), loadHealth()])
    } finally {
      loading.value = false
    }
  }

  async function stop() {
    loading.value = true
    try {
      await proxyApi.stop()
      await Promise.all([loadStatus(), loadHealth()])
    } finally {
      loading.value = false
    }
  }

  async function restart() {
    loading.value = true
    try {
      await proxyApi.restart()
      await Promise.all([loadStatus(), loadHealth()])
    } finally {
      loading.value = false
    }
  }

  async function reload() {
    loading.value = true
    try {
      await proxyApi.reload()
      await Promise.all([loadStatus(), loadHealth()])
    } finally {
      loading.value = false
    }
  }

  return { status, health, loading, logs, isFollowing, maxLogs, loadStatus, loadHealth, loadLogs, start, stop, restart, reload, startLogsFollow, stopLogsFollow }
})
