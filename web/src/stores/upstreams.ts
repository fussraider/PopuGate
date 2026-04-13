import { defineStore } from 'pinia'
import { ref } from 'vue'
import { upstreamsApi } from '@/api/endpoints'
import type { Upstream } from '@/types/models'

export const useUpstreamsStore = defineStore('upstreams', () => {
  const upstreams = ref<Upstream[]>([])
  const loading = ref(false)

  async function load() {
    loading.value = true
    try {
      upstreams.value = (await upstreamsApi.list()) || []
    } finally {
      loading.value = false
    }
  }

  async function add(data: Omit<Upstream, 'id'>) {
    const created = await upstreamsApi.add(data)
    upstreams.value.push(created)
  }

  async function remove(name: string) {
    await upstreamsApi.remove(name)
    upstreams.value = upstreams.value.filter((u) => u.name !== name)
  }

  async function toggle(name: string, enable: boolean) {
    await upstreamsApi.toggle(name, enable)
    const u = upstreams.value.find((x) => x.name === name)
    if (u) u.enabled = enable
  }

  async function test(name: string) {
    await upstreamsApi.test(name)
  }

  return { upstreams, loading, load, add, remove, toggle, test }
})
