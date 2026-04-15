<template>
  <div class="dashboard">
    <!-- Status Cards -->
    <div class="stats-grid">
      <div class="stat-card">
        <div class="stat-icon">▶️</div>
        <div class="stat-info">
          <div class="stat-label">Proxy</div>
          <StatusBadge :variant="proxyRunning ? 'success' : 'danger'">
            {{ proxyRunning ? 'Running' : 'Stopped' }}
          </StatusBadge>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon">🔑</div>
        <div class="stat-info">
          <div class="stat-label">Secrets</div>
          <div class="stat-value">{{ secretsStore.enabledCount }}/{{ secretsStore.secrets?.length || 0 }}</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon">👥</div>
        <div class="stat-info">
          <div class="stat-label">Connections</div>
          <div class="stat-value">{{ proxyStore.status?.conns_current ?? 0 }}</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon">📈</div>
        <div class="stat-info">
          <div class="stat-label">Traffic</div>
          <div class="stat-value">{{ totalTraffic }}</div>
        </div>
      </div>
    </div>

    <!-- Quick Actions -->
    <div class="card mb-lg">
      <h3 class="mb-md">Quick Actions</h3>
      <div class="actions-grid">
        <button class="btn btn-success" :disabled="proxyStore.loading || proxyRunning" @click="proxyAction('start')">
          ▶ Start
        </button>
        <button class="btn btn-danger" :disabled="proxyStore.loading || !proxyRunning" @click="proxyAction('stop')">
          ⏹ Stop
        </button>
        <button class="btn btn-warning" :disabled="proxyStore.loading" @click="proxyAction('restart')">
          🔄 Restart
        </button>
        <button class="btn btn-ghost" :disabled="proxyStore.loading" @click="proxyAction('reload')">
          🔃 Reload
        </button>
      </div>
    </div>

    <!-- Health Status -->
    <div class="card mb-lg">
      <h3 class="mb-md">System Health</h3>
      <InfoGrid>
        <InfoItem label="Docker">
          <StatusBadge :variant="healthStatus(proxyStore.health?.docker)">{{ proxyStore.health?.docker || '—' }}</StatusBadge>
        </InfoItem>
        <InfoItem label="Container">
          <StatusBadge :variant="healthStatus(proxyStore.health?.container)">{{ proxyStore.health?.container || '—' }}</StatusBadge>
        </InfoItem>
        <InfoItem label="Port">
          <StatusBadge :variant="healthStatus(proxyStore.health?.port)">{{ proxyStore.health?.port || '—' }}</StatusBadge>
        </InfoItem>
        <InfoItem label="Metrics">
          <StatusBadge :variant="healthStatus(proxyStore.health?.metrics)">{{ proxyStore.health?.metrics || '—' }}</StatusBadge>
        </InfoItem>
      </InfoGrid>
    </div>

    <!-- Engine Info -->
    <div class="card">
      <h3 class="mb-md">Engine</h3>
      <InfoGrid>
        <InfoItem label="Version">
          <code>{{ dockerStore.engineStatus?.version || '—' }}</code>
        </InfoItem>
        <InfoItem label="Port">
          <span>{{ configStore.settings?.proxy_port }}</span>
        </InfoItem>
        <InfoItem label="Domain">
          <span>{{ configStore.settings?.proxy_domain || '—' }}</span>
        </InfoItem>
        <InfoItem label="Uptime">
          <span>{{ proxyStore.status?.uptime || '—' }}</span>
        </InfoItem>
      </InfoGrid>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useSecretsStore, useProxyStore, useDockerStore, useConfigStore } from '@/stores'
import { useToastStore } from '@/stores/toast'
import { formatBytes } from '@/utils/format'
import StatusBadge from '@/components/common/StatusBadge.vue'
import InfoGrid from '@/components/common/InfoGrid.vue'
import InfoItem from '@/components/common/InfoItem.vue'

const secretsStore = useSecretsStore()
const proxyStore = useProxyStore()
const dockerStore = useDockerStore()
const configStore = useConfigStore()
const toast = useToastStore()

const proxyRunning = computed(() => proxyStore.status?.running)
const totalTraffic = computed(() => {
  const s = proxyStore.status
  if (!s) return '0 B'
  return formatBytes((s.traffic_in || 0) + (s.traffic_out || 0))
})

async function proxyAction(action: 'start' | 'stop' | 'restart' | 'reload') {
  try {
    await proxyStore[action]()
    const labels = { start: 'started', stop: 'stopped', restart: 'restarted', reload: 'config reloaded' }
    toast.success(`Proxy ${labels[action]}`)
  } catch (e: any) {
    toast.error(e.response?.data?.error ?? e.message)
  }
}

function healthStatus(status?: string): 'success' | 'warning' | 'danger' | 'neutral' {
  if (!status) return 'neutral'
  const s = status.toLowerCase()
  if (s.includes('running') || s.includes('listening') || s.includes('responding') || s === 'installed') return 'success'
  if (s.includes('error') || s.includes('not')) return 'danger'
  return 'warning'
}

onMounted(async () => {
  await Promise.all([
    secretsStore.load(),
    proxyStore.loadStatus(),
    proxyStore.loadHealth(),
    dockerStore.loadEngineStatus(),
    configStore.load(),
  ])
})
</script>

<style scoped lang="scss">
@use '@/assets/scss/variables' as *;

.stats-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: $spacing-md;
  margin-bottom: $spacing-lg;
}

.stat-card {
  display: flex;
  align-items: center;
  gap: $spacing-md;
  padding: $spacing-lg;
  background: $bg-card;
  border: 1px solid $border-color;
  border-radius: $border-radius-lg;
  box-shadow: $shadow-sm;
}

.stat-icon { font-size: 2rem; }
.stat-label { font-size: $font-size-xs; color: $text-secondary; text-transform: uppercase; letter-spacing: 0.05em; }
.stat-value { font-size: $font-size-xl; font-weight: $font-weight-bold; }

.actions-grid {
  display: flex;
  gap: $spacing-sm;
  flex-wrap: wrap;
}
</style>
