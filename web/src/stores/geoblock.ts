import { defineStore } from 'pinia'
import { ref } from 'vue'
import { geoblockApi } from '@/api/endpoints'
import type { Settings } from '@/types/models'

export const useGeoblockStore = defineStore('geoblock', () => {
  const mode = ref<'blacklist' | 'whitelist'>('blacklist')
  const countries = ref<string[]>([])
  const loading = ref(false)

  function load(settings: Settings) {
    mode.value = settings.geoblock_mode
    countries.value = settings.blocklist_countries
      ? settings.blocklist_countries.split(',').filter(Boolean)
      : []
  }

  async function addCountry(code: string) {
    loading.value = true
    try {
      await geoblockApi.add(code.toUpperCase())
      const upper = code.toUpperCase()
      if (!countries.value.includes(upper)) countries.value.push(upper)
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

  return { mode, countries, loading, load, addCountry, removeCountry, clear, setMode }
})
