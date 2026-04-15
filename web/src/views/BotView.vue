<template>
  <div>
    <div class="card mb-lg">
      <h3 class="mb-md">Telegram Bot</h3>

      <div class="status-row mb-md">
        <StatusBadge :variant="botStore.enabled ? 'success' : 'neutral'">
          {{ botStore.enabled ? (botStore.running ? 'Running' : 'Configured') : 'Disabled' }}
        </StatusBadge>
      </div>

      <div class="form-group mb-md">
        <label class="form-label">Bot Token</label>
        <input v-model="form.token" class="input" placeholder="123456:ABC-DEF..." type="password" />
      </div>
      <div class="form-group mb-md">
        <label class="form-label">Chat ID</label>
        <div class="input-group">
          <input v-model="form.chatId" class="input" placeholder="-1001234567890" />
          <button class="btn btn-secondary btn-sm" @click="handleDetectChatId">Detect</button>
        </div>
      </div>
      <div class="form-row mb-md">
        <div class="form-group">
          <label class="form-label">Report Interval (hours)</label>
          <input v-model.number="form.interval" class="input" type="number" min="1" />
        </div>
        <div class="form-group">
          <label class="form-label">Server Label</label>
          <input v-model="form.label" class="input" placeholder="My Server" />
        </div>
      </div>

      <div class="flex gap-sm">
        <button class="btn btn-primary" :disabled="botStore.loading" @click="handleSetup">
          {{ botStore.enabled ? 'Update' : 'Setup' }}
        </button>
        <button class="btn btn-secondary" :disabled="botStore.loading" @click="botStore.test()">
          Send Test Message
        </button>
        <button v-if="botStore.enabled" class="btn btn-warning" @click="botStore.toggle(false)">Disable</button>
        <button v-else class="btn btn-success" @click="botStore.toggle(true)">Enable</button>
      </div>

      <div v-if="botStore.message" class="alert alert-info mt-md">{{ botStore.message }}</div>
    </div>

    <div class="card">
      <h3 class="mb-md">Available Commands</h3>
      <div class="commands-list">
        <div v-for="cmd in commands" :key="cmd.cmd" class="command-item">
          <code>{{ cmd.cmd }}</code>
          <span class="text-muted">{{ cmd.desc }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useBotStore, useConfigStore } from '@/stores'
import StatusBadge from '@/components/common/StatusBadge.vue'

const botStore = useBotStore()
const configStore = useConfigStore()

const form = ref({ token: '', chatId: '', interval: 6, label: 'PopuGate' })

const commands = [
  { cmd: '/mp_status', desc: 'Proxy status' },
  { cmd: '/mp_secrets', desc: 'List secrets' },
  { cmd: '/mp_link [label]', desc: 'Proxy links + QR' },
  { cmd: '/mp_add <label>', desc: 'Add secret' },
  { cmd: '/mp_remove <label>', desc: 'Remove secret' },
  { cmd: '/mp_rotate <label>', desc: 'Rotate secret' },
  { cmd: '/mp_restart', desc: 'Restart proxy' },
  { cmd: '/mp_enable <label>', desc: 'Enable secret' },
  { cmd: '/mp_disable <label>', desc: 'Disable secret' },
  { cmd: '/mp_health', desc: 'Health check' },
  { cmd: '/mp_traffic', desc: 'Traffic report' },
  { cmd: '/mp_update', desc: 'Version info' },
  { cmd: '/mp_limits', desc: 'User limits' },
  { cmd: '/mp_setlimit', desc: 'Set limits' },
  { cmd: '/mp_upstreams', desc: 'List upstreams' },
]

async function handleSetup() {
  await botStore.setup(form.value.token, form.value.chatId, form.value.interval, form.value.label)
}

async function handleDetectChatId() {
  const result = await botStore.detectChatId()
  if (result?.chat_id) form.value.chatId = result.chat_id
}

onMounted(async () => {
  await configStore.load()
  await botStore.loadStatus()
  if (configStore.settings) {
    form.value.token = configStore.settings.telegram_bot_token
    form.value.chatId = configStore.settings.telegram_chat_id
    form.value.interval = configStore.settings.telegram_interval
    form.value.label = configStore.settings.telegram_server_label
  }
})
</script>

<style scoped lang="scss">
@use '@/assets/scss/variables' as *;

.commands-list { display: flex; flex-direction: column; gap: $spacing-sm; }
.command-item { display: flex; gap: $spacing-md; align-items: baseline; }
.command-item code { min-width: 180px; }
</style>
