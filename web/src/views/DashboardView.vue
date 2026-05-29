<template>
  <div class="dashboard">
    <!-- Update Banner -->
    <div v-if="updateStore.status?.update_available" class="update-banner mb-lg">
      <Package :size="16" class="update-banner-icon" />
      <span>{{ t('dashboard.update_banner', { current: updateStore.status.current, latest: updateStore.status.latest }) }}</span>
      <router-link :to="{ name: 'Updates' }" class="btn btn-sm btn-warning" style="margin-left: auto;">
        {{ t('updates.check') }} <ArrowUpRight :size="12" />
      </router-link>
    </div>

    <!-- Status Cards -->
    <div class="stats-grid">
      <div class="stat-card stat-card-proxy">
        <div class="stat-icon"><Play :size="28" :stroke-width="1.5" /></div>
        <div class="stat-info">
          <div class="stat-label">{{ t('dashboard.proxy') }}</div>
          <div class="stat-proxy-status">
            <StatusBadge :variant="proxyRunning ? 'success' : 'danger'">
              {{ proxyRunning ? t('dashboard.running') : t('dashboard.stopped') }}
            </StatusBadge>
            <span v-if="proxyStore.status?.uptime" class="stat-uptime">{{ proxyStore.status.uptime }}</span>
          </div>
          <div class="proxy-actions">
            <Tooltip :text="t('dashboard.start_hint')">
              <button class="proxy-action-btn" :disabled="proxyStore.loading || proxyRunning" @click="proxyAction('start')">
                <Loader2 v-if="proxyStore.activeAction === 'start'" :size="11" class="animate-spin" />
                <Play v-else :size="11" />
              </button>
            </Tooltip>
            <Tooltip :text="t('dashboard.stop_hint')">
              <button class="proxy-action-btn" :disabled="proxyStore.loading || !proxyRunning" @click="proxyAction('stop')">
                <Loader2 v-if="proxyStore.activeAction === 'stop'" :size="11" class="animate-spin" />
                <Square v-else :size="11" />
              </button>
            </Tooltip>
            <Tooltip :text="t('dashboard.restart_hint')">
              <button class="proxy-action-btn" :disabled="proxyStore.loading" @click="proxyAction('restart')">
                <Loader2 v-if="proxyStore.activeAction === 'restart'" :size="11" class="animate-spin" />
                <RefreshCw v-else :size="11" />
              </button>
            </Tooltip>
            <Tooltip :text="t('dashboard.reload_hint')">
              <button class="proxy-action-btn" :disabled="proxyStore.loading" @click="proxyAction('reload')">
                <Loader2 v-if="proxyStore.activeAction === 'reload'" :size="11" class="animate-spin" />
                <RotateCw v-else :size="11" />
              </button>
            </Tooltip>
          </div>
        </div>
        <router-link :to="{ name: 'Instances' }" class="stat-card-link">
          <ArrowUpRight :size="16" />
        </router-link>
      </div>
      <div class="stat-card">
        <div class="stat-icon"><KeyRound :size="28" :stroke-width="1.5" /></div>
        <div class="stat-info">
          <div class="stat-label">{{ t('dashboard.secrets') }}</div>
          <div class="stat-value">{{ secretsStore.enabledCount }}/{{ secretsStore.secrets?.length || 0 }}</div>
        </div>
        <router-link :to="{ name: 'Secrets' }" class="stat-card-link">
          <ArrowUpRight :size="16" />
        </router-link>
      </div>
      <div class="stat-card stat-card-sparkline">
        <div class="stat-icon"><Users :size="28" :stroke-width="1.5" /></div>
        <div class="stat-info">
          <div class="stat-label">{{ t('dashboard.connections') }}</div>
          <div class="stat-value-row">
            <span class="stat-value">{{ proxyStore.status?.conns_current ?? 0 }}</span>
            <span v-if="connectionsDelta !== 0" class="stat-delta" :class="connectionsDelta > 0 ? 'stat-delta-up' : 'stat-delta-down'">
              {{ connectionsDelta > 0 ? '↑' : '↓' }}{{ Math.abs(connectionsDelta) }}
            </span>
          </div>
        </div>
        <router-link :to="{ name: 'Traffic' }" class="stat-card-link">
          <ArrowUpRight :size="16" />
        </router-link>
        <div class="sparkline-wrapper">
          <canvas ref="connectionsCanvas"></canvas>
        </div>
      </div>
      <div class="stat-card stat-card-sparkline">
        <div class="stat-icon"><TrendingUp :size="28" :stroke-width="1.5" /></div>
        <div class="stat-info">
          <div class="stat-label">{{ t('dashboard.traffic') }}</div>
          <div class="stat-value">{{ totalTraffic }}</div>
        </div>
        <router-link :to="{ name: 'Traffic' }" class="stat-card-link">
          <ArrowUpRight :size="16" />
        </router-link>
        <div class="sparkline-wrapper">
          <canvas ref="sparklineCanvas"></canvas>
        </div>
      </div>
    </div>

    <!-- Bottom Grid -->
    <div class="dashboard-grid">
      <!-- Services Row -->
      <div class="mini-cards-row">
        <router-link :to="{ name: 'Bot' }" class="mini-card">
          <span class="mini-card-label">{{ t('dashboard.bot') }}</span>
          <StatusBadge :variant="botStore.running ? 'success' : botStore.enabled ? 'danger' : 'neutral'">
            {{ botStore.running ? t('dashboard.running') : botStore.enabled ? t('dashboard.stopped') : t('dashboard.disabled') }}
          </StatusBadge>
        </router-link>

        <router-link :to="{ name: 'Backups' }" class="mini-card">
          <span class="mini-card-label">{{ t('dashboard.backup') }}</span>
          <StatusBadge v-if="lastBackup" :variant="backupAgeHours > 24 ? 'warning' : 'success'">
            {{ timeAgo(lastBackup.created_at) }}
          </StatusBadge>
          <StatusBadge v-else variant="neutral">{{ t('dashboard.no_backups') }}</StatusBadge>
        </router-link>

        <router-link :to="{ name: 'Geoblock' }" class="mini-card">
          <span class="mini-card-label">{{ t('dashboard.geoblock') }}</span>
          <StatusBadge v-if="geoblockStore.countries.length > 0" variant="success">
            {{ geoblockStore.mode === 'blacklist' ? '✕' : '✓' }} {{ geoblockStore.countries.length }}
          </StatusBadge>
          <StatusBadge v-else variant="neutral">{{ t('dashboard.disabled') }}</StatusBadge>
        </router-link>

        <router-link :to="{ name: 'Replication' }" class="mini-card">
          <span class="mini-card-label">{{ t('dashboard.replication') }}</span>
          <StatusBadge v-if="replicationStore.slaves.length > 0" :variant="replicationHealthy ? 'success' : 'warning'">
            {{ replicationStore.slaves.filter(s => s.status === 'connected').length }}/{{ replicationStore.slaves.length }}
          </StatusBadge>
          <StatusBadge v-else variant="neutral">{{ t('dashboard.not_configured') }}</StatusBadge>
        </router-link>
      </div>

      <!-- Resources -->
      <div class="card">
        <h3 class="mb-md card-header">
          {{ t('dashboard.resources') }}
          <router-link :to="{ name: 'System' }" class="card-header-link">
            <ArrowUpRight :size="14" />
          </router-link>
        </h3>
        <div v-if="!systemStore.resources" class="skeleton" style="height: 80px; width: 100%; border-radius: 4px;"></div>
        <div v-else class="resource-grid">
          <div class="resource-row">
            <Cpu :size="14" class="resource-icon" />
            <span class="resource-label">CPU</span>
            <div class="resource-bar">
              <div class="progress-bar" style="height: 4px;">
                <div class="progress-inner" :class="getBarVariant(systemStore.resources.cpu_usage)" :style="{ width: systemStore.resources.cpu_usage + '%' }"></div>
              </div>
            </div>
            <span class="resource-value">{{ systemStore.resources.cpu_usage.toFixed(1) }}%</span>
          </div>
          <div class="resource-row">
            <Database :size="14" class="resource-icon" />
            <span class="resource-label">RAM</span>
            <div class="resource-bar">
              <div class="progress-bar" style="height: 4px;">
                <div class="progress-inner" :class="getBarVariant(ramPercent)" :style="{ width: ramPercent + '%' }"></div>
              </div>
            </div>
            <span class="resource-value">{{ ramPercent.toFixed(1) }}%</span>
          </div>
          <div class="resource-row">
            <HardDrive :size="14" class="resource-icon" />
            <span class="resource-label">DISK</span>
            <div class="resource-bar">
              <div class="progress-bar" style="height: 4px;">
                <div class="progress-inner" :class="getBarVariant(diskPercent)" :style="{ width: diskPercent + '%' }"></div>
              </div>
            </div>
            <span class="resource-value">{{ diskPercent.toFixed(1) }}%</span>
          </div>
          <div class="resource-row">
            <Activity :size="14" class="resource-icon" />
            <span class="resource-label">LOAD</span>
            <span class="resource-value">{{ systemStore.resources.load1.toFixed(2) }}</span>
          </div>
          <div v-if="systemStore.resources.uptime" class="resource-footer">
            <span class="text-muted">{{ t('dashboard.uptime') }}: {{ formatUptime(systemStore.resources.uptime) }}</span>
          </div>
        </div>
      </div>

      <!-- Engine & Health -->
      <div class="card">
        <h3 class="mb-md card-header">
          {{ t('dashboard.engine') }}
          <router-link :to="{ name: 'Docker' }" class="card-header-link">
            <ArrowUpRight :size="14" />
          </router-link>
        </h3>

        <div class="engine-health-grid">
          <div class="engine-health-item">
            <span class="engine-health-label">{{ t('dashboard.docker') }}</span>
            <span class="engine-health-dots"></span>
            <StatusBadge :variant="healthStatus(proxyStore.health?.docker)">{{ proxyStore.health?.docker || '—' }}</StatusBadge>
          </div>
          <div class="engine-health-item">
            <span class="engine-health-label">{{ t('dashboard.health_containers') }}</span>
            <span class="engine-health-dots"></span>
            <StatusBadge :variant="healthStatus(proxyStore.health?.container)">{{ proxyStore.health?.container || '—' }}</StatusBadge>
          </div>
          <div class="engine-health-item">
            <span class="engine-health-label">{{ t('dashboard.health_ports') }}</span>
            <span class="engine-health-dots"></span>
            <StatusBadge :variant="healthStatus(proxyStore.health?.port)">{{ proxyStore.health?.port || '—' }}</StatusBadge>
          </div>
          <div class="engine-health-item">
            <span class="engine-health-label">{{ t('dashboard.metrics') }}</span>
            <span class="engine-health-dots"></span>
            <StatusBadge :variant="healthStatus(proxyStore.health?.metrics)">{{ proxyStore.health?.metrics || '—' }}</StatusBadge>
          </div>
        </div>

        <div class="engine-divider"></div>

        <div class="engine-info-grid">
          <div class="engine-info-item">
            <span class="engine-info-label">{{ t('dashboard.version') }}</span>
            <code>{{ dockerStore.engineStatus?.version || '—' }}</code>
          </div>
          <div class="engine-info-item">
            <span class="engine-info-label">{{ t('dashboard.instances') }}</span>
            <span>{{ runningInstances }}/{{ totalInstances }}</span>
          </div>
          <div class="engine-info-item">
            <span class="engine-info-label">{{ t('dashboard.image') }}</span>
            <StatusBadge :variant="dockerStore.engineStatus?.image_exists ? 'success' : 'danger'">
              {{ dockerStore.engineStatus?.image_exists ? t('dashboard.image_ready') : t('dashboard.image_missing') }}
            </StatusBadge>
          </div>
          <div class="engine-info-item">
            <span class="engine-info-label">{{ t('dashboard.update') }}</span>
            <StatusBadge v-if="dockerStore.telemtUpdateStatus?.updating" variant="warning">
              {{ t('dashboard.updating') }} v{{ dockerStore.telemtUpdateStatus.updating_to }}
            </StatusBadge>
            <StatusBadge v-else-if="dockerStore.applyingUpdate" variant="warning">
              {{ t('dashboard.updating') }}…
            </StatusBadge>
            <StatusBadge v-else-if="dockerStore.telemtUpdateStatus?.update_available" variant="warning">
              {{ t('dashboard.update_available') }} v{{ dockerStore.telemtUpdateStatus.latest?.version }}
            </StatusBadge>
            <StatusBadge v-else variant="success">{{ t('dashboard.up_to_date') }}</StatusBadge>
          </div>
        </div>
      </div>

      <!-- Secrets Health -->
      <div class="card">
        <h3 class="mb-md card-header">
          {{ t('dashboard.secrets_health') }}
          <router-link :to="{ name: 'Secrets' }" class="card-header-link">
            <ArrowUpRight :size="14" />
          </router-link>
        </h3>
        <div v-if="secretsAttention === 0" class="scheduler-ok">
          <CheckCircle :size="14" />
          <span>{{ t('dashboard.secrets_ok') }}</span>
        </div>
        <div v-else class="secrets-issues">
          <div v-if="expiredSecrets > 0" class="secrets-issue">
            <AlertCircle :size="14" class="secrets-issue-icon secrets-issue-danger" />
            <span><strong>{{ expiredSecrets }}</strong> {{ t('dashboard.secrets_expired') }}</span>
          </div>
          <div v-if="quotaWarnSecrets > 0" class="secrets-issue">
            <AlertCircle :size="14" class="secrets-issue-icon secrets-issue-warn" />
            <span><strong>{{ quotaWarnSecrets }}</strong> {{ t('dashboard.secrets_quota') }}</span>
          </div>
          <div v-if="disabledSecrets > 0" class="secrets-issue">
            <AlertCircle :size="14" class="secrets-issue-icon secrets-issue-muted" />
            <span><strong>{{ disabledSecrets }}</strong> {{ t('dashboard.secrets_disabled') }}</span>
          </div>
          <router-link v-if="noSecretInstances > 0" :to="{ name: 'Instances' }" class="secrets-issue secrets-issue-link">
            <AlertCircle :size="14" class="secrets-issue-icon secrets-issue-danger" />
            <span><strong>{{ noSecretInstances }}</strong> {{ t('dashboard.no_secret_instances') }}</span>
            <ArrowUpRight :size="12" class="secrets-issue-arrow" />
          </router-link>
        </div>
      </div>

      <!-- Top Users -->
      <div class="card">
        <h3 class="mb-md card-header">
          {{ t('dashboard.top_users') }}
          <router-link :to="{ name: 'Traffic' }" class="card-header-link">
            <ArrowUpRight :size="14" />
          </router-link>
        </h3>
        <div v-if="topUsers.length === 0" class="text-muted" style="font-size: $font-size-sm;">
          {{ t('dashboard.no_data') }}
        </div>
        <div v-else class="top-users-list">
          <div v-for="user in topUsers" :key="user.label" class="top-user-row">
            <span class="top-user-label">{{ user.label }}</span>
            <div class="top-user-bar">
              <div class="progress-bar" style="height: 4px;">
                <div class="progress-inner" :style="{ width: user.percent + '%' }"></div>
              </div>
            </div>
            <span class="top-user-value">{{ user.total }}</span>
          </div>
        </div>
      </div>

      <!-- Recent Activity -->
      <div class="card">
        <h3 class="mb-md card-header">
          {{ t('dashboard.activity') }}
          <router-link :to="{ name: 'Audit' }" class="card-header-link">
            <ArrowUpRight :size="14" />
          </router-link>
        </h3>
        <div v-if="recentActivity.length === 0" class="text-muted" style="font-size: $font-size-sm;">
          {{ t('dashboard.no_data') }}
        </div>
        <div v-else class="activity-list">
          <div v-for="entry in recentActivity" :key="entry.id" class="activity-item">
            <span class="activity-action" :class="'activity-' + actionVariant(entry.action)">
              {{ entry.action.replace(/_/g, ' ') }}
            </span>
            <span v-if="entry.detail" class="activity-detail">{{ entry.detail }}</span>
            <span class="activity-time">{{ timeAgo(entry.timestamp) }}</span>
          </div>
        </div>
      </div>

      <!-- Scheduler Status -->
      <div v-if="schedulerStore.tasks.length > 0" class="card">
        <h3 class="mb-md card-header">
          {{ t('dashboard.scheduler') }}
          <span v-if="errorTasks.length" class="badge badge-danger scheduler-badge">{{ errorTasks.length }}</span>
          <router-link :to="{ name: 'Scheduler' }" class="card-header-link">
            <ArrowUpRight :size="14" />
          </router-link>
        </h3>

        <div v-if="errorTasks.length" class="scheduler-errors mb-md">
          <div v-for="task in errorTasks.slice(0, 2)" :key="task.name" class="scheduler-error-item">
            <AlertCircle :size="14" class="scheduler-error-icon" />
            <div class="scheduler-error-info">
              <span class="scheduler-error-name">{{ t(`scheduler.tasks.${task.name}.name`) }}</span>
              <span v-if="task.last_run?.error" class="scheduler-error-msg">{{ task.last_run.error }}</span>
            </div>
          </div>
          <router-link v-if="errorTasks.length > 2" :to="{ name: 'Scheduler' }" class="scheduler-more">
            +{{ errorTasks.length - 2 }} {{ t('dashboard.more_errors') }}
          </router-link>
        </div>

        <div class="scheduler-footer">
          <div v-if="errorTasks.length === 0" class="scheduler-ok">
            <CheckCircle :size="14" />
            <span>{{ t('dashboard.scheduler_ok') }}</span>
          </div>
          <span class="text-muted">{{ enabledCount }}/{{ schedulerStore.tasks.length }} {{ t('dashboard.scheduler_active') }}</span>
          <span v-if="lastActivity" class="text-muted scheduler-last"> · {{ formatDate(lastActivity.last_run!.started_at) }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import {computed, onMounted, onUnmounted, ref, watch} from 'vue'
import {useI18n} from 'vue-i18n'
import {
  useAuditStore,
  useBackupStore,
  useBotStore,
  useConfigStore,
  useDockerStore,
  useGeoblockStore,
  useProxyStore,
  useReplicationStore,
  useSecretsStore,
  useSystemStore
} from '@/stores'
import {useTrafficStore} from '@/stores/traffic'
import {useSchedulerStore} from '@/stores/scheduler'
import {useToastStore} from '@/stores/toast'
import {formatBytes, formatDate, timeAgo} from '@/utils/format'
import StatusBadge from '@/components/common/StatusBadge.vue'
import Tooltip from '@/components/common/Tooltip.vue'
import {
  Activity,
  AlertCircle,
  ArrowUpRight,
  CheckCircle,
  Cpu,
  Database,
  HardDrive,
  KeyRound,
  Loader2,
  Package,
  Play,
  RefreshCw,
  RotateCw,
  Square,
  TrendingUp,
  Users
} from '@lucide/vue'
import {useUpdateStore} from '@/stores/update'

const { t } = useI18n()
const secretsStore = useSecretsStore()
const proxyStore = useProxyStore()
const dockerStore = useDockerStore()
const configStore = useConfigStore()
const systemStore = useSystemStore()
const trafficStore = useTrafficStore()
const schedulerStore = useSchedulerStore()
const toast = useToastStore()
const updateStore = useUpdateStore()
const replicationStore = useReplicationStore()
const botStore = useBotStore()
const backupStore = useBackupStore()
const auditStore = useAuditStore()
const geoblockStore = useGeoblockStore()

const sparklineCanvas = ref<HTMLCanvasElement | null>(null)
const connectionsCanvas = ref<HTMLCanvasElement | null>(null)

const proxyRunning = computed(() => proxyStore.status?.running)

const runningInstances = computed(() => proxyStore.status?.instances?.filter(i => i.running).length ?? 0)
const totalInstances = computed(() => proxyStore.status?.instances?.length ?? 0)
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

const errorTasks = computed(() =>
  schedulerStore.tasks.filter(t => t.last_run?.status === 'error')
)

const enabledCount = computed(() =>
  schedulerStore.tasks.filter(t => t.enabled).length
)

const lastActivity = computed(() => {
  const withRun = schedulerStore.tasks.filter(t => t.last_run)
  if (!withRun.length) return null
  return withRun.reduce((a, b) =>
    a.last_run!.started_at > b.last_run!.started_at ? a : b
  )
})

const expiredSecrets = computed(() => {
  const now = Date.now()
  return secretsStore.secrets.filter(s => {
    if (!s.expires_at || s.expires_at === '0' || s.archived_at) return false
    return new Date(s.expires_at).getTime() < now
  }).length
})

const quotaWarnSecrets = computed(() => {
  return secretsStore.secrets.filter(s => {
    if (s.archived_at || !s.quota_bytes || s.quota_bytes === 0) return false
    const used = (s.traffic_in || 0) + (s.traffic_out || 0)
    return used / s.quota_bytes > 0.8
  }).length
})

const disabledSecrets = computed(() => {
  return secretsStore.secrets.filter(s => !s.enabled && !s.archived_at).length
})

const noSecretInstances = computed(() =>
  (proxyStore.status?.instances ?? []).filter(i => i.matching_secret_count === 0).length
)

const secretsAttention = computed(() => expiredSecrets.value + quotaWarnSecrets.value + disabledSecrets.value + noSecretInstances.value)

const connectionsDelta = computed(() => {
  const h = trafficStore.history
  if (!h || h.length < 2) return 0
  const first = h[0].connections || 0
  const last = h[h.length - 1].connections || 0
  return last - first
})

const replicationHealthy = computed(() =>
  replicationStore.slaves.length > 0 && replicationStore.slaves.every(s => s.status === 'connected')
)

const lastBackup = computed(() => {
  const list = backupStore.backups
  if (!list || list.length === 0) return null
  return [...list].sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime())[0]
})

const backupAgeHours = computed(() => {
  if (!lastBackup.value) return Infinity
  return (Date.now() - new Date(lastBackup.value.created_at).getTime()) / 3600000
})

const recentActivity = computed(() => auditStore.entries.slice(0, 5))

const topUsers = computed(() => {
  const users = trafficStore.users
  if (!users || users.length === 0) return []
  const sorted = [...users]
    .map(u => ({ ...u, totalBytes: (u.bytes_in || 0) + (u.bytes_out || 0) }))
    .sort((a, b) => b.totalBytes - a.totalBytes)
    .slice(0, 5)
  const maxBytes = sorted[0]?.totalBytes || 1
  return sorted.map(u => ({
    label: u.label,
    total: formatBytes(u.totalBytes),
    percent: (u.totalBytes / maxBytes) * 100,
  }))
})

function getBarVariant(percent: number) {
  if (percent > 90) return 'danger'
  if (percent > 70) return 'warning'
  return 'success'
}

function formatUptime(seconds: number): string {
  const d = Math.floor(seconds / 86400)
  const h = Math.floor((seconds % 86400) / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  if (d > 0) return `${d}d ${h}h`
  if (h > 0) return `${h}h ${m}m`
  return `${m}m`
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
  } catch {
    // interceptor handles error toast
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

function actionVariant(action: string): 'success' | 'warning' | 'danger' | 'neutral' {
  if (action.includes('create') || action.includes('enable')) return 'success'
  if (action.includes('rotate') || action.includes('archive')) return 'warning'
  if (action.includes('delete') || action.includes('disable')) return 'danger'
  return 'neutral'
}

function buildSparkline() {
  const canvas = sparklineCanvas.value
  if (!canvas) return

  const records = trafficStore.history
  if (!records || records.length < 2) return

  const ctx = canvas.getContext('2d')
  if (!ctx) return

  const dpr = window.devicePixelRatio || 1
  const rect = canvas.getBoundingClientRect()
  if (rect.width === 0 || rect.height === 0) return
  canvas.width = rect.width * dpr
  canvas.height = rect.height * dpr
  ctx.scale(dpr, dpr)

  const w = rect.width
  const h = rect.height

  const data = records.map(r => (r.bytes_in || 0) + (r.bytes_out || 0))
  const max = Math.max(...data)
  if (max === 0) return

  ctx.clearRect(0, 0, w, h)

  const points = data.map((v, i) => ({
    x: (i / (data.length - 1)) * w,
    y: h - (v / max) * h * 0.85,
  }))

  // Gradient fill
  const gradient = ctx.createLinearGradient(0, 0, 0, h)
  gradient.addColorStop(0, 'rgba(59, 130, 246, 0.12)')
  gradient.addColorStop(1, 'rgba(59, 130, 246, 0.01)')

  ctx.beginPath()
  ctx.moveTo(points[0].x, h)
  ctx.lineTo(points[0].x, points[0].y)
  for (let i = 1; i < points.length; i++) {
    ctx.lineTo(points[i].x, points[i].y)
  }
  ctx.lineTo(points[points.length - 1].x, h)
  ctx.closePath()
  ctx.fillStyle = gradient
  ctx.fill()

  // Line
  ctx.beginPath()
  ctx.moveTo(points[0].x, points[0].y)
  for (let i = 1; i < points.length; i++) {
    ctx.lineTo(points[i].x, points[i].y)
  }
  ctx.strokeStyle = 'rgba(59, 130, 246, 0.35)'
  ctx.lineWidth = 1.5
  ctx.stroke()
}

function buildConnectionsSparkline() {
  const canvas = connectionsCanvas.value
  if (!canvas) return
  const records = trafficStore.history
  if (!records || records.length < 2) return
  const data = records.map(r => r.connections || 0)

  const ctx = canvas.getContext('2d')
  if (!ctx) return

  const dpr = window.devicePixelRatio || 1
  const rect = canvas.getBoundingClientRect()
  if (rect.width === 0 || rect.height === 0) return
  canvas.width = rect.width * dpr
  canvas.height = rect.height * dpr
  ctx.scale(dpr, dpr)

  const w = rect.width
  const h = rect.height
  const max = Math.max(...data)
  if (max === 0) return

  ctx.clearRect(0, 0, w, h)

  const points = data.map((v, i) => ({
    x: (i / (data.length - 1)) * w,
    y: h - (v / max) * h * 0.85,
  }))

  const gradient = ctx.createLinearGradient(0, 0, 0, h)
  gradient.addColorStop(0, 'rgba(59, 130, 246, 0.12)')
  gradient.addColorStop(1, 'rgba(59, 130, 246, 0.01)')

  ctx.beginPath()
  ctx.moveTo(points[0].x, h)
  ctx.lineTo(points[0].x, points[0].y)
  for (let i = 1; i < points.length; i++) ctx.lineTo(points[i].x, points[i].y)
  ctx.lineTo(points[points.length - 1].x, h)
  ctx.closePath()
  ctx.fillStyle = gradient
  ctx.fill()

  ctx.beginPath()
  ctx.moveTo(points[0].x, points[0].y)
  for (let i = 1; i < points.length; i++) ctx.lineTo(points[i].x, points[i].y)
  ctx.strokeStyle = 'rgba(59, 130, 246, 0.35)'
  ctx.lineWidth = 1.5
  ctx.stroke()
}

watch(() => trafficStore.history, () => {
  buildSparkline()
  buildConnectionsSparkline()
}, { deep: true })

onMounted(async () => {
  await Promise.all([
    secretsStore.load(),
    proxyStore.loadStatus(),
    proxyStore.loadHealth(),
    dockerStore.loadEngineStatus(),
    dockerStore.loadTelemtUpdateStatus(),
    configStore.load(),
    schedulerStore.load(),
    trafficStore.load(),
    replicationStore.loadSlaves(),
    botStore.loadStatus(),
    backupStore.load(),
    auditStore.load(),
  ])

  if (configStore.settings) {
    geoblockStore.load(configStore.settings)
  }

  systemStore.startResourceStream()

  const now = Math.floor(Date.now() / 1000)
  trafficStore.loadHistory(now - 3600, now, undefined, 'none').then(() => buildSparkline())
})

onUnmounted(() => {
  systemStore.stopResourceStream()
})
</script>

<style scoped lang="scss">
@use '@/assets/scss/variables' as *;

.update-banner {
  display: flex;
  align-items: center;
  gap: $spacing-sm;
  padding: $spacing-sm $spacing-lg;
  background: var(--alert-warning-bg);
  border: 1px solid var(--alert-warning-border);
  border-radius: $border-radius-lg;
  font-size: $font-size-sm;
  color: var(--text-primary);
  flex-wrap: wrap;
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: $spacing-md;
  margin-bottom: $spacing-md;

  @media (max-width: 480px) {
    grid-template-columns: repeat(2, 1fr);
    gap: $spacing-sm;
    margin-bottom: $spacing-sm;
  }
}

.stat-card {
  display: flex;
  align-items: center;
  gap: $spacing-md;
  padding: $spacing-lg;
  min-width: 0;
  background: $bg-card;
  border: 1px solid $border-color;
  border-radius: $border-radius-lg;
  box-shadow: $shadow-sm;

  @media (max-width: 480px) {
    padding: $spacing-sm $spacing-md;
    gap: $spacing-sm;
  }
}

.stat-card-sparkline {
  position: relative;
  overflow: hidden;

  > .stat-icon,
  > .stat-info,
  > .stat-card-link {
    position: relative;
    z-index: 1;
  }
}

.sparkline-wrapper {
  position: absolute;
  inset: 0;
  pointer-events: none;

  canvas {
    width: 100%;
    height: 100%;
    display: block;
  }
}

.stat-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  color: $color-primary;
  flex-shrink: 0;

  @media (max-width: 480px) {
    :deep(svg) {
      width: 22px;
      height: 22px;
    }
  }
}

.stat-label { font-size: $font-size-xs; color: $text-secondary; text-transform: uppercase; letter-spacing: 0.05em; }
.stat-value { font-size: $font-size-xl; font-weight: $font-weight-bold; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

.stat-value-row {
  display: flex;
  align-items: baseline;
  gap: $spacing-xs;
}

.stat-delta {
  font-size: $font-size-xs;
  font-weight: $font-weight-semibold;
}

.stat-delta-up { color: $color-success; }
.stat-delta-down { color: $color-danger; }

.stat-card-link {
  display: flex;
  align-items: center;
  justify-content: center;
  color: $text-secondary;
  opacity: 0.3;
  transition: opacity 0.2s, color 0.2s;
  margin-left: auto;
  text-decoration: none;
  flex-shrink: 0;

  &:hover {
    opacity: 1;
    color: $color-primary;
  }
}

.stat-uptime {
  font-size: $font-size-xs;
  color: $text-secondary;
}

.stat-card-proxy {
  position: relative;
  padding-bottom: 32px;

  .stat-info {
    position: relative;
  }
}

.stat-proxy-status {
  display: flex;
  align-items: center;
  gap: 6px;
}

.proxy-actions {
  position: absolute;
  bottom: -26px;
  left: 0;
  display: flex;
  gap: 2px;
}

.proxy-action-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border: 1px solid $border-color;
  border-radius: $border-radius-sm;
  background: transparent;
  color: $text-secondary;
  cursor: pointer;
  padding: 0;
  transition: all 0.15s;

  &:hover:not(:disabled) {
    background: rgba(99, 102, 241, 0.08);
    color: $color-primary;
    border-color: $color-primary;
  }

  &:disabled {
    opacity: 0.3;
    cursor: not-allowed;
  }
}

.resource-grid {
  display: flex;
  flex-direction: column;
  gap: $spacing-sm;
}

.resource-row {
  display: flex;
  align-items: center;
  gap: $spacing-sm;
}

.resource-icon {
  color: $text-secondary;
  flex-shrink: 0;
}

.resource-label {
  font-size: $font-size-xs;
  color: $text-secondary;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  width: 36px;
  flex-shrink: 0;
}

.resource-bar {
  flex: 1;
  min-width: 80px;
}

.resource-value {
  font-size: $font-size-sm;
  font-weight: $font-weight-semibold;
  width: 48px;
  text-align: right;
  flex-shrink: 0;
}

.resource-footer {
  padding-top: $spacing-xs;
  border-top: 1px solid $border-color;
  font-size: $font-size-xs;
}

.card-header {
  display: flex;
  align-items: center;
  gap: $spacing-sm;
}

.card-header-link {
  display: inline-flex;
  align-items: center;
  color: $text-secondary;
  opacity: 0.3;
  transition: opacity 0.2s, color 0.2s;
  text-decoration: none;

  &:hover {
    opacity: 1;
    color: $color-primary;
  }
}

// Engine card
.engine-health-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: $spacing-sm $spacing-md;
}

.engine-health-item {
  display: flex;
  align-items: center;
  gap: $spacing-xs;
}

.engine-health-label {
  font-size: $font-size-xs;
  color: $text-secondary;
  white-space: nowrap;
}

.engine-health-dots {
  flex: 1;
  min-width: 8px;
  border-bottom: 1px dotted $border-color;
}

.engine-divider {
  border-top: 1px solid $border-color;
  margin: $spacing-md 0;
}

.engine-info-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: $spacing-sm $spacing-md;
}

.engine-info-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.engine-info-label {
  font-size: $font-size-xs;
  color: $text-secondary;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.dashboard-grid {
  column-count: 2;
  column-gap: $spacing-md;

  > .card {
    break-inside: avoid;
    margin-bottom: $spacing-md;
    min-width: 0;
  }

  @media (max-width: 480px) {
    column-count: 1;

    > .card {
      margin-bottom: $spacing-sm;
    }
  }
}

// Secrets Health
.secrets-issues {
  display: flex;
  flex-direction: column;
  gap: $spacing-xs;
}

.secrets-issue {
  display: flex;
  align-items: center;
  gap: $spacing-sm;
  font-size: $font-size-sm;
}

.secrets-issue-icon { flex-shrink: 0; }
.secrets-issue-danger { color: $color-danger; }
.secrets-issue-warn { color: $color-warning; }
.secrets-issue-muted { color: $text-secondary; }

.secrets-issue-link {
  text-decoration: none;
  color: inherit;
  border-radius: $border-radius-sm;
  padding: 2px 4px;
  margin: -2px -4px;
  transition: background 0.15s;

  &:hover {
    background: rgba(239, 68, 68, 0.06);
  }
}

.secrets-issue-arrow {
  margin-left: auto;
  color: $text-secondary;
  opacity: 0;
  transition: opacity 0.15s;
  flex-shrink: 0;
}

.secrets-issue-link:hover .secrets-issue-arrow {
  opacity: 0.6;
}

// Top Users
.top-users-list {
  display: flex;
  flex-direction: column;
  gap: $spacing-sm;
}

.top-user-row {
  display: flex;
  align-items: center;
  gap: $spacing-sm;
}

.top-user-label {
  font-size: $font-size-sm;
  width: 80px;
  flex-shrink: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.top-user-bar {
  flex: 1;
  min-width: 40px;
}

.top-user-value {
  font-size: $font-size-xs;
  color: $text-secondary;
  width: 56px;
  text-align: right;
  flex-shrink: 0;
}

// Scheduler
.scheduler-badge {
  font-size: 10px;
  padding: 1px 6px;
}

.scheduler-errors {
  display: flex;
  flex-direction: column;
  gap: $spacing-xs;
}

.scheduler-more {
  display: inline-block;
  font-size: $font-size-xs;
  color: $color-danger;
  text-decoration: none;
  padding-left: $spacing-xs;
  transition: opacity 0.15s;

  &:hover { opacity: 0.7; }
}

.scheduler-error-item {
  display: flex;
  align-items: flex-start;
  gap: $spacing-sm;
  padding: $spacing-sm;
  background: $color-danger-bg;
  border-radius: $border-radius-sm;
}

.scheduler-error-icon {
  color: $color-danger;
  flex-shrink: 0;
  margin-top: 1px;
}

.scheduler-error-info {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.scheduler-error-name {
  font-weight: $font-weight-medium;
  font-size: $font-size-sm;
}

.scheduler-error-msg {
  font-size: $font-size-xs;
  color: $text-secondary;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.scheduler-ok {
  display: inline-flex;
  align-items: center;
  gap: $spacing-xs;
  color: $color-success;
  font-size: $font-size-sm;
}

.scheduler-footer {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: $spacing-sm;
  font-size: $font-size-xs;
}

.scheduler-last {
  white-space: nowrap;
}

// Mini cards row (bot, backup, geo-block)
.mini-cards-row {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: $spacing-md;
  break-inside: avoid;
  margin-bottom: $spacing-md;

  @media (max-width: 480px) {
    grid-template-columns: repeat(2, 1fr);
    gap: $spacing-sm;
    margin-bottom: $spacing-sm;
  }
}

.mini-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: $spacing-xs;
  padding: $spacing-sm $spacing-xs;
  background: $bg-card;
  border: 1px solid $border-color;
  border-radius: $border-radius-lg;
  text-decoration: none;
  color: inherit;
  box-shadow: $shadow-sm;
  transition: border-color 0.15s;
  text-align: center;

  &:hover {
    border-color: $color-primary;
  }
}

.mini-card-label {
  font-size: 10px;
  color: $text-secondary;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  white-space: nowrap;
  line-height: 1;
}

// Recent Activity
.activity-list {
  display: flex;
  flex-direction: column;
  gap: $spacing-xs;
}

.activity-item {
  display: flex;
  align-items: center;
  gap: $spacing-sm;
  font-size: $font-size-sm;
}

.activity-action {
  font-weight: $font-weight-medium;
  text-transform: capitalize;
}

.activity-success { color: $color-success; }
.activity-warning { color: $color-warning; }
.activity-danger { color: $color-danger; }
.activity-neutral { color: $text-secondary; }

.activity-detail {
  flex: 1;
  min-width: 0;
  color: $text-secondary;
  font-size: $font-size-xs;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.activity-time {
  font-size: $font-size-xs;
  color: $text-secondary;
  white-space: nowrap;
  flex-shrink: 0;
}
</style>
