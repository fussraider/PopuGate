import { ref, computed, onScopeDispose } from 'vue'
import { defineStore } from 'pinia'

export type ThemePreference = 'light' | 'dark' | 'auto'

const valid: ThemePreference[] = ['light', 'dark', 'auto']

export const useThemeStore = defineStore('theme', () => {
  const stored = localStorage.getItem('theme')
  const preference = ref<ThemePreference>(
    stored && valid.includes(stored as ThemePreference) ? (stored as ThemePreference) : 'auto'
  )
  const systemIsDark = ref(false)

  let mediaQuery: MediaQueryList | null = null

  function onMediaChange(e: MediaQueryListEvent | MediaQueryList) {
    systemIsDark.value = e.matches
    apply()
  }

  const resolved = computed(() => {
    if (preference.value !== 'auto') return preference.value
    return systemIsDark.value ? 'dark' : 'light'
  })

  function apply() {
    document.documentElement.setAttribute('data-theme', resolved.value)
  }

  function setTheme(p: ThemePreference) {
    preference.value = p
    localStorage.setItem('theme', p)
    apply()
  }

  function init() {
    mediaQuery = window.matchMedia('(prefers-color-scheme: dark)')
    systemIsDark.value = mediaQuery.matches
    mediaQuery.addEventListener('change', onMediaChange)
    apply()

    onScopeDispose(() => {
      mediaQuery?.removeEventListener('change', onMediaChange)
    })
  }

  return { preference, resolved, setTheme, init }
})
