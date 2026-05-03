import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { execSync } from 'node:child_process'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

const pkg = JSON.parse(readFileSync(resolve(__dirname, 'package.json'), 'utf-8'))

function getVersion(): string {
  if (process.env.VITE_APP_VERSION) {
    return process.env.VITE_APP_VERSION
  }
  try {
    return execSync('git describe --tags --exact-match 2>/dev/null || git rev-parse --short HEAD', {
      encoding: 'utf-8',
      stdio: ['pipe', 'pipe', 'pipe'],
    }).trim()
  } catch {
    return pkg.version
  }
}

function getCommit(): string {
  if (process.env.VITE_APP_COMMIT) {
    return process.env.VITE_APP_COMMIT
  }
  try {
    return execSync('git rev-parse HEAD', {
      encoding: 'utf-8',
      stdio: ['pipe', 'pipe', 'pipe'],
    }).trim()
  } catch {
    return 'unknown'
  }
}

const version = getVersion()
const commit = getCommit()
const githubRepo = 'fussraider/PopuGate'

function getVersionURL(): string {
  if (version.startsWith('v')) {
    return `https://github.com/${githubRepo}/releases/tag/${version}`
  }
  if (commit !== 'unknown') {
    return `https://github.com/${githubRepo}/commit/${commit}`
  }
  return `https://github.com/${githubRepo}`
}

export default defineConfig({
  plugins: [vue()],
  define: {
    __APP_VERSION__: JSON.stringify(version),
    __APP_VERSION_URL__: JSON.stringify(getVersionURL()),
  },
  resolve: {
    alias: {
      '@': '/src',
    },
  },
  css: {
    preprocessorOptions: {
      scss: {
        api: 'modern-compiler',
      },
    },
  },
  server: {
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:8080',
        changeOrigin: true,
      },
      '/swagger': {
        target: 'http://127.0.0.1:8080',
        changeOrigin: true,
      },
    },
  },
  build: {
    rollupOptions: {
      output: {
        manualChunks: {
          vue: ['vue', 'vue-router', 'pinia'],
        },
      },
    },
  },
})
