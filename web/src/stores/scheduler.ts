import { defineStore } from 'pinia'
import { ref } from 'vue'
import { schedulerApi } from '@/api/endpoints'
import type { SchedulerTask, SchedulerHistoryRecord } from '@/types/models'

export const useSchedulerStore = defineStore('scheduler', () => {
  const tasks = ref<SchedulerTask[]>([])
  const history = ref<SchedulerHistoryRecord[]>([])
  const loading = ref(false)
  const toggling = ref<string | null>(null)
  const running = ref<string | null>(null)

  async function load(quiet = false) {
    if (!quiet) loading.value = true
    try {
      tasks.value = (await schedulerApi.listTasks()) || []
    } finally {
      if (!quiet) loading.value = false
    }
  }

  async function toggle(name: string, enabled: boolean) {
    toggling.value = name
    try {
      await schedulerApi.updateTask(name, { enabled })
      const t = tasks.value.find(x => x.name === name)
      if (t) t.enabled = enabled
    } finally {
      toggling.value = null
    }
  }

  async function updateSchedule(name: string, schedule: string) {
    await schedulerApi.updateTask(name, { schedule })
    await load()
  }

  async function runNow(name: string): Promise<SchedulerHistoryRecord | null> {
    running.value = name
    try {
      return await schedulerApi.runTask(name)
    } catch {
      return null
    } finally {
      running.value = null
    }
  }

  async function loadTaskHistory(name: string, limit = 20) {
    history.value = (await schedulerApi.getTaskHistory(name, limit)) || []
  }

  return { tasks, history, loading, toggling, running, load, toggle, updateSchedule, runNow, loadTaskHistory }
})
