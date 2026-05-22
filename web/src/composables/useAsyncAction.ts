import {ref, type Ref} from 'vue'
import {useToastStore} from '@/stores/toast'

export interface UseAsyncActionOptions {
  successMessage?: string
  modal?: Ref<boolean>
}

export function useAsyncAction(options?: UseAsyncActionOptions) {
  const loading = ref(false)
  const toast = useToastStore()

  async function run<T = void>(fn: () => Promise<T>, successMsg?: string): Promise<T | undefined> {
    loading.value = true
    try {
      const result = await fn()
      const msg = successMsg ?? options?.successMessage
      if (msg) toast.success(msg)
      if (options?.modal) options.modal.value = false
      return result
    } catch {
      // interceptor handles error toast
      return undefined
    } finally {
      loading.value = false
    }
  }

  return { loading, run }
}
