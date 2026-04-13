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
        <button class="btn btn-secondary btn-sm" :disabled="trafficStore.liveLoading" @click="trafficStore.loadLive()">
          {{ trafficStore.liveLoading ? 'Loading...' : 'Refresh' }}
        </button>
      </div>
      <div v-if="trafficStore.live">
        <div class="traffic-stats mb-md">
          <div class="traffic-item">
            <span class="traffic-label">Active Connections</span>
            <span class="traffic-value">{{ trafficStore.live.connections }}</span>
          </div>
        </div>
        <div v-if="Object.keys(trafficStore.live.user_metrics || {}).length" class="table-wrapper">
          <table class="table">
            <thead><tr><th>User</th><th>From Client</th><th>To Client</th><th>Active</th></tr></thead>
            <tbody>
              <tr v-for="(m, label) in trafficStore.live.user_metrics" :key="label">
                <td><code>{{ label }}</code></td>
                <td>{{ formatBytes(m.octets_from_client) }}</td>
                <td>{{ formatBytes(m.octets_to_client) }}</td>
                <td>{{ m.connections }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-else class="text-muted text-sm">No active user metrics available.</div>
      </div>
      <div v-else class="text-muted text-sm">Click "Refresh" to load live metrics.</div>
    </div>

    <!-- Per-User Traffic -->
    <div class="card">
      <h3 class="mb-md">Per-User Traffic</h3>
      <div v-if="trafficStore.users && trafficStore.users.length" class="table-wrapper">
        <table class="table">
          <thead><tr><th>User</th><th>↓ In</th><th>↑ Out</th><th>Total</th></tr></thead>
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
import { onMounted } from 'vue'
import { useTrafficStore } from '@/stores/traffic'
import { formatBytes } from '@/utils/format'

const trafficStore = useTrafficStore()

onMounted(() => trafficStore.load())
</script>

<style scoped lang="scss">
@use '@/assets/scss/variables' as *;

.traffic-stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: $spacing-md; }
.traffic-item { display: flex; flex-direction: column; gap: $spacing-xs; }
.traffic-label { font-size: $font-size-xs; color: $text-muted; text-transform: uppercase; }
.traffic-value { font-size: $font-size-xl; font-weight: $font-weight-bold; }
.text-sm { font-size: $font-size-xs; }
</style>
