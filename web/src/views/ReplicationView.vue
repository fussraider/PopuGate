<template>
  <div>
    <!-- Role & Status -->
    <div class="card mb-lg">
      <h3 class="mb-md">Replication</h3>
      <div class="status-row mb-md">
        <StatusBadge :variant="statusBadgeVariant">
          {{ replicationStore.status?.role || 'standalone' }}
        </StatusBadge>
      </div>

      <div class="form-row mb-md">
        <div class="form-group">
          <label class="form-label">Role</label>
          <select v-model="roleForm.role" class="select">
            <option value="standalone">Standalone</option>
            <option value="master">Master</option>
            <option value="slave">Slave</option>
          </select>
        </div>
        <div class="form-group">
          <label class="form-label">Sync Interval (seconds)</label>
          <input v-model.number="roleForm.interval" class="input" type="number" min="10" />
        </div>
      </div>
      <button class="btn btn-primary" @click="handleSetupRole">Apply</button>
    </div>

    <!-- SSH Key -->
    <div class="card mb-lg">
      <h3 class="mb-md">SSH Public Key</h3>
      <div class="flex gap-sm items-center mb-sm">
        <button class="btn btn-secondary btn-sm" @click="handleSSHKeygen">Generate Key</button>
        <button v-if="publicKey" class="btn btn-ghost btn-sm" @click="copyKey">Copy</button>
      </div>
      <code v-if="publicKey" class="ssh-key">{{ publicKey }}</code>
      <span v-else class="text-muted">No key generated yet.</span>
    </div>

    <!-- Slaves -->
    <div class="card mb-lg">
      <div class="flex justify-between items-center mb-md">
        <h3>Slaves</h3>
        <button class="btn btn-primary btn-sm" @click="openAddSlave">+ Add Slave</button>
      </div>

      <div v-if="!(replicationStore.slaves && replicationStore.slaves.length)" class="text-muted text-sm">No slaves configured.</div>

      <div v-else class="table-wrapper">
        <table class="table">
          <thead><tr><th>Host</th><th>Port</th><th>Label</th><th>Last Sync</th><th>Status</th><th>Actions</th></tr></thead>
          <tbody>
            <tr v-for="sl in replicationStore.slaves" :key="sl.host">
              <td><code>{{ sl.host }}</code></td>
              <td>{{ sl.port }}</td>
              <td>{{ sl.label || '—' }}</td>
              <td>{{ sl.last_sync ? new Date(sl.last_sync * 1000).toLocaleString() : 'Never' }}</td>
              <td>
                <StatusBadge :variant="sl.status === 'ok' ? 'success' : 'danger'">
                  {{ sl.status || 'unknown' }}
                </StatusBadge>
              </td>
              <td>
                <div class="actions-cell">
                  <button class="btn btn-ghost btn-sm" title="Test" @click="testSlave(sl.host)">🧪</button>
                  <button class="btn btn-ghost btn-sm" title="Sync" :disabled="replicationStore.syncing"
                          @click="replicationStore.sync(sl.host)">🔄</button>
                  <button class="btn btn-ghost btn-sm" title="Remove" @click="confirmRemove(sl.host)">🗑</button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Test Results -->
    <div v-if="testResult" class="card mb-lg">
      <h3 class="mb-md">Test Result</h3>
      <div class="alert" :class="testResult.ssh_ok ? 'alert-success' : 'alert-danger'">
        SSH: {{ testResult.ssh_ok ? 'OK' : 'Failed' }}<br />
        <span v-if="testResult.docker_status">Docker: {{ testResult.docker_status }}</span>
        <span v-if="testResult.error">{{ testResult.error }}</span>
      </div>
    </div>

    <!-- Add Slave Modal -->
    <Modal v-model="addSlaveModal" title="Add Slave">
      <form @submit.prevent="handleAddSlave">
        <div class="form-group mb-md">
          <label class="form-label">Host</label>
          <input v-model="slaveForm.host" class="input" required placeholder="192.168.1.2" />
        </div>
        <div class="form-row mb-md">
          <div class="form-group">
            <label class="form-label">Port</label>
            <input v-model.number="slaveForm.port" class="input" type="number" value="22" />
          </div>
          <div class="form-group">
            <label class="form-label">Label</label>
            <input v-model="slaveForm.label" class="input" placeholder="slave1" />
          </div>
        </div>
        <div class="modal-footer" style="padding:0;border:none;">
          <button type="button" class="btn btn-secondary" @click="addSlaveModal = false">Cancel</button>
          <button type="submit" class="btn btn-primary">Add</button>
        </div>
      </form>
    </Modal>

    <ConfirmDialog v-model="confirmModal" title="Remove Slave"
                   :message="`Remove slave '${removeTarget}'?`" confirm-text="Remove" @confirm="handleRemove" />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useReplicationStore } from '@/stores/replication'
import Modal from '@/components/common/Modal.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import StatusBadge from '@/components/common/StatusBadge.vue'

const replicationStore = useReplicationStore()

const roleForm = ref({ role: 'standalone', interval: 60 })
const publicKey = ref('')
const testResult = ref<any>(null)

const statusBadgeVariant = computed(() => {
  const role = replicationStore.status?.role
  return role === 'master' ? 'success' : role === 'slave' ? 'info' : 'neutral'
})

async function handleSetupRole() {
  await replicationStore.setup(roleForm.value.role, roleForm.value.interval)
}

async function handleSSHKeygen() {
  publicKey.value = await replicationStore.sshKeygen()
}

function copyKey() {
  navigator.clipboard.writeText(publicKey.value)
}

async function testSlave(host: string) {
  testResult.value = await replicationStore.test(host)
}

const addSlaveModal = ref(false)
const slaveForm = ref({ host: '', port: 22, label: '' })

function openAddSlave() { slaveForm.value = { host: '', port: 22, label: '' }; addSlaveModal.value = true }
async function handleAddSlave() {
  await replicationStore.addSlave(slaveForm.value.host, slaveForm.value.port, slaveForm.value.label)
  addSlaveModal.value = false
}

const confirmModal = ref(false)
const removeTarget = ref('')
function confirmRemove(host: string) { removeTarget.value = host; confirmModal.value = true }
async function handleRemove() { await replicationStore.removeSlave(removeTarget.value); confirmModal.value = false }

onMounted(() => {
  replicationStore.loadStatus()
  replicationStore.loadSlaves()
})
</script>

<style scoped lang="scss">
@use '@/assets/scss/variables' as *;

.ssh-key { display: block; word-break: break-all; font-size: $font-size-xs; background: $color-gray-100; padding: $spacing-sm; border-radius: $border-radius-sm; }
</style>
