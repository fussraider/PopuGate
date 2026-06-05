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
              <th v-for="col in columns" :key="col.key">
                <slot :name="`header-${col.key}`" :column="col">
                  {{ col.header }}
                </slot>
              </th>
              <th v-if="$slots.actions" class="actions-col-header" />
            </tr>
          </thead>
          <TransitionGroup name="row" tag="tbody">
            <tr v-for="item in items" :key="rowKeyFn(item)">
              <td v-if="selectable" class="checkbox-col">
                <input type="checkbox" :checked="isSelected(item)" @change="toggleItem(item)" />
              </td>
              <td v-if="$slots['mobile-actions']" class="mobile-actions-cell">
                <slot name="mobile-actions" :item="item" />
              </td>
              <td v-for="col in columns" :key="col.key">
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
import SkeletonLoader from './SkeletonLoader.vue'
import EmptyState from './EmptyState.vue'

export interface Column {
  key: string
  header: string
  sortable?: boolean
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
</style>
