import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import i18n from './i18n'
import { useThemeStore } from './stores/theme'
import './assets/scss/main.scss'

const app = createApp(App)

const pinia = createPinia()
app.use(pinia)

useThemeStore().init()

app.use(i18n)
app.use(router)

app.mount('#app')
