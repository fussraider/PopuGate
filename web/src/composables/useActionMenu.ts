import {ref} from 'vue'

export function useActionMenu<T = any>() {
  const isOpen = ref(false)
  const activeItem = ref<T | null>(null)

  function open(item: T) {
    activeItem.value = item
    isOpen.value = true
  }

  function close() {
    isOpen.value = false
  }

  return { isOpen, activeItem, open, close }
}
