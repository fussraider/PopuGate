<template>
  <div>
    <div class="card mb-lg">
      <h3 class="mb-md">Geo-Blocking Configuration</h3>

      <div class="form-row mb-lg">
        <div class="form-group">
          <label class="form-label">Mode</label>
          <select v-model="localMode" class="select" @change="handleModeChange">
            <option value="blacklist">Blacklist (block listed countries)</option>
            <option value="whitelist">Whitelist (allow only listed countries)</option>
          </select>
        </div>
        <div class="form-group">
          <label class="form-label">Add Country</label>
          <div class="input-group">
            <input v-model="countryInput" class="input" placeholder="US, DE, CN..."
                   @keydown.enter="handleAddCountry" />
            <button class="btn btn-primary" :disabled="geoblockStore.loading" @click="handleAddCountry">Add</button>
          </div>
        </div>
      </div>

      <div v-if="geoblockStore.countries && geoblockStore.countries.length" class="country-tags">
        <span v-for="cc in geoblockStore.countries" :key="cc" class="country-tag">
          {{ cc }}
          <button class="country-remove" @click="geoblockStore.removeCountry(cc)">&times;</button>
        </span>
      </div>
      <div v-else class="text-muted">No countries configured.</div>

      <button v-if="geoblockStore.countries && geoblockStore.countries.length" class="btn btn-danger btn-sm mt-md"
              @click="geoblockStore.clear()">Clear All</button>
    </div>

    <div class="card">
      <h3 class="mb-md">Help</h3>
      <div class="alert alert-info">
        <strong>Blacklist mode:</strong> Traffic from listed countries will be blocked.<br />
        <strong>Whitelist mode:</strong> Only traffic from listed countries will be allowed.
      </div>
      <p class="mt-sm text-muted text-sm">
        Country codes should be 2-letter ISO codes (e.g., US, DE, CN, RU).
        Changes take effect immediately via iptables/ipset rules.
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useGeoblockStore, useConfigStore } from '@/stores'

const geoblockStore = useGeoblockStore()
const configStore = useConfigStore()

const localMode = ref<'blacklist' | 'whitelist'>('blacklist')
const countryInput = ref('')

async function handleModeChange() {
  await geoblockStore.setMode(localMode.value)
}

async function handleAddCountry() {
  const codes = countryInput.value.split(',').map((c) => c.trim().toUpperCase()).filter(Boolean)
  for (const code of codes) {
    if (/^[A-Z]{2}$/.test(code)) {
      await geoblockStore.addCountry(code)
    }
  }
  countryInput.value = ''
}

onMounted(async () => {
  await configStore.load()
  if (configStore.settings) geoblockStore.load(configStore.settings)
  localMode.value = geoblockStore.mode
})
</script>

<style scoped lang="scss">
@use '@/assets/scss/variables' as *;

.country-tags { display: flex; flex-wrap: wrap; gap: $spacing-sm; }
.country-tag {
  display: inline-flex;
  align-items: center;
  gap: $spacing-xs;
  padding: 4px 10px;
  background: $color-primary-bg;
  color: $color-primary-dark;
  border-radius: 999px;
  font-size: $font-size-sm;
  font-weight: $font-weight-medium;
}
.country-remove {
  background: none;
  border: none;
  cursor: pointer;
  color: inherit;
  opacity: 0.6;
  &:hover { opacity: 1; }
}
</style>
