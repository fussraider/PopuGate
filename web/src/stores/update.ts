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
    // In Docker mode apply() returns before the sidecar has swapped the
    // containers, so the first successful health check may still be answered
    // by the OLD backend — reloading on it re-serves the stale bundle and the
    // UI keeps showing the previous version. Poll until /health reports the
    // new version, then wait one extra tick so the web container (recreated
    // by the sidecar after the backend) is swapped too.
    const previousVersion = status.value?.current ?? ''
    const targetVersion = result.value?.new_version ?? ''
    let newBackendSeen = false
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
        const health = await healthApi.check()
        const version = health.version ?? ''
        const isNewBackend = targetVersion
          ? version === targetVersion
          : previousVersion
            ? version !== previousVersion
            : true
        if (!isNewBackend) return
        if (!newBackendSeen) {
          newBackendSeen = true
          return
        }
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
