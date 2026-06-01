import {defineStore} from 'pinia'
import {reactive, ref} from 'vue'
import {instancesApi} from '@/api/endpoints'
import type {Instance} from '@/types/models'

export const useInstancesStore = defineStore('instances', () => {
  const instances = ref<Instance[]>([])
  const loading = ref(false)
  const actionLoading = reactive<Map<number, string>>(new Map())
  const bulkLoading = ref(false)

  async function load() {
    loading.value = true
    try {
      instances.value = (await instancesApi.list()) || []
    } finally {
      loading.value = false
    }
  }

  async function removeById(id: number) {
    await instancesApi.remove(id)
    instances.value = instances.value.filter(i => i.id !== id)
  }

  function setActionLoading(id: number, action: string | null) {
    if (action) {
      actionLoading.set(id, action)
    } else {
      actionLoading.delete(id)
    }
  }

  async function bulkAction(ids: number[], action: 'start' | 'stop' | 'reload') {
    bulkLoading.value = true
    try {
      const results = await Promise.allSettled(ids.map(id => instancesApi[action](id)))
      const succeeded = results.filter(r => r.status === 'fulfilled').length
      return succeeded
    } finally {
      bulkLoading.value = false
    }
  }

  async function bulkToggle(ids: number[], enabled: boolean) {
    bulkLoading.value = true
    try {
      const results = await Promise.allSettled(ids.map(id => instancesApi.update(id, { enabled } as Partial<Instance>)))
      const succeeded = results.filter(r => r.status === 'fulfilled').length
      return succeeded
    } finally {
      bulkLoading.value = false
    }
  }

  async function bulkRemove(ids: number[]) {
    bulkLoading.value = true
    try {
      const results = await Promise.allSettled(ids.map(id => instancesApi.remove(id)))
      const succeeded = results.filter(r => r.status === 'fulfilled').length
      if (succeeded > 0) {
        const removedSet = new Set(ids)
        instances.value = instances.value.filter(i => !removedSet.has(i.id))
      }
      return succeeded
    } finally {
      bulkLoading.value = false
    }
  }

  return { instances, loading, actionLoading, bulkLoading, load, removeById, setActionLoading, bulkAction, bulkToggle, bulkRemove }
})
