<template>
  <div>
    <div class="page-header flex justify-between items-center mb-lg">
      <div />
      <button class="btn btn-primary" :disabled="backupStore.creating" @click="handleCreate">
        {{ backupStore.creating ? 'Creating...' : 'Create Backup' }}
      </button>
    </div>

    <LoadingSpinner v-if="backupStore.loading" message="Loading backups..." />

    <div v-else-if="!(backupStore.backups && backupStore.backups.length)" class="card empty-state">
      <div class="empty-icon">💾</div>
      <p>No backups found. Create your first backup.</p>
    </div>

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
import { backupApi } from '@/api/endpoints'
import { formatBytes } from '@/utils/format'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'

const backupStore = useBackupStore()

const confirmModal = ref(false)
const removeTarget = ref('')
function confirmRemove(filename: string) { removeTarget.value = filename; confirmModal.value = true }
async function handleRemove() { await backupStore.remove(removeTarget.value); confirmModal.value = false }

const restoreModal = ref(false)
const restoreTarget = ref('')
function handleRestore(filename: string) { restoreTarget.value = filename; restoreModal.value = true }
async function handleRestoreConfirm() {
  await backupStore.restore(restoreTarget.value)
  restoreModal.value = false
  alert('Backup restored. Restart the service for changes to take effect.')
}

async function handleCreate() {
  await backupStore.create()
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
  } catch (err) {
    console.error('Download failed:', err)
    alert('Failed to download backup')
  }
}

onMounted(() => backupStore.load())
</script>

<style scoped lang="scss">
@use '@/assets/scss/variables' as *;
.actions-cell { display: flex; gap: $spacing-sm; }
.btn-danger-text { color: $color-danger; }
</style>
