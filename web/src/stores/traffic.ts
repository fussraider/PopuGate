import { defineStore } from 'pinia'
import { ref } from 'vue'
import { trafficApi } from '@/api/endpoints'
import type { GlobalTraffic, UserTraffic, LiveMetrics } from '@/types/models'

export const useTrafficStore = defineStore('traffic', () => {
  const global = ref<GlobalTraffic | null>(null)
  const users = ref<UserTraffic[]>([])
  const live = ref<LiveMetrics | null>(null)
  const loading = ref(false)
  const liveLoading = ref(false)

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
    } finally {
      liveLoading.value = false
    }
  }

  return { global, users, live, loading, liveLoading, load, loadLive }
})
