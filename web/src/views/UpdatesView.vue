<template>
  <div>
    <div class="card mb-lg">
      <h3 class="mb-md">Check for Updates</h3>

      <div class="status-row mb-md">
        <span>Current: <code>v{{ updateStore.status?.current || '—' }}</code></span>
        <span>Latest: <code>v{{ updateStore.status?.latest || '—' }}</code></span>
        <StatusBadge v-if="updateStore.status" :variant="updateStore.status.update_available ? 'warning' : 'success'">
          {{ updateStore.status.update_available ? 'Update Available' : 'Up to Date' }}
        </StatusBadge>
      </div>

      <div class="flex gap-sm">
        <button class="btn btn-primary" :disabled="updateStore.loading" @click="updateStore.check()">
          {{ updateStore.loading ? 'Checking...' : 'Check' }}
        </button>
        <button v-if="updateStore.status?.update_available" class="btn btn-warning"
                :disabled="updateStore.applying" @click="handleApply">
          {{ updateStore.applying ? 'Applying...' : 'Apply Update' }}
        </button>
      </div>

      <div v-if="updateStore.error" class="alert alert-danger mt-md">{{ updateStore.error }}</div>
      <div v-if="updateStore.result" class="alert alert-success mt-md">
        Updated from v{{ updateStore.result.previous_version }} to v{{ updateStore.result.new_version }}.
        Restart the service to apply changes.
      </div>
    </div>

    <div class="card">
      <h3 class="mb-md">Auto-Update</h3>
      <p class="text-muted text-sm mb-md">
        PopuGate checks for updates periodically via the scheduler. You can also manually check and apply updates here.
        When applying an update, the binary is downloaded and the previous version is backed up.
      </p>
      <div class="alert alert-info">
        After applying an update, restart the service for changes to take effect.
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import { useUpdateStore } from '@/stores/update'
import StatusBadge from '@/components/common/StatusBadge.vue'

const updateStore = useUpdateStore()

async function handleApply() {
  if (confirm('Apply the update? The service will need to be restarted.')) {
    await updateStore.apply()
  }
}

onMounted(() => updateStore.check())
</script>
