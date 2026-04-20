<template>
  <div>
    <div class="card mb-lg">
      <h3 class="mb-md">{{ t('proxy_view.title') }}</h3>
      <div class="status-bar mb-lg">
        <div class="info-item">
          <span class="info-label">{{ t('common.status') }}</span>
          <StatusBadge :variant="proxyStore.status?.running ? 'success' : 'danger'">
            {{ proxyStore.status?.running ? t('dashboard.running') : t('dashboard.stopped') }}
          </StatusBadge>
        </div>
        <div class="info-item">
          <span class="info-label">{{ t('proxy_view.uptime') }}</span>
          <span>{{ proxyStore.status?.uptime || '—' }}</span>
        </div>
        <div class="info-item">
          <span class="info-label">{{ t('dashboard.connections') }}</span>
          <span>{{ proxyStore.status?.conns_current ?? 0 }}</span>
        </div>
        <div class="info-item">
          <span class="info-label">{{ t('geoblock.port') }}</span>
          <code>{{ proxyStore.status?.port }}</code>
        </div>
      </div>

      <div class="actions-grid mb-lg">
        <button class="btn btn-success" :disabled="proxyStore.loading || proxyStore.status?.running"
                @click="proxyStore.start()">
          <Loader2 v-if="proxyStore.activeAction === 'start'" :size="16" class="animate-spin" />
          <Play v-else :size="16" /> {{ t('dashboard.start') }}
        </button>
        <button class="btn btn-danger" :disabled="proxyStore.loading || !proxyStore.status?.running"
                @click="proxyStore.stop()">
          <Loader2 v-if="proxyStore.activeAction === 'stop'" :size="16" class="animate-spin" />
          <Square v-else :size="16" /> {{ t('dashboard.stop') }}
        </button>
        <button class="btn btn-warning" :disabled="proxyStore.loading"
                @click="proxyStore.restart()">
          <Loader2 v-if="proxyStore.activeAction === 'restart'" :size="16" class="animate-spin" />
          <RotateCw v-else :size="16" /> {{ t('dashboard.restart') }}
        </button>
        <button class="btn btn-ghost" :disabled="proxyStore.loading"
                @click="proxyStore.reload()">
          <Loader2 v-if="proxyStore.activeAction === 'reload'" :size="16" class="animate-spin" />
          <RefreshCw v-else :size="16" /> {{ t('proxy_view.reload_config') }}
        </button>
      </div>
    </div>

    <div class="card">
      <div class="flex-between mb-md">
        <h3 class="mb-none">{{ t('proxy_view.logs') }}</h3>
        <div class="flex-center gap-md">
          <div class="log-limit">
            <span class="text-xs text-muted mr-xs">{{ t('proxy_view.lines') }}</span>
            <input type="number" v-model.number="proxyStore.maxLogs" class="input input-sm limit-input" min="10" max="5000">
          </div>
          <ToggleSwitch v-model="isFollowing" :label="t('proxy_view.follow')" />
          <button class="btn btn-secondary btn-sm" @click="proxyStore.loadLogs()" :disabled="isFollowing">
            <RefreshCw :size="14" /> {{ t('proxy_view.refresh') }}
          </button>
        </div>
      </div>
      <pre class="logs-output" ref="logsRef" v-html="ansiToHtml(proxyStore.logs || t('proxy_view.logs_tip'))"></pre>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { useProxyStore } from '@/stores/proxy'
import { ansiToHtml } from '@/utils/ansi'
import StatusBadge from '@/components/common/StatusBadge.vue'
import ToggleSwitch from '@/components/common/ToggleSwitch.vue'
import { Play, Square, RotateCw, RefreshCw, Loader2 } from '@lucide/vue'

const { t } = useI18n()
const proxyStore = useProxyStore()
const logsRef = ref<HTMLElement | null>(null)

const isFollowing = ref(false)

watch(isFollowing, (following) => {
  if (following) {
    proxyStore.startLogsFollow()
  } else {
    proxyStore.stopLogsFollow()
  }
})

onMounted(() => proxyStore.loadStatus())

onUnmounted(() => {
  proxyStore.stopLogsFollow()
})

watch(() => proxyStore.logs, () => {
  if (isFollowing.value && logsRef.value) {
    nextTick(() => {
      logsRef.value!.scrollTop = logsRef.value!.scrollHeight
    })
  }
})
</script>

<style scoped lang="scss">
@use '@/assets/scss/variables' as *;

.status-bar {
  display: flex;
  flex-wrap: wrap;
  gap: $spacing-lg;
}

.actions-grid {
  display: flex;
  gap: $spacing-sm;
  flex-wrap: wrap;
}

.logs-output {
  background: $color-gray-900;
  color: $color-gray-100;
  padding: $spacing-md;
  border-radius: $border-radius;
  font-family: $font-mono;
  font-size: $font-size-sm;
  max-height: 500px;
  overflow-y: auto;
  white-space: pre-wrap;
  word-break: break-all;
  scroll-behavior: smooth;
}

.flex-between {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.flex-center {
  display: flex;
  align-items: center;
}

.gap-md {
  gap: $spacing-md;
}

.mb-none {
  margin-bottom: 0;
}

.log-limit {
  display: flex;
  align-items: center;
  gap: $spacing-xs;
}

.limit-input {
  width: 70px;
  padding: 4px 8px;
  height: 28px;
}

.mr-xs {
  margin-right: $spacing-xs;
}
</style>
