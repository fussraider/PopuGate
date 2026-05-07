import type { AxiosError } from 'axios'

export function getErrorMessage(e: unknown): string {
  if (e instanceof Error && 'response' in e) {
    const axiosErr = e as AxiosError<{ error?: string }>
    return axiosErr.response?.data?.error ?? e.message
  }
  if (e instanceof Error) return e.message
  return String(e)
}
