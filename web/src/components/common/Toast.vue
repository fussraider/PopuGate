<template>
  <TransitionGroup name="toast" tag="div" class="toast-container">
    <div v-for="toast in toastStore.toasts" :key="toast.id" :class="['toast', `toast-${toast.type}`]">
      <span class="toast-icon">{{ toastIcon(toast.type) }}</span>
      <span class="toast-message">{{ toast.message }}</span>
      <button class="toast-close" @click="toastStore.remove(toast.id)">&times;</button>
    </div>
  </TransitionGroup>
</template>

<script setup lang="ts">
import { useToastStore } from '@/stores/toast'

const toastStore = useToastStore()

function toastIcon(type: string) {
  return ({ success: '✓', error: '✗', warning: '⚠', info: 'ℹ' } as Record<string, string>)[type] ?? 'ℹ'
}
</script>

<style scoped lang="scss">
@use '@/assets/scss/variables' as *;

.toast-container {
  position: fixed;
  top: $spacing-lg;
  right: $spacing-lg;
  z-index: $z-toast;
  display: flex;
  flex-direction: column;
  gap: $spacing-sm;
  pointer-events: none;
}

.toast {
  display: flex;
  align-items: center;
  gap: $spacing-sm;
  padding: 12px 16px;
  border-radius: $border-radius;
  background: $bg-card;
  box-shadow: $shadow-lg;
  font-size: $font-size-sm;
  min-width: 280px;
  max-width: 420px;
  pointer-events: auto;

  &.toast-success { border-left: 4px solid $color-success; }
  &.toast-error   { border-left: 4px solid $color-danger; }
  &.toast-warning { border-left: 4px solid $color-warning; }
  &.toast-info    { border-left: 4px solid $color-info; }
}

.toast-icon { font-size: 1.1rem; }
.toast-message { flex: 1; }
.toast-close {
  background: none;
  border: none;
  cursor: pointer;
  color: $text-muted;
  font-size: 1.1rem;
  padding: 0 4px;
  line-height: 1;
}

.toast-enter-active,
.toast-leave-active { transition: all $transition-normal; }
.toast-enter-from { transform: translateX(100%); opacity: 0; }
.toast-leave-to   { transform: translateX(100%); opacity: 0; }
</style>
