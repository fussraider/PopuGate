<template>
  <Modal :model-value="modelValue" :title="title" :max-width="maxWidth" @update:model-value="$emit('update:modelValue', $event)">
    <form :id="formId" @submit.prevent="$emit('submit')">
      <slot />
    </form>
    <template #footer>
      <slot name="footer" />
      <button type="button" class="btn btn-secondary" @click="$emit('update:modelValue', false)">
        {{ cancelText || t('common.cancel') }}
      </button>
      <button type="submit" :form="formId" class="btn btn-primary" :disabled="submitting">
        <span v-if="submitting" class="spinner" />
        {{ submitText || t('common.save') }}
      </button>
    </template>
  </Modal>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useId } from 'vue'
import Modal from './Modal.vue'

const { t } = useI18n()
const formId = useId()

defineProps<{
  modelValue: boolean
  title: string
  maxWidth?: string
  submitting?: boolean
  submitText?: string
  cancelText?: string
}>()

defineEmits<{
  'update:modelValue': [value: boolean]
  submit: []
}>()
</script>
