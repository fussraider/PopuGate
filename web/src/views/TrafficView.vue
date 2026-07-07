<template>
  <div>
    <!-- Global Traffic -->
    <div class="card mb-lg">
      <h3 class="mb-md">{{ t('traffic.title') }}</h3>
      <div v-if="trafficStore.loading" class="text-muted">{{ t('common.loading') }}</div>
      <template v-else-if="trafficStore.global">
        <div class="traffic-stats">
          <div class="traffic-item">
            <span class="traffic-label">↓ {{ t('traffic.download') }}</span>
            <span class="traffic-value">{{ formatBytes(trafficStore.global.bytes_in) }}</span>
          </div>
          <div class="traffic-item">
            <span class="traffic-label">↑ {{ t('traffic.upload') }}</span>
            <span class="traffic-value">{{ formatBytes(trafficStore.global.bytes_out) }}</span>
          </div>
          <div class="traffic-item">
            <span class="traffic-label">{{ t('traffic.total') }}</span>
            <span class="traffic-value">{{ formatBytes(trafficStore.global.bytes_in + trafficStore.global.bytes_out) }}</span>
          </div>
        </div>
      </template>
    </div>

    <!-- Live Metrics -->
    <div class="card mb-lg">
      <div class="flex justify-between items-center mb-md">
        <h3>{{ t('traffic.live_metrics') }}</h3>
        <div v-if="proxyRunning" class="flex items-center gap-md">
          <ToggleSwitch v-model="autoRefresh" :label="t('traffic.auto_refresh')" />
          <button class="btn btn-secondary btn-sm" :disabled="trafficStore.liveLoading" @click="trafficStore.loadLive()">
            <Loader2 v-if="trafficStore.liveLoading" :size="14" class="animate-spin" />
            {{ trafficStore.liveLoading ? t('common.loading') : t('traffic.refresh') }}
          </button>
        </div>
      </div>

      <!-- Proxy not running -->
      <EmptyState v-if="!proxyRunning" :icon="Activity" :message="t('traffic.proxy_not_running')" />

      <!-- Proxy running but metrics not available (engine starting up) -->
      <div v-else-if="trafficStore.liveError && !trafficStore.live" class="empty-state">
        <div class="empty-icon"><Loader2 :size="48" :stroke-width="1.2" class="animate-spin" /></div>
        <p class="text-muted">{{ t('traffic.engine_starting') }}</p>
      </div>

      <!-- Proxy running, metrics loaded -->
      <template v-else-if="trafficStore.live">
        <!-- Connections -->
        <div class="traffic-stats mb-lg">
          <div class="traffic-item">
            <div class="traffic-label">
              {{ t('traffic.active_conns') }}
              <Tooltip :text="t('traffic.active_conns_tip')">
                <Info :size="12" class="ml-xs" />
              </Tooltip>
            </div>
            <span class="traffic-value">{{ trafficStore.live.connections }}</span>
          </div>
          <div class="traffic-item">
            <div class="traffic-label">
              {{ t('traffic.total_conns') }}
              <Tooltip :text="t('traffic.total_conns_tip')">
                <Info :size="12" class="ml-xs" />
              </Tooltip>
            </div>
            <span class="traffic-value">{{ formatInt(trafficStore.live.connections_total) }}</span>
          </div>
          <div class="traffic-item">
            <div class="traffic-label">
              {{ t('traffic.bad_conns') }}
              <Tooltip :text="t('traffic.bad_conns_tip')">
                <Info :size="12" class="ml-xs" />
              </Tooltip>
            </div>
            <span class="traffic-value">{{ formatInt(trafficStore.live.connections_bad_total) }}</span>
          </div>
        </div>

        <!-- Connection Breakdown -->
        <div class="traffic-stats mb-lg">
          <div class="traffic-item">
            <div class="traffic-label">
              {{ t('traffic.me_writers_active') }}
              <Tooltip :text="t('traffic.me_writers_active_tip')">
                <Info :size="12" class="ml-xs" />
              </Tooltip>
            </div>
            <span class="traffic-value">{{ trafficStore.live.me_writers_active }}</span>
          </div>
          <div class="traffic-item">
            <div class="traffic-label">
              {{ t('traffic.me_writers_warm') }}
              <Tooltip :text="t('traffic.me_writers_warm_tip')">
                <Info :size="12" class="ml-xs" />
              </Tooltip>
            </div>
            <span class="traffic-value">{{ trafficStore.live.me_writers_warm }}</span>
          </div>
        </div>

        <!-- Upstream Health -->
        <div class="traffic-stats mb-lg">
          <div class="traffic-item">
            <div class="traffic-label">
              {{ t('traffic.upstream_attempts') }}
              <Tooltip :text="t('traffic.upstream_attempts_tip')">
                <Info :size="12" class="ml-xs" />
              </Tooltip>
            </div>
            <span class="traffic-value">{{ formatInt(trafficStore.live.upstream_attempt_total) }}</span>
          </div>
          <div class="traffic-item">
            <div class="traffic-label">
              {{ t('traffic.upstream_success') }}
              <Tooltip :text="t('traffic.upstream_success_tip')">
                <Info :size="12" class="ml-xs" />
              </Tooltip>
            </div>
            <span class="traffic-value">{{ formatInt(trafficStore.live.upstream_success_total) }}</span>
          </div>
          <div class="traffic-item">
            <div class="traffic-label">
              {{ t('traffic.upstream_failures') }}
              <Tooltip :text="t('traffic.upstream_failures_tip')">
                <Info :size="12" class="ml-xs" />
              </Tooltip>
            </div>
            <span class="traffic-value">{{ formatInt(trafficStore.live.upstream_fail_total) }}</span>
          </div>
          <div class="traffic-item">
            <div class="traffic-label">
              {{ t('traffic.upstream_success_rate') }}
              <Tooltip :text="t('traffic.upstream_success_rate_tip')">
                <Info :size="12" class="ml-xs" />
              </Tooltip>
            </div>
            <span class="traffic-value">{{ upstreamSuccessRate }}%</span>
          </div>
        </div>

        <!-- Error classes (engine diagnostics) -->
        <div v-if="hasErrorClasses" class="table-wrapper mb-lg">
          <h3 class="mb-sm">{{ t('traffic.error_classes') }}</h3>
          <table class="table">
            <thead>
              <tr>
                <th>{{ t('traffic.error_kind') }}</th>
                <th>{{ t('traffic.error_class') }}</th>
                <th>{{ t('traffic.error_count') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="[cls, n] in Object.entries(trafficStore.live.bad_by_class || {})" :key="'bad-' + cls">
                <td>{{ t('traffic.bad_connections') }}</td>
                <td><code>{{ cls }}</code></td>
                <td>{{ formatInt(n) }}</td>
              </tr>
              <tr v-for="[cls, n] in Object.entries(trafficStore.live.handshake_fail_by_class || {})" :key="'hs-' + cls">
                <td>{{ t('traffic.handshake_failures') }}</td>
                <td><code>{{ cls }}</code></td>
                <td>{{ formatInt(n) }}</td>
              </tr>
            </tbody>
          </table>
        </div>

        <!-- Per-User Live Metrics -->
        <div v-if="Object.keys(trafficStore.live.user_metrics || {}).length" class="table-wrapper">
          <table class="table">
            <thead>
              <tr>
                <th>{{ t('traffic.user_table.user') }}</th>
                <th>{{ t('traffic.user_table.in') }}</th>
                <th>{{ t('traffic.user_table.out') }}</th>
                <th>{{ t('dashboard.connections') }}</th>
                <th>Unique IPs</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="(m, label) in trafficStore.live.user_metrics" :key="label">
                <td><code>{{ label }}</code></td>
                <td>{{ formatBytes(m.octets_from_client) }}</td>
                <td>{{ formatBytes(m.octets_to_client) }}</td>
                <td>{{ m.connections }}</td>
                <td>{{ m.unique_ips }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-else class="text-muted text-sm">{{ t('traffic.no_user_metrics') }}</div>
      </template>

      <!-- Proxy running, auto-refresh off, no data yet -->
      <div v-else class="text-muted text-sm">{{ t('traffic.no_traffic') }}</div>
    </div>

    <!-- Traffic History -->
    <div class="card mb-lg">
      <div class="flex justify-between items-center mb-md">
        <h3>{{ t('traffic.history') }}</h3>
        <div class="flex gap-sm">
          <button v-for="range in dateRanges" :key="range.value"
            :class="['btn btn-sm', selectedRange === range.value ? 'btn-primary' : 'btn-secondary']"
            @click="selectRange(range.value)">
            {{ range.label }}
          </button>
        </div>
      </div>
      <div v-if="trafficStore.historyLoading" class="text-muted">{{ t('common.loading') }}</div>
      <TrafficChart v-else-if="trafficStore.history.length" :records="trafficStore.history" />
      <EmptyState v-else :icon="TrendingUp" :message="t('traffic.no_history')" />
    </div>

    <!-- Connections History -->
    <div class="card mb-lg">
      <h3 class="mb-md">{{ t('traffic.connections_history') }}</h3>
      <div v-if="trafficStore.historyLoading" class="text-muted">{{ t('common.loading') }}</div>
      <ConnectionsChart v-else-if="trafficStore.history.length" :records="trafficStore.history" />
      <EmptyState v-else :icon="TrendingUp" :message="t('traffic.no_history')" />
    </div>

    <!-- Per-User Traffic -->
    <div class="card">
      <h3 class="mb-md">{{ t('traffic.per_user_traffic') }}</h3>
      <div v-if="trafficStore.users && trafficStore.users.length > 0" class="per-user-layout">
        <TrafficDonut :users="trafficStore.users" :active-index="hoveredUserIdx" @hover="onDonutHover" />
        <div class="per-user-table">
          <table class="table">
            <thead>
              <tr>
                <th>{{ t('traffic.user_table.user') }}</th>
                <th>{{ t('traffic.user_table.in') }}</th>
                <th>{{ t('traffic.user_table.out') }}</th>
                <th>{{ t('traffic.user_table.total') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="(u, idx) in trafficStore.users" :key="u.label"
                :class="{ 'row-highlight': hoveredUserIdx === idx }"
                @mouseenter="hoveredUserIdx = idx"
                @mouseleave="hoveredUserIdx = null">
                <td><code>{{ u.label }}</code></td>
                <td>{{ formatBytes(u.bytes_in) }}</td>
                <td>{{ formatBytes(u.bytes_out) }}</td>
                <td>{{ formatBytes(u.bytes_in + u.bytes_out) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
      <EmptyState v-else :icon="BarChart3" :message="t('traffic.no_traffic')" />
    </div>
  </div>
</template>

<script setup lang="ts">
import {computed, onMounted, onUnmounted, ref, watch} from 'vue'
import {useI18n} from 'vue-i18n'
import {useTrafficStore} from '@/stores/traffic'
import {useProxyStore} from '@/stores/proxy'
import {formatBytes} from '@/utils/format'
import ToggleSwitch from '@/components/common/ToggleSwitch.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Tooltip from '@/components/common/Tooltip.vue'
import TrafficChart from '@/components/traffic/TrafficChart.vue'
import ConnectionsChart from '@/components/traffic/ConnectionsChart.vue'
import TrafficDonut from '@/components/traffic/TrafficDonut.vue'
import {Activity, BarChart3, Info, Loader2, TrendingUp} from '@lucide/vue'

const { t } = useI18n()
const trafficStore = useTrafficStore()
const proxyStore = useProxyStore()

const autoRefresh = ref(trafficStore.autoRefresh)
const selectedRange = ref('24h')

const dateRanges = [
  { label: '1H', value: '1h' },
  { label: '6H', value: '6h' },
  { label: '24H', value: '24h' },
  { label: '7D', value: '7d' },
  { label: '30D', value: '30d' },
]

function selectRange(range: string) {
  selectedRange.value = range
  const now = Math.floor(Date.now() / 1000)
  const secondsMap: Record<string, number> = {
    '1h': 3600,
    '6h': 21600,
    '24h': 86400,
    '7d': 604800,
    '30d': 2592000,
  }
  const aggregateMap: Record<string, string> = {
    '1h': 'none',
    '6h': 'none',
    '24h': 'hour',
    '7d': 'hour',
    '30d': 'day',
  }
  const start = now - (secondsMap[range] || 86400)
  trafficStore.loadHistory(start, now, undefined, aggregateMap[range])
}

watch(autoRefresh, (val) => {
  trafficStore.toggleAutoRefresh(val)
})

const proxyRunning = computed(() => proxyStore.status?.running ?? false)

const upstreamSuccessRate = computed(() => {
  const live = trafficStore.live
  if (!live || live.upstream_attempt_total === 0) return '—'
  return ((live.upstream_success_total / live.upstream_attempt_total) * 100).toFixed(1)
})

const hasErrorClasses = computed(() => {
  const live = trafficStore.live
  if (!live) return false
  return Object.keys(live.bad_by_class || {}).length > 0 || Object.keys(live.handshake_fail_by_class || {}).length > 0
})

const hoveredUserIdx = ref<number | null>(null)

function onDonutHover(idx: number | null) {
  hoveredUserIdx.value = idx
}

function formatInt(v: number): string {
  if (!v && v !== 0) return '—'
  return Math.round(v).toLocaleString()
}

watch(proxyRunning, (running) => {
  if (running && autoRefresh.value) {
    trafficStore.loadLive()
    trafficStore.startAutoRefresh()
  } else if (!running) {
    trafficStore.stopAutoRefresh()
    trafficStore.reset()
  }
})

onMounted(async () => {
  trafficStore.load()
  selectRange(selectedRange.value)
  await proxyStore.loadStatus()
  if (proxyRunning.value && autoRefresh.value) {
    trafficStore.loadLive()
    trafficStore.startAutoRefresh()
  }
})

onUnmounted(() => {
  trafficStore.stopAutoRefresh()
})
</script>

<style scoped lang="scss">
@use '@/assets/scss/variables' as *;

.per-user-layout {
  display: flex;
  align-items: flex-start;
  gap: $spacing-lg;

  @media (max-width: 768px) {
    flex-direction: column;
    align-items: center;
  }
}

.per-user-table {
  flex: 1;
  min-width: 0;
}

.row-highlight {
  background: rgba(59, 130, 246, 0.08);
}
</style>
