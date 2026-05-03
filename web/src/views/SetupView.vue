<template>
  <div>
    <div class="setup-header text-center mb-lg">
      <div class="setup-logo"><Zap :size="48" :stroke-width="1.5" /></div>
      <h1>{{ t('setup.welcome') }}</h1>
      <p class="text-muted">{{ t('setup.subtitle') }}</p>
    </div>

    <form @submit.prevent="handleSetup">
      <div class="form-group mb-md">
        <label class="form-label">{{ t('setup.admin_password') }}</label>
        <input
          v-model="password"
          class="input"
          type="password"
          :placeholder="t('setup.placeholder')"
          required
          minlength="6"
          autofocus
        />
        <p class="form-hint">{{ t('setup.hint') }}</p>
      </div>

      <div class="form-group mb-lg">
        <label class="form-label">{{ t('setup.confirm_password') }}</label>
        <input
          v-model="confirm"
          class="input"
          type="password"
          :placeholder="t('setup.confirm_placeholder')"
          required
          minlength="6"
        />
      </div>

      <div v-if="error" class="alert alert-danger mb-md">{{ error }}</div>
      <div v-if="success" class="alert alert-success mb-md"><Check :size="16" /> {{ t('setup.success') }}</div>

      <button class="btn btn-primary btn-lg w-full" type="submit" :disabled="loading || !isValid">
        <Loader2 v-if="loading" :size="16" class="animate-spin" />
        {{ loading ? t('setup.setting_up') : t('setup.complete') }}
      </button>
    </form>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { Zap, Check, Loader2 } from '@lucide/vue'

const { t } = useI18n()
const router = useRouter()
const auth = useAuthStore()

const password = ref('')
const confirm = ref('')
const error = ref('')
const success = ref(false)
const loading = ref(false)

const isValid = computed(() =>
  password.value.length >= 6 &&
  password.value === confirm.value
)

async function handleSetup() {
  error.value = ''
  if (!isValid.value) {
    error.value = password.value !== confirm.value
      ? t('setup.mismatch')
      : t('setup.too_short')
    return
  }

  loading.value = true
  try {
    await auth.setup(password.value)
    success.value = true
    setTimeout(() => router.push('/'), 800)
  } catch (e: any) {
    error.value = e.response?.data?.error || e.message || t('setup.failed')
  } finally {
    loading.value = false
  }
}
</script>

<style scoped lang="scss">
@use '@/assets/scss/variables' as *;

.setup-header {
  margin-bottom: $spacing-xl;
}

.setup-logo {
  display: flex;
  justify-content: center;
  margin-bottom: $spacing-sm;
  color: $color-primary;
}

.form-label {
  display: block;
  font-size: $font-size-sm;
  font-weight: $font-weight-medium;
  margin-bottom: $spacing-sm;
  color: $text-secondary;
}

.form-hint {
  font-size: $font-size-xs;
  color: $text-muted;
  margin-top: $spacing-xs;
}

.alert {
  padding: $spacing-sm $spacing-md;
  border-radius: $border-radius;
  font-size: $font-size-sm;
  text-align: center;

  &.alert-success {
    background: var(--color-success-bg);
    color: var(--color-success);
    border: 1px solid var(--alert-success-border);
  }
}
</style>
