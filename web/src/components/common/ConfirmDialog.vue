<template>
  <Modal v-model="visible" :title="title" max-width="420px">
    <p>{{ message }}</p>
    <template #footer>
      <button class="btn btn-secondary" @click="cancel">{{ t('common.cancel') }}</button>
      <button class="btn btn-danger" :disabled="confirming || loading" @click="handleConfirm">
        <span v-if="confirming || loading" class="spinner" />
        {{ confirmText ?? t('common.save') }}
      </button>
    </template>
  </Modal>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Modal from './Modal.vue'

const { t } = useI18n()

const props = defineProps<{
  modelValue: boolean
  title: string
  message: string
  confirmText?: string
  loading?: boolean
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  confirm: []
  cancel: []
}>()

const visible = ref(props.modelValue)
const confirming = ref(false)

watch(() => props.modelValue, (v) => {
  visible.value = v
  if (!v) confirming.value = false
})

watch(visible, (v) => {
  if (!v) emit('update:modelValue', false)
})

function cancel() {
  confirming.value = false
  emit('update:modelValue', false)
  emit('cancel')
}

function handleConfirm() {
  confirming.value = true
  emit('confirm')
}
</script>
