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
        <small class="text-muted">{{ t('bot.hint_token') }}</small>
      </div>
      <div class="form-group mb-md">
        <label class="form-label">{{ t('bot.chat_id') }}</label>
        <div class="input-group">
          <input v-model="form.chatId" class="input" placeholder="-1001234567890" />
          <button class="btn btn-secondary btn-sm" :disabled="!form.token" @click="handleDetectChatId">{{ t('bot.detect') }}</button>
        </div>
        <small class="text-muted">{{ t('bot.hint_chat_id') }}</small>
      </div>
      <div class="form-row mb-md">
        <div class="form-group">
          <label class="form-label">{{ t('bot.interval') }}</label>
          <input v-model.number="form.interval" class="input" type="number" min="1" />
          <small class="text-muted">{{ t('bot.hint_interval') }}</small>
        </div>
        <div class="form-group">
          <label class="form-label">{{ t('bot.server_label') }}</label>
          <input v-model="form.label" class="input" placeholder="My Server" />
          <small class="text-muted">{{ t('bot.hint_label') }}</small>
        </div>
      </div>

      <div class="bot-actions">
        <button class="btn btn-primary" :disabled="botStore.loading || !canSetup" @click="handleSetup">
          <Loader2 v-if="botStore.loading" :size="16" class="animate-spin" />
          {{ botStore.enabled ? t('bot.update') : t('bot.setup') }}
        </button>
        <button class="btn btn-secondary" :disabled="botStore.loading || !isConfigured" @click="botStore.test()">
          <Loader2 v-if="botStore.loading" :size="16" class="animate-spin" />
          {{ t('bot.test') }}
        </button>
        <button class="btn btn-secondary" :disabled="botStore.loading || !isConfigured" @click="botStore.setCommands()">
          <Loader2 v-if="botStore.loading" :size="16" class="animate-spin" />
          {{ t('bot.refresh_commands') }}
        </button>
        <button v-if="botStore.enabled" class="btn btn-warning" :disabled="botStore.loading" @click="botStore.toggle(false)">{{ t('secrets.disable') }}</button>
        <button v-else-if="isConfigured" class="btn btn-success" :disabled="botStore.loading" @click="botStore.toggle(true)">{{ t('secrets.enable') }}</button>
      </div>

      <div v-if="botStore.message" class="alert alert-info mt-md">{{ botStore.message }}</div>
    </div>

    <div class="card">
      <h3 class="mb-md">{{ t('bot.commands') }}</h3>
      <div v-for="group in commandGroups" :key="group.title" class="commands-group">
        <div class="commands-group-title">{{ group.title }}</div>
        <div class="commands-list">
          <div v-for="cmd in group.items" :key="cmd.cmd" class="command-item">
            <code>{{ cmd.cmd }}</code>
            <span class="text-muted">{{ cmd.desc }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import {computed, onMounted, ref} from 'vue'
import {useI18n} from 'vue-i18n'
import {useBotStore, useConfigStore} from '@/stores'
import {Loader2} from '@lucide/vue'
import StatusBadge from '@/components/common/StatusBadge.vue'

const { t } = useI18n()
const botStore = useBotStore()
const configStore = useConfigStore()

const form = ref({ token: '', chatId: '', interval: 6, label: 'PopuGate' })

const canSetup = computed(() => !!form.value.token && !!form.value.chatId)
const isConfigured = computed(() => !!form.value.token)

const commandGroups = computed(() => [
  {
    title: t('bot.group_management'),
    items: [
      { cmd: '/status', desc: t('bot.proxy_status') },
      { cmd: '/health', desc: t('bot.health_check') },
      { cmd: '/restart', desc: t('bot.restart_proxy') },
      { cmd: '/start [label]', desc: t('bot.start_instance') },
      { cmd: '/stop <label>', desc: t('bot.stop_instance') },
    ],
  },
  {
    title: t('bot.group_secrets'),
    items: [
      { cmd: '/secrets', desc: t('bot.list_secrets') },
      { cmd: '/link [label]', desc: t('bot.proxy_links') },
      { cmd: '/add <label>', desc: t('bot.add_secret') },
      { cmd: '/remove <label>', desc: t('bot.remove_secret') },
      { cmd: '/rotate <label>', desc: t('bot.rotate_secret') },
      { cmd: '/enable <label>', desc: t('bot.enable_secret') },
      { cmd: '/disable <label>', desc: t('bot.disable_secret') },
    ],
  },
  {
    title: t('bot.group_limits'),
    items: [
      { cmd: '/limits', desc: t('bot.user_limits') },
      { cmd: '/setlimit <label> <conns> <ips> <quota_mb> [date]', desc: t('bot.set_limits') },
      { cmd: '/traffic', desc: t('bot.traffic_report') },
    ],
  },
  {
    title: t('bot.group_system'),
    items: [
      { cmd: '/upstreams', desc: t('bot.list_upstreams') },
      { cmd: '/tasks', desc: t('bot.scheduled_tasks') },
      { cmd: '/update', desc: t('bot.version_info') },
      { cmd: '/help', desc: t('bot.show_help') },
    ],
  },
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

.commands-group { margin-bottom: $spacing-md; }
.commands-group:last-child { margin-bottom: 0; }
.commands-group-title { font-size: $font-size-xs; color: $color-primary; text-transform: uppercase; letter-spacing: 0.05em; font-weight: $font-weight-semibold; margin-bottom: $spacing-xs; }
.commands-list { display: flex; flex-direction: column; gap: $spacing-xs; }
.command-item { display: flex; gap: $spacing-md; align-items: baseline; flex-wrap: wrap; }
.command-item code { white-space: nowrap; }

.bot-actions { display: flex; gap: $spacing-sm; flex-wrap: wrap; }
</style>
