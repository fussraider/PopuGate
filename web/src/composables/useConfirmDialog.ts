import { ref, type Ref } from 'vue'

export interface ConfirmOptions {
  title: string
  message: string
  confirmText?: string
}

interface ConfirmState {
  modelValue: boolean
  title: string
  message: string
  confirmText: string
  resolver: ((value: boolean) => void) | null
}

export function useConfirmDialog() {
  const confirmState: Ref<ConfirmState> = ref({
    modelValue: false,
    title: '',
    message: '',
    confirmText: '',
    resolver: null,
  })

  function confirm(options: ConfirmOptions): Promise<boolean> {
    return new Promise((resolve) => {
      confirmState.value = {
        modelValue: true,
        title: options.title,
        message: options.message,
        confirmText: options.confirmText ?? 'Confirm',
        resolver: resolve,
      }
    })
  }

  function handleConfirm() {
    confirmState.value.resolver?.(true)
    confirmState.value.modelValue = false
  }

  function handleCancel() {
    confirmState.value.resolver?.(false)
    confirmState.value.modelValue = false
  }

  return { confirmState, confirm, handleConfirm, handleCancel }
}
