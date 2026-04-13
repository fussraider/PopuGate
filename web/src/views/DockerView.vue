<template>
  <div>
    <!-- Docker Status -->
    <div class="card mb-lg">
      <h3 class="mb-md">Docker</h3>
      <div class="status-row mb-md">
        <StatusBadge :variant="dockerStore.dockerStatus?.installed ? 'success' : 'danger'">
          {{ dockerStore.dockerStatus?.installed ? 'Installed' : 'Not Installed' }}
        </StatusBadge>
        <span v-if="dockerStore.dockerStatus?.version" class="text-muted">v{{ dockerStore.dockerStatus.version }}</span>
      </div>
      <button v-if="!dockerStore.dockerStatus?.installed" class="btn btn-primary"
              :disabled="dockerStore.loading" @click="dockerStore.installDocker()">
        Install Docker
      </button>
    </div>

    <!-- Engine Status -->
    <div class="card mb-lg">
      <h3 class="mb-md">Telemt Engine</h3>
      <div class="status-row mb-md">
        <span>Version: <code>{{ dockerStore.engineStatus?.version || '—' }}</code></span>
        <StatusBadge :variant="dockerStore.engineStatus?.image_exists ? 'success' : 'warning'">
          {{ dockerStore.engineStatus?.image_exists ? 'Image Ready' : 'No Image' }}
        </StatusBadge>
      </div>
      <div class="flex gap-sm">
        <button class="btn btn-primary" :disabled="dockerStore.building" @click="dockerStore.buildEngine(false)">
          {{ dockerStore.building ? 'Building...' : 'Build / Pull Engine' }}
        </button>
        <button class="btn btn-warning" :disabled="dockerStore.building" @click="dockerStore.buildEngine(true)">
          Force Rebuild
        </button>
      </div>
      <div v-if="dockerStore.buildResult" class="alert alert-info mt-md">{{ dockerStore.buildResult }}</div>
    </div>

    <!-- Container Info -->
    <div class="card">
      <h3 class="mb-md">Container</h3>
      <div class="info-grid">
        <div class="info-item">
          <span class="info-label">Name</span>
          <code>popugate</code>
        </div>
        <div class="info-item">
          <span class="info-label">Image</span>
          <code>popugate-telemt:latest</code>
        </div>
        <div class="info-item">
          <span class="info-label">Running</span>
          <StatusBadge :variant="proxyStore.status?.running ? 'success' : 'danger'">
            {{ proxyStore.status?.running ? 'Yes' : 'No' }}
          </StatusBadge>
        </div>
        <div v-if="proxyStore.status?.container_id" class="info-item">
          <span class="info-label">Container ID</span>
          <code>{{ proxyStore.status.container_id }}</code>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import { useDockerStore, useProxyStore } from '@/stores'
import StatusBadge from '@/components/common/StatusBadge.vue'

const dockerStore = useDockerStore()
const proxyStore = useProxyStore()

onMounted(() => {
  dockerStore.loadDockerStatus()
  dockerStore.loadEngineStatus()
  proxyStore.loadStatus()
})
</script>

<style scoped lang="scss">
@use '@/assets/scss/variables' as *;

.status-row { display: flex; align-items: center; gap: $spacing-md; flex-wrap: wrap; }
.info-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: $spacing-md; }
.info-item { display: flex; flex-direction: column; gap: $spacing-xs; }
.info-label { font-size: $font-size-xs; color: $text-muted; text-transform: uppercase; }
</style>
