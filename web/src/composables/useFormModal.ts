import {ref, type Ref} from 'vue'
import {useToastStore} from '@/stores/toast'

export function useFormModal<T extends Record<string, any>>(defaults: T) {
  const isOpen = ref(false)
  const form = ref<T>({ ...defaults }) as Ref<T>
  const submitting = ref(false)
  const toast = useToastStore()

  function open(initialValues?: Partial<T>) {
    form.value = { ...defaults, ...initialValues } as T
    isOpen.value = true
  }

  function close() {
    isOpen.value = false
  }

  async function submit(handler: (form: T) => Promise<void>, successMsg?: string): Promise<boolean> {
    submitting.value = true
    try {
      await handler(form.value)
      if (successMsg) toast.success(successMsg)
      isOpen.value = false
      return true
    } catch {
      // interceptor handles error toast
      return false
    } finally {
      submitting.value = false
    }
  }

  return { isOpen, form, submitting, open, close, submit }
}
