<template>
  <div>

    <!-- Filters Bar -->
    <div class="filter-bar">
      <!-- Period Filter -->
      <div class="filter-group">
        <span class="filter-label">{{ t('audit.filter.period') }}</span>
        <select v-model="auditStore.period" class="select select-sm filter-control">
          <option value="all">{{ t('audit.filter.periods.all') }}</option>
          <option value="today">{{ t('audit.filter.periods.today') }}</option>
          <option value="yesterday">{{ t('audit.filter.periods.yesterday') }}</option>
          <option value="week">{{ t('audit.filter.periods.week') }}</option>
          <option value="month">{{ t('audit.filter.periods.month') }}</option>
          <option value="custom">{{ t('audit.filter.periods.custom') }}</option>
        </select>
      </div>

      <!-- Custom Date Inputs -->
      <div v-if="auditStore.period === 'custom'" class="filter-group custom-dates">
        <span class="filter-label">{{ t('audit.filter.date_range') }}</span>
        <div class="date-inputs">
          <input type="date" v-model="startDateStr" class="input input-sm date-input" :placeholder="t('audit.filter.from')" />
          <span class="date-sep">—</span>
          <input type="date" v-model="endDateStr" class="input input-sm date-input" :placeholder="t('audit.filter.to')" />
        </div>
      </div>

      <!-- Authors Filter (Multi-select) -->
      <div class="filter-group">
        <span class="filter-label">{{ t('audit.filter.users') }}</span>
        <div class="dropdown-multiselect" ref="usersDropdownRef">
          <button
            class="dropdown-trigger"
            :class="{ 'is-open': usersDropdownOpen, 'has-value': auditStore.selectedUsers.length > 0 }"
            @click.stop="toggleDropdown('users')"
          >
            <span class="trigger-text">
              {{ auditStore.selectedUsers.length > 0
                ? `${t('audit.filter.selected')}: ${auditStore.selectedUsers.length}`
                : t('audit.filter.all_users') }}
            </span>
            <ChevronDown :size="14" class="trigger-icon" :class="{ 'rotated': usersDropdownOpen }" />
          </button>
          <div v-if="usersDropdownOpen" class="dropdown-menu">
            <div v-if="auditStore.availableUsers.length === 0" class="dropdown-empty">
              {{ t('audit.filter.no_options') }}
            </div>
            <label v-for="user in auditStore.availableUsers" :key="user" class="dropdown-item" @click.stop>
              <input type="checkbox" :value="user" v-model="auditStore.selectedUsers" />
              <span class="item-text">{{ user }}</span>
            </label>
          </div>
        </div>
      </div>

      <!-- Actions Filter (Multi-select) -->
      <div class="filter-group">
        <span class="filter-label">{{ t('audit.filter.actions') }}</span>
        <div class="dropdown-multiselect" ref="actionsDropdownRef">
          <button
            class="dropdown-trigger"
            :class="{ 'is-open': actionsDropdownOpen, 'has-value': auditStore.selectedActions.length > 0 }"
            @click.stop="toggleDropdown('actions')"
          >
            <span class="trigger-text">
              {{ auditStore.selectedActions.length > 0
                ? `${t('audit.filter.selected')}: ${auditStore.selectedActions.length}`
                : t('audit.filter.all_actions') }}
            </span>
            <ChevronDown :size="14" class="trigger-icon" :class="{ 'rotated': actionsDropdownOpen }" />
          </button>
          <div v-if="actionsDropdownOpen" class="dropdown-menu dropdown-menu-right">
            <div v-if="auditStore.availableActions.length === 0" class="dropdown-empty">
              {{ t('audit.filter.no_options') }}
            </div>
            <label v-for="action in auditStore.availableActions" :key="action" class="dropdown-item" @click.stop>
              <input type="checkbox" :value="action" v-model="auditStore.selectedActions" />
              <span class="item-text">{{ action }}</span>
            </label>
          </div>
        </div>
      </div>

      <!-- Reset & Refresh -->
      <div class="filter-actions">
        <button
          v-if="hasActiveFilters"
          class="btn btn-ghost btn-sm text-danger reset-btn"
          @click="auditStore.resetFilters"
        >
          <X :size="14" />
          {{ t('audit.filter.reset') }}
        </button>
        <button class="btn btn-secondary btn-sm" @click="refreshData" :disabled="auditStore.loading" v-tooltip="t('audit.filter.refresh')">
          <RotateCw :size="16" :class="{ 'animate-spin': auditStore.loading }" />
        </button>
      </div>
    </div>

    <!-- Audit Data Table -->
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

    <div v-if="auditStore.hasMore && auditStore.entries.length > 0" class="text-center mt-md">
      <button class="btn btn-secondary btn-sm" @click="auditStore.loadMore()" :disabled="auditStore.loading">
        {{ t('audit.load_more') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuditStore } from '@/stores'
import DataTable from '@/components/common/DataTable.vue'
import StatusBadge from '@/components/common/StatusBadge.vue'
import { FileText, RotateCw, ChevronDown, X } from '@lucide/vue'

const { t } = useI18n()
const auditStore = useAuditStore()

const columns = computed(() => [
  { key: 'timestamp', header: t('audit.table.time') },
  { key: 'action', header: t('audit.table.action') },
  { key: 'user', header: t('audit.table.actor') },
  { key: 'detail', header: t('audit.table.details') },
])

const usersDropdownOpen = ref(false)
const actionsDropdownOpen = ref(false)
const usersDropdownRef = ref<HTMLElement | null>(null)
const actionsDropdownRef = ref<HTMLElement | null>(null)

function toggleDropdown(which: 'users' | 'actions') {
  if (which === 'users') {
    usersDropdownOpen.value = !usersDropdownOpen.value
    if (usersDropdownOpen.value) actionsDropdownOpen.value = false
  } else {
    actionsDropdownOpen.value = !actionsDropdownOpen.value
    if (actionsDropdownOpen.value) usersDropdownOpen.value = false
  }
}

const startDateStr = ref('')
const endDateStr = ref('')

const hasActiveFilters = computed(() => {
  return (
    auditStore.selectedUsers.length > 0 ||
    auditStore.selectedActions.length > 0 ||
    auditStore.period !== 'all' ||
    startDateStr.value !== '' ||
    endDateStr.value !== ''
  )
})

watch(startDateStr, (val) => {
  auditStore.customFrom = val ? Math.floor(new Date(val + 'T00:00:00').getTime() / 1000) : null
})

watch(endDateStr, (val) => {
  auditStore.customTo = val ? Math.floor(new Date(val + 'T23:59:59').getTime() / 1000) : null
})

watch(
  [
    () => auditStore.selectedUsers,
    () => auditStore.selectedActions,
    () => auditStore.period,
  ],
  () => {
    if (auditStore.period !== 'custom') {
      auditStore.load()
    }
  },
  { deep: true }
)

watch(
  [() => auditStore.customFrom, () => auditStore.customTo],
  () => {
    if (auditStore.period === 'custom') {
      auditStore.load()
    }
  }
)

watch(
  () => auditStore.period,
  (newPeriod) => {
    if (newPeriod !== 'custom') {
      startDateStr.value = ''
      endDateStr.value = ''
    }
  }
)

function closeDropdowns(e: Event) {
  const target = e.target as HTMLElement
  if (usersDropdownRef.value && !usersDropdownRef.value.contains(target)) {
    usersDropdownOpen.value = false
  }
  if (actionsDropdownRef.value && !actionsDropdownRef.value.contains(target)) {
    actionsDropdownOpen.value = false
  }
}

function refreshData() {
  auditStore.loadFilters()
  auditStore.load()
}

function actionVariant(action: string): 'success' | 'warning' | 'danger' | 'neutral' {
  if (action.includes('create') || action.includes('enable')) return 'success'
  if (action.includes('rotate') || action.includes('archive')) return 'warning'
  if (action.includes('delete') || action.includes('disable')) return 'danger'
  return 'neutral'
}

function truncate(s: string, n: number): string {
  return s.length > n ? s.slice(0, n) + '...' : s
}

onMounted(() => {
  document.addEventListener('click', closeDropdowns)
  auditStore.loadFilters()
  auditStore.load()
})

onUnmounted(() => {
  document.removeEventListener('click', closeDropdowns)
})
</script>

<style scoped lang="scss">
@use '@/assets/scss/variables' as *;


.details-cell {
  display: inline-block;
  max-width: 300px;
}

.filter-bar {
  display: flex;
  flex-wrap: wrap;
  gap: $spacing-md;
  margin-bottom: $spacing-lg;
  padding: $spacing-md;
  background: var(--bg-card);
  border-radius: $border-radius-lg;
  border: 1px solid var(--border-color);
  align-items: flex-end;
}

.filter-group {
  display: flex;
  flex-direction: column;
  gap: $spacing-xs;
}

.filter-label {
  font-size: 0.75rem;
  font-weight: 600;
  text-transform: uppercase;
  color: var(--text-muted);
  letter-spacing: 0.05em;
}

.filter-control {
  min-width: 140px;
  height: 38px;
}

.custom-dates {
  .date-inputs {
    display: flex;
    align-items: center;
    gap: $spacing-xs;
    height: 38px;
  }
  .date-input {
    width: 130px;
  }
  .date-sep {
    color: var(--text-muted);
    font-size: 0.875rem;
  }
}

.filter-actions {
  margin-left: auto;
  align-self: flex-end;
  display: flex;
  align-items: center;
  gap: $spacing-xs;

  .reset-btn {
    display: flex;
    align-items: center;
    gap: $spacing-xs;
  }
}

.dropdown-multiselect {
  position: relative;

  .dropdown-trigger {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: $spacing-sm;
    width: 100%;
    min-width: 160px;
    height: 38px;
    padding: 8px 12px;
    background: var(--bg-input);
    border: 1px solid var(--border-color);
    border-radius: $border-radius;
    color: var(--text-primary);
    cursor: pointer;
    font-size: $font-size-sm;
    font-family: inherit;
    text-align: left;
    transition: border-color 0.15s ease, box-shadow 0.15s ease;

    &:hover {
      border-color: var(--btn-primary-bg);
    }

    &:active {
      transform: scale(0.99);
    }

    &.is-open {
      outline: none;
      border-color: var(--btn-primary-bg);
      box-shadow: var(--focus-ring);
    }

    &.has-value {
      color: var(--btn-primary-bg);
      border-color: var(--btn-primary-bg);
    }

    .trigger-text {
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      flex: 1;
    }

    .trigger-icon {
      color: var(--text-muted);
      flex-shrink: 0;
      transition: transform 0.2s ease;

      &.rotated {
        transform: rotate(180deg);
      }
    }
  }

  .dropdown-menu {
    position: absolute;
    top: calc(100% + 4px);
    left: 0;
    z-index: 200;
    min-width: 100%;
    max-height: 250px;
    overflow-y: auto;
    background: var(--bg-card);
    border: 1px solid var(--border-color);
    border-radius: $border-radius;
    box-shadow: var(--shadow-lg);
    padding: $spacing-xs;
    display: flex;
    flex-direction: column;
    gap: 2px;

    &.dropdown-menu-right {
      left: auto;
      right: 0;
    }
  }

  .dropdown-empty {
    padding: $spacing-sm $spacing-md;
    font-size: 0.875rem;
    color: var(--text-muted);
    text-align: center;
  }

  .dropdown-item {
    display: flex;
    align-items: center;
    gap: $spacing-sm;
    padding: $spacing-sm $spacing-md;
    border-radius: $border-radius-sm;
    cursor: pointer;
    font-size: $font-size-sm;
    color: var(--text-primary);
    user-select: none;
    transition: background-color 0.15s ease;

    &:hover {
      background: var(--bg-table-hover);
    }

    &:active {
      background: var(--color-primary-bg);
    }

    input[type="checkbox"] {
      width: 15px;
      height: 15px;
      cursor: pointer;
      accent-color: var(--btn-primary-bg);
      flex-shrink: 0;
    }

    .item-text {
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
  }
}

@media (max-width: 768px) {
  .filter-bar {
    flex-direction: column;
    align-items: stretch;
    gap: $spacing-sm;
  }

  .filter-group {
    width: 100%;
  }

  .filter-control,
  .dropdown-multiselect,
  .dropdown-trigger {
    width: 100%;
    min-width: 0;
  }

  .dropdown-multiselect {
    .dropdown-menu,
    .dropdown-menu.dropdown-menu-right {
      left: 0;
      right: 0;
      width: 100%;
    }
  }

  .custom-dates {
    .date-inputs {
      width: 100%;
    }
    .date-input {
      flex: 1;
    }
  }

  .filter-actions {
    margin-left: 0;
    justify-content: flex-end;
  }
}
</style>
