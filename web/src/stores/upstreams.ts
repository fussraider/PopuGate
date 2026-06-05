import {defineStore} from 'pinia'
import {ref} from 'vue'
import {upstreamsApi} from '@/api/endpoints'
import {useAuthStore} from '@/stores/auth'
import type {NetInterface, Upstream, UpstreamTestResult} from '@/types/models'

export const useUpstreamsStore = defineStore('upstreams', () => {
  const upstreams = ref<Upstream[]>([])
  const interfaces = ref<NetInterface[]>([])
  const loading = ref(false)
  const testing = ref<Set<string>>(new Set())
  const toggling = ref<Set<string>>(new Set())
  const testingConfig = ref(false)
  const testResult = ref<UpstreamTestResult | null>(null)
  const checkingHealth = ref<Set<string>>(new Set())
  const bulkLoading = ref(false)

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
    await upstreamsApi.add(data)
    await load()
    // Track background health check
    const newSet = new Set(checkingHealth.value)
    newSet.add(data.name)
    checkingHealth.value = newSet
    setTimeout(async () => {
      await load()
      const updatedSet = new Set(checkingHealth.value)
      updatedSet.delete(data.name)
      checkingHealth.value = updatedSet
    }, 3000)
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
    const newSet = new Set(toggling.value)
    newSet.add(name)
    toggling.value = newSet
    try {
      await upstreamsApi.toggle(name, enable)
      const u = upstreams.value.find((x) => x.name === name)
      if (u) u.enabled = enable
    } finally {
      const updatedSet = new Set(toggling.value)
      updatedSet.delete(name)
      toggling.value = updatedSet
    }
  }

  async function test(name: string): Promise<UpstreamTestResult | null> {
    const newSet = new Set(testing.value)
    newSet.add(name)
    testing.value = newSet
    try {
      const result = await upstreamsApi.test(name)
      if (result) {
        const u = upstreams.value.find((x) => x.name === name)
        if (u) {
          u.last_check_ok = result.ok
          u.latency_ms = result.latency_ms ?? 0
          u.last_error = result.error ?? ''
        }
      }
      return result
    } catch {
      return null
    } finally {
      const updatedSet = new Set(testing.value)
      updatedSet.delete(name)
      testing.value = updatedSet
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

  async function bulkCheck(proxies: string[], onUpdate: (data: any) => void) {
    const authStore = useAuthStore()
    const response = await fetch('/api/v1/upstreams/bulk-check', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${authStore.accessToken}`
      },
      body: JSON.stringify({ proxies })
    })

    if (!response.ok) {
      throw new Error(`Server returned ${response.status}: ${response.statusText}`)
    }

    const reader = response.body?.getReader()
    if (!reader) {
      throw new Error('Response stream is not readable')
    }

    const decoder = new TextDecoder('utf-8')
    let buffer = ''

    while (true) {
      const { value, done } = await reader.read()
      if (done) break

      buffer += decoder.decode(value, { stream: true })
      const lines = buffer.split('\n')
      buffer = lines.pop() || ''

      let currentEvent = ''
      for (const line of lines) {
        const trimmed = line.trim()
        if (trimmed.startsWith('event:')) {
          currentEvent = trimmed.slice(6).trim()
        } else if (trimmed.startsWith('data:')) {
          const dataStr = trimmed.slice(5).trim()
          if (currentEvent === 'progress') {
            try {
              const parsed = JSON.parse(dataStr)
              onUpdate(parsed)
            } catch (e) {
              console.error('Failed to parse SSE progress data:', e)
            }
          }
        }
      }
    }
  }

  async function bulkAdd(upstreamsList: any[]) {
    loading.value = true
    try {
      const res = await upstreamsApi.bulkAdd({ upstreams: upstreamsList })
      await load()
      if (res && res.names && Array.isArray(res.names)) {
        const addedNames = res.names
        const newSet = new Set(checkingHealth.value)
        addedNames.forEach((name: string) => {
          newSet.add(name)
        })
        checkingHealth.value = newSet
        setTimeout(async () => {
          await load()
          const updatedSet = new Set(checkingHealth.value)
          addedNames.forEach((name: string) => {
            updatedSet.delete(name)
          })
          checkingHealth.value = updatedSet
        }, 3000)
      }
      return res
    } finally {
      loading.value = false
    }
  }

  async function bulkToggle(names: string[], enable: boolean) {
    bulkLoading.value = true
    try {
      const results = await Promise.allSettled(names.map(name => toggle(name, enable)))
      results.forEach((r, idx) => {
        if (r.status === 'rejected') {
          console.error(`Bulk toggle failed for upstream ${names[idx]}:`, r.reason)
        }
      })
      const succeeded = results.filter(r => r.status === 'fulfilled').length
      return succeeded
    } finally {
      bulkLoading.value = false
    }
  }

  async function bulkRemove(names: string[]) {
    bulkLoading.value = true
    try {
      const results = await Promise.allSettled(names.map(name => remove(name)))
      results.forEach((r, idx) => {
        if (r.status === 'rejected') {
          console.error(`Bulk remove failed for upstream ${names[idx]}:`, r.reason)
        }
      })
      const succeeded = results.filter(r => r.status === 'fulfilled').length
      return succeeded
    } finally {
      bulkLoading.value = false
    }
  }

  async function bulkTest(names: string[]) {
    bulkLoading.value = true
    try {
      const results = await Promise.allSettled(names.map(name => test(name)))
      results.forEach((r, idx) => {
        if (r.status === 'rejected') {
          console.error(`Bulk test failed for upstream ${names[idx]}:`, r.reason)
        }
      })
      const succeeded = results.filter(r => r.status === 'fulfilled' && r.value !== null).length
      return succeeded
    } finally {
      bulkLoading.value = false
    }
  }

  return { upstreams, interfaces, loading, testing, toggling, testingConfig, testResult, checkingHealth, bulkLoading, load, loadInterfaces, add, update, remove, toggle, test, testConfig, bulkCheck, bulkAdd, bulkToggle, bulkRemove, bulkTest }
})
