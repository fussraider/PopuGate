<template>
  <div class="system-view">
    <!-- System Resources -->
    <div class="card mb-lg">
      <h3 class="mb-md">{{ t('system.resources_title') }}</h3>
      <div v-if="systemStore.resources">
        <div class="resource-grid mb-lg">
          <!-- CPU -->
          <div class="resource-item">
            <div class="flex justify-between items-center mb-xs">
              <span class="text-sm font-medium">{{ t('system.cpu_usage') }}</span>
              <span class="text-sm">{{ systemStore.resources.cpu_usage.toFixed(1) }}%</span>
            </div>
            <div class="progress-bar">
              <div
                class="progress-inner"
                :class="getBarVariant(systemStore.resources.cpu_usage)"
                :style="{ width: systemStore.resources.cpu_usage + '%' }"
              ></div>
            </div>
          </div>

          <!-- Memory -->
          <div class="resource-item">
            <div class="flex justify-between items-center mb-xs">
              <span class="text-sm font-medium">{{ t('system.memory_usage') }}</span>
              <span class="text-sm">
                {{ formatBytes(systemStore.resources.memory_used) }} /
                {{ formatBytes(systemStore.resources.memory_total) }}
              </span>
            </div>
            <div class="progress-bar">
              <div
                class="progress-inner"
                :class="getBarVariant((systemStore.resources.memory_used / systemStore.resources.memory_total) * 100)"
                :style="{ width: (systemStore.resources.memory_used / systemStore.resources.memory_total) * 100 + '%' }"
              ></div>
            </div>
          </div>

          <!-- Disk -->
          <div class="resource-item">
            <div class="flex justify-between items-center mb-xs">
              <span class="text-sm font-medium">{{ t('system.disk_usage') }}</span>
              <span class="text-sm">
                {{ formatBytes(systemStore.resources.disk_used) }} /
                {{ formatBytes(systemStore.resources.disk_total) }}
              </span>
            </div>
            <div class="progress-bar">
              <div
                class="progress-inner"
                :class="getBarVariant((systemStore.resources.disk_used / systemStore.resources.disk_total) * 100)"
                :style="{ width: (systemStore.resources.disk_used / systemStore.resources.disk_total) * 100 + '%' }"
              ></div>
            </div>
          </div>
        </div>

        <InfoGrid>
          <InfoItem :label="t('system.load_avg')">
            <span>{{ systemStore.resources.load1.toFixed(2) }} {{ systemStore.resources.load5.toFixed(2) }} {{ systemStore.resources.load15.toFixed(2) }}</span>
          </InfoItem>
          <InfoItem :label="t('system.uptime')">
            <span>{{ formatUptime(systemStore.resources.uptime) }}</span>
          </InfoItem>
        </InfoGrid>
      </div>
      <div v-else class="text-muted">{{ t('system.loading') }}</div>
    </div>

    <!-- OS Information -->
    <div class="card mb-lg">
      <h3 class="mb-md">{{ t('system.title') }}</h3>
      <div v-if="systemStore.os">
        <InfoGrid>
          <InfoItem :label="t('system.os_family')">
            <span>{{ systemStore.os.family }}</span>
          </InfoItem>
          <InfoItem :label="t('system.version')">
            <span>{{ systemStore.os.version }}</span>
          </InfoItem>
          <InfoItem :label="t('system.arch')">
            <span>{{ systemStore.os.arch }}</span>
          </InfoItem>
        </InfoGrid>
      </div>
      <div v-else class="text-muted">{{ t('system.loading') }}</div>
    </div>

    <!-- Systemd Service Management -->
    <div class="card">
      <div class="flex justify-between items-center mb-md">
        <h3>{{ t('system.service_title') }}</h3>
        <StatusBadge :variant="serviceStatusVariant">
          {{ systemStore.service?.active || t('system.not_installed') }}
        </StatusBadge>
      </div>

      <div v-if="systemStore.service" class="service-details mb-lg">
        <InfoGrid>
          <InfoItem :label="t('system.status')">
            <span>{{ systemStore.service.active }}</span>
          </InfoItem>
          <InfoItem :label="t('system.enabled')">
            <span>{{ systemStore.service.enabled ? t('system.yes') : t('system.no') }}</span>
          </InfoItem>
          <InfoItem v-if="systemStore.service.pid" :label="t('system.main_pid')">
            <span>{{ systemStore.service.pid }}</span>
          </InfoItem>
          <InfoItem v-if="systemStore.service.uptime" :label="t('system.uptime')">
            <span>{{ systemStore.service.uptime }}</span>
          </InfoItem>
        </InfoGrid>
      </div>

      <div v-if="systemStore.service?.supported" class="actions-grid">
        <template v-if="!systemStore.service?.installed">
          <button
            class="btn btn-primary"
            :disabled="systemStore.loading"
            @click="handleInstall"
          >
            <Loader2 v-if="systemStore.loading" :size="16" class="animate-spin" />
            {{ systemStore.loading ? t('system.installing') : t('system.install') }}
          </button>
        </template>

        <template v-else>
          <button
            class="btn btn-warning"
            :disabled="systemStore.loading"
            @click="handleRestart"
          >
            <Loader2 v-if="systemStore.loading" :size="16" class="animate-spin" />
            {{ systemStore.loading ? t('system.restarting') : t('system.restart') }}
          </button>

          <button
            class="btn btn-ghost"
            :disabled="systemStore.loading"
            @click="handleReload"
          >
            <Loader2 v-if="systemStore.loading" :size="16" class="animate-spin" />
            {{ systemStore.loading ? t('system.reloading') : t('system.reload') }}
          </button>

          <button
            class="btn btn-danger"
            :disabled="systemStore.loading"
            @click="handleUninstall"
          >
            <Loader2 v-if="systemStore.loading" :size="16" class="animate-spin" />
            {{ systemStore.loading ? t('system.uninstalling') : t('system.uninstall') }}
          </button>
        </template>
      </div>

      <div v-if="systemStore.service?.supported" class="mt-lg text-muted text-sm">
        <p><strong>{{ t('common.description') }}:</strong> {{ t('system.note') }}</p>
      </div>
      <div v-else-if="systemStore.service" class="mt-lg text-warning text-sm">
        <p><strong>{{ t('common.description') }}:</strong> {{ t('system.unsupported') }}</p>
      </div>
    </div>

    <ConfirmDialog v-bind="confirmState" @confirm="handleConfirm" @cancel="handleCancel" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useSystemStore } from '@/stores/system'
import { useToastStore } from '@/stores/toast'
import { useConfirmDialog } from '@/composables/useConfirmDialog'
import { Loader2 } from '@lucide/vue'
import StatusBadge from '@/components/common/StatusBadge.vue'
import InfoGrid from '@/components/common/InfoGrid.vue'
import InfoItem from '@/components/common/InfoItem.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import { formatBytes } from '@/utils/format'

const { t } = useI18n()
const systemStore = useSystemStore()
const toast = useToastStore()

const { confirmState, confirm, handleConfirm, handleCancel } = useConfirmDialog()

const serviceStatusVariant = computed(() => {
  const status = systemStore.service?.active?.toLowerCase() || ''
  if (status.includes('active') || status.includes('running')) return 'success'
  if (status.includes('inactive') || status.includes('dead')) return 'danger'
  if (status.includes('failed')) return 'danger'
  return 'warning'
})

function getBarVariant(percent: number) {
  if (percent > 90) return 'danger'
  if (percent > 70) return 'warning'
  return 'success'
}

function formatUptime(seconds: number) {
  const d = Math.floor(seconds / 86400)
  const h = Math.floor((seconds % 86400) / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const parts = []
  if (d > 0) parts.push(`${d}d`)
  if (h > 0) parts.push(`${h}h`)
  if (m > 0) parts.push(`${m}m`)
  return parts.length > 0 ? parts.join(' ') : '< 1m'
}

async function handleInstall() {
  try {
    await systemStore.installService()
    toast.success(t('system.installed_success'))
  } catch (e: any) {
    toast.error(e.response?.data?.error || e.message)
  }
}

async function handleUninstall() {
  if (!await confirm({ title: t('system.uninstall'), message: t('system.uninstall_confirm'), confirmText: t('system.uninstall') })) return
  try {
    await systemStore.uninstallService()
    toast.success(t('system.uninstalled_success'))
  } catch (e: any) {
    toast.error(e.response?.data?.error || e.message)
  }
}

async function handleRestart() {
  try {
    await systemStore.restartService()
    toast.success(t('system.restart_success'))
  } catch (e: any) {
    toast.error(e.response?.data?.error || e.message)
  }
}

async function handleReload() {
  try {
    await systemStore.reloadService()
    toast.success(t('system.reload_success'))
  } catch (e: any) {
    toast.error(e.response?.data?.error || e.message)
  }
}

onMounted(async () => {
  await Promise.all([
    systemStore.loadOS(),
    systemStore.loadServiceStatus(),
  ])

  systemStore.startResourceStream()
})

onUnmounted(() => {
  systemStore.stopResourceStream()
})
</script>

<style scoped lang="scss">
@use '@/assets/scss/variables' as *;

.actions-grid {
  display: flex;
  gap: $spacing-sm;
  flex-wrap: wrap;
}

.service-details {
  padding: $spacing-md;
  background: var(--bg-table-hover);
  border-radius: $border-radius;
  border: 1px solid $border-color;
}

.resource-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
  gap: $spacing-lg;
}
</style>
