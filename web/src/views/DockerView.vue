<template>
  <div>
    <!-- Docker Status -->
    <div class="card mb-lg">
      <h3 class="mb-md">{{ t('docker.title') }}</h3>
      <div class="status-row mb-md">
        <StatusBadge :variant="dockerStore.dockerStatus?.installed ? 'success' : 'danger'">
          {{ dockerStore.dockerStatus?.installed ? t('docker.installed') : t('docker.not_installed') }}
        </StatusBadge>
        <span v-if="dockerStore.dockerStatus?.version" class="text-muted">v{{ dockerStore.dockerStatus.version }}</span>
      </div>
      <button v-if="!dockerStore.dockerStatus?.installed" class="btn btn-primary"
              :disabled="dockerStore.loading" @click="dockerStore.installDocker()">
        <Loader2 v-if="dockerStore.loading" :size="16" class="animate-spin" />
        {{ dockerStore.loading ? t('docker.installing') : t('docker.install') }}
      </button>
    </div>

    <!-- Engine Status -->
    <div class="card mb-lg">
      <h3 class="mb-md">{{ t('docker.engine_title') }}</h3>
      <div class="status-row mb-md">
        <span>{{ t('dashboard.version') }}: <code>{{ dockerStore.engineStatus?.version || '—' }}</code></span>
        <StatusBadge :variant="dockerStore.engineStatus?.image_exists ? 'success' : 'warning'">
          {{ dockerStore.engineStatus?.image_exists ? t('docker.image_ready') : t('docker.no_image') }}
        </StatusBadge>
      </div>
      <div class="flex gap-sm">
        <button class="btn btn-primary" :disabled="dockerStore.building" @click="dockerStore.buildEngine(false)">
          <Loader2 v-if="dockerStore.building" :size="16" class="animate-spin" />
          {{ dockerStore.building ? t('docker.building') : t('docker.build_pull') }}
        </button>
        <button class="btn btn-warning" :disabled="dockerStore.building" @click="dockerStore.buildEngine(true)">
          <Loader2 v-if="dockerStore.building" :size="16" class="animate-spin" />
          {{ t('docker.force_rebuild') }}
        </button>
      </div>
      <div v-if="dockerStore.buildResult" class="alert alert-info mt-md">{{ dockerStore.buildResult }}</div>
    </div>

    <!-- Container Info -->
    <div class="card">
      <h3 class="mb-md">{{ t('docker.container_title') }}</h3>
      <InfoGrid>
        <InfoItem :label="t('common.name')">
          <code>popugate</code>
        </InfoItem>
        <InfoItem :label="t('docker.image')">
          <code>popugate-telemt:latest</code>
        </InfoItem>
        <InfoItem :label="t('dashboard.running')">
          <StatusBadge :variant="proxyStore.status?.running ? 'success' : 'danger'">
            {{ proxyStore.status?.running ? t('docker.yes') : t('docker.no') }}
          </StatusBadge>
        </InfoItem>
        <InfoItem v-if="proxyStore.status?.container_id" :label="t('docker.container_id')">
          <code>{{ proxyStore.status.container_id }}</code>
        </InfoItem>
      </InfoGrid>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useDockerStore, useProxyStore } from '@/stores'
import { Loader2 } from '@lucide/vue'
import StatusBadge from '@/components/common/StatusBadge.vue'
import InfoGrid from '@/components/common/InfoGrid.vue'
import InfoItem from '@/components/common/InfoItem.vue'

const { t } = useI18n()
const dockerStore = useDockerStore()
const proxyStore = useProxyStore()

onMounted(() => {
  dockerStore.loadDockerStatus()
  dockerStore.loadEngineStatus()
  proxyStore.loadStatus()
})
</script>
