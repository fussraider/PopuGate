import { defineStore } from 'pinia'
import { ref } from 'vue'
import { dockerApi } from '@/api/endpoints'
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
  const pollTimer = ref<ReturnType<typeof setInterval> | null>(null)
  const releases = ref<TelemtReleaseListItem[]>([])
  const selectedRelease = ref<TelemtReleaseListItem | null>(null)

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
    if (telemtUpdateStatus.value?.updating && !pollTimer.value) {
      startUpdatePoll()
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
    } catch { /* error handled by caller */ }
    applyingUpdate.value = false
    await loadEngineStatus()
    await loadTelemtUpdateStatus()
  }

  function startUpdatePoll() {
    if (pollTimer.value) return
    pollTimer.value = setInterval(async () => {
      try {
        telemtUpdateStatus.value = await dockerApi.engineUpdateStatus()
        if (!telemtUpdateStatus.value?.updating) {
          stopUpdatePoll()
          await loadEngineStatus()
        }
      } catch { /* ignore */ }
    }, 5000)
  }

  function stopUpdatePoll() {
    if (pollTimer.value) {
      clearInterval(pollTimer.value)
      pollTimer.value = null
    }
  }

  return {
    dockerStatus, engineStatus, loading, building, buildResult,
    telemtUpdateStatus, checkingRemote, applyingUpdate,
    releases, selectedRelease,
    loadDockerStatus, loadEngineStatus, installDocker, buildEngine,
    loadTelemtUpdateStatus, loadReleases, checkRemoteTelemt, applyTelemtUpdate,
    stopUpdatePoll,
  }
})
