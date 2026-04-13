<template>
  <div>
    <div class="page-header flex justify-between items-center mb-lg">
      <div />
      <button class="btn btn-primary" @click="openAddModal">+ Add Upstream</button>
    </div>

    <LoadingSpinner v-if="store.loading" message="Loading upstreams..." />

    <div v-else-if="!(store.upstreams?.length)" class="card empty-state">
      <div class="empty-icon">🔀</div>
      <p>No upstreams configured. Traffic will go directly.</p>
    </div>

    <div v-else class="table-wrapper">
      <table class="table">
        <thead>
          <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Address</th>
            <th>Weight</th>
            <th>Interface</th>
            <th>Status</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="up in store.upstreams" :key="up.name">
            <td><code>{{ up.name }}</code></td>
            <td>
              <span class="badge badge-info">{{ up.type }}</span>
            </td>
            <td>{{ up.address || 'direct' }}</td>
            <td>{{ up.weight }}</td>
            <td>{{ up.iface || '—' }}</td>
            <td>
              <StatusBadge :variant="up.enabled ? 'success' : 'neutral'">
                {{ up.enabled ? 'Enabled' : 'Disabled' }}
              </StatusBadge>
            </td>
            <td>
              <div class="actions-cell">
                <button class="btn btn-ghost btn-sm" title="Test" @click="testUpstream(up.name)">🧪</button>
                <button class="btn btn-ghost btn-sm" :title="up.enabled ? 'Disable' : 'Enable'"
                        @click="store.toggle(up.name, !up.enabled)">
                  {{ up.enabled ? '⏸' : '▶️' }}
                </button>
                <button class="btn btn-ghost btn-sm" title="Delete" @click="confirmRemove(up.name)">🗑</button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <Modal v-model="addModal" title="Add Upstream">
      <form @submit.prevent="handleAdd">
        <div class="form-group mb-md">
          <label class="form-label">Name</label>
          <input v-model="form.name" class="input" required placeholder="upstream1" />
        </div>
        <div class="form-group mb-md">
          <label class="form-label">Type</label>
          <select v-model="form.type" class="select">
            <option value="direct">Direct</option>
            <option value="socks5">SOCKS5</option>
            <option value="socks4">SOCKS4</option>
          </select>
        </div>
        <div v-if="form.type !== 'direct'" class="form-group mb-md">
          <label class="form-label">Address (host:port)</label>
          <input v-model="form.address" class="input" required placeholder="127.0.0.1:1080" />
        </div>
        <template v-if="form.type === 'socks5'">
          <div class="form-row mb-sm">
            <div class="form-group">
              <label class="form-label">Username</label>
              <input v-model="form.username" class="input" />
            </div>
            <div class="form-group">
              <label class="form-label">Password</label>
              <input v-model="form.password" class="input" type="password" />
            </div>
          </div>
        </template>
        <div class="form-row mb-sm">
          <div class="form-group">
            <label class="form-label">Weight</label>
            <input v-model.number="form.weight" class="input" type="number" min="1" value="1" />
          </div>
          <div class="form-group">
            <label class="form-label">Interface</label>
            <input v-model="form.iface" class="input" placeholder="eth0" />
          </div>
        </div>
        <div class="modal-footer-inline">
          <button type="button" class="btn btn-secondary" @click="addModal = false">Cancel</button>
          <button type="submit" class="btn btn-primary">Add</button>
        </div>
      </form>
    </Modal>

    <ConfirmDialog v-model="confirmModal" title="Remove Upstream"
                   :message="`Remove upstream '${removeTarget}'?`" confirm-text="Remove"
                   @confirm="handleRemove" />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useUpstreamsStore } from '@/stores/upstreams'
import { useToastStore } from '@/stores/toast'
import Modal from '@/components/common/Modal.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import StatusBadge from '@/components/common/StatusBadge.vue'

const store = useUpstreamsStore()
const toast = useToastStore()

const addModal = ref(false)
const form = ref({ name: '', type: 'direct' as string, address: '', username: '', password: '', weight: 1, iface: '' })

function openAddModal() { form.value = { name: '', type: 'direct', address: '', username: '', password: '', weight: 1, iface: '' }; addModal.value = true }

async function handleAdd() {
  try {
    await store.add(form.value as any)
    addModal.value = false
    toast.success(`Upstream "${form.value.name}" added`)
  } catch (e: any) {
    toast.error(e.response?.data?.error ?? e.message)
  }
}

const confirmModal = ref(false)
const removeTarget = ref('')

function confirmRemove(name: string) { removeTarget.value = name; confirmModal.value = true }

async function handleRemove() {
  try {
    await store.remove(removeTarget.value)
    confirmModal.value = false
    toast.success(`Upstream "${removeTarget.value}" removed`)
  } catch (e: any) {
    toast.error(e.response?.data?.error ?? e.message)
  }
}

async function testUpstream(name: string) {
  try {
    await store.test(name)
    toast.success(`Upstream "${name}" is reachable`)
  } catch (e: any) {
    toast.error(`Upstream "${name}" unreachable: ${e.response?.data?.error ?? e.message}`)
  }
}

onMounted(() => store.load())
</script>

<style scoped lang="scss">
@use '@/assets/scss/variables' as *;
.actions-cell { display: flex; gap: 2px; }
</style>
