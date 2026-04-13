import { defineStore } from 'pinia'
import { ref } from 'vue'
import { configApi } from '@/api/endpoints'
import type { Settings } from '@/types/models'

export const useConfigStore = defineStore('config', () => {
  const settings = ref<Settings | null>(null)
  const loading = ref(false)

  async function load() {
    loading.value = true
    try {
      settings.value = await configApi.getAll()
    } finally {
      loading.value = false
    }
  }

  async function update(data: Partial<Settings>) {
    settings.value = await configApi.update(data)
  }

  return { settings, loading, load, update }
})
