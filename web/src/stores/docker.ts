import { defineStore } from 'pinia'
import { ref } from 'vue'
import { dockerApi } from '@/api/endpoints'
import { useSharedWebSocket } from '@/composables/useWebSocket'
import type { DockerStatus, EngineStatus, TelemtUpdateStatus, TelemtReleaseListItem, DockerUpdateStatus } from '@/types/models'

export const useDockerStore = defineStore('docker', () => {
  const dockerStatus = ref<DockerStatus | null>(null)
  const engineStatus = ref<EngineStatus | null>(null)
  const loading = ref(false)
  const building = ref(false)
  const buildResult = ref<string>('')

  // telemt engine update state
  const telemtUpdateStatus = ref<TelemtUpdateStatus | null>(null)
  const checkingRemote = ref(false)
  const applyingUpdate = ref(false)
  const releases = ref<TelemtReleaseListItem[]>([])
  const selectedRelease = ref<TelemtReleaseListItem | null>(null)

  // host Docker daemon package update state
  const hostUpdateStatus = ref<DockerUpdateStatus | null>(null)
  const checkingHostRemote = ref(false)
  const applyingHostUpdate = ref(false)

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
      await loadEngineStatus()
    } catch (e: any) {
      buildResult.value = `Error: ${e.message}`
    } finally {
      building.value = false
    }
  }

  async function loadTelemtUpdateStatus() {
    try {
      telemtUpdateStatus.value = await dockerApi.engineUpdateStatus()
    } catch { /* ignore */ }

    // Auto-poll while update is in progress
    if (telemtUpdateStatus.value?.updating && ws.subscribers.value === 0) {
      ws.start()
    }
  }

  async function loadReleases() {
    try {
      releases.value = await dockerApi.engineReleases()
    } catch { /* ignore */ }
  }

  async function checkRemoteTelemt() {
    checkingRemote.value = true
    try {
      telemtUpdateStatus.value = await dockerApi.engineCheckRemote()
      await loadReleases()
    } catch (e: any) {
      // ignore
    } finally {
      checkingRemote.value = false
    }
  }

  async function applyTelemtUpdate(version: string, commit: string) {
    applyingUpdate.value = true
    try {
      await dockerApi.engineApplyUpdate(version, commit)
      ws.start()
    } catch { /* error handled by caller */ }
    applyingUpdate.value = false
    await loadEngineStatus()
    await loadTelemtUpdateStatus()
  }

  async function loadHostUpdateStatus() {
    try {
      hostUpdateStatus.value = await dockerApi.updateStatus()
    } catch { /* ignore */ }
  }

  async function checkHostRemote() {
    checkingHostRemote.value = true
    try {
      hostUpdateStatus.value = await dockerApi.updateCheck()
    } catch { /* ignore */ }
    checkingHostRemote.value = false
  }

  async function applyHostUpdate() {
    applyingHostUpdate.value = true
    try {
      await dockerApi.updateApply()
    } catch { /* error handled by caller */ }
    applyingHostUpdate.value = false
    await loadHostUpdateStatus()
  }

  const ws = useSharedWebSocket({
    url: '/api/v1/engine/update/ws',
    onMessage: (data) => {
      telemtUpdateStatus.value = data
      if (!data.updating) {
        ws.stop()
        loadEngineStatus()
      }
    }
  })

  return {
    dockerStatus, engineStatus, loading, building, buildResult,
    telemtUpdateStatus, checkingRemote, applyingUpdate,
    releases, selectedRelease,
    hostUpdateStatus, checkingHostRemote, applyingHostUpdate,
    loadDockerStatus, loadEngineStatus, installDocker, buildEngine,
    loadTelemtUpdateStatus, loadReleases, checkRemoteTelemt, applyTelemtUpdate,
    loadHostUpdateStatus, checkHostRemote, applyHostUpdate,
    stopUpdateStream: ws.stop,
  }
})
