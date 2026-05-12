import {defineStore} from 'pinia'
import {ref} from 'vue'
import {botApi} from '@/api/endpoints'

export const useBotStore = defineStore('bot', () => {
  const enabled = ref(false)
  const running = ref(false)
  const loading = ref(false)
  const message = ref('')

  async function setup(token: string, chatId: string, interval: number, label: string) {
    loading.value = true
    try {
      await botApi.setup(token, chatId, interval, label)
      enabled.value = true
      message.value = 'Bot configured successfully'
    } catch (e: any) {
      message.value = e.message
    } finally {
      loading.value = false
    }
  }

  async function test() {
    loading.value = true
    try {
      await botApi.test()
      message.value = 'Test message sent successfully'
    } catch (e: any) {
      message.value = e.message
    } finally {
      loading.value = false
    }
  }

  async function loadStatus() {
    try {
      const data = await botApi.status()
      enabled.value = data.enabled ?? false
      running.value = data.running ?? false
    } catch { /* ignore */ }
  }

  async function toggle(enable: boolean) {
    loading.value = true
    try {
      await botApi.toggle(enable)
      enabled.value = enable
    } finally {
      loading.value = false
    }
  }

  async function detectChatId() {
    try {
      return await botApi.detectChatId()
    } catch (e: any) {
      message.value = e.message
      return null
    }
  }

  async function setCommands() {
    loading.value = true
    try {
      await botApi.setCommands()
      message.value = 'Bot commands updated'
    } catch (e: any) {
      message.value = e.message
    } finally {
      loading.value = false
    }
  }

  return { enabled, running, loading, message, setup, test, loadStatus, toggle, detectChatId, setCommands }
})
