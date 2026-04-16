<template>
  <div class="relative w-full sm:w-72" ref="groupFilterRef">
    <SplitTrigger
      class="w-full"
      :left-label="groupTriggerLabel"
      :right-label="groupSelectionMeta"
      :left-open="showGroupDropdown"
      :right-active="showGroupMatchToggle && currentGroupMatch === 'exact'"
      :right-disabled="!showGroupMatchToggle"
      :right-title="groupFilterHintText"
      @left-click="toggleGroupDropdown"
      @right-click="toggleGroupMatchMode"
    />
    <div
      v-if="showGroupDropdown"
      class="absolute left-0 top-full z-30 mt-2 w-full rounded-xl border border-gray-200 bg-white p-3 shadow-xl dark:border-dark-600 dark:bg-dark-800"
      @click.stop
    >
      <div class="mb-3 flex items-center justify-between gap-2">
        <p class="text-xs text-gray-500 dark:text-dark-300">{{ groupFilterHintText }}</p>
        <button
          type="button"
          class="text-xs font-medium text-primary-600 hover:text-primary-500"
          @click="clearGroupFilter"
        >
          {{ t('common.reset') }}
        </button>
      </div>
      <div class="space-y-3">
        <div>
          <div class="mb-2 flex items-center justify-between gap-2">
            <p class="text-xs font-medium text-gray-600 dark:text-dark-100">{{ t('admin.accounts.groupIncludeSectionLabel') }}</p>
            <p class="text-[11px] text-gray-400 dark:text-dark-300">{{ groupIncludeHintText }}</p>
          </div>
          <div class="mb-2 flex flex-wrap gap-2">
            <button
              type="button"
              class="rounded-md border px-2.5 py-1 text-xs transition-colors"
              :class="isAllGroupsSelected
                ? 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-500/10 dark:text-primary-300'
                : 'border-gray-200 text-gray-600 hover:border-primary-300 dark:border-dark-600 dark:text-dark-200'"
              @click="selectAllGroups"
            >
              {{ t('admin.accounts.allGroups') }}
            </button>
            <button
              type="button"
              class="rounded-md border px-2.5 py-1 text-xs transition-colors"
              :class="isUngroupedSelected
                ? 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-500/10 dark:text-primary-300'
                : 'border-gray-200 text-gray-600 hover:border-primary-300 dark:border-dark-600 dark:text-dark-200'"
              @click="selectUngroupedGroup"
            >
              {{ t('admin.accounts.ungroupedGroup') }}
            </button>
          </div>
          <div class="max-h-40 space-y-1 overflow-y-auto rounded-lg border border-gray-100 p-1 dark:border-dark-700">
            <label
              v-for="group in groups || []"
              :key="`include-${group.id}`"
              class="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-sm transition-colors hover:bg-gray-50 dark:hover:bg-dark-700"
            >
              <input
                type="checkbox"
                :checked="selectedGroupIds.includes(group.id)"
                class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                @change="toggleGroupSelection(group.id, ($event.target as HTMLInputElement).checked)"
              />
              <span class="min-w-0 flex-1 truncate">{{ group.name }}</span>
            </label>
            <div v-if="!(groups && groups.length)" class="px-2 py-3 text-center text-sm text-gray-500 dark:text-dark-300">
              {{ t('common.noGroupsAvailable') }}
            </div>
          </div>
        </div>
        <div v-if="!isUngroupedSelected">
          <div class="mb-2 flex items-center justify-between gap-2">
            <p class="text-xs font-medium text-gray-600 dark:text-dark-100">{{ t('admin.accounts.groupExcludeSectionLabel') }}</p>
            <p class="text-[11px] text-gray-400 dark:text-dark-300">{{ t('admin.accounts.groupExcludeHint') }}</p>
          </div>
          <div class="max-h-40 space-y-1 overflow-y-auto rounded-lg border border-gray-100 p-1 dark:border-dark-700">
            <label
              v-for="group in groups || []"
              :key="`exclude-${group.id}`"
              class="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-sm transition-colors hover:bg-gray-50 dark:hover:bg-dark-700"
            >
              <input
                type="checkbox"
                :checked="excludedGroupIds.includes(group.id)"
                class="h-4 w-4 rounded border-gray-300 text-red-500 focus:ring-red-400"
                @change="toggleExcludedGroupSelection(group.id, ($event.target as HTMLInputElement).checked)"
              />
              <span class="min-w-0 flex-1 truncate">{{ group.name }}</span>
            </label>
            <div v-if="!(groups && groups.length)" class="px-2 py-3 text-center text-sm text-gray-500 dark:text-dark-300">
              {{ t('common.noGroupsAvailable') }}
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import SplitTrigger from '@/components/common/SplitTrigger.vue'
import type { AdminGroup } from '@/types'
import type { AccountTableParams } from '@/views/admin/accounts/query'

type AccountGroupFilterPatch = Partial<Pick<AccountTableParams, 'group' | 'group_exclude' | 'group_match'>>
type AccountGroupFilterState = Pick<AccountTableParams, 'group' | 'group_exclude' | 'group_match'>

const props = defineProps<{
  filters: Partial<AccountGroupFilterState>
  groups?: AdminGroup[]
}>()

const emit = defineEmits<{
  (e: 'update:filters', value: AccountGroupFilterPatch): void
  (e: 'change'): void
}>()

const { t } = useI18n()

const groupFilterRef = ref<HTMLElement | null>(null)
const showGroupDropdown = ref(false)

const currentGroupFilter = computed(() => {
  const raw = props.filters.group
  return typeof raw === 'string' ? raw.trim() : ''
})

const currentGroupExcludeFilter = computed(() => {
  const raw = props.filters.group_exclude
  return typeof raw === 'string' ? raw.trim() : ''
})

const currentGroupMatch = computed<'contains' | 'exact'>(() => {
  const raw = props.filters.group_match
  return raw === 'exact' ? 'exact' : 'contains'
})

const parseGroupIdList = (raw: string) => raw
  .split(',')
  .map((value) => Number.parseInt(value.trim(), 10))
  .filter((value) => Number.isInteger(value) && value > 0)

const serializeGroupIds = (groupIds: number[]) => [...new Set(groupIds)].sort((a, b) => a - b).join(',')

const isUngroupedSelected = computed(() => currentGroupFilter.value === 'ungrouped')
const selectedGroupIds = computed(() => {
  if (!currentGroupFilter.value || isUngroupedSelected.value) return [] as number[]
  return parseGroupIdList(currentGroupFilter.value)
})
const excludedGroupIds = computed(() => {
  if (!currentGroupExcludeFilter.value || isUngroupedSelected.value) return [] as number[]
  return parseGroupIdList(currentGroupExcludeFilter.value)
})
const showGroupMatchToggle = computed(() => selectedGroupIds.value.length > 0 && !isUngroupedSelected.value)
const hasExcludedGroups = computed(() => excludedGroupIds.value.length > 0)
const isAllGroupsSelected = computed(() => !currentGroupFilter.value)
const resolveGroupNames = (groupIds: number[]) => {
  if (groupIds.length === 0) return [] as string[]
  const groupMap = new Map((props.groups || []).map((group) => [group.id, group.name]))
  return groupIds.map((groupId) => groupMap.get(groupId) || `#${groupId}`)
}
const selectedGroupNames = computed(() => resolveGroupNames(selectedGroupIds.value))
const groupTriggerLabel = computed(() => {
  if (isUngroupedSelected.value) return t('admin.accounts.ungroupedGroup')
  if (selectedGroupNames.value.length === 0) return t('admin.accounts.allGroups')
  if (selectedGroupNames.value.length === 1) return selectedGroupNames.value[0]
  return t('admin.accounts.groupFilterSelectedSummary', {
    first: selectedGroupNames.value[0],
    count: selectedGroupNames.value.length - 1
  })
})
const groupIncludeHintText = computed(() => {
  if (isUngroupedSelected.value) {
    return t('admin.accounts.groupFilterUngroupedHint')
  }
  if (!showGroupMatchToggle.value) {
    return t('admin.accounts.groupFilterHintDefault')
  }
  return currentGroupMatch.value === 'exact'
    ? t('admin.accounts.groupFilterExactHint')
    : t('admin.accounts.groupFilterContainsHint')
})
const groupFilterHintText = computed(() => {
  if (isUngroupedSelected.value) {
    return t('admin.accounts.groupFilterUngroupedHint')
  }
  if (!showGroupMatchToggle.value && hasExcludedGroups.value) {
    return t('admin.accounts.groupFilterExcludeOnlyHint')
  }
  return groupIncludeHintText.value
})
const groupSelectionMeta = computed(() => {
  const parts: string[] = []
  if (!isUngroupedSelected.value && selectedGroupIds.value.length > 0) {
    parts.push(currentGroupMatch.value === 'exact'
      ? t('admin.accounts.groupFilterMetaExact')
      : t('admin.accounts.groupFilterMetaContains'))
  }
  if (hasExcludedGroups.value) {
    parts.push(t('admin.accounts.groupFilterMetaExcludeCount', { count: excludedGroupIds.value.length }))
  }
  if (parts.length === 0) {
    return t('admin.accounts.groupFilterMetaDefault')
  }
  return parts.join(' · ')
})

const applyGroupFilters = (group: string, groupExclude: string, mode: '' | 'exact' = '') => {
  emit('update:filters', {
    group,
    group_exclude: group === 'ungrouped' ? '' : groupExclude,
    group_match: group && group !== 'ungrouped' && mode === 'exact' ? 'exact' : ''
  })
  emit('change')
}

const updateGroupMatch = (mode: 'contains' | 'exact') => {
  if (!showGroupMatchToggle.value) return
  applyGroupFilters(currentGroupFilter.value, currentGroupExcludeFilter.value, mode === 'exact' ? 'exact' : '')
}

const toggleGroupMatchMode = () => {
  if (!showGroupMatchToggle.value) return
  updateGroupMatch(currentGroupMatch.value === 'exact' ? 'contains' : 'exact')
}

const clearGroupFilter = () => {
  applyGroupFilters('', '', '')
}

const selectAllGroups = () => {
  applyGroupFilters('', currentGroupExcludeFilter.value, '')
}

const selectUngroupedGroup = () => {
  applyGroupFilters('ungrouped', '', '')
}

const toggleGroupSelection = (groupId: number, checked: boolean) => {
  const nextIncludeIds = checked
    ? [...selectedGroupIds.value, groupId]
    : selectedGroupIds.value.filter((id) => id !== groupId)
  const nextExcludeIds = checked
    ? excludedGroupIds.value.filter((id) => id !== groupId)
    : excludedGroupIds.value
  const normalizedInclude = serializeGroupIds(nextIncludeIds)
  const normalizedExclude = serializeGroupIds(nextExcludeIds)
  applyGroupFilters(normalizedInclude, normalizedExclude, normalizedInclude && currentGroupMatch.value === 'exact' ? 'exact' : '')
}

const toggleExcludedGroupSelection = (groupId: number, checked: boolean) => {
  const nextExcludeIds = checked
    ? [...excludedGroupIds.value, groupId]
    : excludedGroupIds.value.filter((id) => id !== groupId)
  const nextIncludeIds = checked
    ? selectedGroupIds.value.filter((id) => id !== groupId)
    : selectedGroupIds.value
  const normalizedInclude = serializeGroupIds(nextIncludeIds)
  const normalizedExclude = serializeGroupIds(nextExcludeIds)
  applyGroupFilters(normalizedInclude, normalizedExclude, normalizedInclude && currentGroupMatch.value === 'exact' ? 'exact' : '')
}

const toggleGroupDropdown = () => {
  showGroupDropdown.value = !showGroupDropdown.value
}

const handleClickOutside = (event: MouseEvent) => {
  const target = event.target as Node | null
  if (!target) return
  if (groupFilterRef.value && !groupFilterRef.value.contains(target)) {
    showGroupDropdown.value = false
  }
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
})
</script>
