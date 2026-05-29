import {defineStore} from 'pinia'
import {ref} from 'vue'
import {healthApi, updateApi} from '@/api/endpoints'
import type {UpdateResult, UpdateStatus} from '@/types/models'

export const useUpdateStore = defineStore('update', () => {
  const status = ref<UpdateStatus | null>(null)
  const loading = ref(false)
  const applying = ref(false)
  const restarting = ref(false)
  const result = ref<UpdateResult | null>(null)
  const error = ref('')

  async function check() {
    loading.value = true
    error.value = ''
    try {
      status.value = await updateApi.check()
    } catch (e: any) {
      error.value = e.message
    } finally {
      loading.value = false
    }
  }

  async function apply() {
    applying.value = true
    error.value = ''
    result.value = null
    try {
      result.value = await updateApi.apply()
      restarting.value = true
      pollForRestart()
    } catch (e: any) {
      error.value = e.message
    } finally {
      applying.value = false
    }
  }

  function pollForRestart() {
    let attempts = 0
    const maxAttempts = 60
    const interval = setInterval(async () => {
      attempts++
      if (attempts >= maxAttempts) {
        clearInterval(interval)
        restarting.value = false
        return
      }
      try {
        await healthApi.check()
        clearInterval(interval)
        restarting.value = false
        // Bust browser cache by navigating with timestamp param
        const url = new URL(window.location.href)
        url.searchParams.set('_t', Date.now().toString())
        window.location.replace(url.toString())
      } catch {
        // service not yet available
      }
    }, 5000)
  }

  return { status, loading, applying, restarting, result, error, check, apply }
})
