<template>
  <div>
    <PageHeader>
      <button class="btn btn-primary" :disabled="backupStore.creating" @click="handleCreate">
        <Loader2 v-if="backupStore.creating" :size="16" class="animate-spin" />
        {{ backupStore.creating ? t('backups.creating') : t('backups.create') }}
      </button>
    </PageHeader>

    <LoadingSpinner v-if="backupStore.loading" :message="t('backups.loading')" />

    <EmptyState v-else-if="!(backupStore.backups && backupStore.backups.length)" :icon="Save"
                :message="t('backups.empty')" />

    <div v-else class="table-wrapper">
      <table class="table">
        <thead>
          <tr>
            <th>{{ t('backups.table.filename') }}</th>
            <th>{{ t('backups.table.size') }}</th>
            <th>{{ t('backups.table.created') }}</th>
            <th>{{ t('backups.table.actions') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="b in backupStore.backups" :key="b.filename">
            <td><code class="truncate" style="max-width:300px;display:inline-block">{{ b.filename }}</code></td>
            <td>{{ formatBytes(b.size) }}</td>
            <td>{{ new Date(b.created_at).toLocaleString() }}</td>
            <td>
              <div class="actions-cell">
                <button class="btn btn-ghost btn-sm" :title="t('backups.download')"
                        @click="handleDownload(b.filename)">
                  <Download :size="16" />
                </button>
                <button class="btn btn-warning btn-sm" :disabled="backupStore.restoring"
                        @click="handleRestore(b.filename)">
                  <Loader2 v-if="backupStore.restoring" :size="14" class="animate-spin" />
                  {{ t('backups.restore') }}
                </button>
                <button class="btn btn-ghost btn-sm btn-danger-text" @click="confirmRemove(b.filename)">
                  <Trash2 :size="16" />
                </button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <ConfirmDialog v-model="confirmModal" :title="t('backups.remove_title')"
                   :message="t('backups.confirm_remove', { label: removeTarget })" :confirm-text="t('common.delete')" @confirm="handleRemove" />

    <ConfirmDialog v-model="restoreModal" :title="t('backups.restore_title')"
                   :message="t('backups.confirm_restore', { label: restoreTarget })"
                   :confirm-text="t('backups.restore')" @confirm="handleRestoreConfirm" />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useBackupStore } from '@/stores/backup'
import { useToastStore } from '@/stores/toast'
import { backupApi } from '@/api/endpoints'
import { formatBytes } from '@/utils/format'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import PageHeader from '@/components/common/PageHeader.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import { Save, Download, Trash2, Loader2 } from '@lucide/vue'

const { t } = useI18n()
const backupStore = useBackupStore()
const toast = useToastStore()

const confirmModal = ref(false)
const removeTarget = ref('')
function confirmRemove(filename: string) { removeTarget.value = filename; confirmModal.value = true }
async function handleRemove() {
  try {
    await backupStore.remove(removeTarget.value)
    confirmModal.value = false
    toast.success(t('backups.removed_success', { label: removeTarget.value }))
  } catch (e: any) {
    toast.error(e.response?.data?.error ?? e.message)
  }
}

const restoreModal = ref(false)
const restoreTarget = ref('')
function handleRestore(filename: string) { restoreTarget.value = filename; restoreModal.value = true }
async function handleRestoreConfirm() {
  try {
    await backupStore.restore(restoreTarget.value)
    restoreModal.value = false
    toast.info(t('backups.restored_success'))
  } catch (e: any) {
    toast.error(e.response?.data?.error ?? e.message)
  }
}

async function handleCreate() {
  try {
    await backupStore.create()
    toast.success(t('backups.created_success'))
  } catch (e: any) {
    toast.error(e.response?.data?.error ?? e.message)
  }
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
  } catch (e: any) {
    toast.error(t('backups.download_failed'))
  }
}

onMounted(() => backupStore.load())
</script>
