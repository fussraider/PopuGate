<template>
  <div>
    <PageHeader>
      <button class="btn btn-primary" @click="addModal.open()">+ {{ t('instances.add_instance') }}</button>
    </PageHeader>

    <DataTable
      :columns="columns"
      :items="store.instances"
      :loading="store.loading"
      :empty-icon="Server"
      :empty-message="t('instances.empty')"
      row-key="port"
    >
      <template #cell-port="{ item }"><code>{{ item.port }}</code></template>
      <template #cell-metrics_port="{ item }"><code>{{ item.metrics_port }}</code></template>
      <template #cell-label="{ item }">{{ item.label || '—' }}</template>
      <template #cell-status="{ item }">
        <StatusBadge :variant="item.enabled ? 'success' : 'neutral'">
          {{ item.enabled ? t('instances.enabled') : t('instances.disabled') }}
        </StatusBadge>
      </template>
      <template #actions="{ item }">
        <button class="btn btn-ghost btn-sm btn-danger-text"
                :disabled="store.instances.length <= 1"
                :title="store.instances.length <= 1 ? t('instances.cannot_delete_last') : ''"
                @click="handleRemove(item.port)">
          <Trash2 :size="16" />
        </button>
      </template>
    </DataTable>

    <FormModal v-model="addModal.isOpen.value" :title="t('instances.add_title')" :submitting="addModal.submitting.value"
               :submit-text="t('common.add')" @submit="addModal.submit(f => store.add(f.port, f.label), t('instances.added_success', { port: addModal.form.value.port }))">
      <div class="form-group mb-md">
        <label class="form-label">{{ t('instances.table.port') }}</label>
        <input v-model.number="addModal.form.value.port" class="input" type="number" required min="1" max="65535" />
      </div>
      <div class="form-group mb-md">
        <label class="form-label">{{ t('instances.table.label') }}</label>
        <input v-model="addModal.form.value.label" class="input" :placeholder="t('instances.instance_placeholder')" />
      </div>
    </FormModal>

    <ConfirmDialog v-bind="confirmState" @confirm="handleConfirm" @cancel="handleCancel" />
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useInstancesStore } from '@/stores/instances'
import { useConfirmDialog } from '@/composables/useConfirmDialog'
import { useFormModal } from '@/composables/useFormModal'
import FormModal from '@/components/common/FormModal.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import DataTable from '@/components/common/DataTable.vue'
import StatusBadge from '@/components/common/StatusBadge.vue'
import PageHeader from '@/components/common/PageHeader.vue'
import { Server, Trash2 } from '@lucide/vue'

const { t } = useI18n()
const store = useInstancesStore()

const columns = [
  { key: 'port', header: t('instances.table.port') },
  { key: 'metrics_port', header: t('instances.table.metrics_port') },
  { key: 'label', header: t('instances.table.label') },
  { key: 'status', header: t('instances.table.status') },
]

const { confirmState, confirm, handleConfirm, handleCancel } = useConfirmDialog()
const addModal = useFormModal({ port: 0, label: '' })

async function handleRemove(port: number) {
  if (!await confirm({ title: t('instances.remove_title'), message: t('instances.confirm_remove', { port }), confirmText: t('common.delete') })) return
  try { await store.remove(port) } catch { /* store handles */ }
}

onMounted(() => store.load())
</script>
