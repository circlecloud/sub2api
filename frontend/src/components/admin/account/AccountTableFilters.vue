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
    <Select
      :model-value="filters.group"
      class="w-40"
      :options="groupOptions"
      @update:model-value="updateGroup"
      @change="$emit('change')"
    />
    <Select
      :model-value="currentLastUsedFilter"
      class="w-44"
      :options="lastUsedOptions"
      @update:model-value="updateLastUsedFilter"
      @change="$emit('change')"
    />
    <DateRangePicker
      v-if="currentLastUsedFilter === 'range'"
      class="w-full sm:w-auto"
      :start-date="lastUsedStartDate"
      :end-date="lastUsedEndDate"
      @update:startDate="updateLastUsedStartDate"
      @update:endDate="updateLastUsedEndDate"
      @change="$emit('change')"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Select from '@/components/common/Select.vue'
import SearchInput from '@/components/common/SearchInput.vue'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import type { AdminGroup } from '@/types'

const props = defineProps<{
  searchQuery: string
  filters: Record<string, any>
  groups?: AdminGroup[]
}>()

const emit = defineEmits<{
  (e: 'update:searchQuery', value: string): void
  (e: 'update:filters', value: Record<string, any>): void
  (e: 'change'): void
}>()

const { t } = useI18n()

const formatDateInput = (date: Date): string => {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

const getDefaultLastUsedRange = () => {
  const end = new Date()
  const start = new Date()
  start.setDate(start.getDate() - 6)
  return {
    start: formatDateInput(start),
    end: formatDateInput(end)
  }
}

const currentLastUsedFilter = computed(() => {
  const raw = props.filters.last_used_filter
  return typeof raw === 'string' ? raw : ''
})

const lastUsedStartDate = computed(() => {
  const raw = props.filters.last_used_start_date
  return typeof raw === 'string' ? raw : ''
})

const lastUsedEndDate = computed(() => {
  const raw = props.filters.last_used_end_date
  return typeof raw === 'string' ? raw : ''
})

const emitFilters = (nextFilters: Record<string, any>) => {
  emit('update:filters', nextFilters)
}

const updatePlatform = (value: string | number | boolean | null) => {
  emitFilters({ ...props.filters, platform: value })
}

const updateType = (value: string | number | boolean | null) => {
  emitFilters({ ...props.filters, type: value })
}

const updateStatus = (value: string | number | boolean | null) => {
  emitFilters({ ...props.filters, status: value })
}

const updatePrivacyMode = (value: string | number | boolean | null) => {
  emitFilters({ ...props.filters, privacy_mode: value })
}

const updateGroup = (value: string | number | boolean | null) => {
  emitFilters({ ...props.filters, group: value })
}

const updateLastUsedFilter = (value: string | number | boolean | null) => {
  const nextValue = typeof value === 'string' ? value : ''
  if (nextValue === 'range') {
    const fallbackRange = getDefaultLastUsedRange()
    emitFilters({
      ...props.filters,
      last_used_filter: 'range',
      last_used_start_date: lastUsedStartDate.value || fallbackRange.start,
      last_used_end_date: lastUsedEndDate.value || fallbackRange.end
    })
    return
  }

  emitFilters({
    ...props.filters,
    last_used_filter: nextValue,
    last_used_start_date: '',
    last_used_end_date: ''
  })
}

const updateLastUsedStartDate = (value: string) => {
  emitFilters({
    ...props.filters,
    last_used_filter: 'range',
    last_used_start_date: value
  })
}

const updateLastUsedEndDate = (value: string) => {
  emitFilters({
    ...props.filters,
    last_used_filter: 'range',
    last_used_end_date: value
  })
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

const groupOptions = computed(() => [
  { value: '', label: t('admin.accounts.allGroups') },
  { value: 'ungrouped', label: t('admin.accounts.ungroupedGroup') },
  ...(props.groups || []).map((group) => ({ value: String(group.id), label: group.name }))
])

const lastUsedOptions = computed(() => [
  { value: '', label: t('admin.accounts.lastUsedFilters.all') },
  { value: 'unused', label: t('admin.accounts.lastUsedFilters.unused') },
  { value: 'range', label: t('admin.accounts.lastUsedFilters.range') }
])
</script>
