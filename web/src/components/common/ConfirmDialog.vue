<template>
  <Modal v-model="visible" :title="title" max-width="420px">
    <p>{{ message }}</p>
    <template #footer>
      <button class="btn btn-secondary" @click="cancel">Cancel</button>
      <button class="btn btn-danger" :disabled="confirming" @click="handleConfirm">
        <span v-if="confirming" class="spinner" />
        {{ confirmText ?? 'Confirm' }}
      </button>
    </template>
  </Modal>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import Modal from './Modal.vue'

const props = defineProps<{
  modelValue: boolean
  title: string
  message: string
  confirmText?: string
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  confirm: []
}>()

const visible = ref(props.modelValue)
const confirming = ref(false)

watch(() => props.modelValue, (v) => {
  visible.value = v
  if (!v) confirming.value = false  // сброс при закрытии
})

// синхронизируем обратно, если модал закрыт внутри (click-outside)
watch(visible, (v) => {
  if (!v) emit('update:modelValue', false)
})

function cancel() {
  confirming.value = false
  emit('update:modelValue', false)
}

function handleConfirm() {
  confirming.value = true
  emit('confirm')
  // confirming сбросится через watch при закрытии modelValue
}
</script>
