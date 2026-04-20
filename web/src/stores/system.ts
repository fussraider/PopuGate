import { defineStore } from 'pinia'
import { ref } from 'vue'
import { systemApi } from '@/api/endpoints'
import type { OSType, ServiceStatus } from '@/types/models'

export const useSystemStore = defineStore('system', () => {
  const os = ref<OSType | null>(null)
  const service = ref<ServiceStatus | null>(null)
  const loading = ref(false)

  async function loadOS() {
    try {
      os.value = await systemApi.getOS()
    } catch { /* ignore */ }
  }

  async function loadServiceStatus() {
    try {
      service.value = await systemApi.serviceStatus()
    } catch { /* ignore */ }
  }

  async function installService() {
    loading.value = true
    try {
      await systemApi.installService()
      await loadServiceStatus()
    } finally {
      loading.value = false
    }
  }

  async function uninstallService() {
    loading.value = true
    try {
      await systemApi.uninstallService()
      await loadServiceStatus()
    } finally {
      loading.value = false
    }
  }

  async function restartService() {
    loading.value = true
    try {
      await systemApi.restartService()
      await loadServiceStatus()
    } finally {
      loading.value = false
    }
  }

  async function reloadService() {
    loading.value = true
    try {
      await systemApi.reloadService()
      await loadServiceStatus()
    } finally {
      loading.value = false
    }
  }

  return { os, service, loading, loadOS, loadServiceStatus, installService, uninstallService, restartService, reloadService }
})
