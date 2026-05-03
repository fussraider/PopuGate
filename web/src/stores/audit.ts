import { defineStore } from 'pinia'
import { ref } from 'vue'
import { auditApi } from '@/api/endpoints'
import type { AuditEntry } from '@/types/models'

export const useAuditStore = defineStore('audit', () => {
  const entries = ref<AuditEntry[]>([])
  const loading = ref(false)
  const limit = ref(100)
  const offset = ref(0)

  async function load(newLimit = 100, newOffset = 0) {
    loading.value = true
    try {
      limit.value = newLimit
      offset.value = newOffset
      entries.value = (await auditApi.list(newLimit, newOffset)) || []
    } finally {
      loading.value = false
    }
  }

  async function loadMore() {
    const newOffset = offset.value + limit.value
    loading.value = true
    try {
      const more = (await auditApi.list(limit.value, newOffset)) || []
      offset.value = newOffset
      entries.value = [...entries.value, ...more]
    } finally {
      loading.value = false
    }
  }

  return { entries, loading, limit, offset, load, loadMore }
})
