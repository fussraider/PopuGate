import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { authApi } from '@/api/endpoints'
import type { AuthTokens } from '@/types/models'

export const useAuthStore = defineStore('auth', () => {
  const accessToken = ref(localStorage.getItem('pg_access_token') || '')
  const refreshToken = ref(localStorage.getItem('pg_refresh_token') || '')

  const isAuthenticated = computed(() => !!accessToken.value)

  function setTokens(tokens: AuthTokens) {
    accessToken.value = tokens.accessToken
    refreshToken.value = tokens.refreshToken
    localStorage.setItem('pg_access_token', tokens.accessToken)
    localStorage.setItem('pg_refresh_token', tokens.refreshToken)
  }

  async function setup(password: string) {
    const res = await authApi.setup(password)
    setTokens({ accessToken: res.access_token, refreshToken: res.refresh_token })
  }

  async function login(username: string, password: string) {
    const res = await authApi.login({ username, password })
    setTokens({ accessToken: res.access_token, refreshToken: res.refresh_token })
  }

  async function refresh() {
    if (!refreshToken.value) return false
    try {
      const res = await authApi.refresh(refreshToken.value)
      setTokens({ accessToken: res.access_token, refreshToken: res.refresh_token })
      return true
    } catch {
      return false
    }
  }

  async function logout() {
    try {
      await authApi.logout()
    } catch { /* ignore */ }
    accessToken.value = ''
    refreshToken.value = ''
    localStorage.removeItem('pg_access_token')
    localStorage.removeItem('pg_refresh_token')
  }

  return { accessToken, refreshToken, isAuthenticated, setup, login, logout, refresh }
})
