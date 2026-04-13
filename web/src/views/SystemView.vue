<template>
  <div class="system-view">
    <!-- OS Information -->
    <div class="card mb-lg">
      <h3 class="mb-md">Operating System</h3>
      <div v-if="systemStore.os" class="info-grid">
        <div class="info-item">
          <span class="info-label">OS Family</span>
          <span class="info-value">{{ systemStore.os.family }}</span>
        </div>
        <div class="info-item">
          <span class="info-label">Version</span>
          <span class="info-value">{{ systemStore.os.version }}</span>
        </div>
        <div class="info-item">
          <span class="info-label">Architecture</span>
          <span class="info-value">{{ systemStore.os.arch }}</span>
        </div>
      </div>
      <div v-else class="text-muted">Loading OS information...</div>
    </div>

    <!-- Systemd Service Management -->
    <div class="card">
      <div class="flex justify-between items-center mb-md">
        <h3>Systemd Service</h3>
        <StatusBadge :variant="serviceStatusVariant">
          {{ systemStore.service?.active || 'Not Installed' }}
        </StatusBadge>
      </div>

      <div v-if="systemStore.service" class="service-details mb-lg">
        <div class="info-grid">
          <div class="info-item">
            <span class="info-label">Status</span>
            <span class="info-value">{{ systemStore.service.active }}</span>
          </div>
          <div class="info-item">
            <span class="info-label">Enabled</span>
            <span class="info-value">{{ systemStore.service.enabled ? 'Yes' : 'No' }}</span>
          </div>
          <div v-if="systemStore.service.pid" class="info-item">
            <span class="info-label">Main PID</span>
            <span class="info-value">{{ systemStore.service.pid }}</span>
          </div>
          <div v-if="systemStore.service.uptime" class="info-item">
            <span class="info-label">Uptime</span>
            <span class="info-value">{{ systemStore.service.uptime }}</span>
          </div>
        </div>
      </div>

      <div v-if="systemStore.service?.supported" class="actions-grid">
        <template v-if="!systemStore.service?.installed">
          <button 
            class="btn btn-primary" 
            :disabled="systemStore.loading" 
            @click="handleInstall"
          >
            {{ systemStore.loading ? 'Installing...' : 'Install Service' }}
          </button>
        </template>
        
        <template v-else>
          <button 
            class="btn btn-warning" 
            :disabled="systemStore.loading" 
            @click="handleRestart"
          >
            {{ systemStore.loading ? 'Restarting...' : 'Restart Service' }}
          </button>
          
          <button 
            class="btn btn-ghost" 
            :disabled="systemStore.loading" 
            @click="handleReload"
          >
            {{ systemStore.loading ? 'Reloading...' : 'Reload Config' }}
          </button>
          
          <button 
            class="btn btn-danger" 
            :disabled="systemStore.loading" 
            @click="handleUninstall"
          >
            {{ systemStore.loading ? 'Uninstalling...' : 'Uninstall Service' }}
          </button>
        </template>
      </div>

      <div v-if="systemStore.service?.supported" class="mt-lg text-muted text-sm">
        <p><strong>Note:</strong> Installing the service allows PopuGate to start automatically on boot and restart if it crashes. This action requires root privileges on the server.</p>
      </div>
      <div v-else-if="systemStore.service" class="mt-lg text-warning text-sm">
        <p><strong>Note:</strong> Systemd management is not supported on this operating system. You should manage the application process manually or using other tools available for your OS.</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useSystemStore } from '@/stores/system'
import { useToastStore } from '@/stores/toast'
import StatusBadge from '@/components/common/StatusBadge.vue'

const systemStore = useSystemStore()
const toast = useToastStore()

const serviceStatusVariant = computed(() => {
  const status = systemStore.service?.active?.toLowerCase() || ''
  if (status.includes('active') || status.includes('running')) return 'success'
  if (status.includes('inactive') || status.includes('dead')) return 'danger'
  if (status.includes('failed')) return 'danger'
  return 'warning'
})

async function handleInstall() {
  try {
    await systemStore.installService()
    toast.success('Systemd service installed and enabled')
  } catch (e: any) {
    toast.error(e.response?.data?.error || e.message)
  }
}

async function handleUninstall() {
  if (!confirm('Are you sure you want to uninstall the systemd service?')) return
  try {
    await systemStore.uninstallService()
    toast.success('Systemd service uninstalled')
  } catch (e: any) {
    toast.error(e.response?.data?.error || e.message)
  }
}

async function handleRestart() {
  try {
    await systemStore.restartService()
    toast.success('Service restart signal sent')
    // Status will update after a short delay
    setTimeout(() => systemStore.loadServiceStatus(), 2000)
  } catch (e: any) {
    toast.error(e.response?.data?.error || e.message)
  }
}

async function handleReload() {
  try {
    await systemStore.reloadService()
    toast.success('Service configuration reloaded')
  } catch (e: any) {
    toast.error(e.response?.data?.error || e.message)
  }
}

onMounted(async () => {
  await Promise.all([
    systemStore.loadOS(),
    systemStore.loadServiceStatus()
  ])
})
</script>

<style scoped lang="scss">
@use '@/assets/scss/variables' as *;

.info-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: $spacing-md;
}

.info-item {
  display: flex;
  flex-direction: column;
  gap: $spacing-xs;
}

.info-label {
  font-size: $font-size-xs;
  color: $text-muted;
  text-transform: uppercase;
}

.info-value {
  font-weight: $font-weight-medium;
}

.actions-grid {
  display: flex;
  gap: $spacing-sm;
  flex-wrap: wrap;
}

.service-details {
  padding: $spacing-md;
  background: rgba(0, 0, 0, 0.02);
  border-radius: $border-radius;
  border: 1px solid $border-color;
}
</style>
