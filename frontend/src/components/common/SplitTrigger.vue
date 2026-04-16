<template>
  <div class="split-trigger-root">
    <button
      type="button"
      :class="[
        'split-trigger-left',
        leftOpen && 'split-trigger-left-open',
        leftDisabled && 'split-trigger-left-disabled'
      ]"
      :disabled="leftDisabled"
      :aria-expanded="leftOpen"
      :title="leftTitle"
      @click="$emit('left-click')"
    >
      <span class="split-trigger-value">
        <slot name="left">
          {{ leftLabel }}
        </slot>
      </span>
      <span class="split-trigger-icon">
        <Icon
          name="chevronDown"
          size="md"
          :class="['transition-transform duration-200', leftOpen && 'rotate-180']"
        />
      </span>
    </button>
    <button
      type="button"
      :class="[
        'split-trigger-right',
        rightActive && 'split-trigger-right-active',
        rightDisabled && 'split-trigger-right-disabled'
      ]"
      :disabled="rightDisabled"
      :title="rightTitle"
      @click="$emit('right-click')"
    >
      <span class="split-trigger-value">
        <slot name="right">
          {{ rightLabel }}
        </slot>
      </span>
    </button>
  </div>
</template>

<script setup lang="ts">
import Icon from '@/components/icons/Icon.vue'

interface Props {
  leftLabel?: string
  rightLabel?: string
  leftOpen?: boolean
  leftDisabled?: boolean
  rightActive?: boolean
  rightDisabled?: boolean
  leftTitle?: string
  rightTitle?: string
}

withDefaults(defineProps<Props>(), {
  leftLabel: '',
  rightLabel: '',
  leftOpen: false,
  leftDisabled: false,
  rightActive: false,
  rightDisabled: false,
  leftTitle: '',
  rightTitle: ''
})

defineEmits<{
  (e: 'left-click'): void
  (e: 'right-click'): void
}>()
</script>

<style scoped>
.split-trigger-root {
  @apply grid w-full grid-cols-2 overflow-hidden rounded-xl;
  @apply bg-white dark:bg-dark-800;
  @apply border border-gray-200 dark:border-dark-600;
  @apply text-gray-900 dark:text-gray-100;
  @apply shadow-sm;
}

.split-trigger-left,
.split-trigger-right {
  @apply flex min-w-0 items-center justify-between gap-2;
  @apply px-4 py-2.5 text-sm;
  @apply transition-all duration-200;
  @apply focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/30;
}

.split-trigger-left {
  @apply border-r border-gray-200 dark:border-dark-600;
  @apply hover:border-gray-300 dark:hover:border-dark-500;
  @apply cursor-pointer;
}

.split-trigger-left-open {
  @apply bg-primary-50/70 ring-2 ring-primary-500/30 dark:bg-dark-700/70;
}

.split-trigger-left-disabled {
  @apply cursor-not-allowed bg-gray-100 opacity-60 dark:bg-dark-900;
}

.split-trigger-right {
  @apply justify-center;
  @apply hover:bg-gray-50 dark:hover:bg-dark-700;
  @apply cursor-pointer;
}

.split-trigger-right-active {
  @apply bg-primary-500/10 text-primary-700 hover:bg-primary-500/15 dark:text-primary-300;
}

.split-trigger-right-disabled {
  @apply cursor-not-allowed bg-gray-50 text-gray-400 dark:bg-dark-900 dark:text-dark-400;
}

.split-trigger-value {
  @apply flex-1 truncate text-left;
}

.split-trigger-icon {
  @apply flex-shrink-0 text-gray-400 dark:text-dark-400;
}
</style>
