<template>
  <div>
    <PageHeader>
      <button class="btn btn-primary" @click="openAddModal">+ {{ t('instances.add_instance') }}</button>
    </PageHeader>

    <LoadingSpinner v-if="store.loading" :message="t('instances.loading')" />

    <EmptyState v-else-if="!(store.instances && store.instances.length)" :icon="Server"
                :message="t('instances.empty')" />

    <div v-else class="table-wrapper">
      <table class="table">
        <thead>
          <tr>
            <th>{{ t('instances.table.port') }}</th>
            <th>{{ t('instances.table.metrics_port') }}</th>
            <th>{{ t('instances.table.label') }}</th>
            <th>{{ t('instances.table.status') }}</th>
            <th>{{ t('instances.table.actions') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="inst in store.instances" :key="inst.port">
            <td><code>{{ inst.port }}</code></td>
            <td><code>{{ inst.metrics_port }}</code></td>
            <td>{{ inst.label || '—' }}</td>
            <td>
              <StatusBadge :variant="inst.enabled ? 'success' : 'neutral'">
                {{ inst.enabled ? t('instances.enabled') : t('instances.disabled') }}
              </StatusBadge>
            </td>
            <td>
              <button class="btn btn-ghost btn-sm btn-danger-text" @click="confirmRemove(inst.port)">
                <Trash2 :size="16" />
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <Modal v-model="addModal" :title="t('instances.add_title')">
      <form @submit.prevent="handleAdd">
        <div class="form-group mb-md">
          <label class="form-label">{{ t('instances.table.port') }}</label>
          <input v-model.number="form.port" class="input" type="number" required min="1" max="65535" />
        </div>
        <div class="form-group mb-md">
          <label class="form-label">{{ t('instances.table.label') }}</label>
          <input v-model="form.label" class="input" :placeholder="t('instances.instance_placeholder')" />
        </div>
        <div class="modal-footer" style="padding:0;border:none;">
          <button type="button" class="btn btn-secondary" @click="addModal = false">{{ t('common.cancel') }}</button>
          <button type="submit" class="btn btn-primary" :disabled="store.loading">
            <Loader2 v-if="store.loading" :size="16" class="animate-spin" />
            {{ t('common.add') }}
          </button>
        </div>
      </form>
    </Modal>

    <ConfirmDialog v-model="confirmModal" :title="t('instances.remove_title')"
                   :message="t('instances.confirm_remove', { port: removeTarget })" :confirm-text="t('common.delete')"
                   @confirm="handleRemove" />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useInstancesStore } from '@/stores/instances'
import { useToastStore } from '@/stores/toast'
import Modal from '@/components/common/Modal.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import StatusBadge from '@/components/common/StatusBadge.vue'
import PageHeader from '@/components/common/PageHeader.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import { Server, Trash2, Loader2 } from '@lucide/vue'

const { t } = useI18n()
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
    toast.success(t('instances.added_success', { port: form.value.port }))
  } catch (e: any) {
    toast.error(e.response?.data?.error ?? e.message)
  }
}

function confirmRemove(port: number) { removeTarget.value = port; confirmModal.value = true }

async function handleRemove() {
  try {
    await store.remove(removeTarget.value)
    confirmModal.value = false
    toast.success(t('instances.removed_success', { port: removeTarget.value }))
  } catch (e: any) {
    toast.error(e.response?.data?.error ?? e.message)
  }
}

onMounted(() => store.load())
</script>
