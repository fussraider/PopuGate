import axios, { type AxiosInstance, type AxiosRequestConfig } from 'axios'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'

const apiClient: AxiosInstance = axios.create({
  baseURL: '/api/v1',
  timeout: 30000,
  headers: { 'Content-Type': 'application/json' },
})

// Request interceptor: attach JWT
apiClient.interceptors.request.use((config) => {
  const authStore = useAuthStore()
  if (authStore.accessToken) {
    config.headers.Authorization = `Bearer ${authStore.accessToken}`
  }
  return config
})

// Response interceptor: handle 401 → refresh, 403 → setup redirect
apiClient.interceptors.response.use(
  (res) => res,
  async (error) => {
    const original = error.config as AxiosRequestConfig & { _retry?: boolean }
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
      if (await authStore.refresh()) {
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

    // Show error toast for other errors
    const errorMessage = error.response?.data?.error || error.message || 'Request failed'
    toastStore.error(errorMessage)

    return Promise.reject(error)
  },
)

export default apiClient
