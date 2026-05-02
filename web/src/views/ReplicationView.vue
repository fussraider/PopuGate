<template>
  <div>
    <!-- Role & Status -->
    <div class="card mb-lg">
      <h3 class="mb-md">{{ t('replication.title') }}</h3>
      <div class="status-row mb-md">
        <StatusBadge :variant="statusBadgeVariant">
          {{ replicationStore.status?.role || 'standalone' }}
        </StatusBadge>
      </div>

      <div class="form-row mb-md">
        <div class="form-group">
          <label class="form-label">{{ t('replication.role') }}</label>
          <select v-model="roleForm.role" class="select">
            <option value="standalone">{{ t('replication.standalone') }}</option>
            <option value="master">{{ t('replication.master') }}</option>
            <option value="slave">{{ t('replication.slave') }}</option>
          </select>
        </div>
        <div class="form-group">
          <label class="form-label">{{ t('replication.sync_interval') }}</label>
          <input v-model.number="roleForm.interval" class="input" type="number" min="10" />
        </div>
      </div>
      <button class="btn btn-primary" :disabled="replicationStore.settingUp" @click="handleSetupRole">
        <Loader2 v-if="replicationStore.settingUp" :size="16" class="animate-spin" />
        {{ t('replication.apply') }}
      </button>
    </div>

    <!-- SSH Key -->
    <div class="card mb-lg">
      <h3 class="mb-md">{{ t('replication.ssh_title') }}</h3>
      <div class="flex gap-sm items-center mb-sm">
        <button class="btn btn-secondary btn-sm" :disabled="replicationStore.generatingKey" @click="handleSSHKeygen">
          <Loader2 v-if="replicationStore.generatingKey" :size="14" class="animate-spin" />
          {{ t('replication.generate_key') }}
        </button>
        <button v-if="publicKey" class="btn btn-ghost btn-sm" @click="copyKey">{{ t('replication.copy') }}</button>
      </div>
      <code v-if="publicKey" class="ssh-key">{{ publicKey }}</code>
      <span v-else class="text-muted">{{ t('replication.no_key') }}</span>
    </div>

    <!-- Slaves -->
    <div class="card mb-lg">
      <div class="flex justify-between items-center mb-md">
        <h3>{{ t('replication.slaves_title') }}</h3>
        <button class="btn btn-primary btn-sm" @click="slaveModal.open()">+ {{ t('replication.add_slave') }}</button>
      </div>

      <DataTable
        :columns="slaveColumns"
        :items="replicationStore.slaves ?? []"
        :loading="false"
        :empty-message="t('replication.no_slaves')"
        row-key="host"
      >
        <template #cell-host="{ item }"><code>{{ item.host }}</code></template>
        <template #cell-port="{ item }">{{ item.port }}</template>
        <template #cell-label="{ item }">{{ item.label || '—' }}</template>
        <template #cell-last_sync="{ item }">
          {{ item.last_sync ? new Date(item.last_sync * 1000).toLocaleString() : t('replication.never') }}
        </template>
        <template #cell-status="{ item }">
          <StatusBadge :variant="item.status === 'ok' ? 'success' : 'danger'">
            {{ item.status || 'unknown' }}
          </StatusBadge>
        </template>
        <template #actions="{ item }">
          <button class="btn btn-ghost btn-sm" :title="t('replication.test')" @click="testSlave(item.host)">
            <FlaskConical :size="16" />
          </button>
          <button class="btn btn-ghost btn-sm" :title="t('replication.sync')" :disabled="replicationStore.syncing"
                  @click="replicationStore.sync(item.host)">
            <RefreshCw :size="16" :class="{ 'animate-spin': replicationStore.syncing }" />
          </button>
          <button class="btn btn-ghost btn-sm" :title="t('replication.remove')" @click="handleRemove(item.host)">
            <Trash2 :size="16" class="text-danger" />
          </button>
        </template>
      </DataTable>
    </div>

    <!-- Test Results -->
    <div v-if="testResult" class="card mb-lg">
      <h3 class="mb-md">{{ t('replication.test_result') }}</h3>
      <div class="alert" :class="testResult.ssh_ok ? 'alert-success' : 'alert-danger'">
        {{ t('replication.ssh_ok') }}: {{ testResult.ssh_ok ? 'OK' : 'Failed' }}<br />
        <span v-if="testResult.docker_status">Docker: {{ testResult.docker_status }}</span>
        <span v-if="testResult.error">{{ testResult.error }}</span>
      </div>
    </div>

    <!-- Add Slave Modal -->
    <FormModal v-model="slaveModal.isOpen.value" :title="t('replication.add_slave_title')" :submitting="slaveModal.submitting.value"
               :submit-text="t('common.add')" @submit="slaveModal.submit(f => replicationStore.addSlave(f.host, f.port, f.label))">
      <div class="form-group mb-md">
        <label class="form-label">{{ t('replication.table.host') }}</label>
        <input v-model="slaveModal.form.value.host" class="input" required :placeholder="t('replication.host_placeholder')" />
      </div>
      <div class="form-row mb-md">
        <div class="form-group">
          <label class="form-label">{{ t('replication.table.port') }}</label>
          <input v-model.number="slaveModal.form.value.port" class="input" type="number" value="22" />
        </div>
        <div class="form-group">
          <label class="form-label">{{ t('replication.table.label') }}</label>
          <input v-model="slaveModal.form.value.label" class="input" :placeholder="t('replication.slave_placeholder')" />
        </div>
      </div>
    </FormModal>

    <ConfirmDialog v-bind="confirmState" @confirm="handleConfirm" @cancel="handleCancel" />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useReplicationStore } from '@/stores/replication'
import { useToastStore } from '@/stores/toast'
import { useConfirmDialog } from '@/composables/useConfirmDialog'
import { useFormModal } from '@/composables/useFormModal'
import FormModal from '@/components/common/FormModal.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import DataTable from '@/components/common/DataTable.vue'
import StatusBadge from '@/components/common/StatusBadge.vue'
import { FlaskConical, RefreshCw, Trash2, Loader2 } from '@lucide/vue'

const { t } = useI18n()
const replicationStore = useReplicationStore()
const toast = useToastStore()

const slaveColumns = [
  { key: 'host', header: t('replication.table.host') },
  { key: 'port', header: t('replication.table.port') },
  { key: 'label', header: t('replication.table.label') },
  { key: 'last_sync', header: t('replication.table.last_sync') },
  { key: 'status', header: t('replication.table.status') },
]

const roleForm = ref({ role: 'standalone', interval: 60 })
const publicKey = ref('')
const testResult = ref<any>(null)

const statusBadgeVariant = computed(() => {
  const role = replicationStore.status?.role
  return role === 'master' ? 'success' : role === 'slave' ? 'info' : 'neutral'
})

const { confirmState, confirm, handleConfirm, handleCancel } = useConfirmDialog()
const slaveModal = useFormModal({ host: '', port: 22, label: '' })

async function handleSetupRole() {
  try {
    await replicationStore.setup(roleForm.value.role, roleForm.value.interval)
    toast.success(t('replication.setup_success'))
  } catch (e: any) { toast.error(e.response?.data?.error ?? e.message) }
}

async function handleSSHKeygen() {
  try { publicKey.value = await replicationStore.sshKeygen() }
  catch (e: any) { toast.error(e.response?.data?.error ?? e.message) }
}

function copyKey() { navigator.clipboard.writeText(publicKey.value) }

async function testSlave(host: string) {
  try { testResult.value = await replicationStore.test(host) }
  catch (e: any) { toast.error(e.response?.data?.error ?? e.message) }
}

async function handleRemove(host: string) {
  if (!await confirm({ title: t('replication.remove_slave_title'), message: t('replication.confirm_remove', { label: host }), confirmText: t('replication.remove') })) return
  try { await replicationStore.removeSlave(host) } catch (e: any) { toast.error(e.response?.data?.error ?? e.message) }
}

onMounted(async () => {
  replicationStore.loadStatus()
  replicationStore.loadSlaves()
  publicKey.value = await replicationStore.loadSSHPublicKey()
})
</script>

<style scoped lang="scss">
@use '@/assets/scss/variables' as *;

.ssh-key { display: block; word-break: break-all; font-size: $font-size-xs; background: $color-gray-100; padding: $spacing-sm; border-radius: $border-radius-sm; }
</style>
