<template>
  <div>
    <LoadingSpinner v-if="configStore.loading" :message="t('common.loading')" />

    <div v-else-if="configStore.settings" class="settings-layout">
      <!-- Proxy Settings -->
      <div class="card span-6">
        <h3 class="mb-md">{{ t('settings_view.title') }}</h3>
        <div class="settings-grid">
          <div class="form-group">
            <label class="form-label">{{ t('settings_view.concurrency') }}</label>
            <input v-model.number="form.proxy_concurrency" class="input" type="number" min="1" />
            <small class="text-muted">{{ t('settings_view.hint_concurrency') }}</small>
          </div>
          <div class="form-group">
            <label class="form-label">{{ t('settings_view.cpu_limit') }}</label>
            <input v-model="form.proxy_cpus" class="input" :placeholder="t('settings_view.unlimited_tip')" />
          </div>
          <div class="form-group">
            <label class="form-label">{{ t('settings_view.memory_limit') }}</label>
            <input v-model="form.proxy_memory" class="input" :placeholder="t('settings_view.memory_placeholder')" />
          </div>
          <div class="form-group">
            <label class="form-label">{{ t('settings_view.custom_ip') }}</label>
            <input v-model="form.custom_ip" class="input" :placeholder="t('settings_view.ip_auto_tip')" />
          </div>
          <div class="form-group">
            <label class="form-label">{{ t('settings_view.cert_len') }}</label>
            <input v-model.number="form.fake_cert_len" class="input" type="number" />
            <small class="text-muted">{{ t('settings_view.hint_cert_len') }}</small>
          </div>
        </div>
      </div>

      <!-- Custom Telegram URLs -->
      <div class="card span-3">
        <h3 class="mb-md">{{ t('settings_view.custom_tg_title') }}</h3>
        <p class="text-muted text-sm mb-md">{{ t('settings_view.custom_tg_desc') }}</p>
        <div class="form-group">
          <label class="form-label">{{ t('settings_view.proxy_secret_url') }}</label>
          <input v-model="form.proxy_secret_url" class="input" :placeholder="t('settings_view.url_placeholder')" />
        </div>
        <div class="form-group">
          <label class="form-label">{{ t('settings_view.proxy_config_v4_url') }}</label>
          <input v-model="form.proxy_config_v4_url" class="input" :placeholder="t('settings_view.url_placeholder')" />
        </div>
        <div class="form-group">
          <label class="form-label">{{ t('settings_view.proxy_config_v6_url') }}</label>
          <input v-model="form.proxy_config_v6_url" class="input" :placeholder="t('settings_view.url_placeholder')" />
        </div>
      </div>

      <!-- telemt Engine -->
      <div class="card span-3">
        <h3 class="mb-md">{{ t('settings_view.telemt_engine') }}</h3>
        <p class="text-muted text-sm mb-md">{{ t('settings_view.telemt_engine_desc') }}</p>
        <div class="form-group">
          <label class="form-label">{{ t('settings_view.telemt_version') }}</label>
          <input v-model="form.telemt_version" class="input" :placeholder="t('settings_view.telemt_version_placeholder')" />
        </div>
        <div class="form-group">
          <label class="form-label">{{ t('settings_view.telemt_commit') }}</label>
          <input v-model="form.telemt_commit" class="input" :placeholder="t('settings_view.telemt_commit_placeholder')" />
        </div>
        <div class="form-group">
          <label class="form-label">{{ t('settings_view.telemt_repo') }}</label>
          <input v-model="form.telemt_repo" class="input" :placeholder="t('settings_view.telemt_repo_placeholder')" />
        </div>
      </div>

      <!-- Proxy Protocol -->
      <div class="card span-2">
        <h3 class="mb-md">{{ t('settings_view.proxy_proto') }}</h3>
        <label class="checkbox-label mb-md">
          <input v-model="form.proxy_protocol" type="checkbox" />
          {{ t('settings_view.enable_proxy_proto') }}
        </label>
        <div class="form-group">
          <label class="form-label">{{ t('settings_view.trusted_cidrs') }}</label>
          <input v-model="form.proxy_protocol_trusted_cidrs" class="input" :placeholder="t('settings_view.cidrs_placeholder')" />
        </div>
      </div>

      <!-- Web UI URL -->
      <div class="card span-2">
        <h3 class="mb-md">{{ t('settings_view.web_url') }}</h3>
        <div class="form-group">
          <input v-model="form.web_url" class="input" placeholder="https://popugate.example.com:8090" />
          <small class="text-muted">{{ t('settings_view.web_url_hint') }}</small>
        </div>
      </div>

      <!-- Ad Tag / Middle-Proxy -->
      <div class="card span-2">
        <h3 class="mb-md">{{ t('settings_view.ad_tag_title') }}</h3>
        <div class="setting-toggle">
          <label class="checkbox-label">
            <input v-model="form.use_middle_proxy" type="checkbox" />
            {{ t('settings_view.use_middle_proxy') }}
          </label>
          <small class="text-muted">{{ t('settings_view.use_middle_proxy_hint') }}</small>
        </div>
        <div class="form-group">
          <label class="form-label">{{ t('settings_view.ad_tag_label') }}</label>
          <input v-model="form.ad_tag" class="input" placeholder="32 hex characters" maxlength="32" :disabled="!form.use_middle_proxy" />
        </div>
      </div>

      <!-- Maintenance -->
      <div class="card span-2">
        <h3 class="mb-md">{{ t('settings_view.maintenance') }}</h3>
        <label class="checkbox-label mb-md">
          <input v-model="form.auto_update_enabled" type="checkbox" />
          {{ t('settings_view.auto_update') }}
        </label>
        <label class="checkbox-label mb-md">
          <input v-model="form.maintenance_mode" type="checkbox" />
          {{ t('settings_view.maintenance_mode') }}
        </label>
        <label class="checkbox-label mb-md">
          <input v-model="form.debug" type="checkbox" />
          {{ t('settings_view.debug_mode') }}
        </label>
        <div class="form-group">
          <label class="form-label">{{ t('settings_view.auto_rotate_days') }}</label>
          <input v-model.number="form.secret_auto_rotate_days" class="input input-narrow" type="number" min="0" :placeholder="t('settings_view.disabled_placeholder')" />
          <small class="text-muted">{{ t('settings_view.hint_auto_rotate_days') }}</small>
        </div>
      </div>

      <!-- TCP Optimizations -->
      <div class="card span-2">
        <h3 class="mb-md">{{ t('settings_view.tcp_optimizations') }}</h3>
        <p class="text-muted text-sm mb-md">{{ t('settings_view.tcp_optimizations_desc') }}</p>
        <label class="checkbox-label">
          <input v-model="form.sysctl_optimizations_enabled" type="checkbox" />
          {{ t('settings_view.enable_tcp_optimizations') }}
        </label>
      </div>

      <!-- Backup -->
      <div class="card span-2">
        <h3 class="mb-md">{{ t('settings_view.backup_title') }}</h3>
        <p class="text-muted text-sm mb-md">{{ t('settings_view.backup_desc') }}</p>
        <div class="form-group">
          <label class="form-label">{{ t('settings_view.backup_retention') }}</label>
          <input v-model.number="form.backup_retention_days" class="input input-narrow" type="number" min="1" />
          <small class="text-muted">{{ t('settings_view.hint_backup_retention') }}</small>
        </div>
      </div>

      <!-- Change Password -->
      <div class="card span-6">
        <h3 class="mb-md">{{ t('auth.change_password') }}</h3>
        <div class="settings-grid">
          <div class="form-group">
            <label class="form-label">{{ t('auth.current_password') }}</label>
            <input v-model="pwd.current" class="input" type="password" autocomplete="current-password" />
          </div>
          <div class="form-group">
            <label class="form-label">{{ t('auth.new_password') }}</label>
            <input v-model="pwd.newPassword" class="input" type="password" autocomplete="new-password" :placeholder="t('auth.password_min')" />
          </div>
          <div class="form-group">
            <label class="form-label">{{ t('auth.confirm_new_password') }}</label>
            <input v-model="pwd.confirm" class="input" type="password" autocomplete="new-password" />
          </div>
        </div>
        <div class="flex items-center gap-md mt-md">
          <button class="btn btn-primary" :disabled="changingPwd" @click="handleChangePassword">
            <Loader2 v-if="changingPwd" :size="16" class="animate-spin" />
            {{ changingPwd ? t('auth.changing') : t('auth.change_password') }}
          </button>
        </div>
        <div v-if="pwdMessage" class="alert mt-md" :class="pwdError ? 'alert-danger' : 'alert-success'">
          {{ pwdMessage }}
        </div>
      </div>

      <!-- Actions -->
      <div class="card span-6">
        <div class="flex justify-between items-center">
          <a href="/swagger/index.html" target="_blank" rel="noopener" class="btn btn-secondary btn-sm flex items-center gap-xs">
            <BookOpen :size="16" />
            {{ t('settings_view.api_docs') }}
          </a>
          <div class="flex gap-sm">
            <button class="btn btn-secondary" @click="resetForm">{{ t('settings_view.reset') }}</button>
            <button class="btn btn-primary" :disabled="saving" @click="handleSave">
              <Loader2 v-if="saving" :size="16" class="animate-spin" />
              {{ saving ? t('settings_view.saving') : t('common.save') }}
            </button>
          </div>
        </div>
        <div v-if="saveMessage" class="alert mt-md" :class="saveError ? 'alert-danger' : 'alert-success'">
          {{ saveMessage }}
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import {onMounted, reactive, ref} from 'vue'
import {useI18n} from 'vue-i18n'
import {useConfigStore} from '@/stores/config'
import {authApi} from '@/api/endpoints'
import {BookOpen, Loader2} from '@lucide/vue'
import type {Settings} from '@/types/models'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'

const { t } = useI18n()
const configStore = useConfigStore()

const form = ref<Partial<Settings>>({})
const saving = ref(false)
const saveMessage = ref('')
const saveError = ref(false)

const pwd = reactive({ current: '', newPassword: '', confirm: '' })
const changingPwd = ref(false)
const pwdMessage = ref('')
const pwdError = ref(false)

function resetForm() {
  if (configStore.settings) {
    form.value = { ...configStore.settings }
  }
}

async function handleSave() {
  saving.value = true
  saveMessage.value = ''
  saveError.value = false
  try {
    await configStore.update(form.value)
    saveMessage.value = t('settings_view.saved_success')
  } catch (e: any) {
    saveMessage.value = e.message
    saveError.value = true
  } finally {
    saving.value = false
  }
}

async function handleChangePassword() {
  pwdMessage.value = ''
  pwdError.value = false

  if (!pwd.current || !pwd.newPassword || !pwd.confirm) return
  if (pwd.newPassword !== pwd.confirm) {
    pwdMessage.value = t('auth.password_mismatch')
    pwdError.value = true
    return
  }

  changingPwd.value = true
  try {
    await authApi.changePassword(pwd.current, pwd.newPassword)
    pwdMessage.value = t('auth.password_changed')
    pwd.current = ''
    pwd.newPassword = ''
    pwd.confirm = ''
  } catch (e: any) {
    const data = e.response?.data
    pwdMessage.value = data?.error || t('auth.password_change_failed')
    pwdError.value = true
  } finally {
    changingPwd.value = false
  }
}

onMounted(async () => {
  await configStore.load()
  resetForm()
})
</script>

<style scoped lang="scss">
@use '@/assets/scss/variables' as *;

.settings-layout {
  display: grid;
  grid-template-columns: repeat(6, minmax(0, 1fr));
  gap: $spacing-md;
  align-items: stretch;

  .span-2 { grid-column: span 2; }
  .span-3 { grid-column: span 3; }
  .span-6 { grid-column: span 6; }

  // Tablet: thirds become halves
  @media (max-width: 1100px) {
    .span-2 { grid-column: span 3; }
  }

  // Mobile: single column
  @media (max-width: 768px) {
    .span-2,
    .span-3 { grid-column: span 6; }
  }
}

.settings-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
  gap: $spacing-md;

  @media (max-width: 480px) {
    grid-template-columns: 1fr;
  }
}

// Checkbox with its explanation as one visual unit, clearly separated
// from the following field.
.setting-toggle {
  display: flex;
  flex-direction: column;
  gap: $spacing-xs;
  margin-bottom: $spacing-md;
  padding-bottom: $spacing-md;
  border-bottom: 1px solid $border-color;

  small {
    line-height: 1.4;
  }
}

.input-narrow {
  max-width: 200px;
}
</style>
