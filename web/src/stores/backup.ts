import { defineStore } from 'pinia'
import { ref } from 'vue'
import { backupApi } from '@/api/endpoints'
import type { BackupInfo } from '@/types/models'

export const useBackupStore = defineStore('backup', () => {
  const backups = ref<BackupInfo[]>([])
  const loading = ref(false)
  const creating = ref(false)
  const restoring = ref(false)

  async function load() {
    loading.value = true
    try {
      backups.value = await backupApi.list()
    } finally {
      loading.value = false
    }
  }

  async function create(label?: string) {
    creating.value = true
    try {
      await backupApi.create(label)
      await load()
    } finally {
      creating.value = false
    }
  }

  async function restore(filename: string) {
    restoring.value = true
    try {
      await backupApi.restore(filename)
    } finally {
      restoring.value = false
    }
  }

  async function remove(filename: string) {
    await backupApi.delete(filename)
    backups.value = backups.value.filter((b) => b.filename !== filename)
  }

  return { backups, loading, creating, restoring, load, create, restore, remove }
})
