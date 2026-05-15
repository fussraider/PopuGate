import axios, {type AxiosInstance, type AxiosRequestConfig} from 'axios'
import {useAuthStore} from '@/stores/auth'
import {useToastStore} from '@/stores/toast'

const apiClient: AxiosInstance = axios.create({
  baseURL: '/api/v1',
  timeout: 120000,
  headers: { 'Content-Type': 'application/json' },
})

// Mutex: ensures only one refresh request is in-flight at a time.
// Without this, concurrent 401s each fire their own refresh — the first
// succeeds and blocklists the old refresh token, so the rest fail → logout.
let refreshPromise: Promise<boolean> | null = null

// Skip auth interceptor for logout endpoint to prevent recursion
function isLogoutRequest(url?: string): boolean {
  return !!url && (url.endsWith('/auth/logout') || url.includes('/auth/logout'))
}

// Request interceptor: attach JWT
apiClient.interceptors.request.use((config) => {
  if (isLogoutRequest(config.url)) return config
  const authStore = useAuthStore()
  if (authStore.accessToken) {
    config.headers.Authorization = `Bearer ${authStore.accessToken}`
  }
  return config
})

// Response interceptor: handle 401 → refresh, 403 → setup redirect, show X-Warning toasts
apiClient.interceptors.response.use(
  (res) => {
    const warning = res.headers['x-warning']
    if (warning) {
      const toastStore = useToastStore()
      toastStore.warning(decodeURIComponent(warning))
    }
    return res
  },
  async (error) => {
    const original = error.config as AxiosRequestConfig & { _retry?: boolean; _silent?: boolean }
    const status = error.response?.status
    const toastStore = useToastStore()

    // 403 "initial setup required" → redirect to setup (once)
    if (status === 403 && error.response?.data?.error === 'initial setup required') {
      if (!window.location.pathname.startsWith('/auth/setup')) {
        window.location.href = '/auth/setup'
      }
      return Promise.reject(error)
    }

    // 401 → refresh or logout
    if (status === 401 && !original._retry) {
      original._retry = true
      const authStore = useAuthStore()

      // Reuse in-flight refresh or start a new one
      if (!refreshPromise) {
        refreshPromise = authStore.refresh().finally(() => {
          refreshPromise = null
        })
      }

      const refreshed = await refreshPromise
      if (refreshed) {
        original.headers = {
          ...original.headers,
          Authorization: `Bearer ${authStore.accessToken}`,
        }
        return apiClient(original)
      }
      authStore.logout()
      window.location.href = '/auth/login'
      return Promise.reject(error)
    }

    // Show error toast for other errors (unless silenced)
    if (!original._silent) {
      const errorMessage = error.response?.data?.error || error.message || 'Request failed'
      toastStore.error(errorMessage)
    }

    return Promise.reject(error)
  },
)

export default apiClient
