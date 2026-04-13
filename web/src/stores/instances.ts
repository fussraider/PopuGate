import { defineStore } from 'pinia'
import { ref } from 'vue'
import { instancesApi } from '@/api/endpoints'
import type { Instance } from '@/types/models'

export const useInstancesStore = defineStore('instances', () => {
  const instances = ref<Instance[]>([])
  const loading = ref(false)

  async function load() {
    loading.value = true
    try {
      instances.value = (await instancesApi.list()) || []
    } finally {
      loading.value = false
    }
  }

  async function add(port: number, label: string) {
    const created = await instancesApi.add(port, label)
    instances.value.push(created)
  }

  async function remove(port: number) {
    await instancesApi.remove(port)
    instances.value = instances.value.filter((i) => i.port !== port)
  }

  return { instances, loading, load, add, remove }
})
