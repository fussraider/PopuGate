<template>
  <div class="page-header flex justify-between items-center mb-lg">
    <div v-if="!$slots.default" />
    <slot />
  </div>
</template>
