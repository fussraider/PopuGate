import { defineStore } from 'pinia'
import { ref } from 'vue'
import { dockerApi } from '@/api/endpoints'
import type { DockerStatus, EngineStatus } from '@/types/models'

export const useDockerStore = defineStore('docker', () => {
  const dockerStatus = ref<DockerStatus | null>(null)
  const engineStatus = ref<EngineStatus | null>(null)
  const loading = ref(false)
  const building = ref(false)
  const buildResult = ref<string>('')

  async function loadDockerStatus() {
    try {
      dockerStatus.value = await dockerApi.status()
    } catch { /* ignore */ }
  }

  async function loadEngineStatus() {
    try {
      engineStatus.value = await dockerApi.engineStatus()
    } catch { /* ignore */ }
  }

  async function installDocker() {
    loading.value = true
    try {
      await dockerApi.install()
      await loadDockerStatus()
    } finally {
      loading.value = false
    }
  }

  async function buildEngine(force = false) {
    building.value = true
    buildResult.value = ''
    try {
      const res = await dockerApi.build(force)
      buildResult.value = `[${res.method}] ${res.version}: ${res.message}`
    } catch (e: any) {
      buildResult.value = `Error: ${e.message}`
    } finally {
      building.value = false
    }
  }

  return { dockerStatus, engineStatus, loading, building, buildResult, loadDockerStatus, loadEngineStatus, installDocker, buildEngine }
})
