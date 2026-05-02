<template>
  <div>
    <Transition name="content-fade" mode="out-in">
      <SkeletonLoader v-if="loading" key="skeleton" variant="table" :rows="skeletonRows" :columns="columns.length" />

      <EmptyState v-else-if="!items?.length" key="empty" :icon="emptyIcon!" :message="emptyMessage ?? ''" />

      <div v-else key="table" class="table-wrapper">
        <table class="table">
          <thead>
            <tr>
              <th v-for="col in columns" :key="col.key">
                <slot :name="`header-${col.key}`" :column="col">
                  {{ col.header }}
                </slot>
              </th>
              <th v-if="$slots.actions" />
            </tr>
          </thead>
          <TransitionGroup name="row" tag="tbody">
            <tr v-for="item in items" :key="rowKeyFn(item)">
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
import { type Component, type FunctionalComponent } from 'vue'
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
}>(), {
  skeletonRows: 5,
})

function rowKeyFn(item: any): string | number {
  return typeof props.rowKey === 'function' ? props.rowKey(item) : item[props.rowKey]
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
</style>
