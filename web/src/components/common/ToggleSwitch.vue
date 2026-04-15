<template>
  <label class="toggle-control">
    <input type="checkbox" :checked="modelValue" @change="$emit('update:modelValue', ($event.target as HTMLInputElement).checked)" />
    <span>{{ label }}</span>
  </label>
</template>

<script setup lang="ts">
defineProps<{
  modelValue: boolean
  label: string
}>()

defineEmits<{
  'update:modelValue': [value: boolean]
}>()
</script>
