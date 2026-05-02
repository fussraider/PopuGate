<template>
  <div class="skeleton-loader">
    <!-- Table variant -->
    <template v-if="variant === 'table'">
      <div class="skeleton-table">
        <div class="skeleton skeleton-table-header">
          <div v-for="i in columns" :key="i" class="skeleton skeleton-cell" />
        </div>
        <div v-for="row in rows" :key="row" class="skeleton-table-row">
          <div v-for="col in columns" :key="col" class="skeleton skeleton-cell" />
        </div>
      </div>
    </template>

    <!-- Stat card variant -->
    <template v-else-if="variant === 'stat-card'">
      <div class="skeleton-stats-grid">
        <div v-for="i in rows" :key="i" class="skeleton skeleton-stat-card" />
      </div>
    </template>

    <!-- Card variant -->
    <template v-else-if="variant === 'card'">
      <div class="skeleton-card">
        <div class="skeleton skeleton-title" />
        <div v-for="i in (rows ?? 3)" :key="i" class="skeleton skeleton-line" />
      </div>
    </template>

    <!-- Form variant -->
    <template v-else-if="variant === 'form'">
      <div v-for="i in (rows ?? 3)" :key="i" class="skeleton-form-group">
        <div class="skeleton skeleton-label" />
        <div class="skeleton skeleton-input" />
      </div>
    </template>

    <!-- Text variant (default) -->
    <template v-else>
      <div class="skeleton-text">
        <div v-for="i in (rows ?? 3)" :key="i" class="skeleton skeleton-line" :style="{ width: i === (rows ?? 3) ? '60%' : '100%' }" />
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  variant: 'table' | 'card' | 'stat-card' | 'form' | 'text'
  rows?: number
  columns?: number
}>()
</script>

<style scoped lang="scss">
@use '@/assets/scss/variables' as *;

.skeleton-loader {
  animation: skeleton-fade-in 0.3s ease;
}

@keyframes skeleton-fade-in {
  from { opacity: 0; }
  to { opacity: 1; }
}

.skeleton-table {
  border: 1px solid $border-color;
  border-radius: $border-radius-lg;
  overflow: hidden;
}

.skeleton-table-header {
  display: flex;
  gap: 0;
  padding: 10px 16px;
  background: $color-gray-50;
}

.skeleton-table-row {
  display: flex;
  gap: 0;
  padding: 10px 16px;
  border-top: 1px solid $border-color;
}

.skeleton-cell {
  height: 14px;
  flex: 1;
  margin-right: 12px;

  &:last-child { margin-right: 0; }
}

.skeleton-stats-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: $spacing-md;
}

.skeleton-stat-card {
  height: 80px;
  border-radius: $border-radius-lg;
}

.skeleton-card {
  background: $bg-card;
  border: 1px solid $border-color;
  border-radius: $border-radius-lg;
  padding: $spacing-lg;
}

.skeleton-title {
  height: 20px;
  width: 40%;
  margin-bottom: $spacing-md;
}

.skeleton-line {
  height: 14px;
  width: 100%;
  margin-bottom: $spacing-sm;

  &:last-child { margin-bottom: 0; }
}

.skeleton-form-group {
  margin-bottom: $spacing-md;
}

.skeleton-label {
  height: 12px;
  width: 30%;
  margin-bottom: $spacing-sm;
}

.skeleton-input {
  height: 36px;
  width: 100%;
}

.skeleton-text {
  padding: $spacing-sm 0;
}
</style>
