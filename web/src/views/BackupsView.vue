<template>
  <div>
    <PageHeader>
      <button class="btn btn-primary" :disabled="backupStore.creating" @click="handleCreate">
        <Loader2 v-if="backupStore.creating" :size="16" class="animate-spin" />
        {{ backupStore.creating ? t('backups.creating') : t('backups.create') }}
      </button>
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
        <code class="truncate filename-cell">{{ item.filename }}</code>
      </template>
      <template #cell-size="{ item }">{{ formatBytes(item.size) }}</template>
      <template #cell-created="{ item }">{{ new Date(item.created_at).toLocaleString() }}</template>
      <template #actions="{ item }">
        <button class="btn btn-ghost btn-sm" :title="t('backups.download')" @click="handleDownload(item.filename)">
          <Download :size="16" />
        </button>
        <button class="btn btn-warning btn-sm" :disabled="backupStore.restoring" @click="handleRestore(item.filename)">
          <Loader2 v-if="backupStore.restoring" :size="14" class="animate-spin" />
          {{ t('backups.restore') }}
        </button>
        <button class="btn btn-ghost btn-sm btn-danger-text" @click="handleRemove(item.filename)">
          <Trash2 :size="16" />
        </button>
      </template>
    </DataTable>

    <ConfirmDialog v-bind="confirmState" @confirm="handleConfirm" @cancel="handleCancel" />
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useBackupStore } from '@/stores/backup'
import { useToastStore } from '@/stores/toast'
import { backupApi } from '@/api/endpoints'
import { formatBytes } from '@/utils/format'
import { useConfirmDialog } from '@/composables/useConfirmDialog'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import DataTable from '@/components/common/DataTable.vue'
import PageHeader from '@/components/common/PageHeader.vue'
import { Save, Download, Trash2, Loader2 } from '@lucide/vue'

const { t } = useI18n()
const backupStore = useBackupStore()
const toast = useToastStore()

const columns = [
  { key: 'filename', header: t('backups.table.filename') },
  { key: 'size', header: t('backups.table.size') },
  { key: 'created', header: t('backups.table.created') },
]

const { confirmState, confirm, handleConfirm, handleCancel } = useConfirmDialog()

async function handleRemove(filename: string) {
  if (!await confirm({ title: t('backups.remove_title'), message: t('backups.confirm_remove', { label: filename }), confirmText: t('common.delete') })) return
  try {
    await backupStore.remove(filename)
    toast.success(t('backups.removed_success', { label: filename }))
  } catch (e: any) { toast.error(e.response?.data?.error ?? e.message) }
}

async function handleRestore(filename: string) {
  if (!await confirm({ title: t('backups.restore_title'), message: t('backups.confirm_restore', { label: filename }), confirmText: t('backups.restore') })) return
  try {
    await backupStore.restore(filename)
    toast.info(t('backups.restored_success'))
  } catch (e: any) { toast.error(e.response?.data?.error ?? e.message) }
}

async function handleCreate() {
  try {
    await backupStore.create()
    toast.success(t('backups.created_success'))
  } catch (e: any) { toast.error(e.response?.data?.error ?? e.message) }
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
  } catch (e: any) { toast.error(t('backups.download_failed')) }
}

onMounted(() => backupStore.load())
</script>

<style scoped lang="scss">
.filename-cell {
  display: inline-block;
  max-width: 300px;
}
</style>
