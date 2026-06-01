<template>
  <div>
    <div class="docker-grid">
      <!-- Column 1: Docker Host Daemon -->
      <div class="card flex flex-col justify-between">
        <div>
          <h3 class="mb-md">{{ t('docker.title') }}</h3>
          
          <!-- Installed Status Row -->
          <div class="status-row mb-md">
            <span>{{ t('dashboard.version') }}: <code>{{ dockerStore.dockerStatus?.version || '—' }}</code></span>
            <StatusBadge :variant="dockerStore.dockerStatus?.installed ? 'success' : 'danger'">
              {{ dockerStore.dockerStatus?.installed ? t('docker.installed') : t('docker.not_installed') }}
            </StatusBadge>
          </div>

          <!-- Install Button (if not installed) -->
          <button v-if="!dockerStore.dockerStatus?.installed" class="btn btn-primary mb-md"
                  :disabled="dockerStore.loading" @click="dockerStore.installDocker()">
            <Loader2 v-if="dockerStore.loading" :size="16" class="animate-spin" />
            {{ dockerStore.loading ? t('docker.installing') : t('docker.install') }}
          </button>

          <!-- Divider & Updates Section -->
          <template v-if="dockerStore.dockerStatus?.installed">
            <hr class="divider" />
            
            <h4 class="mb-md text-muted uppercase text-xs tracking-wider font-semibold">{{ t('docker.host_updates_title') }}</h4>

            <!-- Live-restore Info -->
            <div v-if="dockerStore.hostUpdateStatus && !dockerStore.hostUpdateStatus.updating" class="mb-md">
              <div v-if="dockerStore.hostUpdateStatus.live_restore_enabled" class="alert alert-success py-sm text-sm">
                {{ t('docker.live_restore_enabled') }}
              </div>
              <div v-else class="alert alert-warning py-sm text-sm">
                {{ t('docker.live_restore_disabled') }}
              </div>
            </div>

            <!-- Updating in progress alert -->
            <div v-if="dockerStore.hostUpdateStatus?.updating" class="alert alert-info mb-md">
              <Loader2 :size="16" class="animate-spin inline-icon" />
              {{ t('docker.docker_updating_to') }}
            </div>

            <!-- Version Status Row -->
            <div v-if="dockerStore.hostUpdateStatus && !dockerStore.hostUpdateStatus.updating" class="status-row mb-md">
              <span>{{ t('docker.current') }}: <code>{{ dockerStore.hostUpdateStatus.current_version || '—' }}</code></span>
              <span v-if="dockerStore.hostUpdateStatus.latest_version">
                {{ t('docker.latest') }}: <code>{{ dockerStore.hostUpdateStatus.latest_version }}</code>
              </span>
              <StatusBadge v-if="dockerStore.hostUpdateStatus.latest_version"
                           :variant="dockerStore.hostUpdateStatus.update_available ? 'warning' : 'success'">
                {{ dockerStore.hostUpdateStatus.update_available ? t('docker.update_available') : t('docker.up_to_date') }}
              </StatusBadge>
            </div>

            <!-- Changelog Link -->
            <div v-if="dockerStore.hostUpdateStatus?.update_available && dockerStore.hostUpdateStatus?.changelog_url" class="update-links mb-md">
              <a :href="dockerStore.hostUpdateStatus.changelog_url" target="_blank" rel="noopener" class="update-link">
                <ExternalLink :size="14" /> {{ t('updates.release_notes') }}
              </a>
            </div>

            <!-- Loading fallback -->
            <div v-if="!dockerStore.hostUpdateStatus" class="text-muted mb-md">{{ t('common.loading') }}</div>
          </template>
        </div>

        <!-- Action Buttons Footer -->
        <div v-if="dockerStore.dockerStatus?.installed" class="mt-lg pt-md">
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

      <!-- Column 2: Telemt Proxy Engine -->
      <div class="card flex flex-col justify-between">
        <div>
          <h3 class="mb-md">{{ t('docker.engine_title') }}</h3>

          <!-- Image & Version Status Row -->
          <div class="status-row mb-md">
            <span>{{ t('dashboard.version') }}: <code>{{ dockerStore.engineStatus?.version || '—' }}</code></span>
            <StatusBadge :variant="dockerStore.engineStatus?.image_exists ? 'success' : 'warning'">
              {{ dockerStore.engineStatus?.image_exists ? t('docker.image_ready') : t('docker.no_image') }}
            </StatusBadge>
          </div>

          <!-- Build / Pull Actions -->
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

          <!-- Divider & Updates Section -->
          <template v-if="dockerStore.engineStatus?.image_exists">
            <hr class="divider" />

            <h4 class="mb-md text-muted uppercase text-xs tracking-wider font-semibold">{{ t('docker.engine_updates') }}</h4>

            <!-- Updating in progress alert -->
            <div v-if="dockerStore.telemtUpdateStatus?.updating" class="alert alert-info mb-md">
              <Loader2 :size="16" class="animate-spin inline-icon" />
              {{ t('docker.updating_to', { version: dockerStore.telemtUpdateStatus.updating_to }) }}
            </div>

            <!-- Version Status Row -->
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

            <!-- Changelog Link -->
            <div v-if="dockerStore.telemtUpdateStatus?.update_available && dockerStore.telemtUpdateStatus?.latest?.html_url" class="update-links mb-md">
              <a :href="dockerStore.telemtUpdateStatus.latest.html_url" target="_blank" rel="noopener" class="update-link">
                <ExternalLink :size="14" /> {{ t('updates.release_notes') }}
              </a>
            </div>

            <!-- Loading fallback -->
            <div v-if="!dockerStore.telemtUpdateStatus" class="text-muted mb-md">{{ t('common.loading') }}</div>

            <!-- Alternate Releases Selector Dropdown -->
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

        <!-- Action Buttons Footer -->
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
import {onBeforeUnmount, onMounted} from 'vue'
import {useI18n} from 'vue-i18n'
import {useDockerStore} from '@/stores'
import {useConfirmDialog} from '@/composables/useConfirmDialog'
import {ExternalLink, Loader2} from '@lucide/vue'
import StatusBadge from '@/components/common/StatusBadge.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'

const { t } = useI18n()
const dockerStore = useDockerStore()

const { confirmState, confirm, handleConfirm, handleCancel } = useConfirmDialog()

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
  dockerStore.loadDockerStatus()
  dockerStore.loadEngineStatus()
  dockerStore.loadTelemtUpdateStatus()
  dockerStore.loadHostUpdateStatus()
  dockerStore.loadReleases()
})

onBeforeUnmount(() => {
  dockerStore.stopUpdateStream()
})
</script>

<style scoped lang="scss">
@use '@/assets/scss/variables' as *;

.inline-icon {
  display: inline-block;
  vertical-align: middle;
  margin-right: 0.25rem;
}
.btn-group-wrap { display: flex; gap: $spacing-sm; flex-wrap: wrap; }

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
</style>
