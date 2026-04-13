import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { secretsApi } from '@/api/endpoints'
import type { Secret } from '@/types/models'

export const useSecretsStore = defineStore('secrets', () => {
  const secrets = ref<Secret[]>([])
  const loading = ref(false)

  async function load() {
    loading.value = true
    try {
      secrets.value = (await secretsApi.list()) || []
    } finally {
      loading.value = false
    }
  }

  async function add(label: string, secret?: string) {
    const created = await secretsApi.add(label, secret)
    secrets.value.push(created)
  }

  async function remove(label: string, force = false) {
    await secretsApi.remove(label, force)
    secrets.value = secrets.value.filter((s) => s.label !== label)
  }

  async function rotate(label: string) {
    const updated = await secretsApi.rotate(label)
    const idx = secrets.value.findIndex((s) => s.label === label)
    if (idx !== -1) secrets.value[idx] = updated
  }

  async function toggle(label: string, enabled: boolean) {
    await secretsApi.toggle(label, enabled)
    const sec = secrets.value.find((s) => s.label === label)
    if (sec) sec.enabled = enabled
  }

  async function setLimits(
    label: string,
    maxConns: number,
    maxIPs: number,
    quotaBytes: number,
    expiresAt: string,
  ) {
    await secretsApi.setLimits(label, maxConns, maxIPs, quotaBytes, expiresAt)
    const sec = secrets.value.find((s) => s.label === label)
    if (sec) {
      sec.max_conns = maxConns
      sec.max_ips = maxIPs
      sec.quota_bytes = quotaBytes
      sec.expires_at = expiresAt
    }
  }

  async function updateNotes(label: string, notes: string) {
    await secretsApi.updateNotes(label, notes)
    const sec = secrets.value.find((s) => s.label === label)
    if (sec) sec.notes = notes
  }

  async function resetTraffic(label?: string) {
    await secretsApi.resetTraffic(label)
    if (label) {
      const sec = secrets.value.find((s) => s.label === label)
      if (sec) {
        sec.traffic_in = 0
        sec.traffic_out = 0
      }
    } else {
      secrets.value.forEach((s) => {
        s.traffic_in = 0
        s.traffic_out = 0
      })
    }
    await load()
  }

  const enabledCount = computed(() => secrets.value.filter((s) => s.enabled).length)

  return {
    secrets,
    loading,
    load,
    add,
    remove,
    rotate,
    toggle,
    setLimits,
    updateNotes,
    resetTraffic,
    enabledCount,
  }
})
