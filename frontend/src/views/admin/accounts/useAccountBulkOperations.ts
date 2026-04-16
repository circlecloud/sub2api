import { computed, ref, watch, type ComputedRef, type Ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import type { BulkAccountFilters } from '@/api/admin/accounts'
import type { Account, AccountPlatform, AccountType } from '@/types'
import { isAccountPrivacyApplied } from '../accountPrivacy'
import type { AccountTableParams } from './query'

export type BulkOperationProgressTone = 'primary' | 'emerald' | 'cyan' | 'purple' | 'sky'

export type BulkOperationProgressItem = {
  key: string
  tone: BulkOperationProgressTone
  statusText: string
  percent: number
  successCount?: number
  failedCount?: number
}

type FilteredAccountTarget = Pick<Account, 'id' | 'platform' | 'type'>

type BulkEditModalOptions = {
  targetCount?: number
  targetFilters?: BulkAccountFilters | null
  platforms?: AccountPlatform[]
  types?: AccountType[]
}

type BatchTestScope = 'selected' | 'filtered' | 'all'

type UseAccountBulkOperationsOptions = {
  accounts: Ref<Account[]>
  loading: Ref<boolean>
  pagination: {
    total: number
    page_size: number
  }
  selectedIds: Ref<number[]>
  clearSelection: () => void
  setSelectedIds: (ids: number[]) => void
  refreshingAccountId: Ref<number | null>
  refreshingUsageWindowAccountId: Ref<number | null>
  usageManualRefreshToken: Ref<number>
  hasActiveAccountFilters: ComputedRef<boolean>
  buildActiveAccountFilters: (source?: Partial<AccountTableParams>) => BulkAccountFilters
  openBulkEditModal: (
    accountIds: number[],
    targets?: FilteredAccountTarget[],
    options?: BulkEditModalOptions
  ) => void
  openBatchTestModal: (accountIds?: number[], scope?: BatchTestScope) => void
  patchAccountInList: (updatedAccount: Account) => void
  reload: () => Promise<void>
  enterAutoRefreshSilentWindow: () => void
}

const BULK_REFRESH_TOKEN_CONCURRENCY = 5
const BULK_REFRESH_USAGE_WINDOW_CONCURRENCY = 3
const BULK_SET_PRIVACY_CONCURRENCY = 5

const createProgressPercent = (loaded: number, total: number) => {
  if (total <= 0) return 0
  return Math.max(0, Math.min(100, Math.round((loaded / total) * 100)))
}

const getRequestErrorMessage = (error: any, fallback: string) => {
  return error?.response?.data?.message
    || error?.response?.data?.detail
    || error?.message
    || fallback
}

export const useAccountBulkOperations = ({
  accounts,
  loading,
  pagination,
  selectedIds,
  clearSelection,
  setSelectedIds,
  refreshingAccountId,
  refreshingUsageWindowAccountId,
  usageManualRefreshToken,
  hasActiveAccountFilters,
  buildActiveAccountFilters,
  openBulkEditModal,
  openBatchTestModal,
  patchAccountInList,
  reload,
  enterAutoRefreshSilentWindow,
}: UseAccountBulkOperationsOptions) => {
  const { t } = useI18n()
  const appStore = useAppStore()

  const preparingFilteredBulkEdit = ref(false)
  const bulkEditPreparationMode = ref<'filtered' | 'selected' | null>(null)
  const bulkEditPreparationLoaded = ref(0)
  const bulkEditPreparationTotal = ref(0)
  const preparingFilteredBatchTest = ref(false)
  const preparingFilteredUsageRefresh = ref(false)
  const filteredBatchTestPreparationLoaded = ref(0)
  const filteredBatchTestPreparationTotal = ref(0)
  const filteredUsageRefreshPreparationLoaded = ref(0)
  const filteredUsageRefreshPreparationTotal = ref(0)
  const bulkRefreshingTokens = ref(false)
  const bulkRefreshingUsageWindows = ref(false)
  const bulkSettingPrivacy = ref(false)
  const batchTestingAccounts = ref(false)
  const bulkRefreshProcessed = ref(0)
  const bulkRefreshTotal = ref(0)
  const bulkRefreshSuccessCount = ref(0)
  const bulkRefreshFailedCount = ref(0)
  const bulkRefreshUsageWindowProcessed = ref(0)
  const bulkRefreshUsageWindowTotal = ref(0)
  const bulkRefreshUsageWindowSuccessCount = ref(0)
  const bulkRefreshUsageWindowFailedCount = ref(0)
  const bulkSettingPrivacyProcessed = ref(0)
  const bulkSettingPrivacyTotal = ref(0)
  const bulkSettingPrivacySuccessCount = ref(0)
  const bulkSettingPrivacyFailedCount = ref(0)

  const bulkEditFetchPageSize = computed(() => {
    const raw = Number(pagination.page_size)
    if (!Number.isFinite(raw) || raw <= 0) return 20
    return Math.min(1000, Math.max(1, Math.trunc(raw)))
  })

  const bulkActionMenuDisabled = computed(() => {
    return loading.value
      || preparingFilteredBulkEdit.value
      || preparingFilteredBatchTest.value
      || preparingFilteredUsageRefresh.value
      || batchTestingAccounts.value
      || bulkRefreshingTokens.value
      || bulkRefreshingUsageWindows.value
      || bulkSettingPrivacy.value
      || refreshingAccountId.value !== null
      || refreshingUsageWindowAccountId.value !== null
      || pagination.total === 0
  })

  const bulkActionMenuLabel = computed(() => {
    return t('admin.accounts.bulkActions.menuWithCount', { count: pagination.total || 0 })
  })

  const bulkEditPreparationProgressPercent = computed(() => createProgressPercent(
    bulkEditPreparationLoaded.value,
    bulkEditPreparationTotal.value
  ))
  const bulkEditPreparationStatusText = computed(() => {
    if (bulkEditPreparationMode.value === 'selected') {
      if (bulkEditPreparationTotal.value > 0) {
        return t('admin.accounts.bulkEdit.selectedProgressStatus', {
          loaded: bulkEditPreparationLoaded.value,
          total: bulkEditPreparationTotal.value,
          batch: bulkEditFetchPageSize.value
        })
      }
      return t('admin.accounts.bulkEdit.preparingSelectedAction', { count: selectedIds.value.length })
    }
    if (bulkEditPreparationTotal.value > 0) {
      return t('admin.accounts.bulkEdit.progressStatus', {
        loaded: bulkEditPreparationLoaded.value,
        total: bulkEditPreparationTotal.value,
        batch: bulkEditFetchPageSize.value
      })
    }
    return t('admin.accounts.bulkEdit.preparingAction', { count: pagination.total || 0 })
  })

  const filteredBatchTestPreparationProgressPercent = computed(() => createProgressPercent(
    filteredBatchTestPreparationLoaded.value,
    filteredBatchTestPreparationTotal.value
  ))
  const filteredBatchTestPreparationStatusText = computed(() => {
    if (filteredBatchTestPreparationTotal.value > 0) {
      return t('admin.accounts.bulkTest.preparingProgressStatus', {
        loaded: filteredBatchTestPreparationLoaded.value,
        total: filteredBatchTestPreparationTotal.value,
        batch: bulkEditFetchPageSize.value
      })
    }
    return t('admin.accounts.bulkTest.preparingAction', { count: pagination.total || 0 })
  })

  const filteredUsageRefreshPreparationProgressPercent = computed(() => createProgressPercent(
    filteredUsageRefreshPreparationLoaded.value,
    filteredUsageRefreshPreparationTotal.value
  ))
  const filteredUsageRefreshPreparationStatusText = computed(() => {
    if (filteredUsageRefreshPreparationTotal.value > 0) {
      return t('admin.accounts.bulkActions.preparingRefreshUsageWindowProgress', {
        loaded: filteredUsageRefreshPreparationLoaded.value,
        total: filteredUsageRefreshPreparationTotal.value,
        batch: bulkEditFetchPageSize.value
      })
    }
    return t('admin.accounts.bulkActions.preparingRefreshUsageWindow', { count: pagination.total || 0 })
  })

  const bulkRefreshTokenProgressPercent = computed(() => createProgressPercent(
    bulkRefreshProcessed.value,
    bulkRefreshTotal.value
  ))
  const bulkRefreshTokenStatusText = computed(() => {
    if (bulkRefreshTotal.value <= 0) {
      return t('admin.accounts.bulkActions.refreshingAction')
    }
    return t('admin.accounts.bulkActions.refreshingProgress', {
      processed: bulkRefreshProcessed.value,
      total: bulkRefreshTotal.value
    })
  })

  const bulkRefreshUsageWindowProgressPercent = computed(() => createProgressPercent(
    bulkRefreshUsageWindowProcessed.value,
    bulkRefreshUsageWindowTotal.value
  ))
  const bulkRefreshUsageWindowStatusText = computed(() => {
    if (bulkRefreshUsageWindowTotal.value <= 0) {
      return t('admin.accounts.bulkActions.refreshingUsageWindowAction')
    }
    return t('admin.accounts.bulkActions.refreshingUsageWindowProgress', {
      processed: bulkRefreshUsageWindowProcessed.value,
      total: bulkRefreshUsageWindowTotal.value
    })
  })
  const bulkRefreshUsageWindowActionLabel = computed(() => {
    if (!bulkRefreshingUsageWindows.value) {
      return t('admin.accounts.bulkActions.refreshUsageWindow')
    }
    if (bulkRefreshUsageWindowTotal.value <= 0) {
      return t('admin.accounts.bulkActions.refreshingUsageWindowAction')
    }
    return t('admin.accounts.bulkActions.refreshingUsageWindowProgressAction', {
      processed: bulkRefreshUsageWindowProcessed.value,
      total: bulkRefreshUsageWindowTotal.value
    })
  })

  const bulkSettingPrivacyProgressPercent = computed(() => createProgressPercent(
    bulkSettingPrivacyProcessed.value,
    bulkSettingPrivacyTotal.value
  ))
  const bulkSettingPrivacyStatusText = computed(() => {
    if (bulkSettingPrivacyTotal.value <= 0) {
      return t('admin.accounts.bulkActions.settingPrivacyAction')
    }
    return t('admin.accounts.bulkActions.settingPrivacyProgress', {
      processed: bulkSettingPrivacyProcessed.value,
      total: bulkSettingPrivacyTotal.value
    })
  })

  const bulkEditSelectedActionLabel = computed(() => {
    if (preparingFilteredBulkEdit.value && bulkEditPreparationMode.value === 'selected') {
      if (bulkEditPreparationTotal.value > 0) {
        return t('admin.accounts.bulkEdit.preparingSelectedProgressAction', {
          loaded: bulkEditPreparationLoaded.value,
          total: bulkEditPreparationTotal.value
        })
      }
      return t('admin.accounts.bulkEdit.preparingSelectedAction', { count: selectedIds.value.length })
    }
    return t('admin.accounts.bulkActions.edit')
  })

  const bulkOperationProgressItems = computed<BulkOperationProgressItem[]>(() => {
    const items: BulkOperationProgressItem[] = []

    if (preparingFilteredBulkEdit.value) {
      items.push({
        key: 'filtered-bulk-edit',
        tone: 'primary',
        statusText: bulkEditPreparationStatusText.value,
        percent: bulkEditPreparationProgressPercent.value
      })
    }

    if (preparingFilteredBatchTest.value) {
      items.push({
        key: 'filtered-batch-test',
        tone: 'emerald',
        statusText: filteredBatchTestPreparationStatusText.value,
        percent: filteredBatchTestPreparationProgressPercent.value
      })
    }

    if (preparingFilteredUsageRefresh.value) {
      items.push({
        key: 'filtered-usage-refresh',
        tone: 'cyan',
        statusText: filteredUsageRefreshPreparationStatusText.value,
        percent: filteredUsageRefreshPreparationProgressPercent.value
      })
    }

    if (bulkRefreshingTokens.value) {
      items.push({
        key: 'bulk-refresh-token',
        tone: 'purple',
        statusText: bulkRefreshTokenStatusText.value,
        percent: bulkRefreshTokenProgressPercent.value,
        successCount: bulkRefreshSuccessCount.value,
        failedCount: bulkRefreshFailedCount.value
      })
    }

    if (bulkSettingPrivacy.value) {
      items.push({
        key: 'bulk-set-privacy',
        tone: 'emerald',
        statusText: bulkSettingPrivacyStatusText.value,
        percent: bulkSettingPrivacyProgressPercent.value,
        successCount: bulkSettingPrivacySuccessCount.value,
        failedCount: bulkSettingPrivacyFailedCount.value
      })
    }

    if (bulkRefreshingUsageWindows.value) {
      items.push({
        key: 'bulk-refresh-usage-window',
        tone: 'sky',
        statusText: bulkRefreshUsageWindowStatusText.value,
        percent: bulkRefreshUsageWindowProgressPercent.value,
        successCount: bulkRefreshUsageWindowSuccessCount.value,
        failedCount: bulkRefreshUsageWindowFailedCount.value
      })
    }

    return items
  })

  const accountTargetCache = ref<Map<number, FilteredAccountTarget>>(new Map())

  const cacheAccountTargets = (targets: FilteredAccountTarget[]) => {
    if (targets.length === 0) return
    const next = new Map(accountTargetCache.value)
    targets.forEach((target) => {
      next.set(target.id, target)
    })
    accountTargetCache.value = next
  }

  const cacheAccountTargetsFromAccounts = (accountList: Array<Pick<Account, 'id' | 'platform' | 'type'>>) => {
    cacheAccountTargets(
      accountList.map((account) => ({
        id: account.id,
        platform: account.platform,
        type: account.type
      }))
    )
  }

  const getCachedAccountTargets = (accountIds: number[]): FilteredAccountTarget[] =>
    accountIds
      .map((id) => accountTargetCache.value.get(id))
      .filter((target): target is FilteredAccountTarget => target !== undefined)

  watch(
    accounts,
    (rows) => {
      cacheAccountTargetsFromAccounts(rows)
    },
    { immediate: true }
  )

  const resetBulkEditPreparationProgress = () => {
    bulkEditPreparationMode.value = null
    bulkEditPreparationLoaded.value = 0
    bulkEditPreparationTotal.value = 0
  }

  const resetFilteredBatchTestPreparation = () => {
    filteredBatchTestPreparationLoaded.value = 0
    filteredBatchTestPreparationTotal.value = 0
  }

  const resetFilteredUsageRefreshPreparation = () => {
    filteredUsageRefreshPreparationLoaded.value = 0
    filteredUsageRefreshPreparationTotal.value = 0
  }

  const buildFilteredBulkActionFilters = () => buildActiveAccountFilters()

  const fetchAllFilteredAccountIds = async (
    onProgress?: (progress: { loaded: number; total: number }) => void
  ): Promise<number[]> => {
    const filters = buildFilteredBulkActionFilters()
    const resolved = await adminAPI.accounts.resolveBulkUpdateTargets(filters)
    const accountIds = Array.isArray(resolved?.account_ids) ? resolved.account_ids : []
    const total = typeof resolved?.count === 'number' && resolved.count >= 0
      ? resolved.count
      : accountIds.length
    onProgress?.({
      loaded: Math.min(accountIds.length, total),
      total
    })
    return accountIds
  }

  const getFilteredBatchScope = (): BatchTestScope => {
    return hasActiveAccountFilters.value ? 'filtered' : 'all'
  }

  const getUsageWindowScopeLabel = (scope: BatchTestScope, count: number) => {
    if (scope === 'filtered') {
      return t('admin.accounts.bulkActions.filteredScopeLabel', { count })
    }
    if (scope === 'all') {
      return t('admin.accounts.bulkActions.allScopeLabel', { count })
    }
    return t('admin.accounts.bulkActions.selectedScopeLabel', { count })
  }

  const openSelectedBulkEdit = () => {
    if (preparingFilteredBulkEdit.value || preparingFilteredBatchTest.value || preparingFilteredUsageRefresh.value) return
    if (selectedIds.value.length === 0) {
      appStore.showError(t('admin.accounts.bulkEdit.noSelection'))
      return
    }

    const ids = [...selectedIds.value]
    const selectedTargets = getCachedAccountTargets(ids)

    if (selectedTargets.length < ids.length) {
      console.error('Missing cached account targets for bulk edit selection:', {
        selectedIds: ids,
        cachedIds: selectedTargets.map((target) => target.id)
      })
      appStore.showError(t('admin.accounts.bulkEdit.prepareSelectedFailed'))
      return
    }

    openBulkEditModal(ids, selectedTargets)
  }

  const handleOpenFilteredBulkEdit = async () => {
    if (preparingFilteredBulkEdit.value || preparingFilteredBatchTest.value || preparingFilteredUsageRefresh.value) return
    if (batchTestingAccounts.value || bulkRefreshingTokens.value || bulkRefreshingUsageWindows.value || refreshingAccountId.value !== null || refreshingUsageWindowAccountId.value !== null) return
    if (pagination.total === 0) {
      appStore.showError(t('admin.accounts.bulkEdit.noMatches'))
      return
    }

    preparingFilteredBulkEdit.value = true
    bulkEditPreparationMode.value = 'filtered'
    bulkEditPreparationLoaded.value = 0
    bulkEditPreparationTotal.value = pagination.total || 0
    try {
      const filters = buildFilteredBulkActionFilters()
      const preview = await adminAPI.accounts.previewBulkUpdateTargets(filters)
      if (!preview || preview.count <= 0) {
        appStore.showError(t('admin.accounts.bulkEdit.noMatches'))
        return
      }
      bulkEditPreparationLoaded.value = preview.count
      bulkEditPreparationTotal.value = preview.count
      openBulkEditModal([], [], {
        targetCount: preview.count,
        targetFilters: filters,
        platforms: preview.platforms ?? [],
        types: preview.types ?? []
      })
    } catch (error: any) {
      console.error('Failed to prepare bulk edit targets:', error)
      appStore.showError(
        error?.response?.data?.message
          || error?.response?.data?.detail
          || error?.message
          || t('admin.accounts.bulkEdit.prepareFailed')
      )
    } finally {
      preparingFilteredBulkEdit.value = false
      resetBulkEditPreparationProgress()
    }
  }

  const openFilteredBatchTestModal = async () => {
    if (preparingFilteredBatchTest.value || preparingFilteredUsageRefresh.value) return
    if (batchTestingAccounts.value || bulkRefreshingTokens.value || bulkRefreshingUsageWindows.value || refreshingAccountId.value !== null || refreshingUsageWindowAccountId.value !== null) return
    if (pagination.total === 0) {
      appStore.showError(t('admin.accounts.bulkTest.noMatches'))
      return
    }

    preparingFilteredBatchTest.value = true
    filteredBatchTestPreparationLoaded.value = 0
    filteredBatchTestPreparationTotal.value = pagination.total || 0
    try {
      const accountIds = await fetchAllFilteredAccountIds(({ loaded, total }) => {
        filteredBatchTestPreparationLoaded.value = loaded
        filteredBatchTestPreparationTotal.value = total
      })
      if (accountIds.length === 0) {
        appStore.showError(t('admin.accounts.bulkTest.noMatches'))
        return
      }
      openBatchTestModal(accountIds, getFilteredBatchScope())
    } catch (error: any) {
      console.error('Failed to prepare accounts for filtered batch test:', error)
      appStore.showError(getRequestErrorMessage(error, t('admin.accounts.bulkTest.prepareFailed')))
    } finally {
      preparingFilteredBatchTest.value = false
      resetFilteredBatchTestPreparation()
    }
  }

  const runBulkRefreshUsageWindow = async (
    accountIds: number[],
    scope: BatchTestScope
  ) => {
    if (accountIds.length === 0) return
    if (!confirm(t('admin.accounts.bulkActions.refreshUsageWindowConfirm', {
      scope: getUsageWindowScopeLabel(scope, accountIds.length)
    }))) return

    bulkRefreshingUsageWindows.value = true
    bulkRefreshUsageWindowProcessed.value = 0
    bulkRefreshUsageWindowTotal.value = accountIds.length
    bulkRefreshUsageWindowSuccessCount.value = 0
    bulkRefreshUsageWindowFailedCount.value = 0

    const failedIds: number[] = []
    let index = 0

    const worker = async () => {
      while (index < accountIds.length) {
        const currentId = accountIds[index]
        index += 1

        try {
          const usage = await adminAPI.accounts.getUsage(currentId, 'active', { force: true })
          if (usage?.error) {
            console.error(`Failed to refresh usage window for account ${currentId}:`, usage.error)
            failedIds.push(currentId)
            bulkRefreshUsageWindowFailedCount.value += 1
          } else {
            bulkRefreshUsageWindowSuccessCount.value += 1
          }
        } catch (error) {
          console.error(`Failed to refresh usage window for account ${currentId}:`, error)
          failedIds.push(currentId)
          bulkRefreshUsageWindowFailedCount.value += 1
        } finally {
          bulkRefreshUsageWindowProcessed.value += 1
        }
      }
    }

    try {
      const workers = Array.from(
        { length: Math.min(BULK_REFRESH_USAGE_WINDOW_CONCURRENCY, accountIds.length) },
        () => worker()
      )
      await Promise.all(workers)

      enterAutoRefreshSilentWindow()

      if (bulkRefreshUsageWindowSuccessCount.value > 0 && bulkRefreshUsageWindowFailedCount.value === 0) {
        appStore.showSuccess(t('admin.accounts.bulkActions.refreshUsageWindowSuccess', {
          count: bulkRefreshUsageWindowSuccessCount.value
        }))
        if (scope === 'selected') {
          clearSelection()
        }
      } else if (bulkRefreshUsageWindowSuccessCount.value > 0) {
        appStore.showError(t('admin.accounts.bulkActions.partialSuccess', {
          success: bulkRefreshUsageWindowSuccessCount.value,
          failed: bulkRefreshUsageWindowFailedCount.value
        }))
        setSelectedIds(failedIds.length > 0 ? failedIds : accountIds)
      } else {
        appStore.showError(t('admin.accounts.bulkActions.refreshUsageWindowFailed'))
        setSelectedIds(failedIds.length > 0 ? failedIds : accountIds)
      }
    } catch (error) {
      console.error('Failed to bulk refresh usage windows:', error)
      appStore.showError(getRequestErrorMessage(error, t('admin.accounts.bulkActions.refreshUsageWindowFailed')))
    } finally {
      try {
        await reload()
        usageManualRefreshToken.value += 1
      } catch (error) {
        console.error('Failed to reload accounts after bulk usage refresh:', error)
      }
      bulkRefreshingUsageWindows.value = false
    }
  }

  const handleBulkRefreshToken = async () => {
    if (bulkRefreshingTokens.value || bulkRefreshingUsageWindows.value || bulkSettingPrivacy.value || batchTestingAccounts.value || refreshingAccountId.value !== null || refreshingUsageWindowAccountId.value !== null) return

    const accountIds = [...selectedIds.value]
    if (accountIds.length === 0) return
    if (!confirm(t('common.confirm'))) return

    bulkRefreshingTokens.value = true
    bulkRefreshProcessed.value = 0
    bulkRefreshTotal.value = accountIds.length
    bulkRefreshSuccessCount.value = 0
    bulkRefreshFailedCount.value = 0

    const failedIds: number[] = []
    let index = 0

    const worker = async () => {
      while (index < accountIds.length) {
        const currentId = accountIds[index]
        index += 1

        try {
          const updated = await adminAPI.accounts.refreshCredentials(currentId)
          if (updated && typeof updated.id === 'number') {
            patchAccountInList(updated)
          }
          bulkRefreshSuccessCount.value += 1
        } catch (error) {
          console.error(`Failed to refresh token for account ${currentId}:`, error)
          failedIds.push(currentId)
          bulkRefreshFailedCount.value += 1
        } finally {
          bulkRefreshProcessed.value += 1
        }
      }
    }

    try {
      const workers = Array.from(
        { length: Math.min(BULK_REFRESH_TOKEN_CONCURRENCY, accountIds.length) },
        () => worker()
      )
      await Promise.all(workers)

      enterAutoRefreshSilentWindow()

      if (bulkRefreshFailedCount.value > 0) {
        appStore.showError(t('admin.accounts.bulkActions.partialSuccess', {
          success: bulkRefreshSuccessCount.value,
          failed: bulkRefreshFailedCount.value
        }))
        setSelectedIds(failedIds.length > 0 ? failedIds : accountIds)
      } else {
        appStore.showSuccess(t('admin.accounts.bulkActions.refreshTokenSuccess', {
          count: bulkRefreshSuccessCount.value
        }))
        clearSelection()
      }
    } catch (error) {
      console.error('Failed to bulk refresh token:', error)
      appStore.showError(getRequestErrorMessage(error, t('admin.accounts.failedToRefresh')))
    } finally {
      try {
        await reload()
      } catch (error) {
        console.error('Failed to reload accounts after bulk refresh:', error)
      }
      bulkRefreshingTokens.value = false
    }
  }

  const handleBulkRefreshUsageWindow = async () => {
    if (bulkRefreshingUsageWindows.value || bulkRefreshingTokens.value || bulkSettingPrivacy.value || batchTestingAccounts.value || refreshingAccountId.value !== null || refreshingUsageWindowAccountId.value !== null) return

    const accountIds = [...selectedIds.value]
    if (accountIds.length === 0) return
    await runBulkRefreshUsageWindow(accountIds, 'selected')
  }

  const handleBulkSetPrivacy = async () => {
    if (bulkSettingPrivacy.value || bulkRefreshingUsageWindows.value || bulkRefreshingTokens.value || batchTestingAccounts.value || refreshingAccountId.value !== null || refreshingUsageWindowAccountId.value !== null) return

    const accountIds = [...selectedIds.value]
    if (accountIds.length === 0) return

    bulkSettingPrivacy.value = true
    bulkSettingPrivacyProcessed.value = 0
    bulkSettingPrivacyTotal.value = accountIds.length
    bulkSettingPrivacySuccessCount.value = 0
    bulkSettingPrivacyFailedCount.value = 0
    const failedIds: number[] = []
    let index = 0

    const worker = async () => {
      while (index < accountIds.length) {
        const currentId = accountIds[index]
        index += 1

        try {
          const updated = await adminAPI.accounts.setPrivacy(currentId)
          if (updated && typeof updated.id === 'number') {
            patchAccountInList(updated)
          }
          if (isAccountPrivacyApplied(updated)) {
            bulkSettingPrivacySuccessCount.value += 1
          } else {
            failedIds.push(currentId)
            bulkSettingPrivacyFailedCount.value += 1
          }
        } catch (error) {
          console.error(`Failed to set privacy for account ${currentId}:`, error)
          failedIds.push(currentId)
          bulkSettingPrivacyFailedCount.value += 1
        } finally {
          bulkSettingPrivacyProcessed.value += 1
        }
      }
    }

    try {
      const workers = Array.from(
        { length: Math.min(BULK_SET_PRIVACY_CONCURRENCY, accountIds.length) },
        () => worker()
      )
      await Promise.all(workers)

      enterAutoRefreshSilentWindow()

      if (bulkSettingPrivacyFailedCount.value > 0) {
        appStore.showError(t('admin.accounts.bulkActions.partialSuccess', {
          success: bulkSettingPrivacySuccessCount.value,
          failed: bulkSettingPrivacyFailedCount.value
        }))
        setSelectedIds(failedIds.length > 0 ? failedIds : accountIds)
      } else {
        appStore.showSuccess(t('admin.accounts.bulkActions.setPrivacySuccess', {
          count: bulkSettingPrivacySuccessCount.value
        }))
        clearSelection()
      }
    } catch (error) {
      console.error('Failed to bulk set privacy:', error)
      appStore.showError(getRequestErrorMessage(error, t('admin.accounts.privacyFailed')))
    } finally {
      try {
        await reload()
      } catch (error) {
        console.error('Failed to reload accounts after bulk privacy update:', error)
      }
      bulkSettingPrivacy.value = false
    }
  }

  const handleFilteredRefreshUsageWindow = async () => {
    if (preparingFilteredUsageRefresh.value || preparingFilteredBatchTest.value) return
    if (bulkRefreshingUsageWindows.value || bulkRefreshingTokens.value || batchTestingAccounts.value || refreshingAccountId.value !== null || refreshingUsageWindowAccountId.value !== null) return
    if (pagination.total === 0) {
      appStore.showError(t('admin.accounts.bulkActions.refreshUsageWindowNoMatches'))
      return
    }

    preparingFilteredUsageRefresh.value = true
    filteredUsageRefreshPreparationLoaded.value = 0
    filteredUsageRefreshPreparationTotal.value = pagination.total || 0
    try {
      const accountIds = await fetchAllFilteredAccountIds(({ loaded, total }) => {
        filteredUsageRefreshPreparationLoaded.value = loaded
        filteredUsageRefreshPreparationTotal.value = total
      })
      if (accountIds.length === 0) {
        appStore.showError(t('admin.accounts.bulkActions.refreshUsageWindowNoMatches'))
        return
      }
      await runBulkRefreshUsageWindow(accountIds, getFilteredBatchScope())
    } catch (error: any) {
      console.error('Failed to prepare accounts for filtered usage refresh:', error)
      appStore.showError(getRequestErrorMessage(error, t('admin.accounts.bulkActions.refreshUsageWindowPrepareFailed')))
    } finally {
      preparingFilteredUsageRefresh.value = false
      resetFilteredUsageRefreshPreparation()
    }
  }

  const handleBulkActionMenuEdit = () => {
    void handleOpenFilteredBulkEdit()
  }

  const handleBulkActionMenuTest = () => {
    void openFilteredBatchTestModal()
  }

  const handleBulkActionMenuRefreshUsage = () => {
    void handleFilteredRefreshUsageWindow()
  }

  const handleBatchTestRunningChange = (running: boolean) => {
    batchTestingAccounts.value = running
  }

  return {
    preparingFilteredBulkEdit,
    bulkEditPreparationMode,
    bulkEditPreparationLoaded,
    bulkEditPreparationTotal,
    preparingFilteredBatchTest,
    preparingFilteredUsageRefresh,
    filteredBatchTestPreparationLoaded,
    filteredBatchTestPreparationTotal,
    filteredUsageRefreshPreparationLoaded,
    filteredUsageRefreshPreparationTotal,
    bulkRefreshingTokens,
    bulkRefreshingUsageWindows,
    bulkSettingPrivacy,
    batchTestingAccounts,
    bulkRefreshProcessed,
    bulkRefreshTotal,
    bulkRefreshSuccessCount,
    bulkRefreshFailedCount,
    bulkRefreshUsageWindowProcessed,
    bulkRefreshUsageWindowTotal,
    bulkRefreshUsageWindowSuccessCount,
    bulkRefreshUsageWindowFailedCount,
    bulkSettingPrivacyProcessed,
    bulkSettingPrivacyTotal,
    bulkSettingPrivacySuccessCount,
    bulkSettingPrivacyFailedCount,
    bulkActionMenuDisabled,
    bulkActionMenuLabel,
    bulkEditPreparationStatusText,
    filteredBatchTestPreparationStatusText,
    filteredUsageRefreshPreparationStatusText,
    bulkRefreshUsageWindowActionLabel,
    bulkEditSelectedActionLabel,
    bulkOperationProgressItems,
    openSelectedBulkEdit,
    handleOpenFilteredBulkEdit,
    handleBulkActionMenuEdit,
    handleBulkActionMenuTest,
    handleBulkActionMenuRefreshUsage,
    handleBulkRefreshToken,
    handleBulkRefreshUsageWindow,
    handleBulkSetPrivacy,
    handleFilteredRefreshUsageWindow,
    handleBatchTestRunningChange,
    openFilteredBatchTestModal,
  }
}
