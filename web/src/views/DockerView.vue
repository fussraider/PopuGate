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
      <InfoGrid>
        <InfoItem label="Name">
          <code>popugate</code>
        </InfoItem>
        <InfoItem label="Image">
          <code>popugate-telemt:latest</code>
        </InfoItem>
        <InfoItem label="Running">
          <StatusBadge :variant="proxyStore.status?.running ? 'success' : 'danger'">
            {{ proxyStore.status?.running ? 'Yes' : 'No' }}
          </StatusBadge>
        </InfoItem>
        <InfoItem v-if="proxyStore.status?.container_id" label="Container ID">
          <code>{{ proxyStore.status.container_id }}</code>
        </InfoItem>
      </InfoGrid>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import { useDockerStore, useProxyStore } from '@/stores'
import StatusBadge from '@/components/common/StatusBadge.vue'
import InfoGrid from '@/components/common/InfoGrid.vue'
import InfoItem from '@/components/common/InfoItem.vue'

const dockerStore = useDockerStore()
const proxyStore = useProxyStore()

onMounted(() => {
  dockerStore.loadDockerStatus()
  dockerStore.loadEngineStatus()
  proxyStore.loadStatus()
})
</script>
