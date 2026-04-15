<template>
  <div class="card empty-state">
    <div class="empty-icon">{{ icon }}</div>
    <p>{{ message }}</p>
    <slot />
  </div>
</template>

<script setup lang="ts">
defineProps<{
  icon: string
  message: string
}>()
</script>
