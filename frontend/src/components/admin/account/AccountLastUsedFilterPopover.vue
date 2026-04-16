<template>
  <div class="relative w-full sm:w-80" ref="lastUsedFilterRef">
    <SplitTrigger
      class="w-full"
      :left-label="lastUsedStatusLabel"
      :right-label="lastUsedRangeLabel"
      :left-open="showLastUsedModeDropdown"
      :right-active="showLastUsedDatePanel || currentLastUsedFilter === 'range'"
      :right-title="t('admin.accounts.lastUsedFilters.range')"
      @left-click="toggleLastUsedModeDropdown"
      @right-click="toggleLastUsedDatePanel"
    />
    <div
      v-if="showLastUsedModeDropdown"
      class="absolute left-0 top-full z-30 mt-2 w-full rounded-xl border border-gray-200 bg-white p-3 shadow-xl dark:border-dark-600 dark:bg-dark-800"
      @click.stop
    >
      <div class="mb-3 text-xs text-gray-500 dark:text-dark-300">
        {{ t('admin.accounts.lastUsedFilters.range') }}
      </div>
      <div class="flex flex-wrap gap-2">
        <button
          type="button"
          class="rounded-md border px-2.5 py-1 text-xs transition-colors"
          :class="currentLastUsedMode === 'all'
            ? 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-500/10 dark:text-primary-300'
            : 'border-gray-200 text-gray-600 hover:border-primary-300 dark:border-dark-600 dark:text-dark-200'"
          @click="selectLastUsedMode('all')"
        >
          {{ t('admin.accounts.lastUsedFilters.all') }}
        </button>
        <button
          type="button"
          class="rounded-md border px-2.5 py-1 text-xs transition-colors"
          :class="currentLastUsedMode === 'used'
            ? 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-500/10 dark:text-primary-300'
            : 'border-gray-200 text-gray-600 hover:border-primary-300 dark:border-dark-600 dark:text-dark-200'"
          @click="selectLastUsedMode('used')"
        >
          {{ t('admin.accounts.lastUsedFilters.used') }}
        </button>
        <button
          type="button"
          class="rounded-md border px-2.5 py-1 text-xs transition-colors"
          :class="currentLastUsedMode === 'unused'
            ? 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-500/10 dark:text-primary-300'
            : 'border-gray-200 text-gray-600 hover:border-primary-300 dark:border-dark-600 dark:text-dark-200'"
          @click="selectLastUsedMode('unused')"
        >
          {{ t('admin.accounts.lastUsedFilters.unused') }}
        </button>
      </div>
    </div>
    <div
      v-if="showLastUsedDatePanel"
      class="absolute left-0 top-full z-30 mt-2 w-full rounded-xl border border-gray-200 bg-white p-3 shadow-xl dark:border-dark-600 dark:bg-dark-800"
      @click.stop
    >
      <DateRangePicker
        inline
        :start-date="lastUsedDraftStartDate"
        :end-date="lastUsedDraftEndDate"
        @update:startDate="updateLastUsedDraftStartDate"
        @update:endDate="updateLastUsedDraftEndDate"
        @change="applyLastUsedDateRange"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import SplitTrigger from '@/components/common/SplitTrigger.vue'
import type { AccountTableParams } from '@/views/admin/accounts/query'

type AccountLastUsedFilterPatch = Partial<Pick<AccountTableParams, 'last_used_filter' | 'last_used_start_date' | 'last_used_end_date'>>
type AccountLastUsedFilterState = Pick<AccountTableParams, 'last_used_filter' | 'last_used_start_date' | 'last_used_end_date'>

const props = defineProps<{
  filters: Partial<AccountLastUsedFilterState>
}>()

const emit = defineEmits<{
  (e: 'update:filters', value: AccountLastUsedFilterPatch): void
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

const currentLastUsedMode = computed<'all' | 'used' | 'unused'>(() => {
  if (currentLastUsedFilter.value === 'unused') return 'unused'
  if (currentLastUsedFilter.value === 'range') return 'used'
  return 'all'
})

const lastUsedStartDate = computed(() => {
  const raw = props.filters.last_used_start_date
  return typeof raw === 'string' ? raw : ''
})

const lastUsedEndDate = computed(() => {
  const raw = props.filters.last_used_end_date
  return typeof raw === 'string' ? raw : ''
})

const lastUsedStatusLabel = computed(() => {
  if (currentLastUsedMode.value === 'unused') return t('admin.accounts.lastUsedFilters.unused')
  if (currentLastUsedMode.value === 'used') return t('admin.accounts.lastUsedFilters.used')
  return t('admin.accounts.lastUsedFilters.all')
})

const formatLastUsedPreviewDate = (value: string) => {
  if (!value) return ''
  const date = new Date(`${value}T00:00:00`)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}

const lastUsedRangeLabel = computed(() => {
  if (currentLastUsedFilter.value !== 'range' || !lastUsedStartDate.value || !lastUsedEndDate.value) {
    return t('admin.accounts.lastUsedFilters.range')
  }
  return `${formatLastUsedPreviewDate(lastUsedStartDate.value)} - ${formatLastUsedPreviewDate(lastUsedEndDate.value)}`
})

const lastUsedFilterRef = ref<HTMLElement | null>(null)
const showLastUsedModeDropdown = ref(false)
const showLastUsedDatePanel = ref(false)
const lastUsedDraftStartDate = ref('')
const lastUsedDraftEndDate = ref('')

const syncLastUsedDraftRange = () => {
  const fallbackRange = getDefaultLastUsedRange()
  lastUsedDraftStartDate.value = lastUsedStartDate.value || fallbackRange.start
  lastUsedDraftEndDate.value = lastUsedEndDate.value || fallbackRange.end
}

const applyLastUsedFilters = (patch: AccountLastUsedFilterPatch) => {
  emit('update:filters', patch)
  emit('change')
}

const selectLastUsedMode = (mode: 'all' | 'used' | 'unused') => {
  showLastUsedModeDropdown.value = false
  showLastUsedDatePanel.value = false

  if (mode === 'all') {
    applyLastUsedFilters({
      last_used_filter: '',
      last_used_start_date: '',
      last_used_end_date: ''
    })
    return
  }

  if (mode === 'unused') {
    applyLastUsedFilters({
      last_used_filter: 'unused',
      last_used_start_date: '',
      last_used_end_date: ''
    })
    return
  }

  syncLastUsedDraftRange()
  applyLastUsedFilters({
    last_used_filter: 'range',
    last_used_start_date: lastUsedDraftStartDate.value,
    last_used_end_date: lastUsedDraftEndDate.value
  })
}

const toggleLastUsedModeDropdown = () => {
  showLastUsedDatePanel.value = false
  showLastUsedModeDropdown.value = !showLastUsedModeDropdown.value
}

const toggleLastUsedDatePanel = () => {
  showLastUsedModeDropdown.value = false
  syncLastUsedDraftRange()
  showLastUsedDatePanel.value = !showLastUsedDatePanel.value
}

const updateLastUsedDraftStartDate = (value: string) => {
  lastUsedDraftStartDate.value = value
}

const updateLastUsedDraftEndDate = (value: string) => {
  lastUsedDraftEndDate.value = value
}

const applyLastUsedDateRange = (range: { startDate: string; endDate: string; preset: string | null }) => {
  void range.preset
  showLastUsedDatePanel.value = false
  applyLastUsedFilters({
    last_used_filter: 'range',
    last_used_start_date: range.startDate,
    last_used_end_date: range.endDate
  })
}

const handleClickOutside = (event: MouseEvent) => {
  const target = event.target as Node | null
  if (!target) return
  if (lastUsedFilterRef.value && !lastUsedFilterRef.value.contains(target)) {
    showLastUsedModeDropdown.value = false
    showLastUsedDatePanel.value = false
  }
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
})
</script>
