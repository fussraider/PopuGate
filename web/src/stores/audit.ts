import { defineStore } from 'pinia'
import { ref } from 'vue'
import { auditApi } from '@/api/endpoints'
import type { AuditEntry } from '@/types/models'

export const useAuditStore = defineStore('audit', () => {
  const entries = ref<AuditEntry[]>([])
  const loading = ref(false)
  const limit = ref(100)
  const offset = ref(0)
  const hasMore = ref(true)

  // Filters state
  const selectedUsers = ref<string[]>([])
  const selectedActions = ref<string[]>([])
  const period = ref<string>('all')
  const customFrom = ref<number | null>(null)
  const customTo = ref<number | null>(null)

  // Available filter options fetched from backend
  const availableUsers = ref<string[]>([])
  const availableActions = ref<string[]>([])

  // Computes active from/to timestamps based on period
  function getPeriodTimestamps() {
    if (period.value === 'all') return { from: undefined, to: undefined }
    
    const now = new Date()
    const todayStart = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime() / 1000

    if (period.value === 'today') {
      return { from: todayStart, to: undefined }
    }
    if (period.value === 'yesterday') {
      return { from: todayStart - 86400, to: todayStart }
    }
    if (period.value === 'week') {
      return { from: todayStart - 86400 * 7, to: undefined }
    }
    if (period.value === 'month') {
      return { from: todayStart - 86400 * 30, to: undefined }
    }
    if (period.value === 'custom') {
      return {
        from: customFrom.value || undefined,
        to: customTo.value || undefined
      }
    }
    return { from: undefined, to: undefined }
  }

  async function loadFilters() {
    try {
      const data = await auditApi.filters()
      availableUsers.value = data.users || []
      availableActions.value = data.actions || []
    } catch { /* ignore */ }
  }

  async function load(newLimit = 100, newOffset = 0, quiet = false) {
    if (!quiet) loading.value = true
    try {
      limit.value = newLimit
      offset.value = newOffset
      const { from, to } = getPeriodTimestamps()
      const res = await auditApi.list(
        newLimit,
        newOffset,
        selectedUsers.value.length > 0 ? selectedUsers.value : undefined,
        selectedActions.value.length > 0 ? selectedActions.value : undefined,
        from,
        to
      )
      entries.value = res || []
      hasMore.value = (res || []).length === newLimit
    } finally {
      if (!quiet) loading.value = false
    }
  }

  async function loadMore() {
    if (!hasMore.value) return
    const newOffset = offset.value + limit.value
    loading.value = true
    try {
      const { from, to } = getPeriodTimestamps()
      const more = await auditApi.list(
        limit.value,
        newOffset,
        selectedUsers.value.length > 0 ? selectedUsers.value : undefined,
        selectedActions.value.length > 0 ? selectedActions.value : undefined,
        from,
        to
      )
      offset.value = newOffset
      entries.value = [...entries.value, ...(more || [])]
      hasMore.value = (more || []).length === limit.value
    } finally {
      loading.value = false
    }
  }

  function resetFilters() {
    selectedUsers.value = []
    selectedActions.value = []
    period.value = 'all'
    customFrom.value = null
    customTo.value = null
    load()
  }

  return {
    entries,
    loading,
    limit,
    offset,
    hasMore,
    selectedUsers,
    selectedActions,
    period,
    customFrom,
    customTo,
    availableUsers,
    availableActions,
    loadFilters,
    load,
    loadMore,
    resetFilters
  }
})
