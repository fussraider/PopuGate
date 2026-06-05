<template>
  <div>
    <Transition name="content-fade" mode="out-in">
      <SkeletonLoader v-if="(loading && !items?.length) || !initialized" key="skeleton" variant="table" :rows="skeletonRows" :columns="columns.length" />

      <EmptyState v-else-if="!items?.length" key="empty" :icon="emptyIcon!" :message="emptyMessage ?? ''" />

      <div v-else key="table" class="table-wrapper">
        <table class="table">
          <thead>
            <tr>
              <th v-if="selectable" class="checkbox-col">
                <input type="checkbox" :checked="allSelected" :indeterminate="someSelected && !allSelected"
                       @change="toggleAll" />
              </th>
              <th v-if="$slots['mobile-actions']" class="mobile-actions-col" />
              <th v-for="col in columns" :key="col.key"
                  :style="{ width: col.width }"
                  :class="[
                    col.align ? `text-${col.align}` : '',
                    { 'sortable-header': col.sortable, 'sort-active': sortBy === col.key }
                  ]"
                  @click="toggleSort(col)">
                <div class="header-content" :class="[col.align ? `justify-${col.align}` : '']">
                  <slot :name="`header-${col.key}`" :column="col">
                    {{ col.header }}
                  </slot>
                  <span v-if="col.sortable" class="sort-indicator">
                    <ArrowUp v-if="sortBy === col.key && !sortDesc" class="sort-icon active-icon" />
                    <ArrowDown v-else-if="sortBy === col.key && sortDesc" class="sort-icon active-icon" />
                    <ArrowUpDown v-else class="sort-icon inactive-icon" />
                  </span>
                </div>
              </th>
              <th v-if="$slots.actions" class="actions-col-header" />
            </tr>
          </thead>
          <TransitionGroup name="row" tag="tbody">
            <tr v-for="item in sortedItems" :key="rowKeyFn(item)">
              <td v-if="selectable" class="checkbox-col">
                <input type="checkbox" :checked="isSelected(item)" @change="toggleItem(item)" />
              </td>
              <td v-if="$slots['mobile-actions']" class="mobile-actions-cell">
                <slot name="mobile-actions" :item="item" />
              </td>
              <td v-for="col in columns" :key="col.key"
                  :class="[col.align ? `text-${col.align}` : '']">
                <slot :name="`cell-${col.key}`" :item="item" :value="item[col.key]">
                  {{ item[col.key] }}
                </slot>
              </td>
              <td v-if="$slots.actions" class="actions-cell">
                <slot name="actions" :item="item" />
              </td>
            </tr>
          </TransitionGroup>
        </table>
      </div>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import {type Component, computed, type FunctionalComponent, ref, watch} from 'vue'
import { ArrowUp, ArrowDown, ArrowUpDown } from '@lucide/vue'
import SkeletonLoader from './SkeletonLoader.vue'
import EmptyState from './EmptyState.vue'

export interface Column {
  key: string
  header: string
  sortable?: boolean
  sortKey?: string
  width?: string
  align?: 'left' | 'center' | 'right'
}

const props = withDefaults(defineProps<{
  columns: Column[]
  items: any[]
  loading?: boolean
  emptyIcon?: Component | FunctionalComponent
  emptyMessage?: string
  rowKey: string | ((item: any) => string | number)
  skeletonRows?: number
  selectable?: boolean
  selectedKeys?: Set<string | number>
}>(), {
  skeletonRows: 5,
  selectable: false,
})

const emit = defineEmits<{
  'update:selected-keys': [keys: Set<string | number>]
}>()

const initialized = ref(!!props.items?.length)

const sortBy = ref<string | null>(null)
const sortDesc = ref(false)

function toggleSort(col: Column) {
  if (!col.sortable) return
  
  if (sortBy.value === col.key) {
    if (sortDesc.value) {
      sortBy.value = null
      sortDesc.value = false
    } else {
      sortDesc.value = true
    }
  } else {
    sortBy.value = col.key
    sortDesc.value = false
  }
}

function getNestedValue(obj: any, path: string): any {
  if (!path) return undefined
  return path.split('.').reduce((acc, part) => acc && acc[part], obj)
}

const sortedItems = computed(() => {
  if (!sortBy.value) return props.items
  
  const col = props.columns.find(c => c.key === sortBy.value)
  const key = col?.sortKey || sortBy.value
  const isDesc = sortDesc.value
  
  return [...props.items].sort((a, b) => {
    const valA = getNestedValue(a, key)
    const valB = getNestedValue(b, key)
    
    if (valA === valB) return 0
    if (valA === undefined || valA === null) return 1
    if (valB === undefined || valB === null) return -1
    
    // Number comparison
    if (typeof valA === 'number' && typeof valB === 'number') {
      return isDesc ? valB - valA : valA - valB
    }
    
    // Boolean comparison
    if (typeof valA === 'boolean' && typeof valB === 'boolean') {
      return isDesc ? (valA === valB ? 0 : valA ? -1 : 1) : (valA === valB ? 0 : valA ? 1 : -1)
    }
    
    // String comparison
    const strA = String(valA).toLowerCase()
    const strB = String(valB).toLowerCase()
    
    if (strA < strB) return isDesc ? 1 : -1
    if (strA > strB) return isDesc ? -1 : 1
    return 0
  })
})

watch(() => props.loading, (val) => {
  if (val) initialized.value = true
})

watch(() => props.items, () => {
  initialized.value = true
}, { immediate: true })

function rowKeyFn(item: any): string | number {
  return typeof props.rowKey === 'function' ? props.rowKey(item) : item[props.rowKey]
}

function isSelected(item: any): boolean {
  return props.selectedKeys?.has(rowKeyFn(item)) ?? false
}

const allSelected = computed(() => {
  if (!props.items?.length || !props.selectedKeys) return false
  return props.items.every((item) => props.selectedKeys!.has(rowKeyFn(item)))
})

const someSelected = computed(() => {
  if (!props.items?.length || !props.selectedKeys) return false
  return props.items.some((item) => props.selectedKeys!.has(rowKeyFn(item)))
})

function toggleAll() {
  if (!props.selectedKeys) return
  const next = new Set(props.selectedKeys)
  if (allSelected.value) {
    for (const item of props.items) next.delete(rowKeyFn(item))
  } else {
    for (const item of props.items) next.add(rowKeyFn(item))
  }
  emit('update:selected-keys', next)
}

function toggleItem(item: any) {
  if (!props.selectedKeys) return
  const next = new Set(props.selectedKeys)
  const key = rowKeyFn(item)
  if (next.has(key)) {
    next.delete(key)
  } else {
    next.add(key)
  }
  emit('update:selected-keys', next)
}
</script>

<style scoped lang="scss">
@use '@/assets/scss/variables' as *;

.content-fade-enter-active {
  transition: opacity 0.25s ease;
}
.content-fade-enter-from {
  opacity: 0;
}

.row-enter-active {
  transition: all 0.25s ease;
}
.row-enter-from {
  opacity: 0;
  transform: translateX(-10px);
}
.row-leave-active {
  transition: all 0.15s ease;
}
.row-leave-to {
  opacity: 0;
  transform: translateX(10px);
}

.checkbox-col {
  width: 40px;
  text-align: center;

  input[type="checkbox"] {
    cursor: pointer;
    width: 16px;
    height: 16px;
  }
}

.mobile-actions-col,
.mobile-actions-cell {
  display: none;
}

@media (max-width: 768px) {
  .mobile-actions-col,
  .mobile-actions-cell {
    display: table-cell;
    width: 36px;
  }

  .actions-col-header,
  .actions-cell {
    display: none;
  }
}

.sortable-header {
  cursor: pointer;
  user-select: none;
  transition: color 0.15s ease;

  &:hover {
    color: var(--color-primary, #3b82f6) !important;
    
    .sort-icon.inactive-icon {
      opacity: 0.5;
    }
  }
}

.header-content {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  vertical-align: middle;
}

.sort-indicator {
  display: inline-flex;
  align-items: center;
}

.sort-icon {
  width: 14px;
  height: 14px;
  stroke-width: 2.5px;
  transition: opacity 0.15s ease, color 0.15s ease;
  
  &.active-icon {
    color: var(--color-primary, #3b82f6);
    opacity: 1;
  }
  
  &.inactive-icon {
    opacity: 0.25;
  }
}

.text-left { text-align: left; }
.text-center { text-align: center; }
.text-right { text-align: right; }

.justify-left { justify-content: flex-start; }
.justify-center { justify-content: center; }
.justify-right { justify-content: flex-end; }
</style>
