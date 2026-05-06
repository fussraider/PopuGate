import { defineStore } from 'pinia'
import { ref } from 'vue'
import { dockerApi } from '@/api/endpoints'
import { useWebSocket } from '@/composables/useWebSocket'
import type { DockerStatus, EngineStatus, TelemtUpdateStatus, TelemtReleaseListItem } from '@/types/models'

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

  let wsControls: ReturnType<typeof useWebSocket> | null = null

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
    if (telemtUpdateStatus.value?.updating && !wsControls) {
      startUpdateStream()
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
      startUpdateStream()
    } catch { /* error handled by caller */ }
    applyingUpdate.value = false
    await loadEngineStatus()
    await loadTelemtUpdateStatus()
  }

  function startUpdateStream() {
    if (wsControls) return

    const wsUrl = '/api/v1/engine/update/ws'

    wsControls = useWebSocket({
      url: wsUrl,
      onMessage: (data) => {
        telemtUpdateStatus.value = data
        if (!data.updating) {
          stopUpdateStream()
          loadEngineStatus()
        }
      },
    })
    wsControls.connect()
  }

  function stopUpdateStream() {
    if (wsControls) {
      wsControls.disconnect()
      wsControls = null
    }
  }

  return {
    dockerStatus, engineStatus, loading, building, buildResult,
    telemtUpdateStatus, checkingRemote, applyingUpdate,
    releases, selectedRelease,
    loadDockerStatus, loadEngineStatus, installDocker, buildEngine,
    loadTelemtUpdateStatus, loadReleases, checkRemoteTelemt, applyTelemtUpdate,
    stopUpdateStream,
  }
})
