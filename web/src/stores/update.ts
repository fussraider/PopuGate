import { defineStore } from 'pinia'
import { ref } from 'vue'
import { updateApi } from '@/api/endpoints'
import type { UpdateStatus, UpdateResult } from '@/types/models'

export const useUpdateStore = defineStore('update', () => {
  const status = ref<UpdateStatus | null>(null)
  const loading = ref(false)
  const applying = ref(false)
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
    try {
      result.value = await updateApi.apply()
    } catch (e: any) {
      error.value = e.message
    } finally {
      applying.value = false
    }
  }

  return { status, loading, applying, result, error, check, apply }
})
