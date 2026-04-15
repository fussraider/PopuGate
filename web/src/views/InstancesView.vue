<template>
  <div>
    <PageHeader>
      <button class="btn btn-primary" @click="openAddModal">+ Add Instance</button>
    </PageHeader>

    <LoadingSpinner v-if="store.loading" message="Loading instances..." />

    <EmptyState v-else-if="!(store.instances && store.instances.length)" icon="🖥"
                message="No additional instances. Only the primary proxy is running." />

    <div v-else class="table-wrapper">
      <table class="table">
        <thead><tr><th>Port</th><th>Metrics Port</th><th>Label</th><th>Status</th><th>Actions</th></tr></thead>
        <tbody>
          <tr v-for="inst in store.instances" :key="inst.port">
            <td><code>{{ inst.port }}</code></td>
            <td><code>{{ inst.metrics_port }}</code></td>
            <td>{{ inst.label || '—' }}</td>
            <td>
              <StatusBadge :variant="inst.enabled ? 'success' : 'neutral'">
                {{ inst.enabled ? 'Enabled' : 'Disabled' }}
              </StatusBadge>
            </td>
            <td>
              <button class="btn btn-ghost btn-sm btn-danger-text" @click="confirmRemove(inst.port)">🗑</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <Modal v-model="addModal" title="Add Instance">
      <form @submit.prevent="handleAdd">
        <div class="form-group mb-md">
          <label class="form-label">Port</label>
          <input v-model.number="form.port" class="input" type="number" required min="1" max="65535" />
        </div>
        <div class="form-group mb-md">
          <label class="form-label">Label</label>
          <input v-model="form.label" class="input" placeholder="instance1" />
        </div>
        <div class="modal-footer" style="padding:0;border:none;">
          <button type="button" class="btn btn-secondary" @click="addModal = false">Cancel</button>
          <button type="submit" class="btn btn-primary">Add</button>
        </div>
      </form>
    </Modal>

    <ConfirmDialog v-model="confirmModal" title="Remove Instance"
                   :message="`Remove instance on port ${removeTarget}?`" confirm-text="Remove"
                   @confirm="handleRemove" />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useInstancesStore } from '@/stores/instances'
import { useToastStore } from '@/stores/toast'
import Modal from '@/components/common/Modal.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import StatusBadge from '@/components/common/StatusBadge.vue'
import PageHeader from '@/components/common/PageHeader.vue'
import EmptyState from '@/components/common/EmptyState.vue'

const store = useInstancesStore()
const toast = useToastStore()

const addModal = ref(false)
const form = ref({ port: 0, label: '' })
const confirmModal = ref(false)
const removeTarget = ref(0)

function openAddModal() { form.value = { port: 0, label: '' }; addModal.value = true }

async function handleAdd() {
  try {
    await store.add(form.value.port, form.value.label)
    addModal.value = false
    toast.success(`Instance on port ${form.value.port} added`)
  } catch (e: any) {
    toast.error(e.response?.data?.error ?? e.message)
  }
}

function confirmRemove(port: number) { removeTarget.value = port; confirmModal.value = true }

async function handleRemove() {
  try {
    await store.remove(removeTarget.value)
    confirmModal.value = false
    toast.success(`Instance on port ${removeTarget.value} removed`)
  } catch (e: any) {
    toast.error(e.response?.data?.error ?? e.message)
  }
}

onMounted(() => store.load())
</script>
