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
              {{ t('traffic.me_conns') }}
              <Tooltip :text="t('traffic.me_conns_tip')">
                <Info :size="12" class="ml-xs" />
              </Tooltip>
            </div>
            <span class="traffic-value">{{ trafficStore.live.connections_me_current }}</span>
          </div>
          <div class="traffic-item">
            <div class="traffic-label">
              {{ t('traffic.direct_conns') }}
              <Tooltip :text="t('traffic.direct_conns_tip')">
                <Info :size="12" class="ml-xs" />
              </Tooltip>
            </div>
            <span class="traffic-value">{{ trafficStore.live.connections_direct_current }}</span>
          </div>
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

    <!-- Per-User Traffic -->
    <div class="card">
      <h3 class="mb-md">{{ t('traffic.per_user_traffic') }}</h3>
      <div v-if="trafficStore.users && trafficStore.users.length" class="table-wrapper">
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
            <tr v-for="u in trafficStore.users" :key="u.label">
              <td><code>{{ u.label }}</code></td>
              <td>{{ formatBytes(u.bytes_in) }}</td>
              <td>{{ formatBytes(u.bytes_out) }}</td>
              <td>{{ formatBytes(u.bytes_in + u.bytes_out) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <EmptyState v-else :icon="BarChart3" :message="t('traffic.no_traffic')" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, watch, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useTrafficStore } from '@/stores/traffic'
import { useProxyStore } from '@/stores/proxy'
import { formatBytes } from '@/utils/format'
import ToggleSwitch from '@/components/common/ToggleSwitch.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Tooltip from '@/components/common/Tooltip.vue'
import { Activity, Loader2, BarChart3, Info } from '@lucide/vue'

const { t } = useI18n()
const trafficStore = useTrafficStore()
const proxyStore = useProxyStore()

const autoRefresh = ref(trafficStore.autoRefresh)

watch(autoRefresh, (val) => {
  trafficStore.toggleAutoRefresh(val)
  if (val && proxyRunning.value) {
    trafficStore.loadLive()
    trafficStore.startAutoRefresh()
  } else {
    trafficStore.stopAutoRefresh()
  }
})

const proxyRunning = computed(() => proxyStore.status?.running ?? false)

const upstreamSuccessRate = computed(() => {
  const live = trafficStore.live
  if (!live || live.upstream_attempt_total === 0) return '—'
  return ((live.upstream_success_total / live.upstream_attempt_total) * 100).toFixed(1)
})

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
