<template>
  <div>
    <div class="setup-header text-center mb-lg">
      <div class="setup-logo">⚡</div>
      <h1>Welcome to PopuGate</h1>
      <p class="text-muted">Create your admin password to get started</p>
    </div>

    <form @submit.prevent="handleSetup">
      <div class="form-group mb-md">
        <label class="form-label">Admin Password</label>
        <input
          v-model="password"
          class="input"
          type="password"
          placeholder="Min. 6 characters"
          required
          minlength="6"
          autofocus
        />
        <p class="form-hint">This will be your admin password for all future logins</p>
      </div>

      <div class="form-group mb-lg">
        <label class="form-label">Confirm Password</label>
        <input
          v-model="confirm"
          class="input"
          type="password"
          placeholder="Re-enter password"
          required
          minlength="6"
        />
      </div>

      <div v-if="error" class="alert alert-danger mb-md">{{ error }}</div>
      <div v-if="success" class="alert alert-success mb-md">✓ Password set! Redirecting...</div>

      <button class="btn btn-primary btn-lg w-full" type="submit" :disabled="loading || !isValid">
        <span v-if="loading" class="spinner" />
        {{ loading ? 'Setting up...' : 'Complete Setup' }}
      </button>
    </form>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

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
      ? 'Passwords do not match'
      : 'Password must be at least 6 characters'
    return
  }

  loading.value = true
  try {
    await auth.setup(password.value)
    success.value = true
    setTimeout(() => router.push('/'), 800)
  } catch (e: any) {
    error.value = e.response?.data?.error || e.message || 'Setup failed'
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
  font-size: 3rem;
  margin-bottom: $spacing-sm;
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
    background: rgba(16, 185, 129, 0.1);
    color: #10b981;
    border: 1px solid rgba(16, 185, 129, 0.2);
  }
}
</style>
