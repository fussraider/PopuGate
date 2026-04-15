import { ref } from 'vue'
import { useToastStore } from '@/stores/toast'

export function useAsyncAction() {
  const loading = ref(false)
  const toast = useToastStore()

  async function run(fn: () => Promise<void>, successMsg?: string) {
    loading.value = true
    try {
      await fn()
      if (successMsg) toast.success(successMsg)
    } catch (e: any) {
      toast.error(e.response?.data?.error ?? e.message)
    } finally {
      loading.value = false
    }
  }

  return { loading, run }
}
