<template>
  <div>
    <!-- Warning message if geoblocking is unavailable -->
    <div v-if="!geoblockStore.available" class="alert alert-danger mb-lg">
      <AlertTriangle :size="18" class="flex-shrink-0" />
      <div>
        <div style="font-weight: 600;">{{ t('geoblock.unavailable') }}</div>
        <div style="margin-top: 4px; opacity: 0.9;">
          {{ t('geoblock.unavailable_reason', { reason: geoblockStore.error }) }}
        </div>
      </div>
    </div>

    <div class="card mb-lg" :class="{ 'card-disabled': !geoblockStore.available }">
      <h3 class="mb-md">{{ t('geoblock.title') }}</h3>

      <div class="form-row mb-lg">
        <div class="form-group">
          <label class="form-label">{{ t('geoblock.mode') }}</label>
          <select v-model="localMode" class="select" :disabled="geoblockStore.loading || !geoblockStore.available" @change="handleModeChange">
            <option value="blacklist">{{ t('geoblock.blacklist_desc') }}</option>
            <option value="whitelist">{{ t('geoblock.whitelist_desc') }}</option>
          </select>
        </div>
        <div class="form-group">
          <label class="form-label">{{ t('geoblock.add_country') }}</label>
          <div class="input-group">
            <input v-model="countryInput" class="input" :placeholder="t('geoblock.country_placeholder')"
                   :disabled="geoblockStore.loading || !geoblockStore.available"
                   @keydown.enter="handleAddCountry" />
            <button class="btn btn-primary" :disabled="geoblockStore.loading || !geoblockStore.available" @click="handleAddCountry">
              <Loader2 v-if="geoblockStore.loading" :size="16" class="animate-spin" />
              {{ t('common.add') }}
            </button>
          </div>
        </div>
      </div>

      <div v-if="geoblockStore.countries && geoblockStore.countries.length" class="country-tags">
        <span v-for="cc in geoblockStore.countries" :key="cc" class="country-tag">
          {{ cc }}
          <button class="country-remove" :disabled="geoblockStore.loading || !geoblockStore.available" @click="geoblockStore.removeCountry(cc)">&times;</button>
        </span>
      </div>
      <div v-else class="text-muted">{{ t('geoblock.empty') }}</div>

      <button v-if="geoblockStore.countries && geoblockStore.countries.length" class="btn btn-danger btn-sm mt-md"
              :disabled="geoblockStore.loading || !geoblockStore.available" @click="geoblockStore.clear()">
        <Loader2 v-if="geoblockStore.loading" :size="14" class="animate-spin" />
        {{ t('geoblock.clear_all') }}
      </button>
    </div>

    <div class="card">
      <h3 class="mb-md">{{ t('geoblock.help') }}</h3>
      <div class="help-blocks">
        <div class="help-block">
          <strong class="help-block-title">{{ t('geoblock.blacklist_desc') }}</strong>
          <span class="text-muted text-sm">{{ t('geoblock.blacklist_help') }}</span>
        </div>
        <div class="help-block">
          <strong class="help-block-title">{{ t('geoblock.whitelist_desc') }}</strong>
          <span class="text-muted text-sm">{{ t('geoblock.whitelist_help') }}</span>
        </div>
      </div>
      <p class="mt-sm text-muted text-sm">
        {{ t('geoblock.help_tip') }}
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import {onMounted, ref} from 'vue'
import {useI18n} from 'vue-i18n'
import {useGeoblockStore} from '@/stores'
import {Loader2, AlertTriangle} from '@lucide/vue'

const { t } = useI18n()
const geoblockStore = useGeoblockStore()

const localMode = ref<'blacklist' | 'whitelist'>('blacklist')
const countryInput = ref('')

async function handleModeChange() {
  await geoblockStore.setMode(localMode.value)
}

async function handleAddCountry() {
  const codes = countryInput.value.split(',').map((c) => c.trim().toUpperCase()).filter(Boolean)
  for (const code of codes) {
    if (/^[A-Z]{2}$/.test(code)) {
      try {
        await geoblockStore.addCountry(code)
      } catch {
        // Error toast shown by API interceptor; continue with remaining codes
      }
    }
  }
  countryInput.value = ''
}

onMounted(async () => {
  await geoblockStore.load()
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
  &:hover:not(:disabled) { opacity: 1; }
  &:disabled { cursor: not-allowed; opacity: 0.3; }
}

.card-disabled {
  opacity: 0.6;
  * {
    pointer-events: none !important;
  }
}

.help-blocks { display: flex; flex-direction: column; gap: $spacing-sm; }
.help-block {
  display: flex;
  flex-direction: column;
  gap: $spacing-xs;
  padding: $spacing-sm $spacing-md;
  background: $color-info-bg;
  border-radius: $border-radius;
  border: 1px solid var(--alert-info-border);
}
.help-block-title { font-size: $font-size-sm; color: var(--badge-info-text); }
</style>
