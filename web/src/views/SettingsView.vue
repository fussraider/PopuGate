<template>
  <div>
    <LoadingSpinner v-if="configStore.loading" :message="t('common.loading')" />

    <template v-else-if="configStore.settings">
      <!-- Proxy Settings -->
      <div class="card mb-lg">
        <h3 class="mb-md">{{ t('settings_view.title') }}</h3>
        <div class="settings-grid">
          <div class="form-group">
            <label class="form-label">{{ t('instances.table.port') }}</label>
            <input v-model.number="form.proxy_port" class="input" type="number" min="1" max="65535" />
          </div>
          <div class="form-group">
            <label class="form-label">{{ t('settings_view.metrics_port') }}</label>
            <input v-model.number="form.proxy_metrics_port" class="input" type="number" min="1" max="65535" />
          </div>
          <div class="form-group">
            <label class="form-label">{{ t('settings_view.domain') }}</label>
            <input v-model="form.proxy_domain" class="input" placeholder="cloudflare.com" />
          </div>
          <div class="form-group">
            <label class="form-label">{{ t('settings_view.concurrency') }}</label>
            <input v-model.number="form.proxy_concurrency" class="input" type="number" min="1" />
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
          </div>
        </div>
      </div>

      <!-- Proxy Protocol -->
      <div class="card mb-lg">
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

      <!-- Masking -->
      <div class="card mb-lg">
        <h3 class="mb-md">{{ t('settings_view.masking_title') }}</h3>
        <label class="checkbox-label mb-md">
          <input v-model="form.masking_enabled" type="checkbox" />
          {{ t('settings_view.enable_masking') }}
        </label>
        <div class="settings-grid">
          <div class="form-group">
            <label class="form-label">{{ t('settings_view.masking_host') }}</label>
            <input v-model="form.masking_host" class="input" :placeholder="t('settings_view.masking_host_tip')" />
          </div>
          <div class="form-group">
            <label class="form-label">{{ t('settings_view.masking_port') }}</label>
            <input v-model.number="form.masking_port" class="input" type="number" />
          </div>
          <div class="form-group">
            <label class="form-label">{{ t('settings_view.unknown_sni') }}</label>
            <select v-model="form.unknown_sni_action" class="select">
              <option value="mask">{{ t('settings_view.mask_action') }}</option>
              <option value="drop">{{ t('settings_view.drop_action') }}</option>
            </select>
          </div>
        </div>
      </div>

      <!-- Ad Tag -->
      <div class="card mb-lg">
        <h3 class="mb-md">{{ t('settings_view.ad_tag_title') }}</h3>
        <div class="form-group">
          <label class="form-label">{{ t('settings_view.ad_tag_label') }}</label>
          <input v-model="form.ad_tag" class="input" placeholder="32 hex characters" maxlength="32" />
        </div>
      </div>

      <!-- telemt Engine -->
      <div class="card mb-lg">
        <h3 class="mb-md">{{ t('settings_view.telemt_engine') }}</h3>
        <p class="text-muted text-sm mb-md">{{ t('settings_view.telemt_engine_desc') }}</p>
        <div class="settings-grid">
          <div class="form-group">
            <label class="form-label">{{ t('settings_view.telemt_version') }}</label>
            <input v-model="form.telemt_version" class="input" :placeholder="t('settings_view.telemt_version_placeholder')" />
          </div>
          <div class="form-group">
            <label class="form-label">{{ t('settings_view.telemt_commit') }}</label>
            <input v-model="form.telemt_commit" class="input" :placeholder="t('settings_view.telemt_commit_placeholder')" />
          </div>
        </div>
      </div>

      <!-- Auto-Update -->
      <div class="card mb-lg">
        <h3 class="mb-md">{{ t('settings_view.maintenance') }}</h3>
        <label class="checkbox-label mb-md">
          <input v-model="form.auto_update_enabled" type="checkbox" />
          {{ t('settings_view.auto_update') }}
        </label>
        <label class="checkbox-label">
          <input v-model="form.debug" type="checkbox" />
          {{ t('settings_view.debug_mode') }}
        </label>
      </div>

      <!-- Actions -->
      <div class="card">
        <div class="flex justify-between items-center">
          <h3>{{ t('settings_view.save_settings') }}</h3>
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
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useConfigStore } from '@/stores/config'
import { Loader2 } from '@lucide/vue'
import type { Settings } from '@/types/models'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'

const { t } = useI18n()
const configStore = useConfigStore()

const form = ref<Partial<Settings>>({})
const saving = ref(false)
const saveMessage = ref('')
const saveError = ref(false)

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

onMounted(async () => {
  await configStore.load()
  resetForm()
})
</script>

<style scoped lang="scss">
@use '@/assets/scss/variables' as *;

.settings-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
  gap: $spacing-md;

  @media (max-width: 480px) {
    grid-template-columns: 1fr;
  }
}
</style>
