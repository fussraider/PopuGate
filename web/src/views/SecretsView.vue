<template>
  <div>
    <PageHeader>
      <button class="btn btn-primary" @click="addModal.open()">+ {{ t('secrets.add_secret') }}</button>
    </PageHeader>

    <DataTable
      :columns="columns"
      :items="secretsStore.secrets"
      :loading="secretsStore.loading"
      :empty-icon="KeyRound"
      :empty-message="t('secrets.empty')"
      row-key="label"
    >
      <template #cell-label="{ item }">
        <code>{{ item.label }}</code>
        <div v-if="item.notes" class="text-muted text-sm truncate notes-cell">{{ item.notes }}</div>
      </template>
      <template #cell-status="{ item }">
        <StatusBadge :variant="item.enabled ? 'success' : 'danger'">
          {{ item.enabled ? t('secrets.active') : t('secrets.disabled') }}
        </StatusBadge>
      </template>
      <template #cell-traffic="{ item }">
        ↓{{ formatBytes(item.traffic_in || 0) }}<br />
        ↑{{ formatBytes(item.traffic_out || 0) }}
      </template>
      <template #cell-quota="{ item }">
        <template v-if="item.quota_bytes > 0">
          {{ formatBytes(item.quota_bytes) }}
          <div class="quota-bar">
            <div class="quota-fill" :style="{ width: Math.min(quotaPercent(item), 100) + '%' }"
                 :class="{ 'quota-warn': quotaPercent(item) >= 80, 'quota-over': quotaPercent(item) >= 100 }" />
          </div>
        </template>
        <span v-else class="text-muted">{{ t('secrets.unlimited') }}</span>
      </template>
      <template #cell-expires="{ item }">
        {{ formatISODate(item.expires_at) }}
      </template>
      <template #cell-limits="{ item }">
        <span class="text-sm">{{ item.max_conns || '∞' }} {{ t('secrets.conns') }}</span><br />
        <span class="text-sm">{{ item.max_ips || '∞' }} {{ t('secrets.ips') }}</span>
      </template>
      <template #actions="{ item }">
        <button class="btn btn-ghost btn-sm" :title="t('secrets.rotate')"
                :disabled="secretsStore.rotating === item.label" @click="handleRotate(item.label)">
          <Loader2 v-if="secretsStore.rotating === item.label" :size="16" class="animate-spin" />
          <RotateCw v-else :size="16" />
        </button>
        <button class="btn btn-ghost btn-sm" :title="t('secrets.limits_title')" @click="limitsModal.open(item)">
          <Settings :size="16" />
        </button>
        <button class="btn btn-ghost btn-sm" :title="t('secrets.qr')" @click="showQR(item.label)">
          <QrCode :size="16" />
        </button>
        <button class="btn btn-ghost btn-sm" :title="item.enabled ? t('secrets.disable') : t('secrets.enable')"
                :disabled="secretsStore.toggling === item.label" @click="secretsStore.toggle(item.label, !item.enabled)">
          <Loader2 v-if="secretsStore.toggling === item.label" :size="16" class="animate-spin" />
          <component v-else :is="item.enabled ? Pause : Play" :size="16" />
        </button>
        <button class="btn btn-ghost btn-sm btn-danger-text" :title="t('secrets.delete')" @click="handleRemove(item.label)">
          <Trash2 :size="16" />
        </button>
      </template>
    </DataTable>

    <!-- Add Secret Modal -->
    <FormModal v-model="addModal.isOpen.value" :title="t('secrets.add_title')" :submitting="addModal.submitting.value"
               @submit="addModal.submit(f => secretsStore.add(f.label, f.secret || undefined), t('secrets.added_success', { label: addModal.form.value.label }))">
      <div class="form-group mb-md">
        <label class="form-label">{{ t('secrets.table.label') }}</label>
        <input v-model="addModal.form.value.label" class="input" :placeholder="t('secrets.user_placeholder')" required />
      </div>
      <div class="form-group mb-md">
        <label class="form-label">{{ t('secrets.secret_key') }} <span class="text-muted">{{ t('secrets.optional_auto') }}</span></label>
        <input v-model="addModal.form.value.secret" class="input" :placeholder="t('secrets.hex_placeholder')" maxlength="32" />
      </div>
    </FormModal>

    <!-- Limits Modal -->
    <FormModal v-model="limitsModal.isOpen.value" :title="t('secrets.set_limits_title')" :submitting="limitsModal.submitting.value"
               @submit="handleSetLimits()">
      <div class="form-row mb-sm">
        <div class="form-group">
          <label class="form-label">{{ t('secrets.max_conns') }}</label>
          <input v-model.number="limitsModal.form.value.maxConns" class="input" type="number" min="0" :placeholder="t('secrets.unlimited_placeholder')" />
        </div>
        <div class="form-group">
          <label class="form-label">{{ t('secrets.max_ips') }}</label>
          <input v-model.number="limitsModal.form.value.maxIPs" class="input" type="number" min="0" :placeholder="t('secrets.unlimited_placeholder')" />
        </div>
      </div>
      <div class="form-row mb-sm">
        <div class="form-group">
          <label class="form-label">{{ t('secrets.quota_mb') }}</label>
          <input v-model.number="limitsModal.form.value.quotaMB" class="input" type="number" min="0" :placeholder="t('secrets.unlimited_placeholder')" />
        </div>
        <div class="form-group">
          <label class="form-label">{{ t('secrets.table.expires') }}</label>
          <input v-model="limitsModal.form.value.expiresAt" class="input" type="date" />
        </div>
      </div>
    </FormModal>

    <!-- Confirm Dialog (shared for remove & rotate) -->
    <ConfirmDialog v-bind="confirmState" @confirm="handleConfirm" @cancel="handleCancel" />

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
import { useConfirmDialog } from '@/composables/useConfirmDialog'
import { useFormModal } from '@/composables/useFormModal'
import Modal from '@/components/common/Modal.vue'
import FormModal from '@/components/common/FormModal.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import DataTable from '@/components/common/DataTable.vue'
import StatusBadge from '@/components/common/StatusBadge.vue'
import PageHeader from '@/components/common/PageHeader.vue'
import { KeyRound, RotateCw, Settings, QrCode, Play, Pause, Trash2, Loader2 } from '@lucide/vue'

const { t } = useI18n()
const secretsStore = useSecretsStore()
const toast = useToastStore()

const columns = [
  { key: 'label', header: t('secrets.table.label') },
  { key: 'status', header: t('secrets.table.status') },
  { key: 'traffic', header: t('secrets.table.traffic') },
  { key: 'quota', header: t('secrets.table.quota') },
  { key: 'expires', header: t('secrets.table.expires') },
  { key: 'limits', header: t('secrets.table.limits') },
]

// Confirm dialog
const { confirmState, confirm, handleConfirm, handleCancel } = useConfirmDialog()

async function handleRemove(label: string) {
  if (!await confirm({ title: t('secrets.remove_title'), message: t('secrets.confirm_remove', { label }), confirmText: t('common.delete') })) return
  try {
    await secretsStore.remove(label)
    toast.success(t('secrets.removed_success', { label }))
  } catch (e: any) { toast.error(e.response?.data?.error ?? e.message) }
}

async function handleRotate(label: string) {
  if (!await confirm({ title: t('secrets.rotate_title'), message: t('secrets.confirm_rotate', { label }), confirmText: t('secrets.rotate') })) return
  try {
    await secretsStore.rotate(label)
    toast.success(t('secrets.rotated_success', { label }))
  } catch (e: any) { toast.error(e.response?.data?.error ?? e.message) }
}

// Add modal
const addModal = useFormModal({ label: '', secret: '' })

// Limits modal
const limitsTarget = ref('')
const limitsModal = useFormModal({ maxConns: 0, maxIPs: 0, quotaMB: 0, expiresAt: '' })
limitsModal.open = (sec: any) => {
  limitsTarget.value = sec.label
  limitsModal.form.value = {
    maxConns: sec.max_conns,
    maxIPs: sec.max_ips,
    quotaMB: sec.quota_bytes ? Math.round(sec.quota_bytes / (1024 * 1024)) : 0,
    expiresAt: (sec.expires_at && sec.expires_at !== '0') ? sec.expires_at.split('T')[0] : '',
  }
  limitsModal.isOpen.value = true
}

async function handleSetLimits() {
  await limitsModal.submit(async (f) => {
    await secretsStore.setLimits(limitsTarget.value, f.maxConns, f.maxIPs, f.quotaMB * 1024 * 1024, f.expiresAt || '0')
  })
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
    const [blob, linkData] = await Promise.all([secretsApi.getQR(label), secretsApi.getLink(label)])
    qrImage.value = URL.createObjectURL(blob)
    tgLink.value = linkData.tg_link || ''
    webLink.value = linkData.web_link || ''
    qrModal.value = true
  } catch (e: any) { toast.error(t('secrets.load_failed', { label })) }
}

async function copyToClipboard(text: string) {
  try { await navigator.clipboard.writeText(text); toast.success(t('secrets.copied')) }
  catch { toast.error(t('secrets.copy_failed')) }
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
  background: var(--border-color);
  border-radius: 2px;
  margin-top: 4px;
}
.quota-fill { height: 100%; border-radius: 2px; background: $color-success; transition: width 0.3s; }
.quota-warn { background: $color-warning; }
.quota-over { background: $color-danger; }

.notes-cell { max-width: 200px; }

.qr-image { max-width: 256px; border-radius: $border-radius; }

.links-section {
  margin-top: 1rem;
  padding-top: 1rem;
  border-top: 1px solid $border-color;
}

.input-group {
  display: flex;
  gap: 0.5rem;
  min-width: 0;

  input {
    flex: 1;
    font-family: monospace;
    min-width: 0;
  }
}
</style>
