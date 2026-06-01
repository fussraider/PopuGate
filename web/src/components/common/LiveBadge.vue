<template>
  <span class="live-badge" :class="{
    'live-connecting': proxyStore.wsStatus === 'connecting',
    'live-disconnected': proxyStore.wsStatus === 'disconnected'
  }">
    <span class="live-dot"></span>
    <span class="live-label">
      {{
        proxyStore.wsStatus === 'connected'
          ? t('dashboard.ws_connected')
          : proxyStore.wsStatus === 'connecting'
            ? t('dashboard.ws_connecting')
            : t('dashboard.ws_disconnected')
      }}
    </span>
  </span>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useProxyStore } from '@/stores/proxy'

const { t } = useI18n()
const proxyStore = useProxyStore()
</script>
