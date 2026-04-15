<template>
  <div>
    <PageHeader>
      <button class="btn btn-primary" :disabled="backupStore.creating" @click="handleCreate">
        {{ backupStore.creating ? 'Creating...' : 'Create Backup' }}
      </button>
    </PageHeader>

    <LoadingSpinner v-if="backupStore.loading" message="Loading backups..." />

    <EmptyState v-else-if="!(backupStore.backups && backupStore.backups.length)" icon="💾"
                message="No backups found. Create your first backup." />

    <div v-else class="table-wrapper">
      <table class="table">
        <thead><tr><th>Filename</th><th>Size</th><th>Created</th><th>Actions</th></tr></thead>
        <tbody>
          <tr v-for="b in backupStore.backups" :key="b.filename">
            <td><code class="truncate" style="max-width:300px;display:inline-block">{{ b.filename }}</code></td>
            <td>{{ formatBytes(b.size) }}</td>
            <td>{{ new Date(b.created_at).toLocaleString() }}</td>
            <td>
              <div class="actions-cell">
                <button class="btn btn-ghost btn-sm" title="Download"
                        @click="handleDownload(b.filename)">📥</button>
                <button class="btn btn-warning btn-sm" :disabled="backupStore.restoring"
                        @click="handleRestore(b.filename)">Restore</button>
                <button class="btn btn-ghost btn-sm btn-danger-text" @click="confirmRemove(b.filename)">🗑</button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <ConfirmDialog v-model="confirmModal" title="Remove Backup"
                   :message="`Delete backup '${removeTarget}'?`" confirm-text="Delete" @confirm="handleRemove" />

    <ConfirmDialog v-model="restoreModal" title="Restore Backup"
                   :message="`Restore from '${restoreTarget}'? Current configuration will be overwritten.`"
                   confirm-text="Restore" @confirm="handleRestoreConfirm" />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useBackupStore } from '@/stores/backup'
import { useToastStore } from '@/stores/toast'
import { backupApi } from '@/api/endpoints'
import { formatBytes } from '@/utils/format'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import PageHeader from '@/components/common/PageHeader.vue'
import EmptyState from '@/components/common/EmptyState.vue'

const backupStore = useBackupStore()
const toast = useToastStore()

const confirmModal = ref(false)
const removeTarget = ref('')
function confirmRemove(filename: string) { removeTarget.value = filename; confirmModal.value = true }
async function handleRemove() {
  try {
    await backupStore.remove(removeTarget.value)
    confirmModal.value = false
    toast.success(`Backup "${removeTarget.value}" removed`)
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
    toast.info('Backup restored. Restart the service for changes to take effect.')
  } catch (e: any) {
    toast.error(e.response?.data?.error ?? e.message)
  }
}

async function handleCreate() {
  try {
    await backupStore.create()
    toast.success('Backup created')
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
    toast.error('Failed to download backup')
  }
}

onMounted(() => backupStore.load())
</script>
