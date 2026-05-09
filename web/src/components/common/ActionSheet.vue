<template>
  <Teleport to="body">
    <Transition name="sheet">
      <div v-if="modelValue" class="sheet-overlay" @click.self="$emit('update:modelValue', false)">
        <div class="sheet">
          <div class="sheet-header">
            <span class="sheet-title">{{ title }}</span>
            <button class="btn btn-ghost btn-icon" @click="$emit('update:modelValue', false)">&times;</button>
          </div>
          <div class="sheet-body">
            <slot />
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
defineProps<{
  modelValue: boolean
  title?: string
}>()

defineEmits<{
  'update:modelValue': [value: boolean]
}>()
</script>

<style scoped lang="scss">
@use '@/assets/scss/variables' as *;

.sheet-overlay {
  position: fixed;
  inset: 0;
  z-index: 1000;
  background: rgba(0, 0, 0, 0.4);
  display: flex;
  align-items: flex-end;
  justify-content: center;
}

.sheet {
  width: 100%;
  max-width: 480px;
  max-height: 80vh;
  background: $bg-card;
  border-radius: $border-radius $border-radius 0 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.sheet-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: $spacing-md $spacing-md $spacing-sm;
  border-bottom: 1px solid $border-color;
}

.sheet-title {
  font-weight: 500;
  font-size: $font-size-base;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.sheet-body {
  overflow-y: auto;
  padding: $spacing-xs 0;
}

// Transition
.sheet-enter-active,
.sheet-leave-active {
  transition: opacity 0.2s ease;

  .sheet {
    transition: transform 0.25s ease;
  }
}

.sheet-enter-from,
.sheet-leave-to {
  opacity: 0;

  .sheet {
    transform: translateY(100%);
  }
}
</style>
