<template>
  <div>
    <PageHeader>
      <button class="btn btn-primary" @click="openAddModal">+ {{ t('secrets.add_secret') }}</button>
    </PageHeader>

    <LoadingSpinner v-if="secretsStore.loading" :message="t('secrets.loading')" />

    <EmptyState v-else-if="!(secretsStore.secrets?.length)" :icon="KeyRound"
                :message="t('secrets.empty')" />

    <div v-else class="table-wrapper">
      <table class="table">
        <thead>
          <tr>
            <th>{{ t('secrets.table.label') }}</th>
            <th>{{ t('secrets.table.status') }}</th>
            <th>{{ t('secrets.table.traffic') }}</th>
            <th>{{ t('secrets.table.quota') }}</th>
            <th>{{ t('secrets.table.expires') }}</th>
            <th>{{ t('secrets.table.limits') }}</th>
            <th>{{ t('secrets.table.actions') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="sec in secretsStore.secrets" :key="sec.label">
            <td>
              <code>{{ sec.label }}</code>
              <div v-if="sec.notes" class="text-muted text-sm truncate" style="max-width: 200px">{{ sec.notes }}</div>
            </td>
            <td>
              <StatusBadge :variant="sec.enabled ? 'success' : 'danger'">
                {{ sec.enabled ? t('secrets.active') : t('secrets.disabled') }}
              </StatusBadge>
            </td>
            <td>
              ↓{{ formatBytes(sec.traffic_in || 0) }}<br />
              ↑{{ formatBytes(sec.traffic_out || 0) }}
            </td>
            <td>
              <template v-if="sec.quota_bytes > 0">
                {{ formatBytes(sec.quota_bytes) }}
                <div class="quota-bar">
                  <div class="quota-fill" :style="{ width: Math.min(quotaPercent(sec), 100) + '%' }"
                       :class="{ 'quota-warn': quotaPercent(sec) >= 80, 'quota-over': quotaPercent(sec) >= 100 }" />
                </div>
              </template>
              <span v-else class="text-muted">{{ t('secrets.unlimited') }}</span>
            </td>
            <td>{{ formatISODate(sec.expires_at) }}</td>
            <td>
              <span class="text-sm">{{ sec.max_conns || '∞' }} {{ t('secrets.conns') }}</span><br />
              <span class="text-sm">{{ sec.max_ips || '∞' }} {{ t('secrets.ips') }}</span>
            </td>
            <td>
              <div class="actions-cell">
                <button class="btn btn-ghost btn-sm" :title="t('secrets.rotate')"
                        :disabled="secretsStore.rotating === sec.label" @click="confirmRotate(sec.label)">
                  <Loader2 v-if="secretsStore.rotating === sec.label" :size="16" class="animate-spin" />
                  <RotateCw v-else :size="16" />
                </button>
                <button class="btn btn-ghost btn-sm" :title="t('secrets.limits_title')" @click="openLimitsModal(sec)">
                  <Settings :size="16" />
                </button>
                <button class="btn btn-ghost btn-sm" :title="t('secrets.qr')" @click="showQR(sec.label)">
                  <QrCode :size="16" />
                </button>
                <button class="btn btn-ghost btn-sm" :title="sec.enabled ? t('secrets.disable') : t('secrets.enable')"
                        :disabled="secretsStore.toggling === sec.label" @click="secretsStore.toggle(sec.label, !sec.enabled)">
                  <Loader2 v-if="secretsStore.toggling === sec.label" :size="16" class="animate-spin" />
                  <component v-else :is="sec.enabled ? Pause : Play" :size="16" />
                </button>
                <button class="btn btn-ghost btn-sm btn-danger-text" :title="t('secrets.delete')" @click="confirmRemove(sec.label)">
                  <Trash2 :size="16" />
                </button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Add Secret Modal -->
    <Modal v-model="addModal" :title="t('secrets.add_title')">
      <form @submit.prevent="handleAdd">
        <div class="form-group mb-md">
          <label class="form-label">{{ t('secrets.table.label') }}</label>
          <input v-model="addForm.label" class="input" :placeholder="t('secrets.user_placeholder')" required />
        </div>
        <div class="form-group mb-md">
          <label class="form-label">{{ t('secrets.secret_key') }} <span class="text-muted">{{ t('secrets.optional_auto') }}</span></label>
          <input v-model="addForm.secret" class="input" :placeholder="t('secrets.hex_placeholder')" maxlength="32" />
        </div>
        <div class="modal-footer-inline">
          <button type="button" class="btn btn-secondary" @click="addModal = false">{{ t('common.cancel') }}</button>
          <button type="submit" class="btn btn-primary" :disabled="adding">{{ t('common.add') }}</button>
        </div>
      </form>
    </Modal>

    <!-- Limits Modal -->
    <Modal v-model="limitsModal" :title="t('secrets.set_limits_title')">
      <form @submit.prevent="handleSetLimits">
        <div class="form-row mb-sm">
          <div class="form-group">
            <label class="form-label">{{ t('secrets.max_conns') }}</label>
            <input v-model.number="limitsForm.maxConns" class="input" type="number" min="0" :placeholder="t('secrets.unlimited_placeholder')" />
          </div>
          <div class="form-group">
            <label class="form-label">{{ t('secrets.max_ips') }}</label>
            <input v-model.number="limitsForm.maxIPs" class="input" type="number" min="0" :placeholder="t('secrets.unlimited_placeholder')" />
          </div>
        </div>
        <div class="form-row mb-sm">
          <div class="form-group">
            <label class="form-label">{{ t('secrets.quota_mb') }}</label>
            <input v-model.number="limitsForm.quotaMB" class="input" type="number" min="0" :placeholder="t('secrets.unlimited_placeholder')" />
          </div>
          <div class="form-group">
            <label class="form-label">{{ t('secrets.table.expires') }}</label>
            <input v-model="limitsForm.expiresAt" class="input" type="date" />
          </div>
        </div>
        <div class="modal-footer-inline">
          <button type="button" class="btn btn-secondary" @click="limitsModal = false">{{ t('common.cancel') }}</button>
          <button type="submit" class="btn btn-primary">{{ t('common.save') }}</button>
        </div>
      </form>
    </Modal>

    <!-- Confirm Remove -->
    <ConfirmDialog v-model="confirmModal" :title="t('secrets.remove_title')"
                   :message="t('secrets.confirm_remove', { label: removeTarget })"
                   :confirm-text="t('common.delete')" @confirm="handleRemove" />

    <!-- Confirm Rotate -->
    <ConfirmDialog v-model="rotateConfirmModal" :title="t('secrets.rotate_title')"
                   :message="t('secrets.confirm_rotate', { label: rotateTarget })"
                   :confirm-text="t('secrets.rotate')" @confirm="handleRotate" />

    <!-- QR Modal -->
    <Modal v-model="qrModal" :title="t('secrets.connect_title', { label: qrLabel })">
      <div class="qr-container text-center">
        <img v-if="qrImage" :src="qrImage" alt="QR Code" class="qr-image" />
        <p class="text-muted mt-sm mb-md">{{ t('secrets.scan_tip') }}</p>

        <div class="links-section text-left">
          <div class="form-group mb-sm">
            <label class="form-label text-xs">{{ t('secrets.tg_link') }}</label>
            <div class="input-group">
              <input :value="tgLink" class="input input-sm" readonly />
              <button class="btn btn-secondary btn-sm" @click="copyToClipboard(tgLink)">{{ t('secrets.copy') }}</button>
            </div>
          </div>
          <div class="form-group">
            <label class="form-label text-xs">{{ t('secrets.web_link') }}</label>
            <div class="input-group">
              <input :value="webLink" class="input input-sm" readonly />
              <button class="btn btn-secondary btn-sm" @click="copyToClipboard(webLink)">{{ t('secrets.copy') }}</button>
            </div>
          </div>
        </div>
      </div>
    </Modal>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useSecretsStore } from '@/stores'
import { useToastStore } from '@/stores/toast'
import { secretsApi } from '@/api/endpoints'
import { formatBytes, formatISODate } from '@/utils/format'
import Modal from '@/components/common/Modal.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import StatusBadge from '@/components/common/StatusBadge.vue'
import PageHeader from '@/components/common/PageHeader.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import { KeyRound, RotateCw, Settings, QrCode, Play, Pause, Trash2, Loader2 } from '@lucide/vue'

const { t } = useI18n()
const secretsStore = useSecretsStore()
const toast = useToastStore()

// Add
const addModal = ref(false)
const adding = ref(false)
const addForm = ref({ label: '', secret: '' })

function openAddModal() { addForm.value = { label: '', secret: '' }; addModal.value = true }

async function handleAdd() {
  adding.value = true
  try {
    await secretsStore.add(addForm.value.label, addForm.value.secret || undefined)
    addModal.value = false
    toast.success(t('secrets.added_success', { label: addForm.value.label }))
  } catch (e: any) {
    toast.error(e.response?.data?.error ?? e.message)
  } finally {
    adding.value = false
  }
}

// Remove
const confirmModal = ref(false)
const removeTarget = ref('')

function confirmRemove(label: string) { removeTarget.value = label; confirmModal.value = true }

async function handleRemove() {
  try {
    await secretsStore.remove(removeTarget.value)
    confirmModal.value = false
    toast.success(t('secrets.removed_success', { label: removeTarget.value }))
  } catch (e: any) {
    toast.error(e.response?.data?.error ?? e.message)
  }
}

// Rotate
const rotateConfirmModal = ref(false)
const rotateTarget = ref('')

function confirmRotate(label: string) {
  rotateTarget.value = label
  rotateConfirmModal.value = true
}

async function handleRotate() {
  try {
    await secretsStore.rotate(rotateTarget.value)
    rotateConfirmModal.value = false
    toast.success(t('secrets.rotated_success', { label: rotateTarget.value }))
  } catch (e: any) {
    toast.error(e.response?.data?.error ?? e.message)
  }
}

// Limits
const limitsModal = ref(false)
const limitsTarget = ref('')
const limitsForm = ref({ maxConns: 0, maxIPs: 0, quotaMB: 0, expiresAt: '' })

function openLimitsModal(sec: any) {
  limitsTarget.value = sec.label
  limitsForm.value = {
    maxConns: sec.max_conns,
    maxIPs: sec.max_ips,
    quotaMB: sec.quota_bytes ? Math.round(sec.quota_bytes / (1024 * 1024)) : 0,
    expiresAt: (sec.expires_at && sec.expires_at !== '0') ? sec.expires_at.split('T')[0] : '',
  }
  limitsModal.value = true
}

async function handleSetLimits() {
  try {
    await secretsStore.setLimits(
      limitsTarget.value,
      limitsForm.value.maxConns,
      limitsForm.value.maxIPs,
      limitsForm.value.quotaMB * 1024 * 1024,
      limitsForm.value.expiresAt || '0',
    )
    limitsModal.value = false
  } catch (e: any) {
    toast.error(e.response?.data?.error ?? e.message)
  }
}

// QR
const qrModal = ref(false)
const qrLabel = ref('')
const qrImage = ref('')
const tgLink = ref('')
const webLink = ref('')

async function showQR(label: string) {
  qrLabel.value = label
  if (qrImage.value) URL.revokeObjectURL(qrImage.value)
  qrImage.value = ''
  tgLink.value = ''
  webLink.value = ''
  try {
    const [blob, linkData] = await Promise.all([
      secretsApi.getQR(label),
      secretsApi.getLink(label),
    ])
    qrImage.value = URL.createObjectURL(blob)
    tgLink.value = linkData.tg_link || ''
    webLink.value = linkData.web_link || ''
    qrModal.value = true
  } catch (e: any) {
    toast.error(t('secrets.load_failed', { label }))
  }
}

async function copyToClipboard(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    toast.success(t('secrets.copied'))
  } catch (e) {
    toast.error(t('secrets.copy_failed'))
  }
}

function quotaPercent(sec: any): number {
  if (!sec.quota_bytes) return 0
  return ((sec.traffic_in || 0) + (sec.traffic_out || 0)) / sec.quota_bytes * 100
}

onMounted(() => secretsStore.load())
</script>

<style scoped lang="scss">
@use '@/assets/scss/variables' as *;

.quota-bar {
  height: 4px;
  background: $color-gray-200;
  border-radius: 2px;
  margin-top: 4px;
}
.quota-fill { height: 100%; border-radius: 2px; background: $color-success; transition: width 0.3s; }
.quota-warn { background: $color-warning; }
.quota-over { background: $color-danger; }

.qr-image { max-width: 256px; border-radius: $border-radius; }

.links-section {
  margin-top: 1rem;
  padding-top: 1rem;
  border-top: 1px solid $color-gray-200;
}

.input-group {
  display: flex;
  gap: 0.5rem;

  input {
    flex: 1;
    font-family: monospace;
  }
}
</style>
