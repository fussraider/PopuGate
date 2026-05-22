<template>
  <div>
    <!-- Docker + Engine side-by-side -->
    <div class="docker-grid">
      <!-- Docker Status -->
      <div class="card">
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
      <div class="card">
        <h3 class="mb-md">{{ t('docker.engine_title') }}</h3>
        <div class="status-row mb-md">
          <span>{{ t('dashboard.version') }}: <code>{{ dockerStore.engineStatus?.version || '—' }}</code></span>
          <StatusBadge :variant="dockerStore.engineStatus?.image_exists ? 'success' : 'warning'">
            {{ dockerStore.engineStatus?.image_exists ? t('docker.image_ready') : t('docker.no_image') }}
          </StatusBadge>
        </div>
        <div class="btn-group-wrap">
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
    </div>

    <!-- Engine Updates -->
    <div class="card mb-lg">
      <h3 class="mb-md">{{ t('docker.engine_updates') }}</h3>

      <!-- Updating in progress (from server state, survives page refresh) -->
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
      <div class="flex gap-sm">
        <button class="btn btn-secondary" :disabled="dockerStore.checkingRemote || dockerStore.telemtUpdateStatus?.updating" @click="dockerStore.checkRemoteTelemt()">
          <Loader2 v-if="dockerStore.checkingRemote" :size="16" class="animate-spin" />
          {{ dockerStore.checkingRemote ? t('docker.checking') : t('docker.check_updates') }}
        </button>
        <button v-if="dockerStore.telemtUpdateStatus?.update_available && dockerStore.telemtUpdateStatus?.latest && !dockerStore.telemtUpdateStatus?.updating"
                class="btn btn-warning"
                :disabled="dockerStore.applyingUpdate"
                @click="handleEngineUpdate">
          <Loader2 v-if="dockerStore.applyingUpdate" :size="16" class="animate-spin" />
          {{ dockerStore.applyingUpdate ? t('docker.updating') : t('docker.update_engine') }}
        </button>
      </div>
      <div v-if="dockerStore.telemtUpdateStatus?.last_checked" class="text-muted text-sm mt-md">
        {{ t('docker.last_checked') }}: {{ formatDate(dockerStore.telemtUpdateStatus.last_checked) }}
      </div>

      <!-- Releases dropdown -->
      <div v-if="dockerStore.releases.length > 0" class="mt-md">
        <label class="text-sm text-muted">{{ t('docker.select_release') }}</label>
        <div class="flex gap-sm mt-sm">
          <select v-model="dockerStore.selectedRelease" class="select flex-1">
            <option :value="null" disabled>{{ t('docker.choose_release') }}</option>
            <option v-for="r in dockerStore.releases" :key="r.version" :value="r">
              {{ r.tag_name }} ({{ r.commit?.substring(0, 7) }})
            </option>
          </select>
          <button class="btn btn-secondary"
                  :disabled="!dockerStore.selectedRelease || dockerStore.applyingUpdate || dockerStore.telemtUpdateStatus?.updating"
                  @click="handleSelectedRelease">
            {{ t('docker.build_selected') }}
          </button>
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

function formatDate(unixTimestamp: string): string {
  const d = new Date(parseInt(unixTimestamp) * 1000)
  return d.toLocaleString()
}

onMounted(() => {
  dockerStore.loadDockerStatus()
  dockerStore.loadEngineStatus()
  dockerStore.loadTelemtUpdateStatus()
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

  @media (max-width: 768px) {
    grid-template-columns: 1fr;
  }
}
</style>
