<template>
  <div>
    <div class="login-header text-center mb-lg">
      <div class="login-logo">⚡</div>
      <h1>PopuGate</h1>
      <p class="text-muted">MTProto Proxy Manager</p>
    </div>

    <form @submit.prevent="handleLogin">
      <div class="form-group mb-md">
        <label class="form-label">Username</label>
        <input v-model="username" class="input" type="text" placeholder="admin" required autofocus />
      </div>

      <div class="form-group mb-lg">
        <label class="form-label">Password</label>
        <input v-model="password" class="input" type="password" placeholder="••••••••" required />
      </div>

      <div v-if="error" class="alert alert-danger mb-md">{{ error }}</div>

      <button class="btn btn-primary btn-lg w-full" type="submit" :disabled="loading">
        <span v-if="loading" class="spinner" />
        {{ loading ? 'Signing in...' : 'Sign In' }}
      </button>
    </form>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const router = useRouter()
const auth = useAuthStore()

const username = ref('')
const password = ref('')
const error = ref('')
const loading = ref(false)

async function handleLogin() {
  error.value = ''
  loading.value = true
  try {
    await auth.login(username.value, password.value)
    router.push('/')
  } catch (e: any) {
    error.value = e.response?.data?.error || e.message || 'Login failed'
  } finally {
    loading.value = false
  }
}
</script>

<style scoped lang="scss">
@use '@/assets/scss/variables' as *;

.login-header {
  margin-bottom: $spacing-xl;
}

.login-logo {
  font-size: 3rem;
  margin-bottom: $spacing-sm;
}
</style>
