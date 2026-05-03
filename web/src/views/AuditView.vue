<template>
  <div>
    <PageHeader>
      <button class="btn btn-secondary btn-sm" @click="auditStore.load()" :disabled="auditStore.loading">
        <RotateCw :size="16" :class="{ 'animate-spin': auditStore.loading }" />
      </button>
    </PageHeader>

    <DataTable
      :columns="columns"
      :items="auditStore.entries"
      :loading="auditStore.loading"
      :empty-icon="FileText"
      :empty-message="t('audit.empty')"
      row-key="id"
    >
      <template #cell-timestamp="{ item }">
        {{ new Date(item.timestamp * 1000).toLocaleString() }}
      </template>
      <template #cell-action="{ item }">
        <StatusBadge :variant="actionVariant(item.action)">{{ item.action }}</StatusBadge>
      </template>
      <template #cell-detail="{ item }">
        <span v-if="item.detail" class="text-sm text-muted truncate details-cell" v-tooltip="item.detail">
          {{ truncate(item.detail, 80) }}
        </span>
      </template>
    </DataTable>

    <div v-if="auditStore.entries.length > 0" class="text-center mt-md">
      <button class="btn btn-secondary btn-sm" @click="auditStore.loadMore()" :disabled="auditStore.loading">
        {{ t('audit.load_more') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuditStore } from '@/stores'
import DataTable from '@/components/common/DataTable.vue'
import StatusBadge from '@/components/common/StatusBadge.vue'
import PageHeader from '@/components/common/PageHeader.vue'
import { FileText, RotateCw } from '@lucide/vue'

const { t } = useI18n()
const auditStore = useAuditStore()

const columns = computed(() => [
  { key: 'timestamp', header: t('audit.table.time') },
  { key: 'action', header: t('audit.table.action') },
  { key: 'user', header: t('audit.table.actor') },
  { key: 'detail', header: t('audit.table.details') },
])

function actionVariant(action: string): 'success' | 'warning' | 'danger' | 'neutral' {
  if (action.includes('create') || action.includes('enable')) return 'success'
  if (action.includes('rotate') || action.includes('archive')) return 'warning'
  if (action.includes('delete') || action.includes('disable')) return 'danger'
  return 'neutral'
}

function truncate(s: string, n: number): string {
  return s.length > n ? s.slice(0, n) + '...' : s
}

onMounted(() => auditStore.load())
</script>

<style scoped lang="scss">
.details-cell {
  display: inline-block;
  max-width: 300px;
}
</style>
