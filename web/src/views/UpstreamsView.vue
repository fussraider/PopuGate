<template>
  <div>
    <PageHeader>
      <button class="btn btn-primary" @click="openAddModal">+ {{ t('upstreams.add') }}</button>
    </PageHeader>

    <LoadingSpinner v-if="store.loading" :message="t('upstreams.loading')" />

    <EmptyState v-else-if="!(store.upstreams?.length)" :icon="GitBranch"
                :message="t('upstreams.empty')" />

    <div v-else class="table-wrapper">
      <table class="table">
        <thead>
          <tr>
            <th>{{ t('upstreams.table.name') }}</th>
            <th>{{ t('upstreams.table.type') }}</th>
            <th>{{ t('upstreams.table.address') }}</th>
            <th>{{ t('upstreams.table.weight') }}</th>
            <th>{{ t('upstreams.table.interface') }}</th>
            <th>{{ t('upstreams.table.status') }}</th>
            <th>{{ t('upstreams.table.actions') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="up in store.upstreams" :key="up.name">
            <td><code>{{ up.name }}</code></td>
            <td>
              <span class="badge badge-info">{{ up.type }}</span>
            </td>
            <td>{{ up.address || t('upstreams.direct') }}</td>
            <td>{{ up.weight }}</td>
            <td>{{ up.iface || '—' }}</td>
            <td>
              <StatusBadge :variant="up.enabled ? 'success' : 'neutral'">
                {{ up.enabled ? t('instances.enabled') : t('instances.disabled') }}
              </StatusBadge>
            </td>
            <td>
              <div class="actions-cell">
                <button class="btn btn-ghost btn-sm" :title="t('upstreams.test')"
                        :disabled="store.testing === up.name" @click="testUpstream(up.name)">
                  <Loader2 v-if="store.testing === up.name" :size="16" class="animate-spin" />
                  <FlaskConical v-else :size="16" />
                </button>
                <button class="btn btn-ghost btn-sm" :title="up.enabled ? t('secrets.disable') : t('secrets.enable')"
                        :disabled="store.toggling === up.name" @click="store.toggle(up.name, !up.enabled)">
                  <Loader2 v-if="store.toggling === up.name" :size="16" class="animate-spin" />
                  <component v-else :is="up.enabled ? Pause : Play" :size="16" />
                </button>
                <button class="btn btn-ghost btn-sm" :title="t('upstreams.delete')" @click="confirmRemove(up.name)">
                  <Trash2 :size="16" />
                </button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <Modal v-model="addModal" :title="t('upstreams.add_title')">
      <form @submit.prevent="handleAdd">
        <div class="form-group mb-md">
          <label class="form-label">{{ t('upstreams.table.name') }}</label>
          <input v-model="form.name" class="input" required placeholder="upstream1" />
        </div>
        <div class="form-group mb-md">
          <label class="form-label">{{ t('upstreams.table.type') }}</label>
          <select v-model="form.type" class="select">
            <option value="direct">{{ t('upstreams.direct') }}</option>
            <option value="socks5">SOCKS5</option>
            <option value="socks4">SOCKS4</option>
          </select>
        </div>
        <div v-if="form.type !== 'direct'" class="form-group mb-md">
          <label class="form-label">{{ t('upstreams.address_label') }}</label>
          <input v-model="form.address" class="input" required placeholder="127.0.0.1:1080" />
        </div>
        <template v-if="form.type === 'socks5'">
          <div class="form-row mb-sm">
            <div class="form-group">
              <label class="form-label">{{ t('upstreams.username') }}</label>
              <input v-model="form.username" class="input" />
            </div>
            <div class="form-group">
              <label class="form-label">{{ t('upstreams.password') }}</label>
              <input v-model="form.password" class="input" type="password" />
            </div>
          </div>
        </template>
        <div class="form-row mb-sm">
          <div class="form-group">
            <label class="form-label">{{ t('upstreams.table.weight') }}</label>
            <input v-model.number="form.weight" class="input" type="number" min="1" value="1" />
          </div>
          <div class="form-group">
            <label class="form-label">{{ t('upstreams.table.interface') }}</label>
            <select v-model="form.iface" class="select">
              <option value="">Auto</option>
              <option v-for="nic in store.interfaces" :key="nic.name" :value="nic.name">
                {{ nic.name }} <template v-if="nic.addresses.length">( {{ nic.addresses[0] }} )</template>
              </option>
            </select>
          </div>
        </div>
        <div class="modal-footer-inline">
          <button type="button" class="btn btn-secondary" @click="addModal = false">{{ t('common.cancel') }}</button>
          <button type="button" class="btn btn-secondary" :disabled="store.testingConfig" @click="handleTestConfig">
            <Loader2 v-if="store.testingConfig" :size="16" class="animate-spin" />
            <FlaskConical v-else :size="16" />
            {{ t('upstreams.test_config') }}
          </button>
          <button type="submit" class="btn btn-primary">{{ t('common.add') }}</button>
        </div>
        <div v-if="store.testResult" class="test-result" :class="store.testResult.ok ? 'test-ok' : 'test-fail'">
          <template v-if="store.testResult.ok">
            {{ t('upstreams.test_ok') }} <template v-if="store.testResult.exit_ip">— {{ store.testResult.exit_ip }}</template>
            <span v-if="store.testResult.latency_ms" class="test-latency">({{ store.testResult.latency_ms }}ms)</span>
          </template>
          <template v-else>
            {{ t('upstreams.test_fail') }}: {{ store.testResult.error }}
          </template>
        </div>
      </form>
    </Modal>

    <ConfirmDialog v-model="confirmModal" :title="t('upstreams.remove_title')"
                   :message="t('upstreams.confirm_remove', { name: removeTarget })" :confirm-text="t('upstreams.delete')"
                   @confirm="handleRemove" />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useUpstreamsStore } from '@/stores/upstreams'
import { useToastStore } from '@/stores/toast'
import Modal from '@/components/common/Modal.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import StatusBadge from '@/components/common/StatusBadge.vue'
import PageHeader from '@/components/common/PageHeader.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import { GitBranch, FlaskConical, Play, Pause, Trash2, Loader2 } from '@lucide/vue'

const { t } = useI18n()
const store = useUpstreamsStore()
const toast = useToastStore()

const addModal = ref(false)
const form = ref({ name: '', type: 'direct' as string, address: '', username: '', password: '', weight: 1, iface: '' })

async function openAddModal() {
  form.value = { name: '', type: 'direct', address: '', username: '', password: '', weight: 1, iface: '' }
  store.testResult = null
  addModal.value = true
  try { await store.loadInterfaces() } catch { /* non-critical */ }
}

function handleTestConfig() {
  store.testConfig({
    type: form.value.type,
    address: form.value.address,
    username: form.value.username,
    password: form.value.password,
    iface: form.value.iface,
  })
}

async function handleAdd() {
  try {
    await store.add(form.value as any)
    addModal.value = false
    toast.success(t('upstreams.added_success', { name: form.value.name }))
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
    toast.success(t('upstreams.removed_success', { name: removeTarget.value }))
  } catch (e: any) {
    toast.error(e.response?.data?.error ?? e.message)
  }
}

async function testUpstream(name: string) {
  try {
    const result = await store.test(name)
    if (result?.ok) {
      const parts: string[] = []
      if (result.latency_ms) parts.push(`${result.latency_ms}ms`)
      if (result.exit_ip) parts.push(`IP: ${result.exit_ip}`)
      const extra = parts.length ? ` (${parts.join(', ')})` : ''
      toast.success(t('upstreams.test_success', { name, extra }))
    } else {
      toast.error(t('upstreams.test_failed', { name, error: result?.error ?? 'Unknown error' }))
    }
  } catch (e: any) {
    toast.error(t('upstreams.test_failed', { name, error: e.response?.data?.error ?? e.message }))
  }
}

onMounted(() => store.load())
</script>
