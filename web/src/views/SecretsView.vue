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
            <TrendingUp :size="16" /> {{ t('secrets.top') }}
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
          <template v-if="editingTag === item.label">
            <input v-model="editingTagValue" class="input input-xs tag-input"
                   @blur="saveTag(item.label)" @keyup.enter="saveTag(item.label)" ref="tagInput" />
          </template>
          <template v-else>
            <span v-for="tag in splitTags(item.tags)" :key="tag" class="badge badge-info tag-badge">{{ tag }}</span>
            <button class="btn btn-ghost btn-xs" @click="startEditTag(item)" v-tooltip="t('secrets.edit_tags')">
              <Pencil :size="12" />
            </button>
          </template>
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
      <template #actions="{ item }">
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
        <button class="btn btn-ghost btn-sm" v-tooltip="t('secrets.extend')" @click="extendModal.open(item)">
          <CalendarPlus :size="16" />
        </button>
        <button class="btn btn-ghost btn-sm" v-tooltip="t('secrets.reset_traffic')" @click="handleResetTraffic(item.label)">
          <Eraser :size="16" />
        </button>
        <button class="btn btn-ghost btn-sm" v-tooltip="t('secrets.edit_notes')" @click="notesModal.open(item)">
          <StickyNote :size="16" />
        </button>
        <button class="btn btn-ghost btn-sm" v-tooltip="t('secrets.clone')" @click="cloneModal.open(item)">
          <Copy :size="16" />
        </button>
        <button class="btn btn-ghost btn-sm" v-tooltip="t('secrets.rename_title')" @click="renameModal.open(item)">
          <PenLine :size="16" />
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

    <!-- Clone Modal -->
    <FormModal v-model="cloneModal.isOpen.value" :title="t('secrets.clone_title')" :submitting="cloneModal.submitting.value"
               @submit="handleClone()">
      <div class="form-group mb-md">
        <label class="form-label">{{ t('secrets.clone_new_label') }}</label>
        <input v-model="cloneModal.form.value.newLabel" class="input" required />
      </div>
    </FormModal>

    <!-- Rename Modal -->
    <FormModal v-model="renameModal.isOpen.value" :title="t('secrets.rename_title')" :submitting="renameModal.submitting.value"
               @submit="handleRename()">
      <div class="form-group mb-md">
        <label class="form-label">{{ t('secrets.rename_new_label') }}</label>
        <input v-model="renameModal.form.value.newLabel" class="input" required />
      </div>
    </FormModal>

    <!-- Extend Modal -->
    <FormModal v-model="extendModal.isOpen.value" :title="t('secrets.extend_title')" :submitting="extendModal.submitting.value"
               @submit="handleExtend()">
      <div class="form-group mb-md">
        <label class="form-label">{{ t('secrets.extend_days') }}</label>
        <input v-model.number="extendModal.form.value.days" class="input" type="number" min="1" required />
      </div>
    </FormModal>

    <!-- Notes Modal -->
    <FormModal v-model="notesModal.isOpen.value" :title="t('secrets.edit_notes')" :submitting="notesModal.submitting.value"
               @submit="handleNotes()">
      <div class="form-group mb-md">
        <label class="form-label">{{ t('secrets.edit_notes') }}</label>
        <input v-model="notesModal.form.value.notes" class="input" />
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
      <div class="form-row mb-sm">
        <div class="form-group">
          <label class="form-label">{{ t('secrets.quota_mb') }}</label>
          <input v-model.number="bulkLimitsModal.form.value.quotaMB" class="input" type="number" min="0" :placeholder="t('secrets.unlimited_placeholder')" />
        </div>
        <div class="form-group">
          <label class="form-label">{{ t('secrets.table.expires') }}</label>
          <input v-model="bulkLimitsModal.form.value.expiresAt" class="input" type="date" />
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
import { ref, computed, onMounted, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { useSecretsStore } from '@/stores'
import { useToastStore } from '@/stores/toast'
import { secretsApi } from '@/api/endpoints'
import { formatBytes, formatISODate } from '@/utils/format'
import { useConfirmDialog } from '@/composables/useConfirmDialog'
import { useFormModal } from '@/composables/useFormModal'
import type { SecretImportItem } from '@/types/models'
import Modal from '@/components/common/Modal.vue'
import FormModal from '@/components/common/FormModal.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import DataTable from '@/components/common/DataTable.vue'
import StatusBadge from '@/components/common/StatusBadge.vue'
import PageHeader from '@/components/common/PageHeader.vue'
import {
  KeyRound, RotateCw, Settings, QrCode, Play, Pause, Trash2, Loader2,
  Search, Download, Upload, Copy, Archive as ArchiveIcon, ArchiveRestore,
  Clock, Pencil, PenLine, X as XIcon, TrendingUp, CalendarPlus, Eraser,
  StickyNote, Ban, Tags,
} from '@lucide/vue'

const { t } = useI18n()
const secretsStore = useSecretsStore()
const toast = useToastStore()

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
    .filter((s) => {
      const tags = (s.tags || '').split(',').map((t: string) => t.trim()).filter(Boolean)
      return tags.includes(tag)
    })
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

// Tag editing
const editingTag = ref('')
const editingTagValue = ref('')
const tagInput = ref<HTMLInputElement | null>(null)

function splitTags(tags?: string): string[] {
  return (tags || '').split(',').map((t: string) => t.trim()).filter(Boolean)
}

function startEditTag(item: any) {
  editingTag.value = item.label
  editingTagValue.value = item.tags || ''
  nextTick(() => {
    const el = tagInput.value as any
    if (Array.isArray(el)) el[0]?.focus()
    else el?.focus?.()
  })
}

async function saveTag(label: string) {
  if (editingTag.value !== label) return
  try {
    await secretsStore.setTags(label, editingTagValue.value)
    toast.success(t('secrets.tags_saved'))
  } catch (e: any) { toast.error(e.response?.data?.error ?? e.message) }
  editingTag.value = ''
}

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

async function handleArchive(item: any) {
  const label = item.label
  if (item.archived_at) {
    if (!await confirm({ title: t('secrets.unarchive'), message: t('secrets.confirm_unarchive', { label }) })) return
    try {
      await secretsStore.unarchive(label)
      toast.success(t('secrets.unarchived_success', { label }))
    } catch (e: any) { toast.error(e.response?.data?.error ?? e.message) }
  } else {
    if (!await confirm({ title: t('secrets.archive'), message: t('secrets.confirm_archive', { label }) })) return
    try {
      await secretsStore.archive(label)
      toast.success(t('secrets.archived_success', { label }))
    } catch (e: any) { toast.error(e.response?.data?.error ?? e.message) }
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
  } catch (e: any) { toast.error(e.response?.data?.error ?? e.message) }
}

// Rename modal
const renameSource = ref('')
const renameModal = useFormModal({ newLabel: '' })
renameModal.open = (item: any) => {
  renameSource.value = item.label
  renameModal.form.value.newLabel = item.label
  renameModal.isOpen.value = true
}
async function handleRename() {
  try {
    await renameModal.submit(async () => {
      await secretsStore.rename(renameSource.value, renameModal.form.value.newLabel)
    })
    toast.success(t('secrets.renamed_success', { label: renameModal.form.value.newLabel }))
  } catch (e: any) { toast.error(e.response?.data?.error ?? e.message) }
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
    } catch (e: any) { toast.error(e.response?.data?.error ?? e.message) }
    topLoading.value = false
  } else {
    topModal.value = false
  }
}

// Extend modal
const extendTarget = ref('')
const extendModal = useFormModal({ days: 30 })
extendModal.open = (item: any) => {
  extendTarget.value = item.label
  extendModal.form.value.days = 30
  extendModal.isOpen.value = true
}
async function handleExtend() {
  try {
    await extendModal.submit(async () => {
      await secretsStore.extend(extendTarget.value, extendModal.form.value.days)
    })
    toast.success(t('secrets.extended_success', { label: extendTarget.value }))
  } catch (e: any) { toast.error(e.response?.data?.error ?? e.message) }
}

// Notes modal
const notesTarget = ref('')
const notesModal = useFormModal({ notes: '' })
notesModal.open = (item: any) => {
  notesTarget.value = item.label
  notesModal.form.value.notes = item.notes || ''
  notesModal.isOpen.value = true
}
async function handleNotes() {
  try {
    await notesModal.submit(async () => {
      await secretsStore.updateNotes(notesTarget.value, notesModal.form.value.notes)
    })
    toast.success(t('secrets.notes_saved'))
  } catch (e: any) { toast.error(e.response?.data?.error ?? e.message) }
}

// Reset traffic
async function handleResetTraffic(label: string) {
  if (!await confirm({ title: t('secrets.reset_traffic'), message: t('secrets.confirm_reset_traffic', { label }), confirmText: t('secrets.reset_traffic') })) return
  try {
    await secretsStore.resetTraffic(label)
    toast.success(t('secrets.reset_traffic_success', { label }))
  } catch (e: any) { toast.error(e.response?.data?.error ?? e.message) }
}

// Disable expired
async function handleDisableExpired() {
  if (!await confirm({ title: t('secrets.disable_expired'), message: t('secrets.disable_expired_confirm'), confirmText: t('secrets.disable_expired') })) return
  try {
    const count = await secretsStore.disableExpired()
    toast.success(t('secrets.disabled_expired', { count }))
  } catch (e: any) { toast.error(e.response?.data?.error ?? e.message) }
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
  } catch (e: any) { toast.error(e.response?.data?.error ?? e.message) }
}

// Bulk rotate
async function handleBulkRotate() {
  const labels = [...selectedLabels.value]
  if (!await confirm({ title: t('secrets.bulk_rotate'), message: t('secrets.bulk_rotate_confirm', { count: labels.length }), confirmText: t('secrets.bulk_rotate') })) return
  try {
    await secretsStore.bulkRotate(labels)
    toast.success(t('secrets.bulk_rotated', { count: labels.length }))
    selectedLabels.value = new Set()
  } catch (e: any) { toast.error(e.response?.data?.error ?? e.message) }
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
  } catch (e: any) { toast.error(e.response?.data?.error ?? e.message) }
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
  } catch (e: any) { toast.error(e.response?.data?.error ?? e.message) }
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
  } catch (e: any) { toast.error(e.response?.data?.error ?? e.message) }
}

// Import
const importModal = ref(false)
const importPreview = ref(0)
const importData = ref<SecretImportItem[]>([])
const importing = ref(false)

function handleImportFile(e: Event) {
  const file = (e.target as HTMLInputElement).files?.[0]
  if (!file) return
  const reader = new FileReader()
  reader.onload = () => {
    try {
      const parsed = JSON.parse(reader.result as string)
      const items: SecretImportItem[] = Array.isArray(parsed) ? parsed : parsed.secrets || []
      importData.value = items.filter((i: any) => i.label)
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
    await secretsStore.importSecrets(importData.value)
    toast.success(t('secrets.imported_success', { count: importData.value.length }))
    importModal.value = false
    importData.value = []
    importPreview.value = 0
  } catch (e: any) { toast.error(e.response?.data?.error ?? e.message) }
  importing.value = false
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

onMounted(() => {
  secretsStore.load()
  secretsStore.loadTags()
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
}

.header-actions {
  display: flex;
  align-items: center;
  gap: $spacing-sm;
  flex-shrink: 0;
}

.tag-filter-select {
  min-width: 120px;
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
