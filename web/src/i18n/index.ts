import { createI18n } from 'vue-i18n'
import en from './locales/en.json'
import ru from './locales/ru.json'

const messages = {
  en,
  ru
}

// Get browser language or saved language
const savedLocale = localStorage.getItem('locale')
const browserLocale = navigator.language.split('-')[0]
const defaultLocale = savedLocale || (messages.hasOwnProperty(browserLocale) ? browserLocale : 'en')

const i18n = createI18n({
  legacy: false,
  locale: defaultLocale,
  fallbackLocale: 'en',
  messages,
})

export default i18n
