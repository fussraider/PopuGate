<template>
  <Modal v-model="visible" :title="title" max-width="420px">
    <p>{{ message }}</p>
    <template #footer>
      <button class="btn btn-secondary" @click="cancel">{{ t('common.cancel') }}</button>
      <button :class="['btn', confirmBtnClass]" :disabled="confirming || loading" @click="handleConfirm">
        <span v-if="confirming || loading" class="spinner" />
        {{ confirmText ?? t('common.save') }}
      </button>
    </template>
  </Modal>
</template>

<script setup lang="ts">
import {computed, ref, watch} from 'vue'
import {useI18n} from 'vue-i18n'
import Modal from './Modal.vue'

const { t } = useI18n()

const props = defineProps<{
  modelValue: boolean
  title: string
  message: string
  confirmText?: string
  variant?: string
  loading?: boolean
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  confirm: []
  cancel: []
}>()

const visible = ref(props.modelValue)
const confirming = ref(false)

const confirmBtnClass = computed(() => {
  if (props.variant === 'warning') return 'btn-warning'
  if (props.variant === 'primary') return 'btn-primary'
  return 'btn-danger'
})

watch(() => props.modelValue, (v) => {
  visible.value = v
  if (!v) confirming.value = false
})

watch(visible, (v) => {
  if (!v) {
    confirming.value = false
    emit('update:modelValue', false)
    emit('cancel')
  }
})

function cancel() {
  visible.value = false
}

function handleConfirm() {
  confirming.value = true
  emit('confirm')
}
</script>
