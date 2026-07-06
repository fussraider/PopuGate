<template>
  <Modal :model-value="modelValue" :title="t('upstreams.bulk_add_title')" maxWidth="800px" @update:model-value="$emit('update:modelValue', $event)">
    <div class="wizard">
      <!-- Steps Indicator -->
      <div class="steps-indicator">
        <div class="step-node" :class="{ active: step === 1, completed: step > 1 }">
          <span class="step-num">1</span>
          <span class="step-label">{{ t('upstreams.wizard.input') }}</span>
        </div>
        <div class="step-connector" :class="{ completed: step > 1 }" />
        <div class="step-node" :class="{ active: step === 2, completed: step > 2 }">
          <span class="step-num">2</span>
          <span class="step-label">{{ t('upstreams.wizard.verify') }}</span>
        </div>
        <div class="step-connector" :class="{ completed: step > 2 }" />
        <div class="step-node" :class="{ active: step === 3 }">
          <span class="step-num">3</span>
          <span class="step-label">{{ t('upstreams.wizard.configure') }}</span>
        </div>
      </div>

      <!-- Step 1: Input -->
      <div v-if="step === 1" class="step-content">
        <div class="form-group mb-md">
          <label class="form-label">{{ t('upstreams.wizard.list_label') }}</label>
          <textarea
            v-model="rawInput"
            class="textarea"
            rows="10"
            :placeholder="placeholderText"
            required
          />
          <small class="text-muted">{{ t('upstreams.wizard.formats_hint_prefix') }}: socks5://host:port, socks4://host:port, ss://&lt;base64&gt;@host:port, host:port, host:port:user:pass, user:pass@host:port</small>
        </div>

        <div class="form-group mb-md">
          <label class="form-label">{{ t('upstreams.wizard.ping_threshold') }} (ms)</label>
          <input
            v-model.number="pingLimit"
            type="number"
            class="input"
            min="10"
            max="15000"
          />
          <small class="text-muted">{{ t('upstreams.wizard.ping_hint') }}</small>
        </div>
      </div>

      <!-- Step 2: Verification -->
      <div v-if="step === 2" class="step-content">
        <!-- Live Stats Banner -->
        <div class="stats-banner mb-md">
          <div class="stat-item">
            <span class="stat-val">{{ proxiesCheckList.length }}</span>
            <span class="stat-lbl">{{ t('upstreams.wizard.total_parsed') }}</span>
          </div>
          <div class="stat-item">
            <span class="stat-val text-success">{{ workingCount }}</span>
            <span class="stat-lbl">{{ t('upstreams.wizard.working') }}</span>
          </div>
          <div class="stat-item highlight">
            <span class="stat-val text-primary">{{ matchingCount }}</span>
            <span class="stat-lbl">{{ t('upstreams.wizard.matching_ping') }}</span>
          </div>
        </div>

        <!-- Dynamic Latency Filter -->
        <div class="form-group mb-md dynamic-filter">
          <div class="flex justify-between items-center mb-xs">
            <label class="form-label m-0">{{ t('upstreams.wizard.latency_limit') }}: {{ pingLimit }}ms</label>
            <button class="btn btn-xs btn-secondary" :disabled="checking" @click="startVerification">
              <RefreshCw :size="12" class="mr-xs" :class="{ 'animate-spin': checking }" />
              {{ t('upstreams.wizard.recheck') }}
            </button>
          </div>
          <input
            v-model.number="pingLimit"
            type="range"
            min="50"
            max="3000"
            step="50"
            class="range-slider"
          />
        </div>

        <!-- Table of Checked Proxies -->
        <div class="table-container">
          <DataTable
            :columns="columns"
            :items="proxiesCheckList"
            rowKey="input"
            :rowClass="getRowClass"
          >
            <template #cell-input="{ item }">
              <span class="font-mono">{{ item.input }}</span>
            </template>
            
            <template #cell-type="{ item }">
              <span v-if="item.type" class="badge badge-info">{{ item.type }}</span>
              <span v-else>—</span>
            </template>

            <template #cell-exit_ip="{ item }">
              <span class="font-mono text-xs">{{ item.exit_ip || '—' }}</span>
            </template>

            <template #cell-latency_ms="{ item }">
              <span v-if="item.latency_ms !== undefined" :class="getPingClass(item.latency_ms)">
                {{ item.latency_ms }}ms
              </span>
              <span v-else>—</span>
            </template>

            <template #cell-status="{ item }">
              <div class="flex items-center gap-xs">
                <Loader2 v-if="item.status === 'checking'" :size="14" class="animate-spin text-primary" />
                <CheckCircle2 v-else-if="item.status === 'working'" :size="14" class="text-success" />
                <XCircle v-else-if="item.status === 'failed'" :size="14" class="text-danger" v-tooltip="item.error" />
                <Clock v-else :size="14" class="text-muted" />
                <span class="text-xs capitalize">{{ item.status }}</span>
              </div>
            </template>
          </DataTable>
        </div>
      </div>

      <!-- Step 3: Config -->
      <div v-if="step === 3" class="step-content">
        <div class="alert alert-info">
          <span class="font-bold mr-xs">{{ matchingCount }}</span>
          {{ t('upstreams.wizard.ready_to_add') }}
        </div>

        <div class="form-row mb-md">
          <div class="form-group">
            <label class="form-label">{{ t('upstreams.table.weight') }}</label>
            <input v-model.number="weight" class="input" type="number" min="1" max="100" required />
            <small class="text-muted">{{ t('upstreams.hint_weight') }}</small>
          </div>
          <div class="form-group">
            <label class="form-label">{{ t('upstreams.table.interface') }}</label>
            <select v-model="iface" class="select">
              <option value="">Auto</option>
              <option v-for="nic in store.interfaces" :key="nic.name" :value="nic.name">
                {{ nic.name }} <template v-if="nic.addresses.length">( {{ nic.addresses[0] }} )</template>
              </option>
            </select>
            <small class="text-muted">{{ t('upstreams.hint_interface') }}</small>
          </div>
        </div>
      </div>
    </div>

    <!-- Wizard Footer Buttons -->
    <template #footer>
      <button v-if="step > 1" type="button" class="btn btn-secondary" :disabled="submitting" @click="step--">
        {{ t('upstreams.wizard.back') }}
      </button>
      <button v-else type="button" class="btn btn-secondary" @click="$emit('update:modelValue', false)">
        {{ t('common.cancel') }}
      </button>

      <button v-if="step === 1" type="button" class="btn btn-primary" :disabled="!rawInput.trim()" @click="goToStep2">
        {{ t('upstreams.wizard.next') }}
      </button>
      <button v-else-if="step === 2" type="button" class="btn btn-primary" :disabled="checking || matchingCount === 0" @click="step++">
        {{ t('upstreams.wizard.next') }}
      </button>
      <button v-else type="button" class="btn btn-primary" :disabled="submitting" @click="submitBulk">
        <Loader2 v-if="submitting" :size="16" class="animate-spin mr-xs" />
        {{ t('common.add') }} ({{ matchingCount }})
      </button>
    </template>
  </Modal>
</template>

<script setup lang="ts">
import {computed, ref, watch} from 'vue'
import {useI18n} from 'vue-i18n'
import Modal from '@/components/common/Modal.vue'
import {useUpstreamsStore} from '@/stores/upstreams'
import {useToastStore} from '@/stores/toast'
import {CheckCircle2, Clock, Loader2, RefreshCw, XCircle} from '@lucide/vue'
import DataTable from '@/components/common/DataTable.vue'

interface ProxyCheckItem {
  input: string
  status: 'pending' | 'checking' | 'working' | 'failed'
  type?: string
  address?: string
  exit_ip?: string
  latency_ms?: number
  error?: string
}

const props = defineProps<{
  modelValue: boolean
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  added: []
}>()

const { t } = useI18n()
const store = useUpstreamsStore()
const toast = useToastStore()

const step = ref(1)
const rawInput = ref('')
const pingLimit = ref(500)
const checking = ref(false)
const submitting = ref(false)
const weight = ref(10)
const iface = ref('')

const proxiesCheckList = ref<ProxyCheckItem[]>([])

const columns = computed(() => [
  { key: 'input', header: t('upstreams.wizard.col_proxy') },
  { key: 'type', header: t('upstreams.wizard.col_type'), sortable: true },
  { key: 'exit_ip', header: t('upstreams.wizard.col_exit_ip') },
  { key: 'latency_ms', header: t('upstreams.wizard.col_ping'), sortable: true },
  { key: 'status', header: t('upstreams.wizard.col_status'), sortable: true },
])

const placeholderText = `socks5://1.2.3.4:1080
5.6.7.8:1080:username:password
socks4://user@8.8.8.8:8080
ss://2022-blake3-aes-256-gcm:BASE64PASSWORD@9.9.9.9:8388
192.168.1.10:80`

// Watch modal state reset
watch(() => props.modelValue, (isOpen) => {
  if (isOpen) {
    step.value = 1
    rawInput.value = ''
    proxiesCheckList.value = []
    weight.value = 10
    iface.value = ''
    store.loadInterfaces()
  }
})

const workingCount = computed(() => {
  return proxiesCheckList.value.filter(x => x.status === 'working').length
})

const matchingCount = computed(() => {
  return proxiesCheckList.value.filter(x => 
    x.status === 'working' && x.latency_ms !== undefined && x.latency_ms <= pingLimit.value
  ).length
})

function goToStep2() {
  const lines = rawInput.value
    .split('\n')
    .map(x => x.trim())
    .filter(x => x.length > 0)

  if (lines.length === 0) return

  proxiesCheckList.value = lines.map(line => ({
    input: line,
    status: 'pending'
  }))

  step.value = 2
  startVerification()
}

async function startVerification() {
  checking.value = true
  const list = proxiesCheckList.value
  // Reset statuses
  list.forEach(x => {
    x.status = 'checking'
    x.latency_ms = undefined
    x.exit_ip = undefined
    x.error = undefined
    x.type = undefined
  })

  const rawList = list.map(x => x.input)
  try {
    await store.bulkCheck(rawList, (update) => {
      const item = list.find(x => x.input === update.input)
      if (item) {
        if (update.ok) {
          item.status = 'working'
          item.latency_ms = update.latency_ms
          item.exit_ip = update.exit_ip
          item.type = update.type
          item.address = update.address
        } else {
          item.status = 'failed'
          item.error = update.error
        }
      }
    })
  } catch (err: any) {
    toast.error(err.message || 'Verification stream error')
  } finally {
    // Set any remaining checking items to failed
    list.forEach(x => {
      if (x.status === 'checking') x.status = 'failed'
    })
    checking.value = false
  }
}

function getRowClass(item: ProxyCheckItem) {
  if (item.status === 'working') {
    if (item.latency_ms !== undefined && item.latency_ms <= pingLimit.value) {
      return 'row-match'
    }
    return 'row-working-slow'
  }
  if (item.status === 'failed') return 'row-failed'
  return ''
}

function getPingClass(ping: number) {
  if (ping <= pingLimit.value) return 'ping-good'
  return 'ping-bad'
}

async function submitBulk() {
  const finalAdd = proxiesCheckList.value.filter(x => 
    x.status === 'working' && 
    x.latency_ms !== undefined && 
    x.latency_ms <= pingLimit.value
  ).map(x => {
    const creds = parseProxyCredentials(x.input)
    return {
      type: x.type || creds.type,
      address: x.address || creds.address,
      username: creds.username,
      password: creds.password,
      url: creds.url,
      weight: weight.value,
      iface: iface.value
    }
  })

  submitting.value = true
  try {
    const res = await store.bulkAdd(finalAdd)
    if (res.skipped > 0) {
      toast.success(t('upstreams.wizard.success_added_skipped', { count: res.count, skipped: res.skipped }))
    } else {
      toast.success(t('upstreams.wizard.success_added', { count: res.count }))
    }
    if (res.skipped_middle_proxy && res.skipped_middle_proxy.length > 0) {
      toast.warning(t('upstreams.wizard.ss_skipped_middle_proxy', { count: res.skipped_middle_proxy.length }), 8000)
    }
    emit('added')
    emit('update:modelValue', false)
  } catch (err: any) {
    toast.error(err.response?.data?.error || 'Failed to save upstreams')
  } finally {
    submitting.value = false
  }
}

function parseProxyCredentials(line: string) {
  line = line.trim()

  // Shadowsocks: the whole line is an ss:// URL.
  if (line.startsWith('ss://')) {
    return { type: 'shadowsocks', address: '', username: '', password: '', url: line }
  }

  let type = 'socks5'
  if (line.startsWith('socks5://')) {
    line = line.slice(9)
  } else if (line.startsWith('socks4://')) {
    type = 'socks4'
    line = line.slice(9)
  }

  if (line.includes('@')) {
    const parts = line.split('@')
    const creds = parts[0]
    const addr = parts[1]
    const cParts = creds.split(':')
    return {
      type,
      address: addr,
      username: cParts[0] || '',
      password: cParts[1] || '',
      url: ''
    }
  }

  const parts = line.split(':')
  if (parts.length >= 4) {
    return {
      type,
      address: `${parts[0]}:${parts[1]}`,
      username: parts[2] || '',
      password: parts.slice(3).join(':') || '',
      url: ''
    }
  } else if (parts.length === 3) {
    return {
      type,
      address: `${parts[0]}:${parts[1]}`,
      username: parts[2] || '',
      password: '',
      url: ''
    }
  }

  return {
    type,
    address: line,
    username: '',
    password: '',
    url: ''
  }
}
</script>

<style scoped lang="scss">
@use '@/assets/scss/variables' as *;

.wizard {
  display: flex;
  flex-direction: column;
  gap: $spacing-lg;
}

.steps-indicator {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: $spacing-md;

  .step-node {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: $spacing-xs;
    position: relative;
    z-index: 2;

    .step-num {
      width: 32px;
      height: 32px;
      border-radius: 50%;
      background: var(--bg-body);
      border: 2px solid var(--border-color);
      display: flex;
      align-items: center;
      justify-content: center;
      font-weight: bold;
      color: var(--text-muted);
      transition: all 0.2s ease;
    }

    .step-label {
      font-size: $font-size-xs;
      color: var(--text-muted);
      font-weight: 500;
    }

    &.active {
      .step-num {
        background: var(--color-primary-bg);
        border-color: $color-primary;
        color: $color-primary;
      }
      .step-label {
        color: var(--text-primary);
        font-weight: bold;
      }
    }

    &.completed {
      .step-num {
        background: var(--color-success-bg);
        border-color: var(--color-success);
        color: var(--color-success);
      }
    }
  }

  .step-connector {
    flex-grow: 1;
    height: 2px;
    background: var(--border-color);
    margin: 0 $spacing-sm;
    transform: translateY(-12px);
    transition: background-color 0.2s ease;

    &.completed {
      background: var(--color-success);
    }
  }
}

.stats-banner {
  display: flex;
  justify-content: space-around;
  background: var(--bg-body);
  border-radius: $border-radius;
  padding: $spacing-md;
  border: 1px solid var(--border-color);

  .stat-item {
    display: flex;
    flex-direction: column;
    align-items: center;

    .stat-val {
      font-size: $font-size-lg;
      font-weight: bold;
    }

    .stat-lbl {
      font-size: $font-size-xs;
      color: var(--text-muted);
    }

    &.highlight {
      border-left: 1px solid var(--border-color);
      padding-left: $spacing-lg;
    }
  }
}

.dynamic-filter {
  background: var(--bg-body);
  border-radius: $border-radius;
  padding: $spacing-md;
  border: 1px solid var(--border-color);
}

.range-slider {
  width: 100%;
  height: 6px;
  background: var(--border-color);
  border-radius: $border-radius-sm;
  outline: none;
  cursor: pointer;
  -webkit-appearance: none;

  &::-webkit-slider-thumb {
    -webkit-appearance: none;
    width: 18px;
    height: 18px;
    border-radius: 50%;
    background: $color-primary;
    border: 2px solid white;
    box-shadow: $shadow-sm;
  }
}

.ping-good {
  color: var(--color-success);
  font-weight: 600;
}

.ping-bad {
  color: var(--color-warning);
  font-weight: 500;
}

.text-primary {
  color: $color-primary !important;
}

:deep(.table-wrapper) {
  max-height: 250px;
  overflow-y: auto;
  border-radius: $border-radius;
}

:deep(.table th) {
  position: sticky;
  top: 0;
  z-index: 1;
  background: var(--bg-table-header);
}

:deep(.table tr) {
  &.row-match {
    background: rgba(16, 185, 129, 0.08) !important; // soft success
  }
  &.row-working-slow {
    background: rgba(245, 158, 11, 0.08) !important; // soft warning
  }
  &.row-failed {
    opacity: 0.7;
    background: rgba(239, 68, 68, 0.05) !important; // soft danger
  }
}
</style>
