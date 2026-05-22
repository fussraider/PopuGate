<template>
  <div class="auth-layout">
    <div class="auth-card">
      <div class="auth-header">
        <ThemeToggle />
        <LanguageSwitcher />
      </div>
      <router-view />
    </div>
    <div class="auth-footer">
      <a href="https://github.com/fussraider/PopuGate" target="_blank" rel="noopener" class="auth-footer-link">
        <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 22v-4a4.8 4.8 0 0 0-1-3.5c3 0 6-2 6-5.5.08-1.25-.27-2.48-1-3.5.28-1.15.28-2.35 0-3.5 0 0-1 0-3 1.5-2.64-.5-5.36-.5-8 0C6 2 5 2 5 2c-.3 1.15-.3 2.35 0 3.5A5.403 5.403 0 0 0 4 9c0 3.5 3 5.5 6 5.5-.39.49-.68 1.05-.85 1.65S8.93 17.38 9 18v4"/><path d="M9 18c-4.51 2-5-2-7-2"/></svg>
        GitHub
      </a>
      <a v-if="APP_VERSION_URL" :href="APP_VERSION_URL" target="_blank" rel="noopener" class="auth-footer-link">
        <ExternalLink :size="14" /> {{ APP_VERSION }}
      </a>
    </div>
  </div>
  <Toast />
</template>

<script setup lang="ts">
import Toast from '@/components/common/Toast.vue'
import ThemeToggle from '@/components/common/ThemeToggle.vue'
import LanguageSwitcher from '@/components/common/LanguageSwitcher.vue'
import {ExternalLink} from '@lucide/vue'

declare const __APP_VERSION__: string
declare const __APP_VERSION_URL__: string
const APP_VERSION = __APP_VERSION__
const APP_VERSION_URL = __APP_VERSION_URL__
</script>

<style scoped lang="scss">
@use '@/assets/scss/variables' as *;

.auth-layout {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, $color-primary 0%, #8b5cf6 100%);
  padding: $spacing-lg;
}

.auth-card {
  background: $bg-card;
  border-radius: $border-radius-lg;
  box-shadow: $shadow-xl;
  padding: $spacing-2xl;
  width: 100%;
  max-width: 420px;
}

.auth-header {
  display: flex;
  justify-content: flex-end;
  margin-bottom: $spacing-lg;
}

.auth-footer {
  display: flex;
  justify-content: center;
  gap: $spacing-lg;
  margin-top: $spacing-xl;
}

.auth-footer-link {
  display: inline-flex;
  align-items: center;
  gap: $spacing-xs;
  color: rgba(255, 255, 255, 0.7);
  text-decoration: none;
  font-size: $font-size-sm;

  &:hover {
    color: rgba(255, 255, 255, 1);
    text-decoration: underline;
  }
}
</style>
