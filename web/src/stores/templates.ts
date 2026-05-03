import { defineStore } from 'pinia'
import { ref } from 'vue'
import { templatesApi } from '@/api/endpoints'
import type { SecretTemplate } from '@/types/models'

export const useTemplatesStore = defineStore('templates', () => {
  const templates = ref<SecretTemplate[]>([])
  const loading = ref(false)

  async function load() {
    loading.value = true
    try {
      templates.value = (await templatesApi.list()) || []
    } finally {
      loading.value = false
    }
  }

  async function create(data: Omit<SecretTemplate, 'id'>) {
    const created = await templatesApi.create(data)
    templates.value.push(created)
  }

  async function remove(name: string) {
    await templatesApi.remove(name)
    templates.value = templates.value.filter((t) => t.name !== name)
  }

  async function apply(templateName: string, secretLabel: string) {
    await templatesApi.apply(templateName, secretLabel)
  }

  return { templates, loading, load, create, remove, apply }
})
