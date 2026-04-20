<template>
  <div class="tooltip-wrapper">
    <slot />
    <div class="tooltip-box">
      {{ text }}
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{ text: string }>()
</script>

<style scoped lang="scss">
@use '@/assets/scss/variables' as *;

.tooltip-wrapper {
  position: relative;
  display: inline-flex;
  align-items: center;
  cursor: help;

  &:hover .tooltip-box {
    visibility: visible;
    opacity: 1;
    transform: translateX(-50%) translateY(-8px);
  }
}

.tooltip-box {
  visibility: hidden;
  position: absolute;
  bottom: 100%;
  left: 50%;
  transform: translateX(-50%) translateY(0);
  background-color: $color-gray-900;
  color: #fff;
  text-align: center;
  padding: $spacing-xs $spacing-sm;
  border-radius: $border-radius-sm;
  font-size: $font-size-xs;
  line-height: $line-height-tight;
  font-weight: normal;
  width: max-content;
  max-width: 200px;
  z-index: 100;
  opacity: 0;
  transition: opacity 0.2s, transform 0.2s;
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
