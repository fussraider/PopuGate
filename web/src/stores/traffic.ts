import { defineStore } from 'pinia'
import { ref } from 'vue'
import { trafficApi } from '@/api/endpoints'
import type { GlobalTraffic, UserTraffic, LiveMetrics, TrafficHistoryRecord } from '@/types/models'

export const useTrafficStore = defineStore('traffic', () => {
  const global = ref<GlobalTraffic | null>(null)
  const users = ref<UserTraffic[]>([])
  const live = ref<LiveMetrics | null>(null)
  const loading = ref(false)
  const liveLoading = ref(false)
  const liveError = ref(false)
  const autoRefresh = ref(true)
  const history = ref<TrafficHistoryRecord[]>([])
  const historyLoading = ref(false)

  let liveInterval: ReturnType<typeof setInterval> | null = null

  async function load() {
    loading.value = true
    try {
      const data = await trafficApi.get()
      global.value = data.global
      users.value = data.users
    } finally {
      loading.value = false
    }
  }

  async function loadLive() {
    liveLoading.value = true
    try {
      live.value = await trafficApi.getLive()
      liveError.value = false
    } catch {
      liveError.value = true
    } finally {
      liveLoading.value = false
    }
  }

  async function loadHistory(start: number, end: number, label?: string, aggregate?: string) {
    historyLoading.value = true
    try {
      const data = await trafficApi.getHistory(start, end, label, aggregate)
      history.value = data.history ?? []
    } catch {
      history.value = []
    } finally {
      historyLoading.value = false
    }
  }

  function startAutoRefresh() {
    stopAutoRefresh()
    liveInterval = setInterval(() => loadLive(), 5000)
  }

  function stopAutoRefresh() {
    if (liveInterval) {
      clearInterval(liveInterval)
      liveInterval = null
    }
  }

  function toggleAutoRefresh(value: boolean) {
    autoRefresh.value = value
    if (value) {
      loadLive()
      startAutoRefresh()
    } else {
      stopAutoRefresh()
    }
  }

  function reset() {
    live.value = null
    liveError.value = false
  }

  return { global, users, live, loading, liveLoading, liveError, autoRefresh, history, historyLoading, load, loadLive, loadHistory, startAutoRefresh, stopAutoRefresh, toggleAutoRefresh, reset }
})
