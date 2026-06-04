import {defineStore} from 'pinia'
import {ref} from 'vue'
import {geoblockApi} from '@/api/endpoints'
import type {Settings} from '@/types/models'

export const useGeoblockStore = defineStore('geoblock', () => {
  const mode = ref<'blacklist' | 'whitelist'>('blacklist')
  const countries = ref<string[]>([])
  const loading = ref(false)
  const available = ref(true)
  const error = ref('')

  async function load(settings?: Settings) {
    if (settings) {
      mode.value = settings.geoblock_mode
      countries.value = settings.blocklist_countries
        ? settings.blocklist_countries.split(',').filter(Boolean)
        : []
    }
    loading.value = true
    try {
      const res = await geoblockApi.get()
      mode.value = res.mode
      countries.value = res.countries
        ? res.countries.split(',').filter(Boolean)
        : []
      available.value = res.available !== false
      error.value = res.error || ''
    } catch {
      // Ignored: API interceptor shows error toasts
    } finally {
      loading.value = false
    }
  }

  async function addCountry(code: string) {
    loading.value = true
    try {
      await geoblockApi.add(code.toUpperCase())
      const upper = code.toUpperCase()
      if (!countries.value.includes(upper)) countries.value.push(upper)
    } catch {
      // Error toast is shown by the API client interceptor
    } finally {
      loading.value = false
    }
  }

  async function removeCountry(code: string) {
    loading.value = true
    try {
      await geoblockApi.remove(code.toUpperCase())
      countries.value = countries.value.filter((c) => c !== code.toUpperCase())
    } finally {
      loading.value = false
    }
  }

  async function clear() {
    loading.value = true
    try {
      await geoblockApi.clear()
      countries.value = []
    } finally {
      loading.value = false
    }
  }

  async function setMode(m: 'blacklist' | 'whitelist') {
    loading.value = true
    try {
      await geoblockApi.setMode(m)
      mode.value = m
    } finally {
      loading.value = false
    }
  }

  return { mode, countries, loading, available, error, load, addCountry, removeCountry, clear, setMode }
})
