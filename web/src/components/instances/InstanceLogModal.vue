<template>
  <Modal :model-value="modelValue" :title="t('instances.logs_title', { label: instance?.label || String(instance?.port) })" max-width="900px" @update:model-value="$emit('update:modelValue', $event)">
    <div class="flex-between mb-md">
      <div class="flex-center gap-md">
        <div class="log-limit">
          <span class="text-xs text-muted mr-xs">{{ t('instances.logs_lines') }}</span>
          <input type="number" v-model.number="maxLogs" class="input input-sm limit-input" min="10" max="5000">
        </div>
        <ToggleSwitch v-model="isFollowing" :label="t('instances.logs_follow')" />
        <button class="btn btn-secondary btn-sm" @click="loadLogs()" :disabled="isFollowing">
          <RefreshCw :size="14" /> {{ t('instances.logs_refresh') }}
        </button>
      </div>
    </div>
    <pre class="logs-output" ref="logsRef" v-html="ansiToHtml(logs || t('instances.logs_tip'))"></pre>
  </Modal>
</template>

<script setup lang="ts">
import {nextTick, ref, watch} from 'vue'
import {useI18n} from 'vue-i18n'
import {useAuthStore} from '@/stores/auth'
import {instancesApi} from '@/api/endpoints'
import {ansiToHtml} from '@/utils/ansi'
import Modal from '@/components/common/Modal.vue'
import ToggleSwitch from '@/components/common/ToggleSwitch.vue'
import {RefreshCw} from '@lucide/vue'
import type {Instance} from '@/types/models'

const props = defineProps<{
  modelValue: boolean
  instance: Instance | null
}>()

defineEmits<{ 'update:modelValue': [value: boolean] }>()

const { t } = useI18n()
const authStore = useAuthStore()

const logs = ref('')
const isFollowing = ref(false)
const maxLogs = ref(200)
const logsRef = ref<HTMLElement | null>(null)
let eventSource: EventSource | null = null

function truncateLogs(text: string, limit: number): string {
  const lines = text.split('\n')
  return lines.length > limit ? lines.slice(-limit).join('\n') : text
}

async function loadLogs() {
  if (!props.instance) return
  try {
    const data = await instancesApi.logs(props.instance.id, maxLogs.value.toString())
    logs.value = truncateLogs(data, maxLogs.value)
  } catch {
    logs.value = ''
  }
}

function startFollow() {
  stopFollow()
  if (!props.instance) return

  const token = authStore.accessToken || ''
  const url = `/api/v1/instances/${props.instance.id}/logs?tail=${maxLogs.value}&follow=true&token=${encodeURIComponent(token)}`
  eventSource = new EventSource(url)

  eventSource.onmessage = (event) => {
    if (event.data) {
      logs.value = truncateLogs(logs.value + event.data + '\n', maxLogs.value)
    }
  }

  eventSource.onerror = () => {
    stopFollow()
  }
}

function stopFollow() {
  if (eventSource) {
    eventSource.close()
    eventSource = null
  }
}

watch(() => props.modelValue, (open) => {
  if (open && props.instance) {
    loadLogs()
  } else {
    stopFollow()
  }
})

watch(isFollowing, (following) => {
  if (following) {
    startFollow()
  } else {
    stopFollow()
  }
})

watch(() => logs.value, () => {
  if (isFollowing.value && logsRef.value) {
    nextTick(() => {
      logsRef.value!.scrollTop = logsRef.value!.scrollHeight
    })
  }
})
</script>

<style scoped lang="scss">
@use '@/assets/scss/variables' as *;

.logs-output {
  background: $color-gray-900;
  color: $color-gray-100;
  padding: $spacing-md;
  border-radius: $border-radius;
  font-family: $font-mono;
  font-size: $font-size-sm;
  max-height: 500px;
  overflow-y: auto;
  white-space: pre-wrap;
  word-break: break-all;
  scroll-behavior: smooth;
}

.log-limit {
  display: flex;
  align-items: center;
  gap: $spacing-xs;
}

.limit-input {
  width: 70px;
  padding: 4px 8px;
  height: 28px;
}
</style>
