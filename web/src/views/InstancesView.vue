<template>
  <div>
    <PageHeader>
      <button class="btn btn-primary" @click="openAddModal()">+ {{ t('instances.add_instance') }}</button>
    </PageHeader>

    <!-- Global Controls -->
    <div class="card mb-md">
      <div class="flex-between">
        <h3 class="mb-none">{{ t('instances.global_controls') }}</h3>
        <div class="actions-grid">
          <Tooltip :text="t('instances.start_all_hint')">
            <button class="btn btn-success btn-sm" :disabled="proxyStore.loading || allRunning" @click="globalAction('start')">
              <Loader2 v-if="proxyStore.activeAction === 'start'" :size="14" class="animate-spin" />
              <Play v-else :size="14" /> {{ t('instances.start_all') }}
            </button>
          </Tooltip>
          <Tooltip :text="t('instances.stop_all_hint')">
            <button class="btn btn-danger btn-sm" :disabled="proxyStore.loading || noneRunning" @click="globalAction('stop')">
              <Loader2 v-if="proxyStore.activeAction === 'stop'" :size="14" class="animate-spin" />
              <Square v-else :size="14" /> {{ t('instances.stop_all') }}
            </button>
          </Tooltip>
          <Tooltip :text="t('instances.restart_all_hint')">
            <button class="btn btn-warning btn-sm" :disabled="proxyStore.loading" @click="globalAction('restart')">
              <Loader2 v-if="proxyStore.activeAction === 'restart'" :size="14" class="animate-spin" />
              <RotateCw v-else :size="14" /> {{ t('instances.restart_all') }}
            </button>
          </Tooltip>
          <Tooltip :text="t('instances.reload_all_hint')">
            <button class="btn btn-outline btn-sm" :disabled="proxyStore.loading" @click="globalAction('reload')">
              <Loader2 v-if="proxyStore.activeAction === 'reload'" :size="14" class="animate-spin" />
              <RefreshCw v-else :size="14" /> {{ t('instances.reload_all') }}
            </button>
          </Tooltip>
        </div>
      </div>
    </div>

    <!-- Bulk Toolbar -->
    <div v-if="selectedIds.size > 0" class="bulk-toolbar mb-md">
      <span class="badge">{{ selectedIds.size }} {{ t('instances.selected') }}</span>
      <button class="btn btn-success btn-sm" :disabled="store.bulkLoading" @click="handleBulkAction('start')">
        {{ t('instances.bulk_start') }}
      </button>
      <button class="btn btn-danger btn-sm" :disabled="store.bulkLoading" @click="handleBulkAction('stop')">
        {{ t('instances.bulk_stop') }}
      </button>
      <button class="btn btn-outline btn-sm" :disabled="store.bulkLoading" @click="handleBulkAction('reload')">
        {{ t('instances.bulk_reload') }}
      </button>
      <button class="btn btn-sm" :disabled="store.bulkLoading" @click="handleBulkToggle(true)">
        {{ t('instances.bulk_enable') }}
      </button>
      <button class="btn btn-sm" :disabled="store.bulkLoading" @click="handleBulkToggle(false)">
        {{ t('instances.bulk_disable') }}
      </button>
      <button class="btn btn-ghost btn-sm" @click="selectedIds = new Set()">{{ t('instances.clear_selection') }}</button>
    </div>

    <DataTable
      :columns="columns"
      :items="store.instances"
      :loading="store.loading"
      :empty-icon="Server"
      :empty-message="t('instances.empty')"
      row-key="id"
      selectable
      :selected-keys="selectedIds"
      @update:selected-keys="onSelectionChange"
    >
      <template #cell-port="{ item }">
        <code>{{ item.port }}</code>
        <span class="text-muted text-xs">:{{ item.metrics_port }}</span>
      </template>
      <template #cell-label="{ item }">{{ item.label || '—' }}</template>
      <template #cell-tls_domain="{ item }">
        <code>{{ item.tls_domain }}</code>
        <Tooltip v-if="(getInstanceParsed(item.id)?.tlsDomains?.length ?? 0)" :text="domainTooltip(item)">
          <span class="badge badge-info">+{{ getInstanceParsed(item.id)?.tlsDomains?.length }}</span>
        </Tooltip>
      </template>
      <template #cell-fake_tls="{ item }">
        <StatusBadge :variant="item.fake_tls ? 'success' : 'neutral'">
          {{ item.fake_tls ? t('instances.tls_fake') : t('instances.tls_classic') }}
        </StatusBadge>
      </template>
      <template #cell-tags="{ item }">
        <div class="tags-cell">
          <span v-for="tag in (getInstanceParsed(item.id)?.tags ?? [])" :key="tag" class="badge badge-info tag-badge">{{ tag }}</span>
          <span v-if="!(getInstanceParsed(item.id)?.tags?.length)" class="text-muted text-sm">—</span>
        </div>
      </template>
      <template #cell-status="{ item }">
        <div class="status-cell">
          <StatusBadge :variant="statusVariant(item)">
            {{ statusLabel(item) }}
          </StatusBadge>
          <code class="text-xs text-muted">{{ getContainerName(item) }}</code>
        </div>
      </template>
      <template #mobile-actions="{ item }">
        <button class="btn btn-ghost btn-sm" @click="instanceActions.open(item)">
          <MoreVertical :size="16" />
        </button>
      </template>
      <template #actions="{ item }">
        <div class="actions-desktop">
          <button class="btn btn-ghost btn-sm" @click="openEditModal(item)" :title="t('common.edit')">
            <Pencil :size="16" />
          </button>
          <button class="btn btn-ghost btn-sm" @click="handleInstanceAction(item, 'start')" :title="t('instances.start')"
                  :disabled="store.actionLoading.has(item.id) || !item.enabled || isInstanceRunning(item)">
            <Loader2 v-if="store.actionLoading.get(item.id) === 'start'" :size="16" class="animate-spin" />
            <Play v-else :size="16" />
          </button>
          <button class="btn btn-ghost btn-sm" @click="handleInstanceAction(item, 'stop')" :title="t('instances.stop')"
                  :disabled="store.actionLoading.has(item.id) || !isInstanceRunning(item)">
            <Loader2 v-if="store.actionLoading.get(item.id) === 'stop'" :size="16" class="animate-spin" />
            <Square v-else :size="16" />
          </button>
          <button class="btn btn-ghost btn-sm" @click="handleInstanceAction(item, 'reload')" :title="t('instances.reload')"
                  :disabled="store.actionLoading.has(item.id) || !isInstanceRunning(item)">
            <Loader2 v-if="store.actionLoading.get(item.id) === 'reload'" :size="16" class="animate-spin" />
            <RefreshCw v-else :size="16" />
          </button>
          <button v-if="item.tls_fronting && item.fake_tls" class="btn btn-ghost btn-sm"
                  @click="handleRefreshFronting(item)" :title="t('instances.refresh_fronting')"
                  :disabled="store.actionLoading.has(item.id)">
            <Loader2 v-if="store.actionLoading.get(item.id) === 'refresh_fronting'" :size="16" class="animate-spin" />
            <Globe v-else :size="16" />
          </button>
          <button class="btn btn-ghost btn-sm" @click="openLogModal(item)" :title="t('instances.logs')"
                  :disabled="!isInstanceRunning(item)">
            <FileText :size="16" />
          </button>
          <button class="btn btn-ghost btn-sm btn-danger-text"
                  :disabled="store.instances.length <= 1"
                  :title="store.instances.length <= 1 ? t('instances.cannot_delete_last') : ''"
                  @click="handleRemove(item)">
            <Trash2 :size="16" />
          </button>
        </div>
      </template>
    </DataTable>

    <!-- Add/Edit Modal -->
    <FormModal
      v-model="modalOpen"
      :title="editingInstance ? t('instances.edit_title') : t('instances.add_title')"
      :submitting="submitting"
      :submit-text="t('common.save')"
      @submit="handleSubmit"
    >
      <!-- General -->
      <div class="form-card">
        <h4 class="form-card-title">{{ t('instances.section_basic') }}</h4>
        <div class="form-group mb-md">
          <label class="form-label">{{ t('instances.table.label') }}</label>
          <input v-model="form.label" class="input" :placeholder="t('instances.instance_placeholder')" />
          <small class="text-muted">{{ t('instances.label_hint') }}</small>
        </div>
        <div class="form-row mb-md">
          <div class="form-group">
            <label class="form-label">{{ t('instances.table.port') }} *</label>
            <div class="input-with-icon">
              <input :value="form.port || ''" class="input" :class="{ 'pr-lg': portChecks.port }" inputmode="numeric" required :disabled="!!editingInstance" @input="sanitizePort($event, 'port')" @blur="checkPort('port')" />
              <Tooltip v-if="portChecks.port" :text="portChecks.port.available ? t('instances.port_available') : portChecks.port.reason!" class="port-icon-wrapper">
                <CircleCheck v-if="portChecks.port.available" :size="16" class="text-success" />
                <CircleX v-else :size="16" class="text-danger" />
              </Tooltip>
            </div>
          </div>
          <div class="form-group">
            <label class="form-label">{{ t('instances.table.metrics_port') }}</label>
            <div class="input-with-icon">
              <input :value="form.metrics_port || ''" class="input" :class="{ 'pr-lg': portChecks.metrics_port }" inputmode="numeric" @input="sanitizePort($event, 'metrics_port')" @blur="checkPort('metrics_port')" />
              <Tooltip v-if="portChecks.metrics_port" :text="portChecks.metrics_port.available ? t('instances.port_available') : portChecks.metrics_port.reason!" class="port-icon-wrapper">
                <CircleCheck v-if="portChecks.metrics_port.available" :size="16" class="text-success" />
                <CircleX v-else :size="16" class="text-danger" />
              </Tooltip>
            </div>
          </div>
        </div>
        <div class="form-group mb-md">
          <label class="form-label">{{ t('instances.tags') }}</label>
          <TagInput v-model="form.tags" :available-tags="allInstanceTags" :placeholder="t('instances.tags_hint')" />
        </div>
        <div class="form-group">
          <label class="checkbox-label">
            <input v-model="form.enabled" type="checkbox" />
            <span>{{ t('instances.enabled') }}</span>
          </label>
        </div>
      </div>

      <!-- TLS & Masking -->
      <div class="form-card">
        <h4 class="form-card-title">{{ t('instances.section_tls') }}</h4>
        <div class="form-group mb-md">
          <label class="form-label">{{ t('instances.tls_domain') }} *</label>
          <input v-model="form.tls_domain" class="input" placeholder="cloudflare.com" required />
          <small class="text-muted">{{ t('instances.tls_domain_hint') }}</small>
        </div>
        <div class="form-group mb-md">
          <label class="form-label">{{ t('instances.tls_domains') }}</label>
          <input v-model="form.tls_domains_text" class="input" placeholder="vk.com, mail.ru" />
          <small class="text-muted">{{ t('instances.tls_domains_hint') }}</small>
        </div>
        <div class="form-group mb-md">
          <label class="checkbox-label">
            <input v-model="form.fake_tls" type="checkbox" />
            <span>FakeTLS</span>
            <Tooltip :text="t('instances.fake_tls_hint')">
              <Info :size="14" class="text-muted" />
            </Tooltip>
          </label>
        </div>
        <div class="form-row mb-md">
          <div class="form-group">
            <label class="form-label">{{ t('instances.mask_host') }}</label>
            <input v-model="form.mask_host" class="input" :placeholder="form.tls_domain || 'cloudflare.com'" />
          </div>
          <div class="form-group">
            <label class="form-label">{{ t('instances.mask_port') }}</label>
            <input v-model.number="form.mask_port" class="input" type="number" min="1" max="65535" />
          </div>
        </div>
      </div>

      <!-- Anti-Blocking -->
      <div class="form-card">
        <h4 class="form-card-title">{{ t('instances.antiblock_section') }}</h4>
        <div class="form-group mb-md">
          <label class="checkbox-label">
            <input v-model="form.tcp_mss_enabled" type="checkbox" />
            <span>{{ t('instances.tcp_mss_enabled') }}</span>
            <Tooltip :text="t('instances.tcp_mss_enabled_hint')">
              <Info :size="14" class="text-muted" />
            </Tooltip>
          </label>
        </div>
        <div v-if="form.tcp_mss_enabled" class="form-group mb-md">
          <label class="form-label">{{ t('instances.tcp_mss') }}</label>
          <input v-model.number="form.tcp_mss" class="input" type="number" min="1" max="1460" />
          <small class="text-muted">{{ t('instances.tcp_mss_hint') }}</small>
        </div>
        <div class="form-group">
          <label class="checkbox-label">
            <input v-model="form.tls_fronting" type="checkbox" :disabled="!form.fake_tls" />
            <span>{{ t('instances.tls_fronting') }}</span>
            <Tooltip :text="t('instances.tls_fronting_hint')">
              <Info :size="14" class="text-muted" />
            </Tooltip>
          </label>
        </div>
      </div>
    </FormModal>

    <!-- Mobile Action Sheet -->
    <ActionSheet v-model="instanceActions.isOpen.value" :title="instanceActions.activeItem.value?.label || String(instanceActions.activeItem.value?.port)">
      <button class="action-sheet-item" @click="openEditModal(instanceActions.activeItem.value!); instanceActions.close()">
        <Pencil :size="16" /> {{ t('common.edit') }}
      </button>
      <button class="action-sheet-item"
              :disabled="store.actionLoading.has(instanceActions.activeItem.value?.id ?? 0) || !instanceActions.activeItem.value?.enabled || isInstanceRunning(instanceActions.activeItem.value!)"
              @click="handleInstanceAction(instanceActions.activeItem.value!, 'start'); instanceActions.close()">
        <Loader2 v-if="store.actionLoading.get(instanceActions.activeItem.value?.id ?? 0) === 'start'" :size="16" class="animate-spin" />
        <Play v-else :size="16" /> {{ t('instances.start') }}
      </button>
      <button class="action-sheet-item"
              :disabled="store.actionLoading.has(instanceActions.activeItem.value?.id ?? 0) || !isInstanceRunning(instanceActions.activeItem.value!)"
              @click="handleInstanceAction(instanceActions.activeItem.value!, 'stop'); instanceActions.close()">
        <Loader2 v-if="store.actionLoading.get(instanceActions.activeItem.value?.id ?? 0) === 'stop'" :size="16" class="animate-spin" />
        <Square v-else :size="16" /> {{ t('instances.stop') }}
      </button>
      <button class="action-sheet-item"
              :disabled="store.actionLoading.has(instanceActions.activeItem.value?.id ?? 0) || !isInstanceRunning(instanceActions.activeItem.value!)"
              @click="handleInstanceAction(instanceActions.activeItem.value!, 'reload'); instanceActions.close()">
        <Loader2 v-if="store.actionLoading.get(instanceActions.activeItem.value?.id ?? 0) === 'reload'" :size="16" class="animate-spin" />
        <RefreshCw v-else :size="16" /> {{ t('instances.reload') }}
      </button>
      <button v-if="instanceActions.activeItem.value?.tls_fronting && instanceActions.activeItem.value?.fake_tls"
              class="action-sheet-item"
              :disabled="store.actionLoading.has(instanceActions.activeItem.value?.id ?? 0)"
              @click="handleRefreshFronting(instanceActions.activeItem.value!); instanceActions.close()">
        <Loader2 v-if="store.actionLoading.get(instanceActions.activeItem.value?.id ?? 0) === 'refresh_fronting'" :size="16" class="animate-spin" />
        <Globe v-else :size="16" /> {{ t('instances.refresh_fronting') }}
      </button>
      <button class="action-sheet-item" :disabled="!isInstanceRunning(instanceActions.activeItem.value!)"
              @click="openLogModal(instanceActions.activeItem.value!); instanceActions.close()">
        <FileText :size="16" /> {{ t('instances.logs') }}
      </button>
      <button class="action-sheet-item action-danger" :disabled="store.instances.length <= 1"
              @click="handleRemove(instanceActions.activeItem.value!); instanceActions.close()">
        <Trash2 :size="16" /> {{ t('common.delete') }}
      </button>
    </ActionSheet>

    <!-- Log Modal -->
    <InstanceLogModal v-model="logModalOpen" :instance="logModalInstance" />

    <ConfirmDialog v-bind="confirmState" @confirm="handleConfirm" @cancel="handleCancel" />
  </div>
</template>

<script setup lang="ts">
import {computed, onMounted, reactive, ref, watch} from 'vue'
import {useI18n} from 'vue-i18n'
import {useInstancesStore} from '@/stores/instances'
import {useProxyStore} from '@/stores/proxy'
import {useToastStore} from '@/stores/toast'
import {useConfirmDialog} from '@/composables/useConfirmDialog'
import {useActionMenu} from '@/composables/useActionMenu'
import {instancesApi} from '@/api/endpoints'
import {parseJSONTags} from '@/utils/format'
import FormModal from '@/components/common/FormModal.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import DataTable from '@/components/common/DataTable.vue'
import StatusBadge from '@/components/common/StatusBadge.vue'
import PageHeader from '@/components/common/PageHeader.vue'
import ActionSheet from '@/components/common/ActionSheet.vue'
import Tooltip from '@/components/common/Tooltip.vue'
import InstanceLogModal from '@/components/instances/InstanceLogModal.vue'
import TagInput from '@/components/common/TagInput.vue'
import {
  CircleCheck,
  CircleX,
  FileText,
  Globe,
  Info,
  Loader2,
  MoreVertical,
  Pencil,
  Play,
  RefreshCw,
  RotateCw,
  Server,
  Square,
  Trash2
} from '@lucide/vue'
import type {Instance} from '@/types/models'

const { t } = useI18n()
const store = useInstancesStore()
const proxyStore = useProxyStore()
const toast = useToastStore()
const instanceActions = useActionMenu<Instance>()

const selectedIds = ref<Set<number>>(new Set())

function onSelectionChange(keys: Set<string | number>) {
  selectedIds.value = new Set([...keys].map(Number))
}

function statusVariant(item: Instance): 'success' | 'warning' | 'danger' | 'neutral' {
  if (!item.enabled) return 'neutral'
  if (isInstanceRunning(item)) return 'success'
  if (getInstanceSecretCount(item.id) === 0) return 'danger'
  return 'warning'
}

function statusLabel(item: Instance): string {
  if (!item.enabled) return t('instances.disabled')
  if (isInstanceRunning(item)) return t('dashboard.running')
  if (getInstanceSecretCount(item.id) === 0) return t('instances.no_matching_secrets')
  return t('dashboard.stopped')
}

const columns = [
  { key: 'port', header: t('instances.table.port') },
  { key: 'label', header: t('instances.table.label') },
  { key: 'tls_domain', header: t('instances.tls_domain') },
  { key: 'fake_tls', header: t('instances.tls_mode') },
  { key: 'tags', header: t('instances.tags') },
  { key: 'status', header: t('instances.table.status') },
]

const runningInstanceIds = computed(() => {
  const map = new Map<number, { running: boolean; secretCount: number }>()
  for (const i of proxyStore.status?.instances ?? []) {
    map.set(i.id, { running: i.running, secretCount: i.matching_secret_count ?? 0 })
  }
  return map
})

function isInstanceRunning(item: Instance): boolean {
  return runningInstanceIds.value.get(item.id)?.running ?? false
}

function getInstanceSecretCount(id: number): number {
  return runningInstanceIds.value.get(id)?.secretCount ?? 0
}

function getContainerName(item: Instance): string {
  return `popugate-telemt-${item.port}`
}

const parsedInstanceData = computed(() => {
  return store.instances.map(inst => ({
    id: inst.id,
    tlsDomains: parseJSONTags(inst.tls_domains),
    tags: parseJSONTags(inst.tags),
  }))
})

function getInstanceParsed(id: number) {
  return parsedInstanceData.value.find(d => d.id === id)
}

function domainTooltip(item: Instance): string {
  const domains = getInstanceParsed(item.id)?.tlsDomains ?? []
  const max = 5
  const shown = domains.slice(0, max)
  const extra = domains.length > max ? `\n...+${domains.length - max}` : ''
  return shown.join('\n') + extra
}

const allInstanceTags = computed(() => {
  const tagSet = new Set<string>()
  parsedInstanceData.value.forEach(d => d.tags.forEach(t => tagSet.add(t)))
  return [...tagSet]
})

const allRunning = computed(() => {
  const enabled = store.instances.filter(i => i.enabled)
  return enabled.length > 0 && enabled.every(i => isInstanceRunning(i))
})

const noneRunning = computed(() => {
  return !store.instances.some(i => isInstanceRunning(i))
})

const { confirmState, confirm, handleConfirm, handleCancel } = useConfirmDialog()

const portChecks = reactive<Record<string, { available: boolean; reason?: string } | null>>({
  port: null,
  metrics_port: null,
})

async function checkPort(field: 'port' | 'metrics_port') {
  const port = form[field]
  if (!port || port < 1 || port > 65535) {
    portChecks[field] = null
    return
  }
  if (form.port && form.metrics_port && form.port === form.metrics_port) {
    portChecks.metrics_port = { available: false, reason: t('instances.port_conflict_same') }
    portChecks.port = null
    return
  }
  try {
    const excludeId = editingInstance.value?.id
    portChecks[field] = await instancesApi.checkPort(port, excludeId)
  } catch {
    portChecks[field] = null
  }
}

function sanitizePort(event: Event, field: 'port' | 'metrics_port') {
  const el = event.target as HTMLInputElement
  const raw = el.value.replace(/[^0-9]/g, '')
  const num = raw ? parseInt(raw, 10) : 0
  const clamped = Math.min(num, 65535)
  form[field] = clamped
  const display = clamped === 0 ? '' : String(clamped)
  if (el.value !== display) {
    el.value = display
  }
}

const modalOpen = ref(false)
const submitting = ref(false)
const editingInstance = ref<Instance | null>(null)

const logModalOpen = ref(false)
const logModalInstance = ref<Instance | null>(null)

const form = reactive({
  port: 443,
  metrics_port: 0,
  label: '',
  tls_domain: '',
  tls_domains_text: '',
  fake_tls: true,
  mask_host: '',
  mask_port: 443,
  tags: '[]',
  enabled: true,
  tcp_mss_enabled: false,
  tcp_mss: 88,
  tls_fronting: false,
})

watch(() => form.fake_tls, (val) => {
  if (!val) form.tls_fronting = false
})

function commaListToJSON(text: string): string {
  const items = text.split(',').map(s => s.trim()).filter(Boolean)
  return items.length ? JSON.stringify(items) : '[]'
}

function jsonToCommaList(json: string): string {
  return parseJSONTags(json).join(', ')
}

function openAddModal() {
  editingInstance.value = null
  portChecks.port = null
  portChecks.metrics_port = null
  Object.assign(form, { port: 443, metrics_port: 0, label: '', tls_domain: '', tls_domains_text: '', fake_tls: true, mask_host: '', mask_port: 443, tags: '[]', enabled: true, tcp_mss_enabled: false, tcp_mss: 88, tls_fronting: false })
  modalOpen.value = true
}

function openEditModal(item: Instance) {
  editingInstance.value = item
  portChecks.port = null
  portChecks.metrics_port = null
  Object.assign(form, {
    port: item.port,
    metrics_port: item.metrics_port,
    label: item.label,
    tls_domain: item.tls_domain,
    tls_domains_text: jsonToCommaList(item.tls_domains),
    fake_tls: item.fake_tls,
    mask_host: item.mask_host,
    mask_port: item.mask_port || 443,
    tags: item.tags || '[]',
    enabled: item.enabled,
    tcp_mss_enabled: item.tcp_mss_enabled || false,
    tcp_mss: item.tcp_mss || 88,
    tls_fronting: item.tls_fronting || false,
  })
  modalOpen.value = true
}

function openLogModal(item: Instance) {
  logModalInstance.value = item
  logModalOpen.value = true
}

async function handleSubmit() {
  submitting.value = true
  try {
    const data = {
      port: form.port,
      metrics_port: form.metrics_port,
      label: form.label,
      tls_domain: form.tls_domain,
      tls_domains: commaListToJSON(form.tls_domains_text),
      fake_tls: form.fake_tls,
      mask_host: form.mask_host,
      mask_port: form.mask_port,
      tags: form.tags,
      enabled: form.enabled,
      tcp_mss_enabled: form.tcp_mss_enabled,
      tcp_mss: form.tcp_mss_enabled ? form.tcp_mss : undefined,
      tls_fronting: form.tls_fronting,
    }

    if (editingInstance.value) {
      await instancesApi.update(editingInstance.value.id, data)
      toast.success(t('instances.updated_success'))
    } else {
      await instancesApi.add(data)
      toast.success(t('instances.added_success', { port: form.port }))
    }
    modalOpen.value = false
    await Promise.all([store.load(), proxyStore.loadStatus()])
  } catch {
    // interceptor handles error toast
  } finally {
    submitting.value = false
  }
}

async function handleRemove(item: Instance) {
  if (!await confirm({ title: t('instances.remove_title'), message: t('instances.confirm_remove', { label: item.label }), confirmText: t('common.delete') })) return
  try {
    await store.removeById(item.id)
    toast.success(t('instances.removed_success', { label: item.label }))
  } catch {
    // interceptor handles error toast
  }
}

async function handleInstanceAction(item: Instance, action: 'start' | 'stop' | 'reload') {
  store.setActionLoading(item.id, action)
  try {
    await instancesApi[action](item.id)
    const messages: Record<string, string> = {
      start: t('instances.started', { label: item.label }),
      stop: t('instances.stopped', { label: item.label }),
      reload: t('instances.reloaded', { label: item.label }),
    }
    toast.success(messages[action])
    await proxyStore.loadStatus()
  } catch {
    // interceptor handles error toast
  } finally {
    store.setActionLoading(item.id, null)
  }
}

async function handleRefreshFronting(item: Instance) {
  store.setActionLoading(item.id, 'refresh_fronting')
  try {
    await instancesApi.refreshFronting(item.id)
    toast.success(t('instances.fronting_refreshed', { label: item.label }))
  } catch {
    // interceptor handles error toast
  } finally {
    store.setActionLoading(item.id, null)
  }
}

async function globalAction(action: 'start' | 'stop' | 'restart' | 'reload') {
  try {
    await proxyStore[action]()
    const labels: Record<string, string> = {
      start: t('dashboard.started'),
      stop: t('dashboard.stopped_success'),
      restart: t('dashboard.restarted'),
      reload: t('dashboard.reloaded'),
    }
    toast.success(labels[action])
    await store.load()
  } catch {
    // interceptor handles error toast
  }
}

async function handleBulkAction(action: 'start' | 'stop' | 'reload') {
  const ids = [...selectedIds.value]
  try {
    const count = await store.bulkAction(ids, action)
    const labels: Record<string, string> = {
      start: t('instances.bulk_started', { count }),
      stop: t('instances.bulk_stopped', { count }),
      reload: t('instances.bulk_reloaded', { count }),
    }
    toast.success(labels[action])
    selectedIds.value = new Set()
    await Promise.all([store.load(), proxyStore.loadStatus()])
  } catch {
    // interceptor handles error toast
  }
}

async function handleBulkToggle(enabled: boolean) {
  const ids = [...selectedIds.value]
  try {
    const count = await store.bulkToggle(ids, enabled)
    toast.success(enabled ? t('instances.bulk_enabled', { count }) : t('instances.bulk_disabled', { count }))
    selectedIds.value = new Set()
    await store.load()
  } catch {
    // interceptor handles error toast
  }
}

onMounted(() => {
  store.load()
  proxyStore.loadStatus()
})
</script>

<style scoped lang="scss">
@use '@/assets/scss/variables' as *;

.status-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.tags-cell {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  max-width: 200px;
  overflow: hidden;
}

.tag-badge {
  font-size: $font-size-xs;
}

.bulk-toolbar {
  display: flex;
  align-items: center;
  gap: $spacing-sm;
  flex-wrap: wrap;
  padding: $spacing-sm $spacing-md;
  background: $bg-card;
  border: 1px solid $color-primary;
  border-radius: $border-radius;
}

.input-with-icon {
  position: relative;

  .input {
    width: 100%;
  }

  .pr-lg {
    padding-right: $spacing-xl;
  }
}

</style>

<style lang="scss">
@use '@/assets/scss/variables' as *;

.port-icon-wrapper {
  position: absolute;
  right: $spacing-sm;
  top: 50%;
  transform: translateY(-50%);
  display: flex;
  cursor: default;
}
</style>
