<template>
  <div class="tooltip-wrapper" ref="triggerRef" @mouseenter="show" @mouseleave="hide">
    <slot />
    <Teleport to="body">
      <div v-if="visible" class="tooltip-box" :style="positionStyle">
        {{ text }}
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import {computed, ref} from 'vue'

defineProps<{ text: string }>()

const triggerRef = ref<HTMLElement | null>(null)
const visible = ref(false)
const coords = ref({ top: 0, left: 0 })

const positionStyle = computed(() => ({
  top: `${coords.value.top}px`,
  left: `${coords.value.left}px`,
}))

function show() {
  if (!triggerRef.value) return
  const rect = triggerRef.value.getBoundingClientRect()
  coords.value = {
    top: rect.top - 8,
    left: rect.left + rect.width / 2,
  }
  visible.value = true
}

function hide() {
  visible.value = false
}
</script>

<style lang="scss">
@use '@/assets/scss/variables' as *;

.tooltip-box {
  position: fixed;
  transform: translateX(-50%) translateY(-100%);
  background-color: $color-gray-900;
  color: #fff;
  text-align: center;
  padding: $spacing-xs $spacing-sm;
  border-radius: $border-radius-sm;
  font-size: $font-size-xs;
  line-height: $line-height-tight;
  font-weight: normal;
  width: max-content;
  max-width: 280px;
  white-space: pre-line;
  z-index: 99999;
  pointer-events: none;
  box-shadow: $shadow-lg;

  &::after {
    content: "";
    position: absolute;
    top: 100%;
    left: 50%;
    margin-left: -5px;
    border-width: 5px;
    border-style: solid;
    border-color: $color-gray-900 transparent transparent transparent;
  }
}
</style>
