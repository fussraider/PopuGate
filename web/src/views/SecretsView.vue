<template>
  <div>
    <PageHeader>
      <button class="btn btn-primary" @click="openAddModal">+ Add Secret</button>
    </PageHeader>

    <LoadingSpinner v-if="secretsStore.loading" message="Loading secrets..." />

    <EmptyState v-else-if="!(secretsStore.secrets?.length)" icon="🔑"
                message="No secrets configured. Add your first secret to get started." />

    <div v-else class="table-wrapper">
      <table class="table">
        <thead>
          <tr>
            <th>Label</th>
            <th>Status</th>
            <th>Traffic</th>
            <th>Quota</th>
            <th>Expires</th>
            <th>Limits</th>
            <th>Actions</th>
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
                {{ sec.enabled ? 'Active' : 'Disabled' }}
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
              <span v-else class="text-muted">Unlimited</span>
            </td>
            <td>{{ formatISODate(sec.expires_at) }}</td>
            <td>
              <span class="text-sm">{{ sec.max_conns || '∞' }} conns</span><br />
              <span class="text-sm">{{ sec.max_ips || '∞' }} IPs</span>
            </td>
            <td>
              <div class="actions-cell">
                <button class="btn btn-ghost btn-sm" title="Rotate" @click="confirmRotate(sec.label)">🔄</button>
                <button class="btn btn-ghost btn-sm" title="Limits" @click="openLimitsModal(sec)">⚙️</button>
                <button class="btn btn-ghost btn-sm" title="QR" @click="showQR(sec.label)">📱</button>
                <button class="btn btn-ghost btn-sm" :title="sec.enabled ? 'Disable' : 'Enable'"
                        @click="secretsStore.toggle(sec.label, !sec.enabled)">
                  {{ sec.enabled ? '⏸' : '▶️' }}
                </button>
                <button class="btn btn-ghost btn-sm btn-danger-text" title="Delete" @click="confirmRemove(sec.label)">🗑</button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Add Secret Modal -->
    <Modal v-model="addModal" title="Add Secret">
      <form @submit.prevent="handleAdd">
        <div class="form-group mb-md">
          <label class="form-label">Label</label>
          <input v-model="addForm.label" class="input" placeholder="user1" required />
        </div>
        <div class="form-group mb-md">
          <label class="form-label">Secret Key <span class="text-muted">(optional, auto-generated if empty)</span></label>
          <input v-model="addForm.secret" class="input" placeholder="32 hex characters" maxlength="32" />
        </div>
        <div class="modal-footer-inline">
          <button type="button" class="btn btn-secondary" @click="addModal = false">Cancel</button>
          <button type="submit" class="btn btn-primary" :disabled="adding">Add</button>
        </div>
      </form>
    </Modal>

    <!-- Limits Modal -->
    <Modal v-model="limitsModal" title="Set Limits">
      <form @submit.prevent="handleSetLimits">
        <div class="form-row mb-sm">
          <div class="form-group">
            <label class="form-label">Max Connections</label>
            <input v-model.number="limitsForm.maxConns" class="input" type="number" min="0" placeholder="0 = unlimited" />
          </div>
          <div class="form-group">
            <label class="form-label">Max IPs</label>
            <input v-model.number="limitsForm.maxIPs" class="input" type="number" min="0" placeholder="0 = unlimited" />
          </div>
        </div>
        <div class="form-row mb-sm">
          <div class="form-group">
            <label class="form-label">Quota (MB)</label>
            <input v-model.number="limitsForm.quotaMB" class="input" type="number" min="0" placeholder="0 = unlimited" />
          </div>
          <div class="form-group">
            <label class="form-label">Expires</label>
            <input v-model="limitsForm.expiresAt" class="input" type="date" />
          </div>
        </div>
        <div class="modal-footer-inline">
          <button type="button" class="btn btn-secondary" @click="limitsModal = false">Cancel</button>
          <button type="submit" class="btn btn-primary">Save</button>
        </div>
      </form>
    </Modal>

    <!-- Confirm Remove -->
    <ConfirmDialog v-model="confirmModal" title="Remove Secret"
                   :message="`Are you sure you want to remove '${removeTarget}'?`"
                   confirm-text="Remove" @confirm="handleRemove" />

    <!-- Confirm Rotate -->
    <ConfirmDialog v-model="rotateConfirmModal" title="Rotate Secret"
                   :message="`Rotate secret '${rotateTarget}'? The old key will stop working immediately.`"
                   confirm-text="Rotate" @confirm="handleRotate" />

    <!-- QR Modal -->
    <Modal v-model="qrModal" :title="`Connect: ${qrLabel}`">
      <div class="qr-container text-center">
        <img v-if="qrImage" :src="qrImage" alt="QR Code" class="qr-image" />
        <p class="text-muted mt-sm mb-md">Scan with Telegram to connect</p>

        <div class="links-section text-left">
          <div class="form-group mb-sm">
            <label class="form-label text-xs">Telegram Link (tg://)</label>
            <div class="input-group">
              <input :value="tgLink" class="input input-sm" readonly />
              <button class="btn btn-secondary btn-sm" @click="copyToClipboard(tgLink)">Copy</button>
            </div>
          </div>
          <div class="form-group">
            <label class="form-label text-xs">Web Link (https://)</label>
            <div class="input-group">
              <input :value="webLink" class="input input-sm" readonly />
              <button class="btn btn-secondary btn-sm" @click="copyToClipboard(webLink)">Copy</button>
            </div>
          </div>
        </div>
      </div>
    </Modal>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
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
    toast.success(`Secret "${addForm.value.label}" added`)
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
    toast.success(`Secret "${removeTarget.value}" removed`)
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
    toast.success(`Secret "${rotateTarget.value}" rotated`)
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
  await secretsStore.setLimits(
    limitsTarget.value,
    limitsForm.value.maxConns,
    limitsForm.value.maxIPs,
    limitsForm.value.quotaMB * 1024 * 1024,
    limitsForm.value.expiresAt || '0',
  )
  limitsModal.value = false
}

// QR
const qrModal = ref(false)
const qrLabel = ref('')
const qrImage = ref('')
const tgLink = ref('')
const webLink = ref('')

async function showQR(label: string) {
  qrLabel.value = label
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
    toast.error(`Failed to load connection info for "${label}"`)
  }
}

async function copyToClipboard(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    toast.success('Copied to clipboard')
  } catch (e) {
    toast.error('Failed to copy')
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
