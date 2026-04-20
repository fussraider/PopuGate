<template>
  <div>
    <div class="card mb-lg">
      <h3 class="mb-md">{{ t('updates.check') }}</h3>

      <div class="status-row mb-md">
        <span>{{ t('updates.current') }}: <code>v{{ updateStore.status?.current || '—' }}</code></span>
        <span>{{ t('updates.latest') }}: <code>v{{ updateStore.status?.latest || '—' }}</code></span>
        <StatusBadge v-if="updateStore.status" :variant="updateStore.status.update_available ? 'warning' : 'success'">
          {{ updateStore.status.update_available ? t('updates.available') : t('updates.up_to_date') }}
        </StatusBadge>
      </div>

      <div class="flex gap-sm">
        <button class="btn btn-primary" :disabled="updateStore.loading" @click="updateStore.check()">
          <Loader2 v-if="updateStore.loading" :size="16" class="animate-spin" />
          {{ updateStore.loading ? t('updates.checking') : t('updates.check_btn') }}
        </button>
        <button v-if="updateStore.status?.update_available" class="btn btn-warning"
                :disabled="updateStore.applying" @click="handleApply">
          <Loader2 v-if="updateStore.applying" :size="16" class="animate-spin" />
          {{ updateStore.applying ? t('updates.applying') : t('updates.apply') }}
        </button>
      </div>

      <div v-if="updateStore.error" class="alert alert-danger mt-md">{{ updateStore.error }}</div>
      <div v-if="updateStore.result" class="alert alert-success mt-md">
        {{ t('updates.success', { old: updateStore.result.previous_version, new: updateStore.result.new_version }) }}
      </div>
    </div>

    <div class="card">
      <h3 class="mb-md">{{ t('updates.auto_update') }}</h3>
      <p class="text-muted text-sm mb-md">
        {{ t('updates.maintenance_desc') }}
      </p>
      <div class="alert alert-info">
        {{ t('updates.apply_tip') }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useUpdateStore } from '@/stores/update'
import { Loader2 } from '@lucide/vue'
import StatusBadge from '@/components/common/StatusBadge.vue'

const { t } = useI18n()
const updateStore = useUpdateStore()

async function handleApply() {
  if (confirm(t('updates.confirm_apply'))) {
    await updateStore.apply()
  }
}

onMounted(() => updateStore.check())
</script>
