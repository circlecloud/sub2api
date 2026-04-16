<template>
  <div
    v-if="selectedIds.length > 0"
    class="mb-4 flex items-center justify-between rounded-lg bg-primary-50 p-3 dark:bg-primary-900/20"
  >
    <div class="flex flex-wrap items-center gap-2">
      <span class="text-sm font-medium text-primary-900 dark:text-primary-100">
        {{ t('admin.accounts.bulkActions.selected', { count: selectedIds.length }) }}
      </span>
      <button
        @click="$emit('select-page')"
        class="text-xs font-medium text-primary-700 hover:text-primary-800 dark:text-primary-300 dark:hover:text-primary-200"
      >
        {{ t('admin.accounts.bulkActions.selectCurrentPage') }}
      </button>
      <span class="text-gray-300 dark:text-primary-800">•</span>
      <button
        @click="$emit('clear')"
        class="text-xs font-medium text-primary-700 hover:text-primary-800 dark:text-primary-300 dark:hover:text-primary-200"
      >
        {{ t('admin.accounts.bulkActions.clear') }}
      </button>
    </div>
    <div class="flex gap-2">
      <button @click="$emit('delete')" class="btn btn-danger btn-sm">{{ t('admin.accounts.bulkActions.delete') }}</button>
      <button @click="$emit('reset-status')" class="btn btn-secondary btn-sm">{{ t('admin.accounts.bulkActions.resetStatus') }}</button>
      <button
        @click="$emit('refresh-token')"
        :disabled="refreshingToken"
        class="btn btn-secondary btn-sm disabled:cursor-not-allowed disabled:opacity-60"
      >
        {{ refreshingToken ? t('admin.accounts.bulkActions.refreshingAction') : t('admin.accounts.bulkActions.refreshToken') }}
      </button>
      <button
        @click="$emit('test')"
        :disabled="testingAccounts"
        class="btn btn-secondary btn-sm disabled:cursor-not-allowed disabled:opacity-60"
      >
        {{ testingAccounts ? t('admin.accounts.bulkTest.testingAction') : t('admin.accounts.bulkTest.action') }}
      </button>
      <button
        @click="$emit('refresh-usage-window')"
        :disabled="refreshingUsageWindow"
        class="btn btn-secondary btn-sm disabled:cursor-not-allowed disabled:opacity-60"
      >
        {{ refreshUsageWindowLabel || t('admin.accounts.bulkActions.refreshUsageWindow') }}
      </button>
      <button
        @click="$emit('set-privacy')"
        :disabled="settingPrivacy"
        class="btn btn-secondary btn-sm disabled:cursor-not-allowed disabled:opacity-60"
      >
        {{ settingPrivacy ? t('admin.accounts.bulkActions.settingPrivacyAction') : t('admin.accounts.bulkActions.setPrivacy') }}
      </button>
      <button @click="$emit('toggle-schedulable', true)" class="btn btn-success btn-sm">{{ t('admin.accounts.bulkActions.enableScheduling') }}</button>
      <button @click="$emit('toggle-schedulable', false)" class="btn btn-warning btn-sm">{{ t('admin.accounts.bulkActions.disableScheduling') }}</button>
      <button
        @click="$emit('edit')"
        :disabled="preparingEdit"
        class="btn btn-primary btn-sm disabled:cursor-not-allowed disabled:opacity-60"
      >
        {{ editLabel || t('admin.accounts.bulkActions.edit') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

withDefaults(defineProps<{
  selectedIds: number[]
  refreshingToken?: boolean
  testingAccounts?: boolean
  refreshingUsageWindow?: boolean
  refreshUsageWindowLabel?: string
  settingPrivacy?: boolean
  preparingEdit?: boolean
  editLabel?: string
}>(), {
  refreshingToken: false,
  testingAccounts: false,
  refreshingUsageWindow: false,
  refreshUsageWindowLabel: '',
  settingPrivacy: false,
  preparingEdit: false,

  editLabel: ''
})

defineEmits(['delete', 'edit', 'test', 'clear', 'select-page', 'toggle-schedulable', 'reset-status', 'refresh-token', 'refresh-usage-window', 'set-privacy'])

const { t } = useI18n()
</script>
