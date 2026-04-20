<template>
  <div class="card empty-state">
    <div class="empty-icon"><component :is="icon" :size="48" :stroke-width="1.2" /></div>
    <p>{{ message }}</p>
    <slot />
  </div>
</template>

<script setup lang="ts">
import type { Component } from 'vue'

defineProps<{
  icon: Component
  message: string
}>()
</script>
