<template>
  <div class="language-switcher">
    <button
      v-for="lang in languages"
      :key="lang.code"
      class="lang-btn"
      :class="{ active: locale === lang.code }"
      @click="setLanguage(lang.code)"
    >
      {{ lang.label }}
    </button>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

const { locale } = useI18n()

const languages = [
  { code: 'en', label: 'EN' },
  { code: 'ru', label: 'RU' }
]

function setLanguage(lang: string) {
  locale.value = lang
  localStorage.setItem('locale', lang)
}
</script>

<style scoped lang="scss">
@use '@/assets/scss/variables' as *;

.language-switcher {
  display: flex;
  gap: $spacing-xs;
  background: rgba(0, 0, 0, 0.1);
  padding: 4px;
  border-radius: $border-radius;
}

.lang-btn {
  background: none;
  border: none;
  padding: 4px 8px;
  font-size: $font-size-xs;
  font-weight: $font-weight-semibold;
  color: $text-muted;
  cursor: pointer;
  border-radius: $border-radius;
  transition: all $transition-fast;

  &:hover {
    color: $text-primary;
    background: rgba(255, 255, 255, 0.05);
  }

  &.active {
    background: $color-primary;
    color: $color-white;
  }
}
</style>
