<template>
  <div>
    <div class="card mb-lg">
      <h3 class="mb-md">{{ t('bot.title') }}</h3>

      <div class="status-row mb-md">
        <StatusBadge :variant="botStore.enabled ? 'success' : 'neutral'">
          {{ botStore.enabled ? (botStore.running ? t('dashboard.running') : t('bot.configured')) : t('secrets.disabled') }}
        </StatusBadge>
      </div>

      <div class="form-group mb-md">
        <label class="form-label">{{ t('bot.token') }}</label>
        <input v-model="form.token" class="input" placeholder="123456:ABC-DEF..." type="password" />
      </div>
      <div class="form-group mb-md">
        <label class="form-label">{{ t('bot.chat_id') }}</label>
        <div class="input-group">
          <input v-model="form.chatId" class="input" placeholder="-1001234567890" />
          <button class="btn btn-secondary btn-sm" @click="handleDetectChatId">{{ t('bot.detect') }}</button>
        </div>
      </div>
      <div class="form-row mb-md">
        <div class="form-group">
          <label class="form-label">{{ t('bot.interval') }}</label>
          <input v-model.number="form.interval" class="input" type="number" min="1" />
        </div>
        <div class="form-group">
          <label class="form-label">{{ t('bot.server_label') }}</label>
          <input v-model="form.label" class="input" placeholder="My Server" />
        </div>
      </div>

      <div class="flex gap-sm">
        <button class="btn btn-primary" :disabled="botStore.loading" @click="handleSetup">
          <Loader2 v-if="botStore.loading" :size="16" class="animate-spin" />
          {{ botStore.enabled ? t('bot.update') : t('bot.setup') }}
        </button>
        <button class="btn btn-secondary" :disabled="botStore.loading" @click="botStore.test()">
          <Loader2 v-if="botStore.loading" :size="16" class="animate-spin" />
          {{ t('bot.test') }}
        </button>
        <button v-if="botStore.enabled" class="btn btn-warning" :disabled="botStore.loading" @click="botStore.toggle(false)">{{ t('secrets.disable') }}</button>
        <button v-else class="btn btn-success" :disabled="botStore.loading" @click="botStore.toggle(true)">{{ t('secrets.enable') }}</button>
      </div>

      <div v-if="botStore.message" class="alert alert-info mt-md">{{ botStore.message }}</div>
    </div>

    <div class="card">
      <h3 class="mb-md">{{ t('bot.commands') }}</h3>
      <div class="commands-list">
        <div v-for="cmd in localizedCommands" :key="cmd.cmd" class="command-item">
          <code>{{ cmd.cmd }}</code>
          <span class="text-muted">{{ cmd.desc }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useBotStore, useConfigStore } from '@/stores'
import { Loader2 } from '@lucide/vue'
import StatusBadge from '@/components/common/StatusBadge.vue'

const { t } = useI18n()
const botStore = useBotStore()
const configStore = useConfigStore()

const form = ref({ token: '', chatId: '', interval: 6, label: 'PopuGate' })

const localizedCommands = computed(() => [
  { cmd: '/status', desc: t('bot.proxy_status') },
  { cmd: '/secrets', desc: t('bot.list_secrets') },
  { cmd: '/link [label]', desc: t('bot.proxy_links') },
  { cmd: '/add <label>', desc: t('bot.add_secret') },
  { cmd: '/remove <label>', desc: t('bot.remove_secret') },
  { cmd: '/rotate <label>', desc: t('bot.rotate_secret') },
  { cmd: '/restart', desc: t('bot.restart_proxy') },
  { cmd: '/enable <label>', desc: t('bot.enable_secret') },
  { cmd: '/disable <label>', desc: t('bot.disable_secret') },
  { cmd: '/health', desc: t('bot.health_check') },
  { cmd: '/traffic', desc: t('bot.traffic_report') },
  { cmd: '/update', desc: t('bot.version_info') },
  { cmd: '/limits', desc: t('bot.user_limits') },
  { cmd: '/setlimit', desc: t('bot.set_limits') },
  { cmd: '/upstreams', desc: t('bot.list_upstreams') },
])

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
