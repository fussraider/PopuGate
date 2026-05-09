<template>
  <div>
    <PageHeader>
      <button class="btn btn-primary" @click="openAddModal">+ {{ t('upstreams.add') }}</button>
    </PageHeader>

    <DataTable
      :columns="columns"
      :items="store.upstreams"
      :loading="store.loading"
      :empty-icon="GitBranch"
      :empty-message="t('upstreams.empty')"
      row-key="name"
    >
      <template #cell-name="{ item }"><code>{{ item.name }}</code></template>
      <template #cell-type="{ item }">
        <span class="badge badge-info">{{ item.type }}</span>
      </template>
      <template #cell-address="{ item }">{{ item.address || t('upstreams.direct') }}</template>
      <template #cell-weight="{ item }">{{ item.weight }}</template>
      <template #cell-interface="{ item }">{{ item.iface || '—' }}</template>
      <template #cell-health="{ item }">
        <template v-if="store.checkingHealth.has(item.name)">
          <StatusBadge variant="neutral">
            <Loader2 :size="12" class="animate-spin" /> {{ t('upstreams.health_checking') }}
          </StatusBadge>
        </template>
        <template v-else>
          <StatusBadge :variant="getHealthVariant(item.last_check_ok)">
            {{ getHealthLabel(item.last_check_ok) }}
          </StatusBadge>
          <div v-if="item.latency_ms" class="text-xs text-muted mt-xs">
            {{ item.latency_ms }}ms
          </div>
        </template>
      </template>
      <template #cell-status="{ item }">
        <StatusBadge :variant="item.enabled ? 'success' : 'neutral'">
          {{ item.enabled ? t('instances.enabled') : t('instances.disabled') }}
        </StatusBadge>
      </template>
      <template #mobile-actions="{ item }">
        <button class="btn btn-ghost btn-sm" @click="upstreamActions.open(item)">
          <MoreVertical :size="16" />
        </button>
      </template>
      <template #actions="{ item }">
        <div class="actions-desktop">
          <button class="btn btn-ghost btn-sm" v-tooltip="t('upstreams.test')"
                  :disabled="store.testing === item.name" @click="testUpstream(item.name)">
            <Loader2 v-if="store.testing === item.name" :size="16" class="animate-spin" />
            <FlaskConical v-else :size="16" />
          </button>
          <button class="btn btn-ghost btn-sm" v-tooltip="t('upstreams.edit')" @click="openEditModal(item)">
            <Pencil :size="16" />
          </button>
          <button class="btn btn-ghost btn-sm" v-tooltip="item.enabled ? t('secrets.disable') : t('secrets.enable')"
                  :disabled="store.toggling === item.name" @click="store.toggle(item.name, !item.enabled)">
            <Loader2 v-if="store.toggling === item.name" :size="16" class="animate-spin" />
            <component v-else :is="item.enabled ? Pause : Play" :size="16" />
          </button>
          <button class="btn btn-ghost btn-sm" v-tooltip="t('upstreams.delete')" @click="handleRemove(item.name)">
            <Trash2 :size="16" />
          </button>
        </div>
      </template>
    </DataTable>

    <!-- Mobile Action Sheet -->
    <ActionSheet v-model="upstreamActions.isOpen.value" :title="upstreamActions.activeItem.value?.name">
      <button class="action-sheet-item" :disabled="store.testing === upstreamActions.activeItem.value?.name"
              @click="testUpstream(upstreamActions.activeItem.value!.name); upstreamActions.close()">
        <FlaskConical :size="16" /> {{ t('upstreams.test') }}
      </button>
      <button class="action-sheet-item" @click="openEditModal(upstreamActions.activeItem.value!); upstreamActions.close()">
        <Pencil :size="16" /> {{ t('upstreams.edit') }}
      </button>
      <button class="action-sheet-item"
              :disabled="store.toggling === upstreamActions.activeItem.value?.name"
              @click="store.toggle(upstreamActions.activeItem.value!.name, !upstreamActions.activeItem.value!.enabled); upstreamActions.close()">
        <component :is="upstreamActions.activeItem.value?.enabled ? Pause : Play" :size="16" />
        {{ upstreamActions.activeItem.value?.enabled ? t('secrets.disable') : t('secrets.enable') }}
      </button>
      <button class="action-sheet-item action-danger"
              @click="handleRemove(upstreamActions.activeItem.value!.name); upstreamActions.close()">
        <Trash2 :size="16" /> {{ t('upstreams.delete') }}
      </button>
    </ActionSheet>

    <!-- Add/Edit Modal -->
    <FormModal v-model="modalOpen" :title="isEdit ? t('upstreams.edit_title') : t('upstreams.add_title')"
               :submitting="false" :submit-text="isEdit ? t('common.save') : t('common.add')"
               @submit="handleSubmit">
      <div class="form-group mb-md">
        <label class="form-label">{{ isEdit ? t('upstreams.name_label') : t('upstreams.table.name') }}</label>
        <input v-model="form.name" class="input" :disabled="isEdit" required placeholder="upstream1" />
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
      <template #footer>
        <button type="button" class="btn btn-secondary" :disabled="store.testingConfig" @click="handleTestConfig">
          <Loader2 v-if="store.testingConfig" :size="16" class="animate-spin" />
          <FlaskConical v-else :size="16" />
          {{ t('upstreams.test_config') }}
        </button>
      </template>
      <div v-if="store.testResult" class="test-result" :class="store.testResult.ok ? 'test-ok' : 'test-fail'">
        <template v-if="store.testResult.ok">
          {{ t('upstreams.test_ok') }} <template v-if="store.testResult.exit_ip">— {{ store.testResult.exit_ip }}</template>
          <span v-if="store.testResult.latency_ms" class="test-latency">({{ store.testResult.latency_ms }}ms)</span>
        </template>
        <template v-else>
          {{ t('upstreams.test_fail') }}: {{ store.testResult.error }}
        </template>
      </div>
    </FormModal>

    <ConfirmDialog v-bind="confirmState" @confirm="handleConfirm" @cancel="handleCancel" />
  </div>
</template>

<script setup lang="ts">
import {onMounted, ref, watch} from 'vue'
import {useI18n} from 'vue-i18n'
import {useUpstreamsStore} from '@/stores/upstreams'
import {useToastStore} from '@/stores/toast'
import {useConfirmDialog} from '@/composables/useConfirmDialog'
import {useActionMenu} from '@/composables/useActionMenu'
import FormModal from '@/components/common/FormModal.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import DataTable from '@/components/common/DataTable.vue'
import StatusBadge from '@/components/common/StatusBadge.vue'
import PageHeader from '@/components/common/PageHeader.vue'
import ActionSheet from '@/components/common/ActionSheet.vue'
import {FlaskConical, GitBranch, Loader2, MoreVertical, Pause, Pencil, Play, Trash2} from '@lucide/vue'

const { t } = useI18n()
const store = useUpstreamsStore()
const toast = useToastStore()
const upstreamActions = useActionMenu()

const columns = [
  { key: 'name', header: t('upstreams.table.name') },
  { key: 'type', header: t('upstreams.table.type') },
  { key: 'address', header: t('upstreams.table.address') },
  { key: 'weight', header: t('upstreams.table.weight') },
  { key: 'interface', header: t('upstreams.table.interface') },
  { key: 'health', header: t('upstreams.table.health') },
  { key: 'status', header: t('upstreams.table.status') },
]

// Confirm dialog
const { confirmState, confirm, handleConfirm, handleCancel } = useConfirmDialog()

function getHealthVariant(ok: boolean | null | undefined) {
  if (ok === null || ok === undefined) return 'neutral'
  return ok ? 'success' : 'danger'
}

function getHealthLabel(ok: boolean | null | undefined) {
  if (ok === null || ok === undefined) return t('upstreams.health_unknown')
  return ok ? 'OK' : 'FAIL'
}

async function handleRemove(name: string) {
  if (!await confirm({ title: t('upstreams.remove_title'), message: t('upstreams.confirm_remove', { name }), confirmText: t('upstreams.delete') })) return
  try {
    await store.remove(name)
    toast.success(t('upstreams.removed_success', { name }))
  } catch (e: any) { toast.error(e.response?.data?.error ?? e.message) }
}

// Modal state
const modalOpen = ref(false)
const isEdit = ref(false)
const editTarget = ref('')
const form = ref({ name: '', type: 'direct' as string, address: '', username: '', password: '', weight: 1, iface: '' })

const defaultForm = { name: '', type: 'direct' as string, address: '', username: '', password: '', weight: 1, iface: '' }

// Auto-parse pasted proxy string (host:port:user:pass) into separate fields
watch(() => form.value.address, (val) => {
  if (!val || form.value.type === 'direct') return

  let hostPort: string
  let user: string
  let pass: string

  if (val.startsWith('[')) {
    const bracketEnd = val.indexOf(']')
    if (bracketEnd === -1) return
    const afterBracket = val.substring(bracketEnd + 1)
    const parts = afterBracket.split(':')
    if (parts.length < 4) return
    hostPort = val.substring(0, bracketEnd + 1) + ':' + parts[1]
    user = parts[2]
    pass = parts.slice(3).join(':')
  } else {
    const parts = val.split(':')
    if (parts.length < 4) return
    hostPort = parts[0] + ':' + parts[1]
    user = parts[2]
    pass = parts.slice(3).join(':')
  }

  form.value.address = hostPort
  form.value.username = user
  form.value.password = pass
  if (form.value.type !== 'socks5') form.value.type = 'socks5'
})

async function openAddModal() {
  isEdit.value = false
  form.value = { ...defaultForm }
  store.testResult = null
  modalOpen.value = true
  try { await store.loadInterfaces() } catch { /* non-critical */ }
}

async function openEditModal(up: any) {
  isEdit.value = true
  editTarget.value = up.name
  form.value = { name: up.name, type: up.type, address: up.address ?? '', username: up.username ?? '', password: up.password ?? '', weight: up.weight || 1, iface: up.iface ?? '' }
  store.testResult = null
  modalOpen.value = true
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

async function handleSubmit() {
  try {
    if (isEdit.value) {
      await store.update(editTarget.value, form.value as any)
      toast.success(t('upstreams.updated_success', { name: editTarget.value }))
    } else {
      await store.add(form.value as any)
      toast.success(t('upstreams.added_success', { name: form.value.name }))
    }
    modalOpen.value = false
  } catch (e: any) { toast.error(e.response?.data?.error ?? e.message) }
}

async function testUpstream(name: string) {
  try {
    const result = await store.test(name)
    // Update local health state regardless of test outcome
    const u = store.upstreams.find((x) => x.name === name)
    if (u && result) {
      u.last_check_ok = result.ok
      u.latency_ms = result.latency_ms ?? 0
      u.last_error = result.error ?? ''
    }
    if (result?.ok) {
      const parts: string[] = []
      if (result.latency_ms) parts.push(`${result.latency_ms}ms`)
      if (result.exit_ip) parts.push(`IP: ${result.exit_ip}`)
      const extra = parts.length ? ` (${parts.join(', ')})` : ''
      toast.success(t('upstreams.test_success', { name, extra }))
    } else {
      toast.error(t('upstreams.test_failed', { name, error: result?.error ?? 'Unknown error' }))
    }
  } catch (e: any) { toast.error(t('upstreams.test_failed', { name, error: e.response?.data?.error ?? e.message })) }
}

onMounted(() => store.load())
</script>
