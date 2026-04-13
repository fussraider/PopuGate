import { defineStore } from 'pinia'
import { ref } from 'vue'
import { replicationApi } from '@/api/endpoints'
import type { Slave, SyncResult, SlaveTestResult } from '@/types/models'

export const useReplicationStore = defineStore('replication', () => {
  const slaves = ref<Slave[]>([])
  const loading = ref(false)
  const syncing = ref(false)
  const status = ref<any>(null)
  const syncResults = ref<SyncResult[]>([])

  async function loadStatus() {
    try {
      status.value = await replicationApi.status()
    } catch { /* ignore */ }
  }

  async function loadSlaves() {
    loading.value = true
    try {
      slaves.value = (await replicationApi.listSlaves()) || []
    } finally {
      loading.value = false
    }
  }

  async function setup(role: string, syncInterval?: number) {
    await replicationApi.setup({ role, sync_interval: syncInterval })
  }

  async function addSlave(host: string, port: number, label: string) {
    const created = await replicationApi.addSlave(host, port, label)
    slaves.value.push(created)
  }

  async function removeSlave(host: string) {
    await replicationApi.removeSlave(host)
    slaves.value = slaves.value.filter((s) => s.host !== host)
  }

  async function sync(host: string) {
    syncing.value = true
    try {
      const result = await replicationApi.sync(host)
      syncResults.value.push(result)
      await loadSlaves()
    } finally {
      syncing.value = false
    }
  }

  async function test(host: string): Promise<SlaveTestResult> {
    return await replicationApi.test(host)
  }

  async function sshKeygen(): Promise<string> {
    const res = await replicationApi.sshKeygen()
    return res.public_key || res.key || ''
  }

  return { slaves, loading, syncing, status, syncResults, loadStatus, loadSlaves, setup, addSlave, removeSlave, sync, test, sshKeygen }
})
