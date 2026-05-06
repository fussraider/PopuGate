<template>
  <div class="dashboard">
    <!-- Status Cards -->
    <div class="stats-grid">
      <div class="stat-card">
        <div class="stat-icon"><Play :size="28" :stroke-width="1.5" /></div>
        <div class="stat-info">
          <div class="stat-label">{{ t('dashboard.proxy') }}</div>
          <StatusBadge :variant="proxyRunning ? 'success' : 'danger'">
            {{ proxyRunning ? t('dashboard.running') : t('dashboard.stopped') }}
          </StatusBadge>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon"><KeyRound :size="28" :stroke-width="1.5" /></div>
        <div class="stat-info">
          <div class="stat-label">{{ t('dashboard.secrets') }}</div>
          <div class="stat-value">{{ secretsStore.enabledCount }}/{{ secretsStore.secrets?.length || 0 }}</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon"><Users :size="28" :stroke-width="1.5" /></div>
        <div class="stat-info">
          <div class="stat-label">{{ t('dashboard.connections') }}</div>
          <div class="stat-value">{{ proxyStore.status?.conns_current ?? 0 }}</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon"><TrendingUp :size="28" :stroke-width="1.5" /></div>
        <div class="stat-info">
          <div class="stat-label">{{ t('dashboard.traffic') }}</div>
          <div class="stat-value">{{ totalTraffic }}</div>
        </div>
      </div>
    </div>

    <!-- Resource Cards -->
    <div v-if="!systemStore.resources" class="stats-grid mb-lg">
      <div v-for="i in 4" :key="i" class="stat-card">
        <div class="skeleton" style="height: 56px; width: 100%; border-radius: 4px;"></div>
      </div>
    </div>
    <template v-else>
      <div class="stats-grid mb-lg">
        <div class="stat-card">
          <div class="stat-icon"><Cpu :size="28" :stroke-width="1.5" /></div>
          <div class="stat-info w-full">
            <div class="stat-label">CPU</div>
            <div class="stat-value mb-xs">{{ systemStore.resources.cpu_usage.toFixed(1) }}%</div>
            <div class="progress-bar" style="height: 4px;">
              <div class="progress-inner" :class="getBarVariant(systemStore.resources.cpu_usage)" :style="{ width: systemStore.resources.cpu_usage + '%' }"></div>
            </div>
          </div>
        </div>
        <div class="stat-card">
          <div class="stat-icon"><Database :size="28" :stroke-width="1.5" /></div>
          <div class="stat-info w-full">
            <div class="stat-label">RAM</div>
            <div class="stat-value mb-xs">{{ ramPercent.toFixed(1) }}%</div>
            <div class="progress-bar" style="height: 4px;">
              <div class="progress-inner" :class="getBarVariant(ramPercent)" :style="{ width: ramPercent + '%' }"></div>
            </div>
          </div>
        </div>
        <div class="stat-card">
          <div class="stat-icon"><HardDrive :size="28" :stroke-width="1.5" /></div>
          <div class="stat-info w-full">
            <div class="stat-label">DISK</div>
            <div class="stat-value mb-xs">{{ diskPercent.toFixed(1) }}%</div>
            <div class="progress-bar" style="height: 4px;">
              <div class="progress-inner" :class="getBarVariant(diskPercent)" :style="{ width: diskPercent + '%' }"></div>
            </div>
          </div>
        </div>
        <div class="stat-card">
          <div class="stat-icon"><Activity :size="28" :stroke-width="1.5" /></div>
          <div class="stat-info">
            <div class="stat-label">LOAD</div>
            <div class="stat-value">{{ systemStore.resources.load1.toFixed(2) }}</div>
          </div>
        </div>
      </div>
    </template>

    <!-- Quick Actions -->
    <div class="card mb-lg">
      <h3 class="mb-md">{{ t('dashboard.quick_actions') }}</h3>
      <div class="actions-grid">
        <Tooltip :text="t('dashboard.start_hint')">
          <button class="btn btn-success" :disabled="proxyStore.loading || proxyRunning" @click="proxyAction('start')">
            <Loader2 v-if="proxyStore.activeAction === 'start'" :size="16" class="animate-spin" />
            <Play v-else :size="16" /> {{ t('dashboard.start') }}
          </button>
        </Tooltip>
        <Tooltip :text="t('dashboard.stop_hint')">
          <button class="btn btn-danger" :disabled="proxyStore.loading || !proxyRunning" @click="proxyAction('stop')">
            <Loader2 v-if="proxyStore.activeAction === 'stop'" :size="16" class="animate-spin" />
            <Square v-else :size="16" /> {{ t('dashboard.stop') }}
          </button>
        </Tooltip>
        <Tooltip :text="t('dashboard.restart_hint')">
          <button class="btn btn-warning" :disabled="proxyStore.loading" @click="proxyAction('restart')">
            <Loader2 v-if="proxyStore.activeAction === 'restart'" :size="16" class="animate-spin" />
            <RefreshCw v-else :size="16" /> {{ t('dashboard.restart') }}
          </button>
        </Tooltip>
        <Tooltip :text="t('dashboard.reload_hint')">
          <button class="btn btn-outline" :disabled="proxyStore.loading" @click="proxyAction('reload')">
            <Loader2 v-if="proxyStore.activeAction === 'reload'" :size="16" class="animate-spin" />
            <RotateCw v-else :size="16" /> {{ t('dashboard.reload') }}
          </button>
        </Tooltip>
      </div>
    </div>

    <!-- Health Status -->
    <div class="card mb-lg">
      <h3 class="mb-md">{{ t('dashboard.system_health') }}</h3>
      <InfoGrid>
        <InfoItem :label="t('dashboard.docker')">
          <StatusBadge :variant="healthStatus(proxyStore.health?.docker)">{{ proxyStore.health?.docker || '—' }}</StatusBadge>
        </InfoItem>
        <InfoItem :label="t('dashboard.container')">
          <StatusBadge :variant="healthStatus(proxyStore.health?.container)">{{ proxyStore.health?.container || '—' }}</StatusBadge>
        </InfoItem>
        <InfoItem :label="t('dashboard.port')">
          <StatusBadge :variant="healthStatus(proxyStore.health?.port)">{{ proxyStore.health?.port || '—' }}</StatusBadge>
        </InfoItem>
        <InfoItem :label="t('dashboard.metrics')">
          <StatusBadge :variant="healthStatus(proxyStore.health?.metrics)">{{ proxyStore.health?.metrics || '—' }}</StatusBadge>
        </InfoItem>
      </InfoGrid>
    </div>

    <!-- Engine Info -->
    <div class="card">
      <h3 class="mb-md">{{ t('dashboard.engine') }}</h3>
      <InfoGrid>
        <InfoItem :label="t('dashboard.version')">
          <code>{{ dockerStore.engineStatus?.version || '—' }}</code>
        </InfoItem>
        <InfoItem :label="t('dashboard.port')">
          <span>{{ configStore.settings?.proxy_port }}</span>
        </InfoItem>
        <InfoItem :label="t('dashboard.domain')">
          <span>{{ configStore.settings?.proxy_domain || '—' }}</span>
        </InfoItem>
        <InfoItem :label="t('dashboard.uptime')">
          <span>{{ proxyStore.status?.uptime || '—' }}</span>
        </InfoItem>
      </InfoGrid>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useSecretsStore, useProxyStore, useDockerStore, useConfigStore, useSystemStore } from '@/stores'
import { useToastStore } from '@/stores/toast'
import { formatBytes } from '@/utils/format'
import StatusBadge from '@/components/common/StatusBadge.vue'
import InfoGrid from '@/components/common/InfoGrid.vue'
import InfoItem from '@/components/common/InfoItem.vue'
import Tooltip from '@/components/common/Tooltip.vue'
import { Play, KeyRound, Users, TrendingUp, Square, RefreshCw, RotateCw, Loader2, Cpu, Database, HardDrive, Activity } from '@lucide/vue'

const { t } = useI18n()
const secretsStore = useSecretsStore()
const proxyStore = useProxyStore()
const dockerStore = useDockerStore()
const configStore = useConfigStore()
const systemStore = useSystemStore()
const toast = useToastStore()

const proxyRunning = computed(() => proxyStore.status?.running)
const totalTraffic = computed(() => {
  const s = proxyStore.status
  if (!s) return '0 B'
  return formatBytes((s.traffic_in || 0) + (s.traffic_out || 0))
})

const ramPercent = computed(() => {
  const r = systemStore.resources
  if (!r || r.memory_total === 0) return 0
  return (r.memory_used / r.memory_total) * 100
})

const diskPercent = computed(() => {
  const r = systemStore.resources
  if (!r || r.disk_total === 0) return 0
  return (r.disk_used / r.disk_total) * 100
})

function getBarVariant(percent: number) {
  if (percent > 90) return 'danger'
  if (percent > 70) return 'warning'
  return 'success'
}

async function proxyAction(action: 'start' | 'stop' | 'restart' | 'reload') {
  try {
    await proxyStore[action]()
    const labels = {
      start: t('dashboard.started'),
      stop: t('dashboard.stopped_success'),
      restart: t('dashboard.restarted'),
      reload: t('dashboard.reloaded')
    }
    toast.success(labels[action])
  } catch (e: any) {
    toast.error(e.response?.data?.error ?? e.message)
  }
}

function healthStatus(status?: string): 'success' | 'warning' | 'danger' | 'neutral' {
  if (!status) return 'neutral'
  const s = status.toLowerCase()

  // Parse "X/Y running|listening|responding" format
  const match = s.match(/^(\d+)\/(\d+)\s/)
  if (match) {
    const current = parseInt(match[1], 10)
    const total = parseInt(match[2], 10)
    if (current >= total && total > 0) return 'success'
    if (current > 0) return 'warning'
    return 'danger'
  }

  if (s === 'installed') return 'success'
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

  systemStore.startResourceStream()
})

onUnmounted(() => {
  systemStore.stopResourceStream()
})
</script>

<style scoped lang="scss">
@use '@/assets/scss/variables' as *;

.stats-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: $spacing-md;
  margin-bottom: $spacing-lg;

  @media (max-width: 480px) {
    grid-template-columns: 1fr;
  }
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

.stat-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  color: $color-primary;
  flex-shrink: 0;
}
.stat-label { font-size: $font-size-xs; color: $text-secondary; text-transform: uppercase; letter-spacing: 0.05em; }
.stat-value { font-size: $font-size-xl; font-weight: $font-weight-bold; }

.actions-grid {
  display: flex;
  gap: $spacing-sm;
  flex-wrap: wrap;
}
</style>
