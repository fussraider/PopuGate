import { defineStore } from 'pinia'
import { ref } from 'vue'
import { systemApi } from '@/api/endpoints'
import { useSharedWebSocket } from '@/composables/useWebSocket'
import type { OSType, ServiceStatus, SystemResources } from '@/types/models'

export const useSystemStore = defineStore('system', () => {
  const os = ref<OSType | null>(null)
  const service = ref<ServiceStatus | null>(null)
  const resources = ref<SystemResources | null>(null)
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

  async function loadResources() {
    try {
      resources.value = await systemApi.getResources()
    } catch { /* ignore */ }
  }

  const ws = useSharedWebSocket({
    url: '/api/v1/system/resources/ws',
    onMessage: (data) => {
      resources.value = data
    }
  })

  return { 
    os, service, resources, loading, 
    loadOS, loadServiceStatus, installService, uninstallService, 
    restartService, reloadService, loadResources,
    startResourceStream: ws.start, stopResourceStream: ws.stop
  }
})
