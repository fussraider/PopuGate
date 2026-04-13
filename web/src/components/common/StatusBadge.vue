<template>
  <span :class="['badge', `badge-${variant ?? 'neutral'}`]">
    <slot />
  </span>
</template>

<script setup lang="ts">
defineProps<{
  variant?: 'success' | 'warning' | 'danger' | 'info' | 'neutral'
}>()
</script>
