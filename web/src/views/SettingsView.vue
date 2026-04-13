<template>
  <div>
    <LoadingSpinner v-if="configStore.loading" message="Loading settings..." />

    <template v-else-if="configStore.settings">
      <!-- Proxy Settings -->
      <div class="card mb-lg">
        <h3 class="mb-md">Proxy</h3>
        <div class="settings-grid">
          <div class="form-group">
            <label class="form-label">Port</label>
            <input v-model.number="form.proxy_port" class="input" type="number" min="1" max="65535" />
          </div>
          <div class="form-group">
            <label class="form-label">Metrics Port</label>
            <input v-model.number="form.proxy_metrics_port" class="input" type="number" min="1" max="65535" />
          </div>
          <div class="form-group">
            <label class="form-label">Domain</label>
            <input v-model="form.proxy_domain" class="input" placeholder="cloudflare.com" />
          </div>
          <div class="form-group">
            <label class="form-label">Concurrency</label>
            <input v-model.number="form.proxy_concurrency" class="input" type="number" min="1" />
          </div>
          <div class="form-group">
            <label class="form-label">CPU Limit</label>
            <input v-model="form.proxy_cpus" class="input" placeholder="empty = unlimited" />
          </div>
          <div class="form-group">
            <label class="form-label">Memory Limit</label>
            <input v-model="form.proxy_memory" class="input" placeholder="e.g. 256m, 1g" />
          </div>
          <div class="form-group">
            <label class="form-label">Custom IP</label>
            <input v-model="form.custom_ip" class="input" placeholder="empty = auto-detect" />
          </div>
          <div class="form-group">
            <label class="form-label">Cert Length</label>
            <input v-model.number="form.fake_cert_len" class="input" type="number" />
          </div>
        </div>
      </div>

      <!-- Proxy Protocol -->
      <div class="card mb-lg">
        <h3 class="mb-md">HAProxy PROXY Protocol</h3>
        <label class="checkbox-label mb-md">
          <input v-model="form.proxy_protocol" type="checkbox" />
          Enable PROXY Protocol
        </label>
        <div class="form-group">
          <label class="form-label">Trusted CIDRs</label>
          <input v-model="form.proxy_protocol_trusted_cidrs" class="input" placeholder="10.0.0.0/8,172.16.0.0/12" />
        </div>
      </div>

      <!-- Masking -->
      <div class="card mb-lg">
        <h3 class="mb-md">Traffic Masking</h3>
        <label class="checkbox-label mb-md">
          <input v-model="form.masking_enabled" type="checkbox" />
          Enable Masking
        </label>
        <div class="settings-grid">
          <div class="form-group">
            <label class="form-label">Masking Host</label>
            <input v-model="form.masking_host" class="input" placeholder="empty = use domain" />
          </div>
          <div class="form-group">
            <label class="form-label">Masking Port</label>
            <input v-model.number="form.masking_port" class="input" type="number" />
          </div>
          <div class="form-group">
            <label class="form-label">Unknown SNI Action</label>
            <select v-model="form.unknown_sni_action" class="select">
              <option value="mask">Mask (forward to real site)</option>
              <option value="drop">Drop</option>
            </select>
          </div>
        </div>
      </div>

      <!-- Ad Tag -->
      <div class="card mb-lg">
        <h3 class="mb-md">Ad Tag</h3>
        <div class="form-group">
          <label class="form-label">Ad Tag (from @MTProxyBot)</label>
          <input v-model="form.ad_tag" class="input" placeholder="32 hex characters" maxlength="32" />
        </div>
      </div>

      <!-- Auto-Update -->
      <div class="card mb-lg">
        <h3 class="mb-md">Maintenance</h3>
        <label class="checkbox-label mb-md">
          <input v-model="form.auto_update_enabled" type="checkbox" />
          Enable automatic update checks
        </label>
        <label class="checkbox-label">
          <input v-model="form.debug" type="checkbox" />
          Enable Debug Mode (Gin verbose logging)
        </label>
      </div>

      <!-- Actions -->
      <div class="card">
        <div class="flex justify-between items-center">
          <h3>Save Settings</h3>
          <div class="flex gap-sm">
            <button class="btn btn-secondary" @click="resetForm">Reset</button>
            <button class="btn btn-primary" :disabled="saving" @click="handleSave">
              {{ saving ? 'Saving...' : 'Save' }}
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
import { useConfigStore } from '@/stores/config'
import type { Settings } from '@/types/models'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'

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
    saveMessage.value = 'Settings saved successfully'
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
}
</style>
