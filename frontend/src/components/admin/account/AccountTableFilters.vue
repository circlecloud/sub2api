<template>
  <div class="flex flex-wrap items-center gap-3">
    <SearchInput
      :model-value="searchQuery"
      :placeholder="t('admin.accounts.searchAccounts')"
      class="w-full sm:w-64"
      @update:model-value="$emit('update:searchQuery', $event)"
      @search="$emit('change')"
    />

    <Select
      :model-value="filters.platform"
      class="w-40"
      :options="platformOptions"
      @update:model-value="updatePlatform"
      @change="$emit('change')"
    />
    <Select
      :model-value="filters.type"
      class="w-40"
      :options="typeOptions"
      @update:model-value="updateType"
      @change="$emit('change')"
    />
    <Select
      :model-value="filters.status"
      class="w-40"
      :options="statusOptions"
      @update:model-value="updateStatus"
      @change="$emit('change')"
    />
    <Select
      :model-value="filters.privacy_mode"
      class="w-40"
      :options="privacyOptions"
      @update:model-value="updatePrivacyMode"
      @change="$emit('change')"
    />
    <AccountGroupFilterPopover
      :filters="filters"
      :groups="groups"
      @update:filters="emitFilters"
      @change="$emit('change')"
    />
    <AccountLastUsedFilterPopover
      :filters="filters"
      @update:filters="emitFilters"
      @change="$emit('change')"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Select from '@/components/common/Select.vue'
import SearchInput from '@/components/common/SearchInput.vue'
import AccountGroupFilterPopover from './AccountGroupFilterPopover.vue'
import AccountLastUsedFilterPopover from './AccountLastUsedFilterPopover.vue'
import type { AdminGroup } from '@/types'
import type { AccountTableParams } from '@/views/admin/accounts/query'

type AccountTableFilterPatch = Partial<AccountTableParams>

defineProps<{
  searchQuery: string
  filters: Partial<AccountTableParams>
  groups?: AdminGroup[]
}>()

const emit = defineEmits<{
  (e: 'update:searchQuery', value: string): void
  (e: 'update:filters', value: AccountTableFilterPatch): void
  (e: 'change'): void
}>()

const { t } = useI18n()

const emitFilters = (patch: AccountTableFilterPatch) => {
  emit('update:filters', patch)
}

const normalizeFilterValue = (value: string | number | boolean | null) => {
  if (typeof value === 'string') return value
  if (value === null) return ''
  return String(value)
}

const updatePlatform = (value: string | number | boolean | null) => {
  emitFilters({ platform: normalizeFilterValue(value) })
}

const updateType = (value: string | number | boolean | null) => {
  emitFilters({ type: normalizeFilterValue(value) })
}

const updateStatus = (value: string | number | boolean | null) => {
  emitFilters({ status: normalizeFilterValue(value) })
}

const updatePrivacyMode = (value: string | number | boolean | null) => {
  emitFilters({ privacy_mode: normalizeFilterValue(value) })
}

const platformOptions = computed(() => [
  { value: '', label: t('admin.accounts.allPlatforms') },
  { value: 'anthropic', label: 'Anthropic' },
  { value: 'openai', label: 'OpenAI' },
  { value: 'gemini', label: 'Gemini' },
  { value: 'antigravity', label: 'Antigravity' },
  { value: 'sora', label: 'Sora' }
])

const typeOptions = computed(() => [
  { value: '', label: t('admin.accounts.allTypes') },
  { value: 'oauth', label: t('admin.accounts.oauthType') },
  { value: 'setup-token', label: t('admin.accounts.setupToken') },
  { value: 'apikey', label: t('admin.accounts.apiKey') },
  { value: 'bedrock', label: 'AWS Bedrock' }
])

const statusOptions = computed(() => [
  { value: '', label: t('admin.accounts.allStatus') },
  { value: 'active', label: t('admin.accounts.status.active') },
  { value: 'inactive', label: t('admin.accounts.status.inactive') },
  { value: 'error', label: t('admin.accounts.status.error') },
  { value: 'rate_limited', label: t('admin.accounts.status.rateLimited') },
  { value: 'temp_unschedulable', label: t('admin.accounts.status.tempUnschedulable') },
  { value: 'unschedulable', label: t('admin.accounts.status.unschedulable') }
])

const privacyOptions = computed(() => [
  { value: '', label: t('admin.accounts.allPrivacyModes') },
  { value: '__unset__', label: t('admin.accounts.privacyUnset') },
  { value: 'training_off', label: 'Privacy' },
  { value: 'training_set_cf_blocked', label: 'CF' },
  { value: 'training_set_failed', label: 'Fail' }
])
</script>
