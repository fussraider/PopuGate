<template>
  <div>
    <div class="card mb-lg">
      <h3 class="mb-md">{{ t('geoblock.title') }}</h3>

      <div class="form-row mb-lg">
        <div class="form-group">
          <label class="form-label">{{ t('geoblock.mode') }}</label>
          <select v-model="localMode" class="select" @change="handleModeChange">
            <option value="blacklist">{{ t('geoblock.blacklist_desc') }}</option>
            <option value="whitelist">{{ t('geoblock.whitelist_desc') }}</option>
          </select>
        </div>
        <div class="form-group">
          <label class="form-label">{{ t('geoblock.add_country') }}</label>
          <div class="input-group">
            <input v-model="countryInput" class="input" :placeholder="t('geoblock.country_placeholder')"
                   @keydown.enter="handleAddCountry" />
            <button class="btn btn-primary" :disabled="geoblockStore.loading" @click="handleAddCountry">
              <Loader2 v-if="geoblockStore.loading" :size="16" class="animate-spin" />
              {{ t('common.add') }}
            </button>
          </div>
        </div>
      </div>

      <div v-if="geoblockStore.countries && geoblockStore.countries.length" class="country-tags">
        <span v-for="cc in geoblockStore.countries" :key="cc" class="country-tag">
          {{ cc }}
          <button class="country-remove" @click="geoblockStore.removeCountry(cc)">&times;</button>
        </span>
      </div>
      <div v-else class="text-muted">{{ t('geoblock.empty') }}</div>

      <button v-if="geoblockStore.countries && geoblockStore.countries.length" class="btn btn-danger btn-sm mt-md"
              :disabled="geoblockStore.loading" @click="geoblockStore.clear()">
        <Loader2 v-if="geoblockStore.loading" :size="14" class="animate-spin" />
        {{ t('geoblock.clear_all') }}
      </button>
    </div>

    <div class="card">
      <h3 class="mb-md">{{ t('geoblock.help') }}</h3>
      <div class="alert alert-info">
        <strong>{{ t('geoblock.blacklist_desc') }}:</strong> {{ t('geoblock.blacklist_help') }}<br />
        <strong>{{ t('geoblock.whitelist_desc') }}:</strong> {{ t('geoblock.whitelist_help') }}
      </div>
      <p class="mt-sm text-muted text-sm">
        {{ t('geoblock.help_tip') }}
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useGeoblockStore, useConfigStore } from '@/stores'
import { Loader2 } from '@lucide/vue'

const { t } = useI18n()
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
