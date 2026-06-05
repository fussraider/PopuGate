import { defineStore } from 'pinia'
import { ref } from 'vue'
import { systemApi } from '@/api/endpoints'
import { useWebSocket } from '@/composables/useWebSocket'
import type { OSType, ServiceStatus, SystemResources } from '@/types/models'

export const useSystemStore = defineStore('system', () => {
  const os = ref<OSType | null>(null)
  const service = ref<ServiceStatus | null>(null)
  const resources = ref<SystemResources | null>(null)
  const loading = ref(false)
  
  let wsControls: ReturnType<typeof useWebSocket> | null = null

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

  function startResourceStream() {
    if (wsControls) return

    const wsUrl = '/api/v1/system/resources/ws'
    
    const currentControls = useWebSocket({
      url: wsUrl,
      onMessage: (data) => {
        if (wsControls !== currentControls) return
        resources.value = data
      }
    })
    wsControls = currentControls
    wsControls.connect()
  }

  function stopResourceStream() {
    wsControls?.disconnect()
    wsControls = null
  }

  return { 
    os, service, resources, loading, 
    loadOS, loadServiceStatus, installService, uninstallService, 
    restartService, reloadService, loadResources,
    startResourceStream, stopResourceStream
  }
})
