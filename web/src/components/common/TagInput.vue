<template>
  <div class="tag-input-wrapper" ref="wrapperRef">
    <div class="tag-chips" @click="focusInput">
      <span v-for="tag in tags" :key="tag" class="badge badge-info tag-chip">
        {{ tag }}
        <button class="tag-remove" @click.stop="removeTag(tag)">&times;</button>
      </span>
      <input
        ref="inputRef"
        v-model="inputValue"
        class="tag-field"
        :placeholder="tags.length ? '' : placeholder"
        @keydown.enter.prevent="addCurrent"
        @keydown.backspace="handleBackspace"
        @keydown.escape="closeSuggestions"
        @input="onInput"
        @blur="onBlur"
      />
    </div>
    <div v-if="showSuggestions && filteredSuggestions.length" class="tag-suggestions">
      <button
        v-for="s in filteredSuggestions"
        :key="s"
        class="tag-suggestion-item"
        @mousedown.prevent="addTag(s)"
      >
        {{ s }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import {computed, nextTick, onBeforeUnmount, onMounted, ref, watch} from 'vue'

const props = withDefaults(defineProps<{
  modelValue: string
  availableTags?: string[]
  placeholder?: string
}>(), {
  availableTags: () => [],
  placeholder: '',
})

const emit = defineEmits<{
  'update:modelValue': [value: string]
  'blur': []
}>()

const inputValue = ref('')
const showSuggestions = ref(false)
const inputRef = ref<HTMLInputElement | null>(null)
const wrapperRef = ref<HTMLElement | null>(null)

const tags = computed(() => {
  if (!props.modelValue || props.modelValue === '[]') return []
  try { return JSON.parse(props.modelValue) } catch { return [] }
})

const filteredSuggestions = computed(() => {
  const current = new Set(tags.value)
  const query = inputValue.value.trim().toLowerCase()
  return props.availableTags.filter(t =>
    !current.has(t) &&
    (!query || t.toLowerCase().includes(query))
  )
})

function emitTags(newTags: string[]) {
  emit('update:modelValue', JSON.stringify(newTags))
}

function addTag(tag: string) {
  const trimmed = tag.trim()
  if (!trimmed || tags.value.includes(trimmed)) return
  emitTags([...tags.value, trimmed])
  inputValue.value = ''
  showSuggestions.value = false
  nextTick(() => inputRef.value?.focus())
}

function removeTag(tag: string) {
  emitTags(tags.value.filter((t: string) => t !== tag))
}

function addCurrent() {
  const val = inputValue.value.trim()
  if (!val) return
  // If input matches a suggestion exactly, use it
  addTag(val)
}

function handleBackspace() {
  if (inputValue.value === '' && tags.value.length > 0) {
    emitTags(tags.value.slice(0, -1))
  }
}

function onInput() {
  showSuggestions.value = filteredSuggestions.value.length > 0
}

function closeSuggestions() {
  showSuggestions.value = false
}

function focusInput() {
  inputRef.value?.focus()
}

function onBlur() {
  // Delay to allow suggestion click to register first
  setTimeout(() => {
    showSuggestions.value = false
    emit('blur')
  }, 150)
}

function handleClickOutside(e: MouseEvent) {
  if (wrapperRef.value && !wrapperRef.value.contains(e.target as Node)) {
    showSuggestions.value = false
  }
}

watch(() => props.modelValue, () => {
  showSuggestions.value = false
})

onMounted(() => document.addEventListener('click', handleClickOutside))
onBeforeUnmount(() => document.removeEventListener('click', handleClickOutside))
</script>

<style scoped lang="scss">
@use '@/assets/scss/variables' as *;

.tag-input-wrapper {
  position: relative;
}

.tag-chips {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px;
  padding: 6px 8px;
  min-height: 38px;
  border: 1px solid $border-color;
  border-radius: $border-radius;
  background: $bg-input;
  cursor: text;
  transition: border-color 0.2s;

  &:focus-within {
    border-color: $color-primary;
  }
}

.tag-chip {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  font-size: $font-size-xs;
  white-space: nowrap;
}

.tag-remove {
  background: none;
  border: none;
  color: inherit;
  opacity: 0.6;
  cursor: pointer;
  padding: 0 2px;
  font-size: 14px;
  line-height: 1;

  &:hover { opacity: 1; }
}

.tag-field {
  flex: 1;
  min-width: 60px;
  border: none;
  outline: none;
  background: transparent;
  font-size: $font-size-sm;
  color: inherit;
  padding: 0;

  &::placeholder {
    color: $text-secondary;
    opacity: 0.6;
  }
}

.tag-suggestions {
  position: absolute;
  top: 100%;
  left: 0;
  right: 0;
  z-index: 9999;
  background: $bg-card;
  border: 1px solid $border-color;
  border-radius: $border-radius;
  box-shadow: $shadow-lg;
  max-height: 200px;
  overflow-y: auto;
}

.tag-suggestion-item {
  display: block;
  width: 100%;
  text-align: left;
  padding: $spacing-xs $spacing-sm;
  border: none;
  background: none;
  color: inherit;
  font-size: $font-size-sm;
  cursor: pointer;

  &:hover {
    background: $color-primary-bg;
  }
}
</style>
