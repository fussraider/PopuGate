<template>
  <div class="system-view">
    <div class="system-grid">
      <!-- 1. System Resources Card -->
      <div class="card flex flex-col justify-between">
        <div>
          <h3 class="mb-md">{{ t('system.resources_title') }}</h3>
          <div v-if="systemStore.resources">
            <div class="resource-grid-compact mb-md">
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
                    :class="getBarVariant(safePercent(systemStore.resources.memory_used, systemStore.resources.memory_total))"
                    :style="{ width: safePercent(systemStore.resources.memory_used, systemStore.resources.memory_total) + '%' }"
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
                    :class="getBarVariant(safePercent(systemStore.resources.disk_used, systemStore.resources.disk_total))"
                    :style="{ width: safePercent(systemStore.resources.disk_used, systemStore.resources.disk_total) + '%' }"
                  ></div>
                </div>
              </div>
            </div>
          </div>
          <div v-else class="text-muted mb-md">{{ t('system.loading') }}</div>
        </div>

        <div v-if="systemStore.resources" class="mt-md pt-sm">
          <InfoGrid>
            <InfoItem :label="t('system.load_avg')">
              <span class="load-avg-values">
                <span v-tooltip="t('system.load_1m_tip')">{{ systemStore.resources.load1.toFixed(2) }}</span>
                <span v-tooltip="t('system.load_5m_tip')">{{ systemStore.resources.load5.toFixed(2) }}</span>
                <span v-tooltip="t('system.load_15m_tip')">{{ systemStore.resources.load15.toFixed(2) }}</span>
              </span>
            </InfoItem>
            <InfoItem :label="t('system.uptime')">
              <span>{{ formatUptime(systemStore.resources.uptime) }}</span>
            </InfoItem>
          </InfoGrid>
        </div>
      </div>

      <!-- 2. OS Information & Service Management Card -->
      <div class="card flex flex-col justify-between">
        <div>
          <h3 class="mb-md">{{ t('system.title') }}</h3>
          <div v-if="systemStore.os" class="mb-md">
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
          <div v-else class="text-muted mb-md">{{ t('system.loading') }}</div>

          <hr class="divider" />

          <div class="flex justify-between items-center mb-md">
            <h3 class="text-md font-semibold">{{ t('system.service_title') }}</h3>
            <StatusBadge :variant="serviceStatusVariant">
              {{ systemStore.service?.active || t('system.not_installed') }}
            </StatusBadge>
          </div>

          <div v-if="systemStore.service" class="service-details mb-md">
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
        </div>

        <div v-if="systemStore.service?.supported" class="mt-md pt-sm">
          <div class="flex gap-sm">
            <template v-if="!systemStore.service?.installed">
              <button
                class="btn btn-primary btn-sm"
                :disabled="systemStore.loading"
                @click="handleInstall"
              >
                <Loader2 v-if="systemStore.loading" :size="16" class="animate-spin" />
                {{ systemStore.loading ? t('system.installing') : t('system.install') }}
              </button>
            </template>

            <template v-else>
              <button
                class="btn btn-warning btn-sm"
                :disabled="systemStore.loading"
                @click="handleRestart"
              >
                <Loader2 v-if="systemStore.loading" :size="16" class="animate-spin" />
                {{ systemStore.loading ? t('system.restarting') : t('system.restart') }}
              </button>

              <button
                class="btn btn-ghost btn-sm"
                :disabled="systemStore.loading"
                @click="handleReload"
              >
                <Loader2 v-if="systemStore.loading" :size="16" class="animate-spin" />
                {{ systemStore.loading ? t('system.reloading') : t('system.reload') }}
              </button>

              <button
                class="btn btn-danger btn-sm"
                :disabled="systemStore.loading"
                @click="handleUninstall"
              >
                <Loader2 v-if="systemStore.loading" :size="16" class="animate-spin" />
                {{ systemStore.loading ? t('system.uninstalling') : t('system.uninstall') }}
              </button>
            </template>
          </div>

          <div v-if="systemStore.service?.installed" class="text-muted text-xs mt-sm">
            {{ t('system.note') }}
          </div>
        </div>
        <div v-else-if="systemStore.service" class="mt-md text-warning text-xs">
          {{ t('system.unsupported') }}
        </div>
      </div>
    </div>

    <!-- 4. PopuGate Updates -->
    <div class="card mb-lg">
      <div class="flex items-center gap-md mb-md">
        <h3>{{ t('updates.section_title', 'Обновления PopuGate') }}</h3>
        <StatusBadge v-if="updateStore.status" :variant="updateStore.status.mode === 'docker' ? 'info' : 'neutral'">
          {{ updateStore.status.mode === 'docker' ? t('updates.mode_docker') : t('updates.mode_binary') }}
        </StatusBadge>
      </div>

      <div class="status-row mb-md">
        <span>{{ t('updates.current') }}: <code>v{{ updateStore.status?.current || '—' }}</code></span>
        <span>{{ t('updates.latest') }}: <code>v{{ updateStore.status?.latest || '—' }}</code></span>
        <StatusBadge v-if="updateStore.status" :variant="updateStore.status.update_available ? 'warning' : 'success'">
          {{ updateStore.status.update_available ? t('updates.available') : t('updates.up_to_date') }}
        </StatusBadge>
      </div>

      <div v-if="updateStore.status?.update_available" class="update-links mb-md">
        <a v-if="updateStore.status.url" :href="updateStore.status.url" target="_blank" rel="noopener" class="update-link">
          <ExternalLink :size="14" /> {{ t('updates.release_notes') }}
        </a>
        <a href="https://github.com/fussraider/PopuGate/blob/master/CHANGELOG.md" target="_blank" rel="noopener" class="update-link">
          <FileText :size="14" /> {{ t('updates.changelog') }}
        </a>
      </div>

      <div class="flex gap-sm">
        <button class="btn btn-primary btn-sm" :disabled="updateStore.loading" @click="updateStore.check()">
          <Loader2 v-if="updateStore.loading" :size="16" class="animate-spin" />
          {{ updateStore.loading ? t('updates.checking') : t('updates.check_btn') }}
        </button>
        <button v-if="updateStore.status?.update_available" class="btn btn-warning btn-sm"
                :disabled="updateStore.applying || updateStore.restarting" @click="handleApply">
          <Loader2 v-if="updateStore.applying || updateStore.restarting" :size="16" class="animate-spin" />
          {{ updateStore.restarting ? t('updates.restarting') : (updateStore.applying ? t('updates.applying') : t('updates.apply')) }}
        </button>
      </div>

      <div v-if="updateStore.error" class="alert alert-danger mt-md">{{ updateStore.error }}</div>

      <div v-if="updateStore.result && updateStore.status?.mode === 'docker'" class="alert alert-success mt-md">
        {{ t('updates.success_docker', { old: updateStore.result.previous_version, new: updateStore.result.new_version }) }}
      </div>
      <div v-if="updateStore.result && updateStore.status?.mode === 'binary'" class="alert alert-success mt-md">
        {{ t('updates.success', { old: updateStore.result.previous_version, new: updateStore.result.new_version }) }}
      </div>

      <!-- Auto-Update Subsection -->
      <hr class="divider" />
      <div class="auto-update-section">
        <div class="flex justify-between items-center mb-sm">
          <h4 class="font-semibold">{{ t('updates.auto_update_title', 'Автообновление') }}</h4>
          <StatusBadge :variant="autoUpdateTask?.enabled ? 'success' : 'neutral'">
            {{ autoUpdateTask?.enabled ? t('updates.auto_update_enabled', 'Включено') : t('updates.auto_update_disabled', 'Отключено') }}
          </StatusBadge>
        </div>

        <p class="text-sm text-muted mb-sm">
          {{ t('updates.auto_update_desc', 'При включении PopuGate будет автоматически скачивать и применять обновления по расписанию. Telegram-уведомления будут отправляться, если бот настроен.') }}
        </p>

        <div v-if="autoUpdateTask" class="mb-sm text-sm">
          <span class="text-muted">{{ t('scheduler.table.schedule') }}:</span>
          <code class="ml-xs">{{ autoUpdateTask.effective_schedule || autoUpdateTask.default_schedule }}</code>
        </div>

        <div class="flex gap-sm items-center flex-wrap">
          <button
            class="btn btn-sm"
            :class="autoUpdateTask?.enabled ? 'btn-danger' : 'btn-primary'"
            :disabled="schedulerStore.toggling === 'auto-update'"
            @click="toggleAutoUpdate"
          >
            <Loader2 v-if="schedulerStore.toggling === 'auto-update'" :size="14" class="animate-spin" />
            {{ autoUpdateTask?.enabled ? t('updates.auto_update_disable', 'Отключить автообновление') : t('updates.auto_update_enable', 'Включить автообновление') }}
          </button>
          
          <button
            v-if="autoUpdateTask"
            class="btn btn-secondary btn-sm"
            :disabled="schedulerStore.running === 'auto-update'"
            @click="runAutoUpdateNow"
          >
            <Loader2 v-if="schedulerStore.running === 'auto-update'" :size="14" class="animate-spin" />
            {{ t('updates.auto_update_run_now', 'Запустить сейчас') }}
          </button>

          <RouterLink to="/scheduler" class="text-sm text-muted underline">
            {{ t('updates.auto_update_scheduler_link', 'Изменить расписание в Планировщике →') }}
          </RouterLink>
        </div>

        <div class="alert alert-warning mt-md text-sm">
          {{ t('updates.auto_update_warning', '⚠️ Автообновление перезапустит сервис. Включайте только если понимаете последствия.') }}
        </div>
      </div>
    </div>

    <!-- 5. Docker daemon and Telemt Grid -->
    <div class="docker-grid">
      <!-- Docker Host Daemon -->
      <div class="card flex flex-col justify-between">
        <div>
          <h3 class="mb-md">{{ t('docker.title') }}</h3>
          
          <div class="status-row mb-md">
            <span>{{ t('dashboard.version') }}: <code>{{ dockerStore.dockerStatus?.version || '—' }}</code></span>
            <StatusBadge :variant="dockerStore.dockerStatus?.installed ? 'success' : 'danger'">
              {{ dockerStore.dockerStatus?.installed ? t('docker.installed') : t('docker.not_installed') }}
            </StatusBadge>
          </div>

          <button v-if="!dockerStore.dockerStatus?.installed" class="btn btn-primary mb-md"
                  :disabled="dockerStore.loading" @click="dockerStore.installDocker()">
            <Loader2 v-if="dockerStore.loading" :size="16" class="animate-spin" />
            {{ dockerStore.loading ? t('docker.installing') : t('docker.install') }}
          </button>

          <template v-if="dockerStore.dockerStatus?.installed">
            <hr class="divider" />
            
            <h4 class="mb-md text-muted uppercase text-xs tracking-wider font-semibold">{{ t('docker.host_updates_title') }}</h4>

            <div v-if="dockerStore.hostUpdateStatus && !dockerStore.hostUpdateStatus.supported" class="alert alert-warning mb-md text-sm">
              {{ t('docker.host_updates_not_supported', 'Обновление Docker Engine недоступно при запуске PopuGate в контейнере. Пожалуйста, обновите Docker на хосте вручную.') }}
            </div>

            <div v-if="dockerStore.hostUpdateStatus && !dockerStore.hostUpdateStatus.updating" class="mb-md">
              <div v-if="dockerStore.hostUpdateStatus.live_restore_enabled" class="alert alert-success py-sm text-sm">
                {{ t('docker.live_restore_enabled') }}
              </div>
              <div v-else class="alert alert-warning py-sm text-sm">
                {{ t('docker.live_restore_disabled') }}
              </div>
            </div>

            <div v-if="dockerStore.hostUpdateStatus?.updating" class="alert alert-info mb-md">
              <Loader2 :size="16" class="animate-spin inline-icon" />
              {{ t('docker.docker_updating_to') }}
            </div>

            <div v-if="dockerStore.hostUpdateStatus && !dockerStore.hostUpdateStatus.updating" class="status-row mb-md">
              <span>{{ t('docker.current') }}: <code>{{ dockerStore.hostUpdateStatus.current_version || '—' }}</code></span>
              <span v-if="dockerStore.hostUpdateStatus.supported && dockerStore.hostUpdateStatus.latest_version">
                {{ t('docker.latest') }}: <code>{{ dockerStore.hostUpdateStatus.latest_version }}</code>
              </span>
              <StatusBadge v-if="dockerStore.hostUpdateStatus.supported && dockerStore.hostUpdateStatus.latest_version"
                           :variant="dockerStore.hostUpdateStatus.update_available ? 'warning' : 'success'">
                {{ dockerStore.hostUpdateStatus.update_available ? t('docker.update_available') : t('docker.up_to_date') }}
              </StatusBadge>
            </div>

            <div v-if="dockerStore.hostUpdateStatus?.supported && dockerStore.hostUpdateStatus?.update_available && dockerStore.hostUpdateStatus?.changelog_url" class="update-links mb-md">
              <a :href="dockerStore.hostUpdateStatus.changelog_url" target="_blank" rel="noopener" class="update-link">
                <ExternalLink :size="14" /> {{ t('updates.release_notes') }}
              </a>
            </div>

            <div v-if="!dockerStore.hostUpdateStatus" class="text-muted mb-md">{{ t('common.loading') }}</div>
          </template>
        </div>

        <div v-if="dockerStore.dockerStatus?.installed && dockerStore.hostUpdateStatus?.supported !== false" class="mt-lg pt-md">
          <div class="flex gap-sm">
            <button class="btn btn-secondary btn-sm" :disabled="dockerStore.checkingHostRemote || dockerStore.hostUpdateStatus?.updating" @click="dockerStore.checkHostRemote()">
              <Loader2 v-if="dockerStore.checkingHostRemote" :size="16" class="animate-spin" />
              {{ dockerStore.checkingHostRemote ? t('docker.checking') : t('docker.check_updates') }}
            </button>
            <button v-if="dockerStore.hostUpdateStatus?.update_available && !dockerStore.hostUpdateStatus?.updating"
                    class="btn btn-warning btn-sm"
                    :disabled="dockerStore.applyingHostUpdate"
                    @click="handleHostUpdate">
              <Loader2 v-if="dockerStore.applyingHostUpdate" :size="16" class="animate-spin" />
              {{ dockerStore.applyingHostUpdate ? t('docker.updating') : t('docker.update_docker') }}
            </button>
          </div>
          <div v-if="dockerStore.hostUpdateStatus?.last_checked" class="text-muted text-xs mt-md">
            {{ t('docker.last_checked') }}: {{ formatDate(dockerStore.hostUpdateStatus.last_checked) }}
          </div>
        </div>
      </div>

      <!-- Telemt Proxy Engine -->
      <div class="card flex flex-col justify-between">
        <div>
          <h3 class="mb-md">{{ t('docker.engine_title') }}</h3>

          <div class="status-row mb-md">
            <span>{{ t('dashboard.version') }}: <code>{{ dockerStore.engineStatus?.version || '—' }}</code></span>
            <StatusBadge :variant="dockerStore.engineStatus?.image_exists ? 'success' : 'warning'">
              {{ dockerStore.engineStatus?.image_exists ? t('docker.image_ready') : t('docker.no_image') }}
            </StatusBadge>
          </div>

          <div class="btn-group-wrap mb-md">
            <button class="btn btn-primary btn-sm" :disabled="dockerStore.building" @click="dockerStore.buildEngine(false)">
              <Loader2 v-if="dockerStore.building" :size="16" class="animate-spin" />
              {{ dockerStore.building ? t('docker.building') : t('docker.build_pull') }}
            </button>
            <button class="btn btn-warning btn-sm" :disabled="dockerStore.building" @click="dockerStore.buildEngine(true)">
              <Loader2 v-if="dockerStore.building" :size="16" class="animate-spin" />
              {{ t('docker.force_rebuild') }}
            </button>
          </div>
          <div v-if="dockerStore.buildResult" class="alert alert-info py-sm text-sm mb-md">{{ dockerStore.buildResult }}</div>

          <template v-if="dockerStore.engineStatus?.image_exists">
            <hr class="divider" />

            <h4 class="mb-md text-muted uppercase text-xs tracking-wider font-semibold">{{ t('docker.engine_updates') }}</h4>

            <div v-if="dockerStore.telemtUpdateStatus?.updating" class="alert alert-info mb-md">
              <Loader2 :size="16" class="animate-spin inline-icon" />
              {{ t('docker.updating_to', { version: dockerStore.telemtUpdateStatus.updating_to }) }}
            </div>

            <div v-if="dockerStore.telemtUpdateStatus && !dockerStore.telemtUpdateStatus.updating" class="status-row mb-md">
              <span>{{ t('docker.current') }}: <code>{{ dockerStore.telemtUpdateStatus.current || '—' }}</code></span>
              <span v-if="dockerStore.telemtUpdateStatus.latest">
                {{ t('docker.latest') }}: <code>{{ dockerStore.telemtUpdateStatus.latest.version }}</code>
              </span>
              <StatusBadge v-if="dockerStore.telemtUpdateStatus.latest"
                           :variant="dockerStore.telemtUpdateStatus.update_available ? 'warning' : 'success'">
                {{ dockerStore.telemtUpdateStatus.update_available ? t('docker.update_available') : t('docker.up_to_date') }}
              </StatusBadge>
            </div>

            <div v-if="dockerStore.telemtUpdateStatus?.update_available && dockerStore.telemtUpdateStatus?.latest?.html_url" class="update-links mb-md">
              <a :href="dockerStore.telemtUpdateStatus.latest.html_url" target="_blank" rel="noopener" class="update-link">
                <ExternalLink :size="14" /> {{ t('updates.release_notes') }}
              </a>
            </div>

            <div v-if="!dockerStore.telemtUpdateStatus" class="text-muted mb-md">{{ t('common.loading') }}</div>

            <div v-if="dockerStore.releases.length > 0 && !dockerStore.telemtUpdateStatus?.updating" class="mt-md">
              <label class="text-xs text-muted font-semibold block mb-sm">{{ t('docker.select_release') }}</label>
              <div class="flex gap-sm">
                <select v-model="dockerStore.selectedRelease" class="select flex-1 select-sm py-xs">
                  <option :value="null" disabled>{{ t('docker.choose_release') }}</option>
                  <option v-for="r in dockerStore.releases" :key="r.version" :value="r">
                    {{ r.tag_name }} ({{ r.commit?.substring(0, 7) }})
                  </option>
                </select>
                <button class="btn btn-secondary btn-sm"
                        :disabled="!dockerStore.selectedRelease || dockerStore.applyingUpdate || dockerStore.telemtUpdateStatus?.updating"
                        @click="handleSelectedRelease">
                  {{ t('docker.build_selected') }}
                </button>
              </div>
            </div>
          </template>
        </div>

        <div v-if="dockerStore.engineStatus?.image_exists" class="mt-lg pt-md">
          <div class="flex gap-sm">
            <button class="btn btn-secondary btn-sm" :disabled="dockerStore.checkingRemote || dockerStore.telemtUpdateStatus?.updating" @click="dockerStore.checkRemoteTelemt()">
              <Loader2 v-if="dockerStore.checkingRemote" :size="16" class="animate-spin" />
              {{ dockerStore.checkingRemote ? t('docker.checking') : t('docker.check_updates') }}
            </button>
            <button v-if="dockerStore.telemtUpdateStatus?.update_available && dockerStore.telemtUpdateStatus?.latest && !dockerStore.telemtUpdateStatus?.updating"
                    class="btn btn-warning btn-sm"
                    :disabled="dockerStore.applyingUpdate"
                    @click="handleEngineUpdate">
              <Loader2 v-if="dockerStore.applyingUpdate" :size="16" class="animate-spin" />
              {{ dockerStore.applyingUpdate ? t('docker.updating') : t('docker.update_engine') }}
            </button>
          </div>
          <div v-if="dockerStore.telemtUpdateStatus?.last_checked" class="text-muted text-xs mt-md">
            {{ t('docker.last_checked') }}: {{ formatDate(dockerStore.telemtUpdateStatus.last_checked) }}
          </div>
        </div>
      </div>
    </div>

    <ConfirmDialog v-bind="confirmState" @confirm="handleConfirm" @cancel="handleCancel" />
  </div>
</template>

<script setup lang="ts">
import {computed, onMounted, onBeforeUnmount} from 'vue'
import {useI18n} from 'vue-i18n'
import {useSystemStore} from '@/stores/system'
import {useDockerStore} from '@/stores/docker'
import {useUpdateStore} from '@/stores/update'
import {useSchedulerStore} from '@/stores/scheduler'
import {useToastStore} from '@/stores/toast'
import {useConfirmDialog} from '@/composables/useConfirmDialog'
import {ExternalLink, FileText, Loader2} from '@lucide/vue'
import StatusBadge from '@/components/common/StatusBadge.vue'
import InfoGrid from '@/components/common/InfoGrid.vue'
import InfoItem from '@/components/common/InfoItem.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import {formatBytes} from '@/utils/format'

const { t } = useI18n()
const systemStore = useSystemStore()
const dockerStore = useDockerStore()
const updateStore = useUpdateStore()
const schedulerStore = useSchedulerStore()
const toast = useToastStore()

const { confirmState, confirm, handleConfirm, handleCancel } = useConfirmDialog()

// 1. System state computed & helpers
const serviceStatusVariant = computed(() => {
  const status = systemStore.service?.active?.toLowerCase() || ''
  if (status.includes('inactive') || status.includes('dead')) return 'danger'
  if (status.includes('failed')) return 'danger'
  if (status.includes('active') || status.includes('running')) return 'success'
  return 'warning'
})

function getBarVariant(percent: number) {
  if (percent > 90) return 'danger'
  if (percent > 70) return 'warning'
  return 'success'
}

function safePercent(used: number, total: number): number {
  return total > 0 ? (used / total) * 100 : 0
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

// 2. Systemd actions
async function handleInstall() {
  try {
    await systemStore.installService()
    toast.success(t('system.installed_success'))
  } catch {
    // interceptor handles error toast
  }
}

async function handleUninstall() {
  if (!await confirm({ title: t('system.uninstall'), message: t('system.uninstall_confirm'), confirmText: t('system.uninstall') })) return
  try {
    await systemStore.uninstallService()
    toast.success(t('system.uninstalled_success'))
  } catch {
    // interceptor handles error toast
  }
}

async function handleRestart() {
  try {
    await systemStore.restartService()
    toast.success(t('system.restart_success'))
  } catch {
    // interceptor handles error toast
  }
}

async function handleReload() {
  try {
    await systemStore.reloadService()
    toast.success(t('system.reload_success'))
  } catch {
    // interceptor handles error toast
  }
}

// 3. Updates actions
async function handleApply() {
  const msg = updateStore.status?.mode === 'docker'
    ? t('updates.confirm_apply_docker')
    : t('updates.confirm_apply')
  if (!await confirm({ title: t('updates.apply'), message: msg, confirmText: t('updates.apply') })) return
  await updateStore.apply()
}

// 4. Auto-update actions
const autoUpdateTask = computed(() =>
  schedulerStore.tasks.find(t => t.name === 'auto-update') ?? null
)

async function toggleAutoUpdate() {
  const task = autoUpdateTask.value
  if (!task) return
  await schedulerStore.toggle('auto-update', !task.enabled)
  await schedulerStore.load(true) // reload to sync state
}

async function runAutoUpdateNow() {
  if (!await confirm({
    title: t('updates.auto_update_title', 'Автообновление'),
    message: t('updates.confirm_run_now_msg', 'Вы уверены, что хотите запустить задачу автообновления сейчас? При наличии обновления служба будет обновлена и перезапущена.'),
    confirmText: t('updates.run_now_btn', 'Запустить')
  })) return
  
  await schedulerStore.runNow('auto-update')
  toast.success(t('updates.run_now_success', 'Задача автообновления успешно запущена.'))
}

// 5. Docker actions
async function handleEngineUpdate() {
  const latest = dockerStore.telemtUpdateStatus?.latest
  if (!latest) return
  if (!await confirm({ title: t('docker.update_engine'), message: t('docker.confirm_engine_update', { version: latest.version }), confirmText: t('docker.update_engine') })) return
  await dockerStore.applyTelemtUpdate(latest.version, latest.commit || '')
}

async function handleSelectedRelease() {
  const release = dockerStore.selectedRelease
  if (!release) return
  if (!await confirm({ title: t('docker.update_engine'), message: t('docker.confirm_engine_update', { version: release.version }), confirmText: t('docker.build_selected') })) return
  await dockerStore.applyTelemtUpdate(release.version, release.commit || '')
}

async function handleHostUpdate() {
  if (!await confirm({
    title: t('docker.update_docker'),
    message: t('docker.confirm_host_update'),
    confirmText: t('docker.update_docker')
  })) return
  await dockerStore.applyHostUpdate()
}

function formatDate(unixTimestamp: string): string {
  const d = new Date(parseInt(unixTimestamp) * 1000)
  return d.toLocaleString()
}

onMounted(() => {
  systemStore.loadOS().catch(e => console.error('loadOS failed:', e))
  systemStore.loadServiceStatus().catch(e => console.error('loadServiceStatus failed:', e))
  dockerStore.loadDockerStatus().catch(e => console.error('loadDockerStatus failed:', e))
  dockerStore.loadEngineStatus().catch(e => console.error('loadEngineStatus failed:', e))
  dockerStore.loadTelemtUpdateStatus().catch(e => console.error('loadTelemtUpdateStatus failed:', e))
  dockerStore.loadHostUpdateStatus().catch(e => console.error('loadHostUpdateStatus failed:', e))
  dockerStore.loadReleases().catch(e => console.error('loadReleases failed:', e))
  schedulerStore.load(true).catch(e => console.error('schedulerStore.load failed:', e))

  updateStore.check().catch(e => console.error('updateStore.check failed:', e))
  systemStore.startResourceStream()
})

onBeforeUnmount(() => {
  systemStore.stopResourceStream()
  dockerStore.stopUpdateStream()
})
</script>

<style scoped lang="scss">
@use '@/assets/scss/variables' as *;

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

.load-avg-values {
  display: inline-flex;
  gap: $spacing-sm;

  > span {
    padding: 1px 6px;
    background: var(--bg-table-hover);
    border-radius: 3px;
    font-family: monospace;
    cursor: help;
  }
}

.inline-icon {
  display: inline-block;
  vertical-align: middle;
  margin-right: 0.25rem;
}

.btn-group-wrap {
  display: flex;
  gap: $spacing-sm;
  flex-wrap: wrap;
}

.divider {
  border: 0;
  border-top: 1px solid $border-color;
  margin: 1.5rem 0;
}

.update-links {
  display: flex;
  gap: $spacing-md;
}

.update-link {
  display: inline-flex;
  align-items: center;
  gap: $spacing-xs;
  color: var(--color-primary);
  text-decoration: none;
  font-size: $font-size-sm;

  &:hover { text-decoration: underline; }
}

.system-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: $spacing-md;
  margin-bottom: $spacing-lg;
  align-items: stretch;

  @media (max-width: 768px) {
    grid-template-columns: 1fr;
  }
}

.resource-grid-compact {
  display: flex;
  flex-direction: column;
  gap: $spacing-md;
}

.docker-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: $spacing-md;
  margin-bottom: $spacing-lg;
  align-items: stretch;

  @media (max-width: 768px) {
    grid-template-columns: 1fr;
  }
}

.auto-update-section {
  padding-top: $spacing-xs;
}
</style>
