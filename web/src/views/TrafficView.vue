<template>
  <div>
    <!-- Global Traffic -->
    <div class="card mb-lg">
      <h3 class="mb-md">Global Traffic</h3>
      <div v-if="trafficStore.loading" class="text-muted">Loading...</div>
      <template v-else-if="trafficStore.global">
        <div class="traffic-stats">
          <div class="traffic-item">
            <span class="traffic-label">↓ Download</span>
            <span class="traffic-value">{{ formatBytes(trafficStore.global.bytes_in) }}</span>
          </div>
          <div class="traffic-item">
            <span class="traffic-label">↑ Upload</span>
            <span class="traffic-value">{{ formatBytes(trafficStore.global.bytes_out) }}</span>
          </div>
          <div class="traffic-item">
            <span class="traffic-label">Total</span>
            <span class="traffic-value">{{ formatBytes(trafficStore.global.bytes_in + trafficStore.global.bytes_out) }}</span>
          </div>
        </div>
      </template>
    </div>

    <!-- Live Metrics -->
    <div class="card mb-lg">
      <div class="flex justify-between items-center mb-md">
        <h3>Live Metrics</h3>
        <div v-if="proxyRunning" class="flex items-center gap-md">
          <label class="toggle-label">
            <input type="checkbox" :checked="trafficStore.autoRefresh" @change="trafficStore.toggleAutoRefresh(!trafficStore.autoRefresh)" />
            <span class="toggle-text">Auto-refresh</span>
          </label>
          <button class="btn btn-secondary btn-sm" :disabled="trafficStore.liveLoading" @click="trafficStore.loadLive()">
            {{ trafficStore.liveLoading ? 'Loading...' : 'Refresh' }}
          </button>
        </div>
      </div>

      <!-- Proxy not running -->
      <div v-if="!proxyRunning" class="empty-state">
        <div class="empty-icon status-stopped">&#9632;</div>
        <p class="text-muted">Proxy engine is not running. Start the proxy to see live metrics.</p>
      </div>

      <!-- Proxy running but metrics not available (engine starting up) -->
      <div v-else-if="trafficStore.liveError && !trafficStore.live" class="empty-state">
        <div class="empty-icon status-waiting">&#8987;</div>
        <p class="text-muted">Engine is starting up, waiting for metrics...</p>
      </div>

      <!-- Proxy running, metrics loaded -->
      <template v-else-if="trafficStore.live">
        <!-- Connections -->
        <div class="traffic-stats mb-lg">
          <div class="traffic-item">
            <span class="traffic-label">Active Connections</span>
            <span class="traffic-value">{{ trafficStore.live.connections }}</span>
          </div>
          <div class="traffic-item">
            <span class="traffic-label">Total Connections</span>
            <span class="traffic-value">{{ formatInt(trafficStore.live.connections_total) }}</span>
          </div>
          <div class="traffic-item">
            <span class="traffic-label">Bad Connections</span>
            <span class="traffic-value">{{ formatInt(trafficStore.live.connections_bad_total) }}</span>
          </div>
        </div>

        <!-- Connection Breakdown -->
        <div class="traffic-stats mb-lg">
          <div class="traffic-item">
            <span class="traffic-label">ME Connections</span>
            <span class="traffic-value">{{ trafficStore.live.connections_me_current }}</span>
          </div>
          <div class="traffic-item">
            <span class="traffic-label">Direct Connections</span>
            <span class="traffic-value">{{ trafficStore.live.connections_direct_current }}</span>
          </div>
          <div class="traffic-item">
            <span class="traffic-label">ME Writers Active</span>
            <span class="traffic-value">{{ trafficStore.live.me_writers_active }}</span>
          </div>
          <div class="traffic-item">
            <span class="traffic-label">ME Writers Warm</span>
            <span class="traffic-value">{{ trafficStore.live.me_writers_warm }}</span>
          </div>
        </div>

        <!-- Upstream Health -->
        <div class="traffic-stats mb-lg">
          <div class="traffic-item">
            <span class="traffic-label">Upstream Attempts</span>
            <span class="traffic-value">{{ formatInt(trafficStore.live.upstream_attempt_total) }}</span>
          </div>
          <div class="traffic-item">
            <span class="traffic-label">Upstream Success</span>
            <span class="traffic-value">{{ formatInt(trafficStore.live.upstream_success_total) }}</span>
          </div>
          <div class="traffic-item">
            <span class="traffic-label">Upstream Failures</span>
            <span class="traffic-value">{{ formatInt(trafficStore.live.upstream_fail_total) }}</span>
          </div>
          <div class="traffic-item">
            <span class="traffic-label">Upstream Success Rate</span>
            <span class="traffic-value">{{ upstreamSuccessRate }}%</span>
          </div>
        </div>

        <!-- Per-User Live Metrics -->
        <div v-if="Object.keys(trafficStore.live.user_metrics || {}).length" class="table-wrapper">
          <table class="table">
            <thead>
              <tr>
                <th>User</th>
                <th>From Client</th>
                <th>To Client</th>
                <th>Active</th>
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
        <div v-else class="text-muted text-sm">No active user metrics available.</div>
      </template>

      <!-- Proxy running, auto-refresh off, no data yet -->
      <div v-else class="text-muted text-sm">Click "Refresh" or enable auto-refresh to load live metrics.</div>
    </div>

    <!-- Per-User Traffic -->
    <div class="card">
      <h3 class="mb-md">Per-User Traffic</h3>
      <div v-if="trafficStore.users && trafficStore.users.length" class="table-wrapper">
        <table class="table">
          <thead>
            <tr>
              <th>User</th>
              <th>↓ In</th>
              <th>↑ Out</th>
              <th>Total</th>
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
      <div v-else class="empty-state">
        <div class="empty-icon">📈</div>
        <p class="text-muted">No traffic data yet. Start the proxy and wait for traffic to flow.</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, watch } from 'vue'
import { useTrafficStore } from '@/stores/traffic'
import { useProxyStore } from '@/stores/proxy'
import { formatBytes } from '@/utils/format'

const trafficStore = useTrafficStore()
const proxyStore = useProxyStore()

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
  if (running && trafficStore.autoRefresh) {
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
  if (proxyRunning.value && trafficStore.autoRefresh) {
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

.traffic-stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: $spacing-md; }
.traffic-item { display: flex; flex-direction: column; gap: $spacing-xs; }
.traffic-label { font-size: $font-size-xs; color: $text-muted; text-transform: uppercase; }
.traffic-value { font-size: $font-size-xl; font-weight: $font-weight-bold; }
.text-sm { font-size: $font-size-xs; }

.toggle-label {
  display: flex;
  align-items: center;
  gap: $spacing-xs;
  cursor: pointer;
  user-select: none;
}

.toggle-text {
  font-size: $font-size-sm;
  color: $text-secondary;
}

.status-stopped {
  color: #{$text-muted};
  font-size: 2.5rem;
  line-height: 1;
}

.status-waiting {
  color: #{$text-secondary};
  font-size: 2.5rem;
  line-height: 1;
  animation: pulse 1.5s ease-in-out infinite;
}

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
}
</style>
