<template>
  <div>
    <PageHeader>
      <div class="header-actions">
        <span v-if="!backupStore.loading" class="encryption-badge" :class="backupStore.encryptionEnabled ? 'badge-on' : 'badge-off'" v-tooltip="backupStore.encryptionEnabled ? t('backups.encryption_on_hint') : t('backups.encryption_off_hint')">
          <Shield v-if="backupStore.encryptionEnabled" :size="14" />
          <ShieldOff v-else :size="14" />
          {{ backupStore.encryptionEnabled ? t('backups.encryption_on') : t('backups.encryption_off') }}
        </span>
        <button class="btn btn-primary" :disabled="backupStore.creating" @click="handleCreate">
          <Loader2 v-if="backupStore.creating" :size="16" class="animate-spin" />
          {{ backupStore.creating ? t('backups.creating') : t('backups.create') }}
        </button>
      </div>
    </PageHeader>

    <DataTable
      :columns="columns"
      :items="backupStore.backups"
      :loading="backupStore.loading"
      :empty-icon="Save"
      :empty-message="t('backups.empty')"
      row-key="filename"
    >
      <template #cell-filename="{ item }">
        <div class="filename-wrapper">
          <Lock v-if="item.encrypted" :size="14" class="icon-encrypted" v-tooltip="t('backups.encrypted')" />
          <Unlock v-else :size="14" class="icon-unencrypted" v-tooltip="t('backups.not_encrypted')" />
          <code class="truncate filename-cell">{{ item.filename }}</code>
        </div>
      </template>
      <template #cell-size="{ item }">{{ formatBytes(item.size) }}</template>
      <template #cell-created="{ item }">{{ new Date(item.created_at).toLocaleString() }}</template>
      <template #mobile-actions="{ item }">
        <button class="btn btn-ghost btn-sm" @click="backupActions.open(item)">
          <MoreVertical :size="16" />
        </button>
      </template>
      <template #actions="{ item }">
        <div class="actions-desktop">
          <button class="btn btn-ghost btn-sm" v-tooltip="t('backups.download')" @click="handleDownload(item.filename)">
            <Download :size="16" />
          </button>
          <button class="btn btn-warning btn-sm" :disabled="backupStore.restoring" @click="handleRestore(item.filename)">
            <Loader2 v-if="backupStore.restoring" :size="14" class="animate-spin" />
            {{ t('backups.restore') }}
          </button>
          <button class="btn btn-ghost btn-sm btn-danger-text" @click="handleRemove(item.filename)">
            <Trash2 :size="16" />
          </button>
        </div>
      </template>
    </DataTable>

    <!-- Mobile Action Sheet -->
    <ActionSheet v-model="backupActions.isOpen.value" :title="backupActions.activeItem.value?.filename">
      <button class="action-sheet-item" @click="handleDownload(backupActions.activeItem.value!.filename); backupActions.close()">
        <Download :size="16" /> {{ t('backups.download') }}
      </button>
      <button class="action-sheet-item" :disabled="backupStore.restoring"
              @click="handleRestore(backupActions.activeItem.value!.filename); backupActions.close()">
        <Loader2 v-if="backupStore.restoring" :size="14" class="animate-spin" />
        <RefreshCw v-else :size="16" /> {{ t('backups.restore') }}
      </button>
      <button class="action-sheet-item action-danger"
              @click="handleRemove(backupActions.activeItem.value!.filename); backupActions.close()">
        <Trash2 :size="16" /> {{ t('common.delete') }}
      </button>
    </ActionSheet>

    <ConfirmDialog v-bind="confirmState" @confirm="handleConfirm" @cancel="handleCancel" />
  </div>
</template>

<script setup lang="ts">
import {computed, onMounted} from 'vue'
import {useI18n} from 'vue-i18n'
import {useBackupStore} from '@/stores/backup'
import {useToastStore} from '@/stores/toast'
import {backupApi} from '@/api/endpoints'
import {formatBytes} from '@/utils/format'
import {useConfirmDialog} from '@/composables/useConfirmDialog'
import {useActionMenu} from '@/composables/useActionMenu'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import DataTable from '@/components/common/DataTable.vue'
import PageHeader from '@/components/common/PageHeader.vue'
import ActionSheet from '@/components/common/ActionSheet.vue'
import {Download, Loader2, Lock, MoreVertical, RefreshCw, Save, Shield, ShieldOff, Trash2, Unlock} from '@lucide/vue'

const { t } = useI18n()
const backupStore = useBackupStore()
const toast = useToastStore()
const backupActions = useActionMenu()

const columns = computed(() => [
  { key: 'filename', header: t('backups.table.filename'), sortable: true },
  { key: 'size', header: t('backups.table.size'), sortable: true },
  { key: 'created', header: t('backups.table.created'), sortable: true, sortKey: 'created_at' },
])

const { confirmState, confirm, handleConfirm, handleCancel } = useConfirmDialog()

async function handleRemove(filename: string) {
  if (!await confirm({ title: t('backups.remove_title'), message: t('backups.confirm_remove', { label: filename }), confirmText: t('common.delete') })) return
  try {
    await backupStore.remove(filename)
    toast.success(t('backups.removed_success', { label: filename }))
  } catch { /* interceptor handles error toast */ }
}

async function handleRestore(filename: string) {
  if (!await confirm({ title: t('backups.restore_title'), message: t('backups.confirm_restore', { label: filename }), confirmText: t('backups.restore') })) return
  try {
    await backupStore.restore(filename)
    toast.info(t('backups.restored_success'))
  } catch { /* interceptor handles error toast */ }
}

async function handleCreate() {
  try {
    await backupStore.create()
    toast.success(t('backups.created_success'))
  } catch { /* interceptor handles error toast */ }
}

async function handleDownload(filename: string) {
  try {
    const blob = await backupApi.download(filename)
    const url = window.URL.createObjectURL(new Blob([blob]))
    const link = document.createElement('a')
    link.href = url
    link.setAttribute('download', filename)
    document.body.appendChild(link)
    link.click()
    link.remove()
    window.URL.revokeObjectURL(url)
  } catch { /* interceptor handles error toast */ }
}

onMounted(() => backupStore.load())
</script>

<style scoped lang="scss">
.header-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}

.encryption-badge {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 4px 10px;
  border-radius: 6px;
  font-size: 0.8rem;
  font-weight: 500;
  white-space: nowrap;
}

.badge-on {
  background: rgba(34, 197, 94, 0.12);
  color: var(--color-success, #22c55e);
}

.badge-off {
  background: rgba(156, 163, 175, 0.12);
  color: var(--color-muted, #9ca3af);
}

.filename-wrapper {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.filename-cell {
  display: inline-block;
  max-width: 300px;
}

.icon-encrypted {
  color: var(--color-success, #22c55e);
  flex-shrink: 0;
}

.icon-unencrypted {
  color: var(--color-muted, #9ca3af);
  flex-shrink: 0;
}
</style>
