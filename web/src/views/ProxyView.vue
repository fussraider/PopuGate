<template>
  <div>
    <div class="card mb-lg">
      <h3 class="mb-md">Proxy Control</h3>
      <div class="status-bar mb-lg">
        <div class="status-item">
          <span class="status-label">Status</span>
          <StatusBadge :variant="proxyStore.status?.running ? 'success' : 'danger'">
            {{ proxyStore.status?.running ? 'Running' : 'Stopped' }}
          </StatusBadge>
        </div>
        <div class="status-item">
          <span class="status-label">Uptime</span>
          <span>{{ proxyStore.status?.uptime || '—' }}</span>
        </div>
        <div class="status-item">
          <span class="status-label">Connections</span>
          <span>{{ proxyStore.status?.conns_current ?? 0 }}</span>
        </div>
        <div class="status-item">
          <span class="status-label">Port</span>
          <code>{{ proxyStore.status?.port }}</code>
        </div>
      </div>

      <div class="actions-grid mb-lg">
        <button class="btn btn-success" :disabled="proxyStore.loading || proxyStore.status?.running"
                @click="proxyStore.start()">▶ Start</button>
        <button class="btn btn-danger" :disabled="proxyStore.loading || !proxyStore.status?.running"
                @click="proxyStore.stop()">⏹ Stop</button>
        <button class="btn btn-warning" :disabled="proxyStore.loading"
                @click="proxyStore.restart()">🔄 Restart</button>
        <button class="btn btn-ghost" :disabled="proxyStore.loading"
                @click="proxyStore.reload()">🔃 Reload Config</button>
      </div>
    </div>

    <div class="card">
      <div class="flex-between mb-md">
        <h3 class="mb-none">Logs</h3>
        <div class="flex-center gap-md">
          <div class="log-limit">
            <span class="text-xs text-muted mr-xs">Lines:</span>
            <input type="number" v-model.number="proxyStore.maxLogs" class="input input-sm limit-input" min="10" max="5000">
          </div>
          <label class="toggle-control">
            <input type="checkbox" :checked="proxyStore.isFollowing" @change="handleFollowToggle">
            <span class="toggle-label">Follow Logs (SSE)</span>
          </label>
          <button class="btn btn-secondary btn-sm" @click="proxyStore.loadLogs()" :disabled="proxyStore.isFollowing">
            Refresh Logs
          </button>
        </div>
      </div>
      <pre class="logs-output" ref="logsRef" v-html="ansiToHtml(proxyStore.logs || 'Click &quot;Refresh Logs&quot; to load')"></pre>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch, nextTick } from 'vue'
import { useProxyStore } from '@/stores/proxy'
import { ansiToHtml } from '@/utils/ansi'
import StatusBadge from '@/components/common/StatusBadge.vue'

const proxyStore = useProxyStore()
const logsRef = ref<HTMLElement | null>(null)

onMounted(() => proxyStore.loadStatus())

onUnmounted(() => {
  proxyStore.stopLogsFollow()
})

const handleFollowToggle = (e: Event) => {
  const checked = (e.target as HTMLInputElement).checked
  if (checked) {
    proxyStore.startLogsFollow()
  } else {
    proxyStore.stopLogsFollow()
  }
}

// Auto-scroll to bottom when logs update in follow mode
watch(() => proxyStore.logs, () => {
  if (proxyStore.isFollowing && logsRef.value) {
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

.status-item {
  display: flex;
  flex-direction: column;
  gap: $spacing-xs;
}

.status-label {
  font-size: $font-size-xs;
  color: $text-muted;
  text-transform: uppercase;
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

.toggle-control {
  display: flex;
  align-items: center;
  gap: $spacing-xs;
  cursor: pointer;
  user-select: none;
  font-size: $font-size-sm;

  input {
    cursor: pointer;
  }
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

.text-xs {
  font-size: $font-size-xs;
}

.mr-xs {
  margin-right: $spacing-xs;
}
</style>
