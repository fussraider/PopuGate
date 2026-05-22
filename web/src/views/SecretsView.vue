<template>
  <div>
    <!-- Header with search + actions -->
    <PageHeader>
      <div class="header-row">
        <div class="search-box">
          <Search :size="16" class="search-icon" />
          <input v-model="searchInput" class="input" :placeholder="t('secrets.search_placeholder')"
                 @input="debouncedSearch" />
          <button v-if="searchInput" class="btn btn-ghost btn-xs search-clear" @click="clearSearch">
            <XIcon :size="14" />
          </button>
        </div>
        <div class="header-actions">
          <select v-model="secretsStore.selectedTagFilter" class="input input-sm tag-filter-select">
            <option value="">{{ t('secrets.all_tags') }}</option>
            <option v-for="tag in secretsStore.allTags" :key="tag" :value="tag">{{ tag }}</option>
          </select>
          <button v-if="secretsStore.selectedTagFilter" class="btn btn-secondary btn-sm"
                  @click="selectAllByTag(secretsStore.selectedTagFilter)">
            <Tags :size="14" /> {{ t('secrets.select_by_tag', { tag: secretsStore.selectedTagFilter }) }}
          </button>
          <label class="toggle-label">
            <input type="checkbox" v-model="secretsStore.showArchived" />
            {{ t('secrets.show_archived') }}
          </label>
          <button class="btn btn-secondary btn-sm" :class="{ active: topModal }" @click="toggleTop" v-tooltip="t('secrets.top_hint')">
            <TrendingUp :size="16" />
          </button>
          <button class="btn btn-secondary btn-sm" @click="handleExport" v-tooltip="t('secrets.export')">
            <Download :size="16" />
          </button>
          <button class="btn btn-secondary btn-sm" @click="handleDisableExpired" v-tooltip="t('secrets.disable_expired')">
            <Ban :size="16" />
          </button>
          <button class="btn btn-secondary btn-sm" @click="importModal = true" v-tooltip="t('secrets.import')">
            <Upload :size="16" />
          </button>
          <button class="btn btn-primary" @click="addModal.open()">+ {{ t('secrets.add_secret') }}</button>
        </div>
      </div>
    </PageHeader>

    <!-- Bulk action toolbar -->
    <div v-if="selectedLabels.size > 0" class="bulk-toolbar card mb-md">
      <span class="badge badge-info">{{ selectedLabels.size }} {{ t('secrets.selected') }}</span>
      <button class="btn btn-secondary btn-sm" @click="bulkExtendModal.open()" :disabled="secretsStore.bulkLoading">
        <Clock :size="14" /> {{ t('secrets.bulk_extend') }}
      </button>
      <button class="btn btn-warning btn-sm" @click="handleBulkRotate" :disabled="secretsStore.bulkLoading">
        <RotateCw :size="14" /> {{ t('secrets.bulk_rotate') }}
      </button>
      <button class="btn btn-secondary btn-sm" @click="handleBulkToggle(true)" :disabled="secretsStore.bulkLoading">
        <Play :size="14" /> {{ t('secrets.bulk_enable') }}
      </button>
      <button class="btn btn-secondary btn-sm" @click="handleBulkToggle(false)" :disabled="secretsStore.bulkLoading">
        <Pause :size="14" /> {{ t('secrets.bulk_disable') }}
      </button>
      <button class="btn btn-secondary btn-sm" @click="bulkLimitsModal.open()" :disabled="secretsStore.bulkLoading">
        <Settings :size="14" /> {{ t('secrets.bulk_set_limits') }}
      </button>
      <button class="btn btn-ghost btn-sm" @click="selectedLabels = new Set()">
        {{ t('secrets.clear_selection') }}
      </button>
    </div>

    <DataTable
      :columns="columns"
      :items="secretsStore.tagFilteredItems"
      :loading="secretsStore.loading"
      :empty-icon="KeyRound"
      :empty-message="t('secrets.empty')"
      row-key="label"
      selectable
      :selected-keys="selectedLabels"
      @update:selected-keys="onSelectionChange"
    >
      <template #cell-label="{ item }">
        <code>{{ item.label }}</code>
        <div v-if="item.notes" class="text-muted text-sm truncate notes-cell">{{ item.notes }}</div>
      </template>
      <template #cell-tags="{ item }">
        <div class="tags-cell">
          <span v-for="tag in parseJSONTags(item.tags)" :key="tag" class="badge badge-info tag-badge">{{ tag }}</span>
        </div>
      </template>
      <template #cell-status="{ item }">
        <div class="status-group">
          <StatusBadge :variant="item.enabled ? 'success' : 'danger'">
            {{ item.enabled ? t('secrets.active') : t('secrets.disabled') }}
          </StatusBadge>
          <StatusBadge v-if="item.archived_at" variant="neutral">
            {{ t('secrets.archived') }}
          </StatusBadge>
        </div>
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
      <template #mobile-actions="{ item }">
        <button class="btn btn-ghost btn-sm" @click="secretActions.open(item)">
          <MoreVertical :size="16" />
        </button>
      </template>
      <template #actions="{ item }">
        <div class="actions-desktop">
          <button class="btn btn-ghost btn-sm" v-tooltip="t('common.edit')" @click="editModal.open(item)">
            <Pencil :size="16" />
          </button>
          <button class="btn btn-ghost btn-sm" v-tooltip="t('secrets.rotate')"
                  :disabled="secretsStore.rotating === item.label" @click="handleRotate(item.label)">
            <Loader2 v-if="secretsStore.rotating === item.label" :size="16" class="animate-spin" />
            <RotateCw v-else :size="16" />
          </button>
          <button class="btn btn-ghost btn-sm" v-tooltip="t('secrets.limits_title')" @click="limitsModal.open(item)">
            <Settings :size="16" />
          </button>
          <button class="btn btn-ghost btn-sm" v-tooltip="t('secrets.qr')" @click="showQR(item.label)">
            <QrCode :size="16" />
          </button>
          <button class="btn btn-ghost btn-sm" v-tooltip="t('secrets.clone')" @click="cloneModal.open(item)">
            <Copy :size="16" />
          </button>
          <button class="btn btn-ghost btn-sm"
                  v-tooltip="item.archived_at ? t('secrets.unarchive') : t('secrets.archive')"
                  @click="handleArchive(item)">
            <component :is="item.archived_at ? ArchiveRestore : ArchiveIcon" :size="16" />
          </button>
          <button class="btn btn-ghost btn-sm" v-tooltip="item.enabled ? t('secrets.disable') : t('secrets.enable')"
                  :disabled="secretsStore.toggling === item.label" @click="secretsStore.toggle(item.label, !item.enabled)">
            <Loader2 v-if="secretsStore.toggling === item.label" :size="16" class="animate-spin" />
            <component v-else :is="item.enabled ? Pause : Play" :size="16" />
          </button>
          <button class="btn btn-ghost btn-sm btn-danger-text" v-tooltip="t('secrets.delete')" @click="handleRemove(item.label)">
            <Trash2 :size="16" />
          </button>
        </div>
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
      <div class="form-card">
        <h4 class="form-card-title">{{ t('secrets.section_limits') }}</h4>
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
        <div class="form-row">
          <div class="form-group">
            <label class="form-label">{{ t('secrets.quota_mb') }}</label>
            <input v-model.number="limitsModal.form.value.quotaMB" class="input" type="number" min="0" :placeholder="t('secrets.unlimited_placeholder')" />
          </div>
          <div class="form-group">
            <label class="form-label">{{ t('secrets.table.expires') }}</label>
            <input v-model="limitsModal.form.value.expiresAt" class="input" type="date" />
          </div>
        </div>
      </div>
    </FormModal>

    <!-- Clone Modal -->
    <FormModal v-model="cloneModal.isOpen.value" :title="t('secrets.clone_title')" :submitting="cloneModal.submitting.value"
               @submit="handleClone()">
      <div class="form-group mb-md">
        <label class="form-label">{{ t('secrets.clone_new_label') }}</label>
        <input v-model="cloneModal.form.value.newLabel" class="input" required />
      </div>
    </FormModal>

    <!-- Edit Secret Modal -->
    <FormModal v-model="editModal.isOpen.value" :title="t('secrets.edit_title', { label: editTarget })" :submitting="editModal.submitting.value"
               @submit="handleEdit()">
      <div class="form-card">
        <h4 class="form-card-title">{{ t('secrets.section_info') }}</h4>
        <div class="form-group mb-md">
          <label class="form-label">{{ t('secrets.rename_new_label') }}</label>
          <input v-model="editModal.form.value.label" class="input" required />
        </div>
        <div class="form-group mb-md">
          <label class="form-label">{{ t('secrets.edit_tags') }}</label>
          <TagInput v-model="editModal.form.value.tags" :available-tags="secretsStore.allTags" :placeholder="t('secrets.tags_placeholder')" />
        </div>
        <div class="form-group">
          <label class="form-label">{{ t('secrets.edit_notes') }}</label>
          <textarea v-model="editModal.form.value.notes" class="input" rows="3"></textarea>
        </div>
      </div>
      <div class="form-card">
        <h4 class="form-card-title">{{ t('secrets.section_limits') }}</h4>
        <div class="form-group">
          <label class="form-label">{{ t('secrets.extend_days') }}</label>
          <input v-model.number="editModal.form.value.extendDays" class="input" type="number" min="0" />
          <small class="text-muted">{{ t('secrets.extend_edit_hint') }}</small>
        </div>
      </div>
    </FormModal>

    <!-- Top by Traffic Modal -->
    <Modal v-model="topModal" :title="t('secrets.top')">
      <div v-if="topLoading" class="text-center mb-md"><Loader2 :size="24" class="animate-spin" /></div>
      <div v-else-if="topItems.length === 0" class="text-muted">{{ t('secrets.top_empty') }}</div>
      <table v-else class="top-table">
        <thead>
          <tr>
            <th>#</th>
            <th>{{ t('secrets.table.label') }}</th>
            <th>{{ t('secrets.table.traffic') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(item, idx) in topItems" :key="item.label">
            <td>{{ idx + 1 }}</td>
            <td><code>{{ item.label }}</code></td>
            <td>↓{{ formatBytes(item.traffic_in || 0) }} / ↑{{ formatBytes(item.traffic_out || 0) }}</td>
          </tr>
        </tbody>
      </table>
      <div class="modal-footer-inline mt-md">
        <button class="btn btn-secondary" @click="topModal = false">{{ t('common.cancel') }}</button>
      </div>
    </Modal>

    <!-- Bulk Extend Modal -->
    <FormModal v-model="bulkExtendModal.isOpen.value" :title="t('secrets.bulk_extend_title')" :submitting="bulkExtendModal.submitting.value"
               @submit="handleBulkExtend()">
      <p class="text-muted mb-md">{{ selectedLabels.size }} {{ t('secrets.selected') }}</p>
      <div class="form-group mb-md">
        <label class="form-label">{{ t('secrets.bulk_extend_days') }}</label>
        <input v-model.number="bulkExtendModal.form.value.days" class="input" type="number" min="1" required />
      </div>
    </FormModal>

    <!-- Bulk Set Limits Modal -->
    <FormModal v-model="bulkLimitsModal.isOpen.value" :title="t('secrets.bulk_set_limits')" :submitting="bulkLimitsModal.submitting.value"
               @submit="handleBulkSetLimits()">
      <p class="text-muted mb-md">{{ selectedLabels.size }} {{ t('secrets.selected') }}</p>
      <div class="form-card">
        <h4 class="form-card-title">{{ t('secrets.section_limits') }}</h4>
        <div class="form-row mb-sm">
          <div class="form-group">
            <label class="form-label">{{ t('secrets.max_conns') }}</label>
            <input v-model.number="bulkLimitsModal.form.value.maxConns" class="input" type="number" min="0" :placeholder="t('secrets.unlimited_placeholder')" />
          </div>
          <div class="form-group">
            <label class="form-label">{{ t('secrets.max_ips') }}</label>
            <input v-model.number="bulkLimitsModal.form.value.maxIPs" class="input" type="number" min="0" :placeholder="t('secrets.unlimited_placeholder')" />
          </div>
        </div>
        <div class="form-row">
          <div class="form-group">
            <label class="form-label">{{ t('secrets.quota_mb') }}</label>
            <input v-model.number="bulkLimitsModal.form.value.quotaMB" class="input" type="number" min="0" :placeholder="t('secrets.unlimited_placeholder')" />
          </div>
          <div class="form-group">
            <label class="form-label">{{ t('secrets.table.expires') }}</label>
            <input v-model="bulkLimitsModal.form.value.expiresAt" class="input" type="date" />
          </div>
        </div>
      </div>
    </FormModal>

    <!-- Import Modal -->
    <Modal v-model="importModal" :title="t('secrets.import_title')">
      <div class="form-group mb-md">
        <input type="file" accept=".json" class="input" @change="handleImportFile" />
      </div>
      <p v-if="importPreview > 0" class="text-muted mb-md">
        {{ t('secrets.import_preview', { count: importPreview }) }}
      </p>
      <div class="modal-footer-inline">
        <button class="btn btn-secondary" @click="importModal = false">{{ t('common.cancel') }}</button>
        <button class="btn btn-primary" :disabled="importPreview === 0 || importing" @click="handleImport">
          <Loader2 v-if="importing" :size="16" class="animate-spin" />
          {{ t('secrets.import') }}
        </button>
      </div>
    </Modal>

    <!-- Confirm Dialog -->
    <ConfirmDialog v-bind="confirmState" @confirm="handleConfirm" @cancel="handleCancel" />

    <!-- QR Modal -->
    <Modal v-model="qrModal" :title="t('secrets.connect_title', { label: qrLabel })">
      <div class="qr-container">
        <div v-if="proxyLinks.length" class="flex-center gap-sm mb-md">
          <button class="btn btn-secondary btn-sm" @click="copyAllLinks">{{ t('secrets.copy_all_links') }}</button>
          <button class="btn btn-secondary btn-sm" @click="exportLinks">{{ t('secrets.export_links') }}</button>
        </div>
        <div class="links-section text-left">
          <div v-for="(link, idx) in proxyLinks" :key="idx" class="link-group mb-md">
            <div class="link-group-header">
              <span class="badge badge-info">{{ link.instance_label || (':' + link.instance_port) }}</span>
              <code class="text-sm">{{ link.domain }}</code>
            </div>
            <div class="form-group mb-xs">
              <div class="input-group">
                <input :value="link.tg_link" class="input input-sm" readonly />
                <button class="btn btn-secondary btn-sm" v-tooltip="t('secrets.copy')" @click="copyToClipboard(link.tg_link)"><Copy :size="14" /></button>
                <button class="btn btn-secondary btn-sm" :class="{ active: qrActiveLink === idx + '-tg' }" v-tooltip="t('secrets.show_qr')" @click="toggleLinkQR(idx + '-tg', link.tg_link)"><QrCode :size="14" /></button>
              </div>
              <Transition name="qr-slide">
                <div v-if="qrActiveLink === idx + '-tg'" class="qr-inline mt-sm">
                  <div class="qr-card">
                    <img :src="qrDataUrl" alt="QR" class="qr-image-sm" />
                  </div>
                </div>
              </Transition>
            </div>
            <div class="form-group">
              <div class="input-group">
                <input :value="link.web_link" class="input input-sm" readonly />
                <button class="btn btn-secondary btn-sm" v-tooltip="t('secrets.copy')" @click="copyToClipboard(link.web_link)"><Copy :size="14" /></button>
                <button class="btn btn-secondary btn-sm" :class="{ active: qrActiveLink === idx + '-web' }" v-tooltip="t('secrets.show_qr')" @click="toggleLinkQR(idx + '-web', link.web_link)"><QrCode :size="14" /></button>
              </div>
              <Transition name="qr-slide">
                <div v-if="qrActiveLink === idx + '-web'" class="qr-inline mt-sm">
                  <div class="qr-card">
                    <img :src="qrDataUrl" alt="QR" class="qr-image-sm" />
                  </div>
                </div>
              </Transition>
            </div>
          </div>
        </div>
      </div>
    </Modal>
    <!-- Mobile Action Sheet -->
    <ActionSheet v-model="secretActions.isOpen.value" :title="secretActions.activeItem.value?.label">
      <button class="action-sheet-item" @click="editModal.open(secretActions.activeItem.value!); secretActions.close()">
        <Pencil :size="16" /> {{ t('common.edit') }}
      </button>
      <button class="action-sheet-item" :disabled="secretsStore.rotating === secretActions.activeItem.value?.label"
              @click="handleRotate(secretActions.activeItem.value!.label); secretActions.close()">
        <RotateCw :size="16" /> {{ t('secrets.rotate') }}
      </button>
      <button class="action-sheet-item" @click="limitsModal.open(secretActions.activeItem.value!); secretActions.close()">
        <Settings :size="16" /> {{ t('secrets.limits_title') }}
      </button>
      <button class="action-sheet-item" @click="showQR(secretActions.activeItem.value!.label); secretActions.close()">
        <QrCode :size="16" /> {{ t('secrets.qr') }}
      </button>
      <button class="action-sheet-item" @click="handleResetTraffic(secretActions.activeItem.value!.label); secretActions.close()">
        <Eraser :size="16" /> {{ t('secrets.reset_traffic') }}
      </button>
      <button class="action-sheet-item" @click="cloneModal.open(secretActions.activeItem.value!); secretActions.close()">
        <Copy :size="16" /> {{ t('secrets.clone') }}
      </button>
      <button class="action-sheet-item"
              @click="handleArchive(secretActions.activeItem.value!); secretActions.close()">
        <component :is="secretActions.activeItem.value?.archived_at ? ArchiveRestore : ArchiveIcon" :size="16" />
        {{ secretActions.activeItem.value?.archived_at ? t('secrets.unarchive') : t('secrets.archive') }}
      </button>
      <button class="action-sheet-item"
              :disabled="secretsStore.toggling === secretActions.activeItem.value?.label"
              @click="secretsStore.toggle(secretActions.activeItem.value!.label, !secretActions.activeItem.value!.enabled); secretActions.close()">
        <component :is="secretActions.activeItem.value?.enabled ? Pause : Play" :size="16" />
        {{ secretActions.activeItem.value?.enabled ? t('secrets.disable') : t('secrets.enable') }}
      </button>
      <button class="action-sheet-item action-danger"
              @click="handleRemove(secretActions.activeItem.value!.label); secretActions.close()">
        <Trash2 :size="16" /> {{ t('secrets.delete') }}
      </button>
    </ActionSheet>
  </div>
</template>

<script setup lang="ts">
import {computed, onMounted, onUnmounted, ref} from 'vue'
import {useI18n} from 'vue-i18n'
import {useSecretsStore} from '@/stores'
import {useToastStore} from '@/stores/toast'
import {secretsApi} from '@/api/endpoints'
import {formatBytes, formatISODate, parseJSONTags} from '@/utils/format'
import {useConfirmDialog} from '@/composables/useConfirmDialog'
import {useFormModal} from '@/composables/useFormModal'
import {useActionMenu} from '@/composables/useActionMenu'
import type {ProxyLink, SecretImportItem} from '@/types/models'
import Modal from '@/components/common/Modal.vue'
import ActionSheet from '@/components/common/ActionSheet.vue'
import FormModal from '@/components/common/FormModal.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import DataTable from '@/components/common/DataTable.vue'
import StatusBadge from '@/components/common/StatusBadge.vue'
import PageHeader from '@/components/common/PageHeader.vue'
import TagInput from '@/components/common/TagInput.vue'
import {
  Archive as ArchiveIcon,
  ArchiveRestore,
  Ban,
  Clock,
  Copy,
  Download,
  Eraser,
  KeyRound,
  Loader2,
  MoreVertical,
  Pause,
  Pencil,
  Play,
  QrCode,
  RotateCw,
  Search,
  Settings,
  Tags,
  Trash2,
  TrendingUp,
  Upload,
  X as XIcon,
} from '@lucide/vue'
import QRCodeStyling from 'qr-code-styling'
import popugateLogo from '@/assets/images/icons/icon-192x192.png'

const { t } = useI18n()
const secretsStore = useSecretsStore()
const toast = useToastStore()
const secretActions = useActionMenu()

const columns = computed(() => [
  { key: 'label', header: t('secrets.table.label') },
  { key: 'tags', header: t('secrets.table.tags') },
  { key: 'status', header: t('secrets.table.status') },
  { key: 'traffic', header: t('secrets.table.traffic') },
  { key: 'quota', header: t('secrets.table.quota') },
  { key: 'expires', header: t('secrets.table.expires') },
  { key: 'limits', header: t('secrets.table.limits') },
])

// Selection
const selectedLabels = ref<Set<string>>(new Set())
function onSelectionChange(keys: Set<string | number>) {
  selectedLabels.value = new Set([...keys] as string[])
}

function selectAllByTag(tag: string) {
  const matching = secretsStore.tagFilteredItems
    .filter((s) => parseJSONTags(s.tags).includes(tag))
    .map((s) => s.label)
  selectedLabels.value = new Set(matching)
}

// Search
const searchInput = ref('')
let searchTimer: ReturnType<typeof setTimeout> | null = null
function debouncedSearch() {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => secretsStore.search(searchInput.value), 300)
}
function clearSearch() {
  searchInput.value = ''
  secretsStore.search('')
}

// Confirm dialog
const { confirmState, confirm, handleConfirm, handleCancel } = useConfirmDialog()

async function handleRemove(label: string) {
  if (!await confirm({ title: t('secrets.remove_title'), message: t('secrets.confirm_remove', { label }), confirmText: t('common.delete') })) return
  try {
    await secretsStore.remove(label)
    toast.success(t('secrets.removed_success', { label }))
  } catch { /* interceptor handles error toast */ }
}

async function handleRotate(label: string) {
  if (!await confirm({ title: t('secrets.rotate_title'), message: t('secrets.confirm_rotate', { label }), confirmText: t('secrets.rotate') })) return
  try {
    await secretsStore.rotate(label)
    toast.success(t('secrets.rotated_success', { label }))
  } catch { /* interceptor handles error toast */ }
}

async function handleArchive(item: any) {
  const label = item.label
  if (item.archived_at) {
    if (!await confirm({ title: t('secrets.unarchive'), message: t('secrets.confirm_unarchive', { label }) })) return
    try {
      await secretsStore.unarchive(label)
      toast.success(t('secrets.unarchived_success', { label }))
    } catch { /* interceptor handles error toast */ }
  } else {
    if (!await confirm({ title: t('secrets.archive'), message: t('secrets.confirm_archive', { label }) })) return
    try {
      await secretsStore.archive(label)
      toast.success(t('secrets.archived_success', { label }))
    } catch { /* interceptor handles error toast */ }
  }
}

// Add modal
const addModal = useFormModal({ label: '', secret: '' })

// Clone modal
const cloneSource = ref('')
const cloneModal = useFormModal({ newLabel: '' })
cloneModal.open = (item: any) => {
  cloneSource.value = item.label
  cloneModal.form.value.newLabel = ''
  cloneModal.isOpen.value = true
}
async function handleClone() {
  try {
    await cloneModal.submit(async () => {
      await secretsStore.clone(cloneSource.value, cloneModal.form.value.newLabel)
    })
    toast.success(t('secrets.cloned_success', { label: cloneModal.form.value.newLabel }))
  } catch { /* interceptor handles error toast */ }
}

// Edit modal (label, tags, notes, extend)
const editTarget = ref('')
const editModal = useFormModal({ label: '', tags: '', notes: '', extendDays: 0 })
editModal.open = (item: any) => {
  editTarget.value = item.label
  editModal.form.value.label = item.label
  editModal.form.value.tags = item.tags || ''
  editModal.form.value.notes = item.notes || ''
  editModal.form.value.extendDays = 0
  editModal.isOpen.value = true
}
async function handleEdit() {
  const f = editModal.form.value
  try {
    await editModal.submit(async () => {
      const promises: Promise<void>[] = []
      if (f.label !== editTarget.value) {
        promises.push(secretsStore.rename(editTarget.value, f.label))
      }
      if (f.tags !== (secretsStore.secrets.find(s => s.label === editTarget.value || s.label === f.label)?.tags || '')) {
        const target = f.label !== editTarget.value ? f.label : editTarget.value
        promises.push(secretsStore.setTags(target, f.tags))
      }
      if (f.notes !== (secretsStore.secrets.find(s => s.label === editTarget.value || s.label === f.label)?.notes || '')) {
        const target = f.label !== editTarget.value ? f.label : editTarget.value
        promises.push(secretsStore.updateNotes(target, f.notes))
      }
      if (f.extendDays > 0) {
        const target = f.label !== editTarget.value ? f.label : editTarget.value
        promises.push(secretsStore.extend(target, f.extendDays))
      }
      await Promise.all(promises)
    })
    toast.success(t('secrets.edit_saved', { label: f.label }))
  } catch { /* interceptor handles error toast */ }
}

// Top by traffic
const topModal = ref(false)
const topItems = ref<any[]>([])
const topLoading = ref(false)

async function toggleTop() {
  if (!topModal.value) {
    topModal.value = true
    topLoading.value = true
    try {
      topItems.value = await secretsStore.loadTop(10)
    } catch { /* interceptor handles error toast */ }
    topLoading.value = false
  } else {
    topModal.value = false
  }
}

// Reset traffic
async function handleResetTraffic(label: string) {
  if (!await confirm({ title: t('secrets.reset_traffic'), message: t('secrets.confirm_reset_traffic', { label }), confirmText: t('secrets.reset_traffic') })) return
  try {
    await secretsStore.resetTraffic(label)
    toast.success(t('secrets.reset_traffic_success', { label }))
  } catch { /* interceptor handles error toast */ }
}

// Disable expired
async function handleDisableExpired() {
  if (!await confirm({ title: t('secrets.disable_expired'), message: t('secrets.disable_expired_confirm'), confirmText: t('secrets.disable_expired') })) return
  try {
    const count = await secretsStore.disableExpired()
    toast.success(t('secrets.disabled_expired', { count }))
  } catch { /* interceptor handles error toast */ }
}

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

// Bulk extend modal
const bulkExtendModal = useFormModal({ days: 30 })
async function handleBulkExtend() {
  const labels = [...selectedLabels.value]
  try {
    await bulkExtendModal.submit(async (f) => {
      await secretsStore.bulkExtend(labels, f.days)
    })
    toast.success(t('secrets.bulk_extended', { count: labels.length }))
    selectedLabels.value = new Set()
  } catch { /* interceptor handles error toast */ }
}

// Bulk rotate
async function handleBulkRotate() {
  const labels = [...selectedLabels.value]
  if (!await confirm({ title: t('secrets.bulk_rotate'), message: t('secrets.bulk_rotate_confirm', { count: labels.length }), confirmText: t('secrets.bulk_rotate') })) return
  try {
    await secretsStore.bulkRotate(labels)
    toast.success(t('secrets.bulk_rotated', { count: labels.length }))
    selectedLabels.value = new Set()
  } catch { /* interceptor handles error toast */ }
}

// Bulk toggle
async function handleBulkToggle(enable: boolean) {
  const labels = [...selectedLabels.value]
  if (!await confirm({
    title: enable ? t('secrets.bulk_enable') : t('secrets.bulk_disable'),
    message: enable ? t('secrets.bulk_enable_confirm', { count: labels.length }) : t('secrets.bulk_disable_confirm', { count: labels.length }),
    confirmText: enable ? t('secrets.bulk_enable') : t('secrets.bulk_disable'),
  })) return
  try {
    await secretsStore.bulkToggle(labels, enable)
    toast.success(enable ? t('secrets.bulk_enabled', { count: labels.length }) : t('secrets.bulk_disabled', { count: labels.length }))
    selectedLabels.value = new Set()
  } catch { /* interceptor handles error toast */ }
}

// Bulk set limits modal
const bulkLimitsModal = useFormModal({ maxConns: 0, maxIPs: 0, quotaMB: 0, expiresAt: '' })
async function handleBulkSetLimits() {
  const labels = [...selectedLabels.value]
  try {
    await bulkLimitsModal.submit(async (f) => {
      const limits: Record<string, any> = {}
      if (f.maxConns > 0) limits.max_conns = f.maxConns
      if (f.maxIPs > 0) limits.max_ips = f.maxIPs
      if (f.quotaMB > 0) limits.quota_bytes = f.quotaMB * 1024 * 1024
      if (f.expiresAt) limits.expires_at = f.expiresAt
      await secretsStore.bulkSetLimits(labels, limits)
    })
    toast.success(t('secrets.bulk_limits_set', { count: labels.length }))
    selectedLabels.value = new Set()
  } catch { /* interceptor handles error toast */ }
}

// Export
async function handleExport() {
  try {
    const data = await secretsStore.exportAll()
    const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = 'popugate-secrets.json'
    a.click()
    URL.revokeObjectURL(url)
  } catch { /* interceptor handles error toast */ }
}

// Import
const importModal = ref(false)
const importPreview = ref(0)
const importData = ref<SecretImportItem[]>([])
const importing = ref(false)

function handleImportFile(e: Event) {
  const file = (e.target as HTMLInputElement).files?.[0]
  if (!file) return
  const MAX_IMPORT_SIZE = 10 * 1024 * 1024 // 10 MB
  if (file.size > MAX_IMPORT_SIZE) {
    toast.error(t('secrets.import_failed'))
    return
  }
  const reader = new FileReader()
  reader.onload = () => {
    try {
      const parsed = JSON.parse(reader.result as string)
      const items: SecretImportItem[] = Array.isArray(parsed) ? parsed : parsed.secrets || []
      importData.value = items.filter((i): i is SecretImportItem =>
        typeof i === 'object' && i !== null && typeof i.label === 'string' && i.label.trim() !== ''
      )
      importPreview.value = importData.value.length
    } catch {
      toast.error(t('secrets.import_failed'))
      importPreview.value = 0
    }
  }
  reader.readAsText(file)
}

async function handleImport() {
  if (!importData.value.length) return
  importing.value = true
  try {
    const result = await secretsStore.importSecrets(importData.value)
    const imported = result?.imported?.length ?? 0
    const skipped = result?.skipped?.length ?? 0
    const errors = result?.errors?.length ?? 0
    if (imported > 0) toast.success(t('secrets.imported_success', { count: imported }))
    if (skipped > 0) toast.warning(t('secrets.imported_skipped', { count: skipped }))
    if (errors > 0) toast.error(t('secrets.imported_errors', { count: errors }))
    importModal.value = false
    importData.value = []
    importPreview.value = 0
  } catch { /* interceptor handles error toast */ }
  importing.value = false
}

// QR
const qrModal = ref(false)
const qrLabel = ref('')
const qrActiveLink = ref('')
const qrDataUrl = ref('')
const proxyLinks = ref<ProxyLink[]>([])

async function showQR(label: string) {
  qrLabel.value = label
  qrActiveLink.value = ''
  qrDataUrl.value = ''
  proxyLinks.value = []
  try {
    const linkData = await secretsApi.getLink(label)
    proxyLinks.value = linkData.links || []
    qrModal.value = true
  } catch { /* interceptor handles error toast */ }
}

async function toggleLinkQR(key: string, text: string) {
  if (qrActiveLink.value === key) {
    if (qrDataUrl.value.startsWith('blob:')) URL.revokeObjectURL(qrDataUrl.value)
    qrActiveLink.value = ''
    qrDataUrl.value = ''
    return
  }
  qrActiveLink.value = key
  qrDataUrl.value = ''
  try {
    const isDark = document.documentElement.getAttribute('data-theme') === 'dark'
    const dotColor = isDark ? '#a5b4fc' : '#1e1b4b'
    const cornerColor = isDark ? '#818cf8' : '#312e81'
    const dotCornerColor = isDark ? '#6366f1' : '#4338ca'
    const bgColor = isDark ? '#1e1b4b' : '#ffffff'

    const qr = new QRCodeStyling({
      width: 280,
      height: 280,
      type: 'canvas',
      data: text,
      margin: 5,
      image: popugateLogo,
      dotsOptions: { color: dotColor, type: 'rounded' },
      cornersSquareOptions: { color: cornerColor, type: 'extra-rounded' },
      cornersDotOptions: { color: dotCornerColor, type: 'dot' },
      backgroundOptions: { color: bgColor },
      imageOptions: { margin: 4, imageSize: 0.3 },
    })
    const blob = await qr.getRawData('png')
    if (!blob) throw new Error('empty')
    if (qrDataUrl.value.startsWith('blob:')) URL.revokeObjectURL(qrDataUrl.value)
    qrDataUrl.value = URL.createObjectURL(blob as Blob)
  } catch {
    toast.error(t('secrets.qr_failed'))
    qrActiveLink.value = ''
  }
}

async function copyToClipboard(text: string) {
  try { await navigator.clipboard.writeText(text); toast.success(t('secrets.copied')) }
  catch { toast.error(t('secrets.copy_failed')) }
}

async function copyAllLinks() {
  const text = proxyLinks.value.map(l => l.tg_link).join('\n')
  try { await navigator.clipboard.writeText(text); toast.success(t('secrets.links_copied')) }
  catch { toast.error(t('secrets.copy_failed')) }
}

function exportLinks() {
  const lines = proxyLinks.value.map(l =>
    `[${l.instance_label || ':' + l.instance_port}] ${l.domain}\n  tg: ${l.tg_link}\n  web: ${l.web_link}`
  )
  const blob = new Blob([lines.join('\n\n')], { type: 'text/plain' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `${qrLabel.value}-links.txt`
  a.click()
  URL.revokeObjectURL(url)
}

function quotaPercent(sec: any): number {
  if (!sec.quota_bytes) return 0
  return ((sec.traffic_in || 0) + (sec.traffic_out || 0)) / sec.quota_bytes * 100
}

onMounted(() => {
  secretsStore.load()
  secretsStore.loadTags()
})

onUnmounted(() => {
  if (searchTimer) clearTimeout(searchTimer)
})
</script>

<style scoped lang="scss">
@use '@/assets/scss/variables' as *;

.header-row {
  display: flex;
  align-items: center;
  gap: $spacing-md;
  width: 100%;
  flex-wrap: wrap;
}

.search-box {
  position: relative;
  flex: 1;
  min-width: 200px;

  .search-icon {
    position: absolute;
    left: 10px;
    top: 50%;
    transform: translateY(-50%);
    color: var(--text-muted);
  }

  input {
    padding-left: 32px;
    padding-right: 28px;
  }

  .search-clear {
    position: absolute;
    right: 4px;
    top: 50%;
    transform: translateY(-50%);
  }

  @media (max-width: 480px) {
    flex: 1 1 100%;
    min-width: 0;
  }
}

.header-actions {
  display: flex;
  align-items: center;
  gap: $spacing-sm;
  flex-shrink: 0;

  @media (max-width: 480px) {
    flex-wrap: wrap;
    flex-shrink: 1;
  }
}

.tag-filter-select {
  min-width: 120px;

  @media (max-width: 480px) {
    min-width: 0;
    flex: 1;
  }
}

.toggle-label {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: $font-size-sm;
  color: var(--text-muted);
  cursor: pointer;
  white-space: nowrap;

  input { cursor: pointer; }
}

.bulk-toolbar {
  display: flex;
  align-items: center;
  gap: $spacing-sm;
  padding: $spacing-sm $spacing-md;
}

.status-group {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
}

.tags-cell {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-wrap: wrap;
  max-width: 200px;
  overflow: hidden;
}

.tag-badge {
  font-size: 11px;
  padding: 1px 6px;
}

.tag-input {
  min-width: 120px;
}

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

.top-table {
  width: 100%;
  border-collapse: collapse;

  th, td {
    padding: $spacing-xs $spacing-sm;
    text-align: left;
    border-bottom: 1px solid $border-color;
  }

  th {
    font-size: $font-size-xs;
    font-weight: $font-weight-semibold;
    color: var(--text-muted);
    text-transform: uppercase;
  }

  td code { font-size: $font-size-sm; }
}

.qr-image-sm { width: 100%; border-radius: $border-radius; display: block; }

.qr-card {
  display: inline-block;
  padding: 0.5rem;
  border-radius: $border-radius-lg;
  background: var(--bg-card);
  border: 1px solid var(--border-color);
  box-shadow: var(--shadow-sm);
}

.qr-inline { display: flex; justify-content: center; padding: 0.5rem 0; }

.qr-slide-enter-active { transition: all 0.25s ease-out; }
.qr-slide-enter-from { opacity: 0; transform: translateY(-8px) scale(0.97); }

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

.link-group {
  padding-bottom: 0.5rem;
  border-bottom: 1px solid var(--border-color);

  &:last-child {
    border-bottom: none;
    padding-bottom: 0;
  }
}

.link-group-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-bottom: 0.25rem;
}
</style>
