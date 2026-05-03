<template>
  <div class="auth-layout">
    <div class="auth-card">
      <div class="auth-header">
        <ThemeToggle />
        <LanguageSwitcher />
      </div>
      <router-view />
    </div>
  </div>
  <Toast />
</template>

<script setup lang="ts">
import Toast from '@/components/common/Toast.vue'
import ThemeToggle from '@/components/common/ThemeToggle.vue'
import LanguageSwitcher from '@/components/common/LanguageSwitcher.vue'
</script>

<style scoped lang="scss">
@use '@/assets/scss/variables' as *;

.auth-layout {
  min-height: 100vh;
  display: flex;
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
</style>
