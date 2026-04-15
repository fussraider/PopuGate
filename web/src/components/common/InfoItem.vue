<template>
  <div class="info-item">
    <span class="info-label">{{ label }}</span>
    <slot />
  </div>
</template>

<script setup lang="ts">
defineProps<{
  label: string
}>()
</script>
