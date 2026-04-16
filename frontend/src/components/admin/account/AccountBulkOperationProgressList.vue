<template>
  <div v-if="progressItems.length > 0">
    <div
      v-for="item in progressItems"
      :key="item.key"
      :class="['mt-2 rounded-lg border px-3 py-3', progressContainerClass(item.tone)]"
    >
      <div :class="['mb-2 flex items-center justify-between gap-3 text-sm', progressTextClass(item.tone)]">
        <span>{{ item.statusText }}</span>
        <span class="font-medium">{{ item.percent }}%</span>
      </div>
      <div
        :class="['h-2 overflow-hidden rounded-full', progressTrackClass(item.tone)]"
        role="progressbar"
        :aria-valuenow="item.percent"
        aria-valuemin="0"
        aria-valuemax="100"
        :aria-label="item.statusText"
      >
        <div
          :class="['h-full rounded-full transition-all duration-300 ease-out', progressBarClass(item.tone)]"
          :style="{ width: `${item.percent}%` }"
        />
      </div>
      <div
        v-if="typeof item.successCount === 'number' || typeof item.failedCount === 'number'"
        :class="['mt-2 flex flex-wrap items-center gap-3 text-xs', progressMetaClass(item.tone)]"
      >
        <span v-if="typeof item.successCount === 'number'">{{ t('admin.accounts.bulkActions.progressSuccess', { count: item.successCount }) }}</span>
        <span v-if="typeof item.failedCount === 'number'">{{ t('admin.accounts.bulkActions.progressFailed', { count: item.failedCount }) }}</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { BulkOperationProgressItem, BulkOperationProgressTone } from '@/views/admin/accounts/useAccountBulkOperations'

defineProps<{
  progressItems: BulkOperationProgressItem[]
}>()

const { t } = useI18n()

const progressContainerClass = (tone: BulkOperationProgressTone) => {
  switch (tone) {
    case 'primary':
      return 'border-primary-200 bg-primary-50 dark:border-primary-700/40 dark:bg-primary-900/20'
    case 'emerald':
      return 'border-emerald-200 bg-emerald-50 dark:border-emerald-700/40 dark:bg-emerald-900/20'
    case 'cyan':
      return 'border-cyan-200 bg-cyan-50 dark:border-cyan-700/40 dark:bg-cyan-900/20'
    case 'purple':
      return 'border-purple-200 bg-purple-50 dark:border-purple-700/40 dark:bg-purple-900/20'
    case 'sky':
      return 'border-sky-200 bg-sky-50 dark:border-sky-700/40 dark:bg-sky-900/20'
  }
}

const progressTextClass = (tone: BulkOperationProgressTone) => {
  switch (tone) {
    case 'primary':
      return 'text-primary-800 dark:text-primary-200'
    case 'emerald':
      return 'text-emerald-800 dark:text-emerald-200'
    case 'cyan':
      return 'text-cyan-800 dark:text-cyan-200'
    case 'purple':
      return 'text-purple-800 dark:text-purple-200'
    case 'sky':
      return 'text-sky-800 dark:text-sky-200'
  }
}

const progressTrackClass = (tone: BulkOperationProgressTone) => {
  switch (tone) {
    case 'primary':
      return 'bg-primary-100 dark:bg-primary-900/40'
    case 'emerald':
      return 'bg-emerald-100 dark:bg-emerald-900/40'
    case 'cyan':
      return 'bg-cyan-100 dark:bg-cyan-900/40'
    case 'purple':
      return 'bg-purple-100 dark:bg-purple-900/40'
    case 'sky':
      return 'bg-sky-100 dark:bg-sky-900/40'
  }
}

const progressBarClass = (tone: BulkOperationProgressTone) => {
  switch (tone) {
    case 'primary':
      return 'bg-primary-500 dark:bg-primary-400'
    case 'emerald':
      return 'bg-emerald-500 dark:bg-emerald-400'
    case 'cyan':
      return 'bg-cyan-500 dark:bg-cyan-400'
    case 'purple':
      return 'bg-purple-500 dark:bg-purple-400'
    case 'sky':
      return 'bg-sky-500 dark:bg-sky-400'
  }
}

const progressMetaClass = (tone: BulkOperationProgressTone) => {
  switch (tone) {
    case 'primary':
      return 'text-primary-700 dark:text-primary-300'
    case 'emerald':
      return 'text-emerald-700 dark:text-emerald-300'
    case 'cyan':
      return 'text-cyan-700 dark:text-cyan-300'
    case 'purple':
      return 'text-purple-700 dark:text-purple-300'
    case 'sky':
      return 'text-sky-700 dark:text-sky-300'
  }
}
</script>
