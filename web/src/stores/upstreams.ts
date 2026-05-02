import { defineStore } from 'pinia'
import { ref } from 'vue'
import { upstreamsApi } from '@/api/endpoints'
import type { Upstream, NetInterface, UpstreamTestResult } from '@/types/models'

export const useUpstreamsStore = defineStore('upstreams', () => {
  const upstreams = ref<Upstream[]>([])
  const interfaces = ref<NetInterface[]>([])
  const loading = ref(false)
  const testing = ref<string | null>(null)
  const toggling = ref<string | null>(null)
  const testingConfig = ref(false)
  const testResult = ref<UpstreamTestResult | null>(null)

  async function load() {
    loading.value = true
    try {
      upstreams.value = (await upstreamsApi.list()) || []
    } finally {
      loading.value = false
    }
  }

  async function loadInterfaces() {
    interfaces.value = (await upstreamsApi.interfaces()) || []
  }

  async function add(data: Omit<Upstream, 'id'>) {
    const created = await upstreamsApi.add(data)
    upstreams.value.push(created)
  }

  async function update(name: string, data: Omit<Upstream, 'id' | 'name' | 'enabled'>) {
    const updated = await upstreamsApi.update(name, data)
    const idx = upstreams.value.findIndex((u) => u.name === name)
    if (idx !== -1) upstreams.value[idx] = updated
  }

  async function remove(name: string) {
    await upstreamsApi.remove(name)
    upstreams.value = upstreams.value.filter((u) => u.name !== name)
  }

  async function toggle(name: string, enable: boolean) {
    toggling.value = name
    try {
      await upstreamsApi.toggle(name, enable)
      const u = upstreams.value.find((x) => x.name === name)
      if (u) u.enabled = enable
    } finally {
      toggling.value = null
    }
  }

  async function test(name: string): Promise<UpstreamTestResult | null> {
    testing.value = name
    try {
      return await upstreamsApi.test(name)
    } catch {
      return null
    } finally {
      testing.value = null
    }
  }

  async function testConfig(data: { type: string; address?: string; username?: string; password?: string; iface?: string }) {
    testingConfig.value = true
    testResult.value = null
    try {
      testResult.value = await upstreamsApi.testConfig(data)
    } catch {
      testResult.value = { ok: false, error: 'Connection failed' }
    } finally {
      testingConfig.value = false
    }
  }

  return { upstreams, interfaces, loading, testing, toggling, testingConfig, testResult, load, loadInterfaces, add, update, remove, toggle, test, testConfig }
})
