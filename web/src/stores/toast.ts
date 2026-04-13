import { defineStore } from 'pinia'
import { ref } from 'vue'

export type ToastType = 'success' | 'error' | 'warning' | 'info'

interface ToastItem {
  id: number
  type: ToastType
  message: string
}

let nextId = 0

export const useToastStore = defineStore('toast', () => {
  const toasts = ref<ToastItem[]>([])

  function add(type: ToastType, message: string, duration = 4000) {
    const id = ++nextId
    toasts.value.push({ id, type, message })
    if (duration > 0) setTimeout(() => remove(id), duration)
  }

  function remove(id: number) {
    toasts.value = toasts.value.filter((t) => t.id !== id)
  }

  const success = (message: string, duration?: number) => add('success', message, duration)
  const error   = (message: string, duration?: number) => add('error',   message, duration)
  const warning = (message: string, duration?: number) => add('warning', message, duration)
  const info    = (message: string, duration?: number) => add('info',    message, duration)

  return { toasts, add, remove, success, error, warning, info }
})
