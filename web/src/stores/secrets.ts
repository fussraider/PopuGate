import {defineStore} from 'pinia'
import {computed, ref} from 'vue'
import {secretsApi} from '@/api/endpoints'
import {parseJSONTags} from '@/utils/format'
import type {Secret, SecretImportItem} from '@/types/models'

export const useSecretsStore = defineStore('secrets', () => {
  const secrets = ref<Secret[]>([])
  const loading = ref(false)
  const toggling = ref<string | null>(null)
  const rotating = ref<string | null>(null)
  const bulkLoading = ref(false)

  const searchQuery = ref('')
  const searchResults = ref<Secret[]>([])
  const showArchived = ref(false)

  const allTags = ref<string[]>([])
  const selectedTagFilter = ref('')

  async function load(quiet = false) {
    if (!quiet) loading.value = true
    try {
      secrets.value = (await secretsApi.list()) || []
    } finally {
      if (!quiet) loading.value = false
    }
  }

  async function loadTags() {
    try {
      const data = await secretsApi.listTags()
      allTags.value = data.tags || []
    } catch { /* ignore */ }
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
    rotating.value = label
    try {
      const updated = await secretsApi.rotate(label)
      const idx = secrets.value.findIndex((s) => s.label === label)
      if (idx !== -1) secrets.value[idx] = updated
    } finally {
      rotating.value = null
    }
  }

  async function toggle(label: string, enabled: boolean) {
    toggling.value = label
    try {
      await secretsApi.toggle(label, enabled)
      const sec = secrets.value.find((s) => s.label === label)
      if (sec) sec.enabled = enabled
    } finally {
      toggling.value = null
    }
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
    await load()
  }

  async function setTags(label: string, tags: string) {
    await secretsApi.setTags(label, tags)
    const sec = secrets.value.find((s) => s.label === label)
    if (sec) sec.tags = tags
  }

  async function archive(label: string) {
    await secretsApi.archive(label)
    const sec = secrets.value.find((s) => s.label === label)
    if (sec) sec.archived_at = Math.floor(Date.now() / 1000)
  }

  async function unarchive(label: string) {
    await secretsApi.unarchive(label)
    const sec = secrets.value.find((s) => s.label === label)
    if (sec) sec.archived_at = 0
  }

  async function clone(label: string, newLabel: string) {
    const created = await secretsApi.clone(label, newLabel)
    secrets.value.push(created)
  }

  async function rename(label: string, newLabel: string) {
    await secretsApi.rename(label, newLabel)
    const sec = secrets.value.find((s) => s.label === label)
    if (sec) sec.label = newLabel
  }

  async function extend(label: string, days: number) {
    const updated = await secretsApi.extend(label, days)
    const idx = secrets.value.findIndex((s) => s.label === label)
    if (idx !== -1) secrets.value[idx] = updated
  }

  async function disableExpired(): Promise<number> {
    const res = await secretsApi.disableExpired()
    await load()
    return res.disabled
  }

  async function bulkExtend(labels: string[], days: number, tag?: string) {
    bulkLoading.value = true
    try {
      await secretsApi.bulkExtend(labels, days, tag)
      await load()
    } finally {
      bulkLoading.value = false
    }
  }

  async function bulkRotate(labels: string[], tag?: string) {
    bulkLoading.value = true
    try {
      await secretsApi.bulkRotate(labels, tag)
      await load()
    } finally {
      bulkLoading.value = false
    }
  }

  async function bulkToggle(labels: string[], enable: boolean, tag?: string) {
    bulkLoading.value = true
    try {
      await secretsApi.bulkToggle(labels, enable, tag)
      await load()
    } finally {
      bulkLoading.value = false
    }
  }

  async function bulkSetLimits(
    labels: string[],
    limits: { max_conns?: number; max_ips?: number; quota_bytes?: number; expires_at?: string },
    tag?: string,
  ) {
    bulkLoading.value = true
    try {
      await secretsApi.bulkSetLimits(labels, limits, tag)
      await load()
    } finally {
      bulkLoading.value = false
    }
  }

  async function search(query: string) {
    if (!query.trim()) {
      searchQuery.value = ''
      searchResults.value = []
      return
    }
    searchQuery.value = query
    searchResults.value = (await secretsApi.search(query)) || []
  }

  async function loadTop(limit = 10): Promise<Secret[]> {
    return secretsApi.top(limit)
  }

  async function exportAll(): Promise<Secret[]> {
    return secretsApi.exportAll()
  }

  async function importSecrets(items: SecretImportItem[]) {
    const result = await secretsApi.importSecrets(items)
    await load()
    return result
  }

  const enabledCount = computed(() => secrets.value.filter((s) => s.enabled).length)

  const activeSecrets = computed(() =>
    showArchived.value
      ? secrets.value
      : secrets.value.filter((s) => !s.archived_at),
  )

  const displayItems = computed(() =>
    searchQuery.value ? searchResults.value : activeSecrets.value,
  )

  const tagFilteredItems = computed(() => {
    if (!selectedTagFilter.value) return displayItems.value
    return displayItems.value.filter((s) => {
      const tags = parseJSONTags(s.tags)
      return tags.includes(selectedTagFilter.value!)
    })
  })

  return {
    secrets,
    loading,
    toggling,
    rotating,
    bulkLoading,
    searchQuery,
    searchResults,
    showArchived,
    allTags,
    selectedTagFilter,
    load,
    loadTags,
    add,
    remove,
    rotate,
    toggle,
    setLimits,
    updateNotes,
    resetTraffic,
    setTags,
    archive,
    unarchive,
    clone,
    rename,
    extend,
    disableExpired,
    loadTop,
    bulkExtend,
    bulkRotate,
    bulkToggle,
    bulkSetLimits,
    search,
    exportAll,
    importSecrets,
    enabledCount,
    activeSecrets,
    displayItems,
    tagFilteredItems,
  }
})
