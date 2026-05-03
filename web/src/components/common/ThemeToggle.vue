<template>
  <div class="theme-toggle">
    <button
      class="theme-btn"
      :class="{ active: true }"
      @click="cycleTheme"
      :title="label"
    >
      <Monitor v-if="preference === 'auto'" :size="14" />
      <Sun v-else-if="preference === 'light'" :size="14" />
      <Moon v-else :size="14" />
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useThemeStore, type ThemePreference } from '@/stores/theme'
import { Monitor, Sun, Moon } from '@lucide/vue'

const { t } = useI18n()
const theme = useThemeStore()

const preference = computed(() => theme.preference)

const label = computed(() => {
  switch (preference.value) {
    case 'auto': return t('common.theme_auto')
    case 'light': return t('common.theme_light')
    case 'dark': return t('common.theme_dark')
  }
})

const order: ThemePreference[] = ['auto', 'light', 'dark']

function cycleTheme() {
  const idx = order.indexOf(preference.value)
  theme.setTheme(order[(idx + 1) % order.length])
}
</script>

<style scoped lang="scss">
@use '@/assets/scss/variables' as *;

.theme-toggle {
  display: flex;
  background: var(--bg-code);
  padding: 4px;
  border-radius: $border-radius;
}

.theme-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  background: none;
  border: none;
  padding: 4px 8px;
  color: $text-muted;
  cursor: pointer;
  border-radius: $border-radius;
  transition: all $transition-fast;

  &:hover {
    color: $text-primary;
    background: var(--bg-table-hover);
  }
}
</style>
