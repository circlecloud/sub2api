<template>
  <div class="rounded-lg border border-gray-300 bg-white p-2 dark:border-dark-500 dark:bg-dark-700">
    <div class="flex flex-wrap items-center gap-2">
      <span
        v-for="(value, index) in values"
        :key="`${index}-${value}`"
        class="inline-flex items-center gap-1 rounded bg-gray-100 px-2 py-1 text-xs font-mono text-gray-700 dark:bg-dark-600 dark:text-gray-200"
        data-testid="positive-integer-tag"
      >
        <span data-testid="positive-integer-tag-value">{{ value }}</span>
        <button
          type="button"
          class="rounded-full px-1 text-gray-500 hover:bg-gray-200 hover:text-gray-700 dark:text-gray-300 dark:hover:bg-dark-500 dark:hover:text-white"
          :aria-label="`${removeAriaLabel} ${value}`"
          data-testid="positive-integer-tag-remove"
          @click="removeAt(index)"
        >
          ×
        </button>
      </span>

      <div
        class="flex min-w-[140px] flex-1 items-center gap-1 rounded border border-transparent px-2 py-1 focus-within:border-primary-300 dark:focus-within:border-primary-700"
      >
        <input
          v-model="draft"
          type="text"
          inputmode="numeric"
          class="w-full bg-transparent text-sm font-mono text-gray-900 outline-none placeholder:text-gray-400 dark:text-white dark:placeholder:text-gray-500"
          :placeholder="placeholder"
          :aria-label="inputAriaLabel"
          data-testid="positive-integer-tags-input"
          @keydown="handleKeydown"
          @blur="commitDraft"
          @paste="handlePaste"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'

const separatorKeys = new Set(['Enter', 'Tab', ',', '，'])

const props = withDefaults(
  defineProps<{
    modelValue?: number[]
    placeholder?: string
    inputAriaLabel?: string
    removeAriaLabel?: string
  }>(),
  {
    modelValue: () => [],
    placeholder: '',
    inputAriaLabel: 'Positive integer tags input',
    removeAriaLabel: 'Remove value'
  }
)

const emit = defineEmits<{
  (event: 'update:modelValue', value: number[]): void
}>()

const draft = ref('')

const values = computed(() =>
  Array.isArray(props.modelValue)
    ? props.modelValue.filter((value) => Number.isInteger(value) && value > 0)
    : []
)

function parsePositiveIntegerItems(raw: string): number[] {
  return raw
    .split(/[\n,，]+/)
    .map((item) => item.trim())
    .filter(Boolean)
    .flatMap((item) => {
      if (!/^\d+$/.test(item)) {
        return []
      }

      const value = Number(item)
      return Number.isInteger(value) && value > 0 ? [value] : []
    })
}

function appendValues(raw: string): boolean {
  const nextValues = parsePositiveIntegerItems(raw)
  if (nextValues.length === 0) {
    return false
  }

  emit('update:modelValue', [...values.value, ...nextValues])
  return true
}

function commitDraft() {
  if (!draft.value.trim()) {
    return
  }

  if (appendValues(draft.value)) {
    draft.value = ''
  }
}

function removeAt(index: number) {
  emit(
    'update:modelValue',
    values.value.filter((_, itemIndex) => itemIndex !== index)
  )
}

function handleKeydown(event: KeyboardEvent) {
  if (event.isComposing || !separatorKeys.has(event.key)) {
    return
  }

  const canAppendDraft = parsePositiveIntegerItems(draft.value).length > 0
  if (event.key === 'Tab' && !canAppendDraft) {
    return
  }

  event.preventDefault()

  if (canAppendDraft && appendValues(draft.value)) {
    draft.value = ''
  }
}

function handlePaste(event: ClipboardEvent) {
  const text = event.clipboardData?.getData('text') || ''
  if (!text.trim()) {
    return
  }

  event.preventDefault()

  if (appendValues(text)) {
    draft.value = ''
  }
}
</script>
