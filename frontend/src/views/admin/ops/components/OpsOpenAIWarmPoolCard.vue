<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import EmptyState from '@/components/common/EmptyState.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { useAppStore } from '@/stores/app'
import {
  opsAPI,
  type OpsOpenAIWarmPoolAccount,
  type OpsOpenAIWarmPoolBucket,
  type OpsOpenAIWarmPoolGroupCoverage,
  type OpsOpenAIWarmPoolStatsResponse,
  type OpsSystemLog
} from '@/api/admin/ops'
import { formatDateTime } from '../utils/opsFormatters'

interface Props {
  groupIdFilter?: number | null
  refreshToken: number
}

const props = withDefaults(defineProps<Props>(), {
  groupIdFilter: null
})

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const logsLoading = ref(false)
const triggeringGlobalRefill = ref(false)
const errorMessage = ref('')
const logErrorMessage = ref('')
const response = ref<OpsOpenAIWarmPoolStatsResponse | null>(null)
const logRows = ref<OpsSystemLog[]>([])
const readyListVisible = ref(false)
const readyListLoading = ref(false)
const readyListLoadingMore = ref(false)
const readyListErrorMessage = ref('')
const readyListPage = ref(0)
const readyListHasMore = ref(false)
const readyAccountRows = ref<OpsOpenAIWarmPoolAccount[]>([])

const summary = computed(() => response.value?.summary ?? {
  tracked_account_count: 0,
  bucket_ready_account_count: 0,
  global_ready_account_count: 0,
  active_group_count: 0,
  global_target_per_active_group: 0,
  global_refill_per_active_group: 0,
  groups_below_target_count: 0,
  groups_below_refill_count: 0,
  probing_account_count: 0,
  cooling_account_count: 0,
  network_error_pool_count: 0,
  take_count: 0,
  network_error_pool_full: false,
  last_bucket_maintenance_at: null,
  last_global_maintenance_at: null,
})

const bucketRows = computed(() => response.value?.buckets ?? [])
const accountRows = computed(() => response.value?.accounts ?? [])
const globalCoverageRows = computed<OpsOpenAIWarmPoolGroupCoverage[]>(() => response.value?.global_coverages ?? [])
const globalCoverageMap = computed(() => new Map(globalCoverageRows.value.map((row) => [row.group_id, row] as const)))
const warmPoolLogComponent = 'service.openai_warm_pool'
const READY_LIST_PAGE_SIZE = 20

const showAccountTable = computed(() => typeof props.groupIdFilter === 'number' && props.groupIdFilter > 0)
const realtimeDisabled = computed(() => response.value !== null && !response.value.enabled)
const warmPoolDisabled = computed(() => response.value !== null && response.value.enabled && !response.value.warm_pool_enabled)
const readerUnavailable = computed(() => response.value !== null && response.value.enabled && response.value.warm_pool_enabled && !response.value.reader_ready)
const bootstrappingHintVisible = computed(() => (
  response.value !== null
  && response.value.enabled
  && response.value.warm_pool_enabled
  && response.value.reader_ready
  && response.value.bootstrapping === true
))
const filteredLogRows = computed(() => logRows.value.slice(0, 12))
const readyListShowGroupColumn = computed(() => !showAccountTable.value)
const canTriggerGlobalRefill = computed(() => {
  if (loading.value || triggeringGlobalRefill.value) return false
  if (realtimeDisabled.value || warmPoolDisabled.value || readerUnavailable.value) return false
  return summary.value.active_group_count > 0
})
const isRefreshing = computed(() => loading.value || logsLoading.value)

function formatTimestamp(value?: string | null): string {
  if (!value) return '-'
  const formatted = formatDateTime(value)
  return formatted || '-'
}

function formatBucketName(row: OpsOpenAIWarmPoolBucket): string {
  if (row.group_name?.trim()) return row.group_name.trim()
  if (row.group_id > 0) return `#${row.group_id}`
  return t('admin.ops.openaiWarmPool.sharedBucket')
}

function stateClass(state: string): string {
  switch (state) {
    case 'ready':
      return 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
    case 'probing':
      return 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400'
    case 'cooling':
      return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400'
    case 'network_error':
      return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400'
    default:
      return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300'
  }
}

function formatState(state: string): string {
  return t(`admin.ops.openaiWarmPool.state.${state || 'idle'}`)
}

function getBucketCoverageRow(groupId: number): OpsOpenAIWarmPoolGroupCoverage | null {
  return globalCoverageMap.value.get(groupId) ?? null
}

function formatBucketReadyCell(row: OpsOpenAIWarmPoolBucket): string {
  const coverage = getBucketCoverageRow(row.group_id)
  if (!coverage) return String(row.bucket_ready_accounts)
  return `${row.bucket_ready_accounts}(${coverage.coverage_count})`
}

function bucketStatusKey(row: OpsOpenAIWarmPoolBucket): string {
  if (row.schedulable_accounts < row.bucket_refill_below) return 'all'
  if (row.bucket_ready_accounts < row.bucket_refill_below) {
    return row.probing_accounts > 0 ? 'refilling' : 'pendingRefill'
  }
  return 'normal'
}

function poolStatusClass(status: string): string {
  switch (status) {
    case 'normal':
      return 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
    case 'pendingRefill':
    case 'refilling':
      return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
    case 'all':
      return 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'
    default:
      return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300'
  }
}

function bucketStatusClass(row: OpsOpenAIWarmPoolBucket): string {
  return poolStatusClass(bucketStatusKey(row))
}

function formatBucketStatus(row: OpsOpenAIWarmPoolBucket): string {
  return t(`admin.ops.openaiWarmPool.bucketStatus.${bucketStatusKey(row)}`)
}

const globalStatusKey = computed(() => {
  if (summary.value.active_group_count <= 0) return ''
  if (summary.value.groups_below_refill_count <= 0) return 'normal'
  const belowRefillGroupIds = globalCoverageRows.value
    .filter((row) => row.coverage_count < row.refill_below)
    .map((row) => row.group_id)
  if (belowRefillGroupIds.length === 0) return 'normal'
  const capacityExhausted = belowRefillGroupIds.every((groupId) => {
    const bucket = bucketRows.value.find((row) => row.group_id === groupId)
    return !!bucket && bucket.schedulable_accounts < bucket.bucket_refill_below
  })
  if (capacityExhausted) return 'all'
  if (summary.value.probing_account_count > 0 || response.value?.bootstrapping === true) return 'refilling'
  return 'pendingRefill'
})

const globalStatusClass = computed(() => poolStatusClass(globalStatusKey.value))

function formatGlobalStatus(status: string): string {
  return status ? t(`admin.ops.openaiWarmPool.globalStatus.${status}`) : ''
}

function formatAccountGroups(row: OpsOpenAIWarmPoolAccount): string[] {
  const groups = Array.isArray(row.groups) ? row.groups : []
  if (groups.length === 0) return [t('admin.ops.openaiWarmPool.sharedBucket')]
  return groups.map((group) => {
    if (group.group_name?.trim()) return group.group_name.trim()
    if (group.group_id > 0) return `#${group.group_id}`
    return t('admin.ops.openaiWarmPool.sharedBucket')
  })
}

function getExtraString(extra: Record<string, any> | undefined, key: string): string {
  if (!extra) return ''
  const value = extra[key]
  if (value == null) return ''
  if (typeof value === 'string') return value.trim()
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  return ''
}

function buildWarmPoolLogQuery() {
  const query: Record<string, any> = {
    page: 1,
    page_size: 12,
    time_range: '24h',
    component: warmPoolLogComponent,
    platform: 'openai',
  }
  if (typeof props.groupIdFilter === 'number' && props.groupIdFilter > 0) {
    query.q = `"group_id":${props.groupIdFilter}`
  }
  return query
}

function logLevelClass(level: string): string {
  const normalized = String(level || '').toLowerCase()
  if (normalized === 'error' || normalized === 'fatal') return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
  if (normalized === 'warn' || normalized === 'warning') return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
  return 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'
}

const warmPoolLegacyMessageMap: Record<string, string> = {
  'warm pool startup is waiting for default proxy probe to pass': '预热池启动正在等待默认代理探测通过',
  'warm pool startup gate passed and prewarming is now enabled': '预热池启动门禁已通过，现已启用预热',
  'warm pool global maintenance failed to load schedulable accounts': '预热池全局维护加载可调度账号失败',
  'warm pool bucket maintenance failed to load schedulable accounts': '预热池分组池维护加载可调度账号失败',
  'warm pool bucket refill skipped because feature disabled': '预热池分组池补池已跳过，功能当前已关闭',
  'warm pool bucket refill skipped because bucket is cooling down': '预热池分组池补池已跳过，分组池仍在冷却中',
  'warm pool bucket refill failed to load schedulable accounts': '预热池分组池补池加载可调度账号失败',
  'warm pool bucket refill skipped because no schedulable accounts were found': '预热池分组池补池已跳过，未找到可调度账号',
  'warm pool bucket refill completed': '预热池分组池补池完成',
  'warm pool initial bucket fill could not continue filling global pool': '预热池初始化分组池补满后无法继续补全局池',
  'warm pool global refill failed to load schedulable accounts': '预热池全局池补池加载可调度账号失败',
  'warm pool global refill completed': '预热池全局池补池完成',
  'warm pool probe batch started': '预热池探测批次已开始',
  'warm pool probe batch reached target and stopped launching new probes': '预热池探测批次已达到目标，停止继续发起探测',
  'warm pool probe batch completed': '预热池探测批次完成',
  'warm pool ready account expired and is being rechecked before refresh': '预热池就绪账号已过期，正在刷新前复检',
  'warm pool probe failed to acquire account slot': '预热池探测获取账号槽位失败',
  'warm pool probe hit network timeout and moved account into NetworkError': '预热池探测遇到网络超时，账号已移入网络异常池',
  'warm pool probe hit a network-class usage refresh error and moved account into NetworkError': '预热池探测遇到网络类用量刷新错误，账号已移入网络异常池',
  'warm pool probe finished after probe timeout and moved account into NetworkError': '预热池探测在超时后返回，账号已移入网络异常池',
  'warm pool probe returned empty usage payload': '预热池探测返回了空的用量结果',
  'warm pool probe failed because account needs reauthorization': '预热池探测失败：账号需要重新授权',
  'warm pool probe failed because upstream marked account forbidden': '预热池探测失败：上游将账号标记为禁止使用',
  'warm pool probe observed network_error usage response and moved account into NetworkError': '预热池探测检测到 network_error 用量响应，账号已移入网络异常池',
  'warm pool probe failed because usage response reported an error': '预热池探测失败：用量响应返回错误',
  'warm pool probe failed to reload account state': '预热池探测刷新账号状态失败',
  'warm pool probe failed because refreshed account is no longer schedulable': '预热池探测失败：刷新后的账号已不可调度',
  'warm pool expired ready account passed recheck and refreshed global deadline': '预热池过期就绪账号已通过复检，并刷新了全局有效期',
  'warm pool probe succeeded and account entered Global ready state': '预热池探测成功，账号已进入全局池就绪状态'
}

function getWarmPoolLogAccountName(row: OpsSystemLog): string {
  return getExtraString(row.extra, 'account_name')
}

function formatWarmPoolLogAccountLabel(row: OpsSystemLog): string {
  const accountName = getWarmPoolLogAccountName(row)
  const accountId = typeof row.account_id === 'number' && row.account_id > 0 ? row.account_id : null
  if (accountName && accountId) return `${accountName} (#${accountId})`
  if (accountName) return accountName
  if (accountId) return `#${accountId}`
  return ''
}

function formatWarmPoolLogMessage(row: OpsSystemLog): string {
  const parts: string[] = []
  const rawMessage = String(row.message || '').trim()
  const message = warmPoolLegacyMessageMap[rawMessage] || rawMessage
  if (message) parts.push(message)
  const extra = row.extra || {}
  const requestedModel = getExtraString(extra, 'requested_model')
  const error = getExtraString(extra, 'error')
  const errorCode = getExtraString(extra, 'error_code')
  const currentReady = getExtraString(extra, 'current_ready')
  const finalReady = getExtraString(extra, 'final_ready')
  const accountName = getWarmPoolLogAccountName(row)
  const accountId = typeof row.account_id === 'number' && row.account_id > 0 ? row.account_id : null
  const accountLabel = formatWarmPoolLogAccountLabel(row)
  const messageHasAccount = !!(
    accountLabel && (
      (accountName && message.includes(accountName)) ||
      (accountId && message.includes(`#${accountId}`))
    )
  )
  if (requestedModel) parts.push(`模型=${requestedModel}`)
  if (accountLabel && !messageHasAccount) parts.push(`账号=${accountLabel}`)
  if (currentReady) parts.push(`当前就绪数=${currentReady}`)
  if (finalReady) parts.push(`最终就绪数=${finalReady}`)
  if (errorCode) parts.push(`代码=${errorCode}`)
  if (error) parts.push(`错误=${error}`)
  return parts.join(' · ')
}

function resetReadyListState() {
  readyAccountRows.value = []
  readyListPage.value = 0
  readyListHasMore.value = false
  readyListLoadingMore.value = false
  readyListErrorMessage.value = ''
}

async function loadReadyAccounts(options: { append?: boolean } = {}) {
  const append = options.append === true
  if (append) {
    if (readyListLoading.value || readyListLoadingMore.value || !readyListVisible.value || !readyListHasMore.value) return
  } else if (readyListLoading.value) {
    return
  }

  const nextPage = append ? readyListPage.value + 1 : 1
  const loadingState = append ? readyListLoadingMore : readyListLoading
  if (!append) {
    resetReadyListState()
  }

  loadingState.value = true
  readyListErrorMessage.value = ''
  try {
    const result = await opsAPI.getOpenAIWarmPoolStats(props.groupIdFilter, {
      includeAccount: true,
      accountsOnly: true,
      accountState: 'ready',
      page: nextPage,
      page_size: READY_LIST_PAGE_SIZE
    })
    const incomingRows = result.accounts || []
    readyAccountRows.value = append ? [...readyAccountRows.value, ...incomingRows] : incomingRows
    readyListPage.value = typeof result.page === 'number' && result.page > 0 ? result.page : nextPage
    if (typeof result.pages === 'number' && result.pages > 0) {
      readyListHasMore.value = readyListPage.value < result.pages
    } else {
      readyListHasMore.value = false
    }
  } catch (error: any) {
    const detail =
      error?.response?.data?.detail ||
      error?.message ||
      t('admin.ops.openaiWarmPool.readyListLoadFailed')
    if (append) {
      appStore.showError(detail)
    } else {
      readyAccountRows.value = []
      readyListPage.value = 0
      readyListHasMore.value = false
      readyListErrorMessage.value = detail
    }
  } finally {
    loadingState.value = false
  }
}

async function openReadyList() {
  readyListVisible.value = true
  await loadReadyAccounts()
}

async function loadMoreReadyAccounts() {
  await loadReadyAccounts({ append: true })
}

function closeReadyList() {
  readyListVisible.value = false
}

async function triggerGlobalRefill() {
  if (!canTriggerGlobalRefill.value) return
  triggeringGlobalRefill.value = true
  try {
    await opsAPI.triggerOpenAIWarmPoolGlobalRefill()
    await loadData()
    if (readyListVisible.value) {
      await loadReadyAccounts()
    }
    appStore.showSuccess(t('admin.ops.openaiWarmPool.triggerGlobalRefillSuccess'))
  } catch (error: any) {
    const detail = error?.response?.data?.detail || error?.message || t('admin.ops.openaiWarmPool.triggerGlobalRefillFailed')
    appStore.showError(detail)
  } finally {
    triggeringGlobalRefill.value = false
  }
}

async function loadStats() {
  loading.value = true
  errorMessage.value = ''
  try {
    response.value = await opsAPI.getOpenAIWarmPoolStats(props.groupIdFilter)
  } catch (error: any) {
    console.error('[OpsOpenAIWarmPoolCard] Failed to load data', error)
    response.value = null
    errorMessage.value = error?.response?.data?.detail || error?.message || t('admin.ops.openaiWarmPool.loadFailed')
  } finally {
    loading.value = false
  }
}

async function loadLogs() {
  logsLoading.value = true
  logErrorMessage.value = ''
  try {
    const result = await opsAPI.listSystemLogs(buildWarmPoolLogQuery())
    logRows.value = result.items || []
  } catch (error: any) {
    console.error('[OpsOpenAIWarmPoolCard] Failed to load logs', error)
    logErrorMessage.value = error?.response?.data?.detail || error?.message || t('admin.ops.openaiWarmPool.logLoadFailed')
  } finally {
    logsLoading.value = false
  }
}

async function loadData() {
  await Promise.allSettled([
    loadStats(),
    loadLogs()
  ])
}

watch(
  [
    () => props.groupIdFilter,
    () => props.refreshToken
  ],
  ([groupId], [previousGroupId]) => {
    void loadData()
    if (readyListVisible.value && groupId !== previousGroupId) {
      void loadReadyAccounts()
    }
  },
  { immediate: true }
)
</script>

<template>
  <section v-if="!warmPoolDisabled" class="card p-4 md:p-5">
    <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
      <div>
        <h3 class="text-sm font-bold text-gray-900 dark:text-white">
          {{ t('admin.ops.openaiWarmPool.title') }}
        </h3>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.ops.openaiWarmPool.description') }}
        </p>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <button
          type="button"
          class="inline-flex items-center rounded-lg bg-primary-100 px-2 py-1 text-[11px] font-semibold text-primary-700 transition-colors hover:bg-primary-200 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-primary-900/30 dark:text-primary-300 dark:hover:bg-primary-900/40"
          :disabled="!canTriggerGlobalRefill"
          @click="triggerGlobalRefill"
        >
          {{ triggeringGlobalRefill ? t('admin.ops.openaiWarmPool.triggerGlobalRefillLoading') : t('admin.ops.openaiWarmPool.triggerGlobalRefill') }}
        </button>
        <button
          class="flex items-center gap-1 rounded-lg bg-gray-100 px-2 py-1 text-[11px] font-semibold text-gray-700 transition-colors hover:bg-gray-200 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-dark-700 dark:text-gray-300 dark:hover:bg-dark-600"
          :disabled="isRefreshing"
          :title="t('common.refresh')"
          @click="loadData"
        >
          <svg class="h-3 w-3" :class="{ 'animate-spin': isRefreshing }" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
          </svg>
          {{ t('common.refresh') }}
        </button>
      </div>
    </div>

    <div v-if="errorMessage" class="mb-4 rounded-lg bg-red-50 px-3 py-2 text-xs text-red-600 dark:bg-red-900/20 dark:text-red-400">
      {{ errorMessage }}
    </div>

    <div v-else-if="loading && !response" class="py-8 text-center text-sm text-gray-500 dark:text-gray-400">
      {{ t('admin.ops.loadingText') }}
    </div>

    <div v-else-if="realtimeDisabled" class="rounded-xl border border-dashed border-gray-200 px-4 py-8 text-center text-sm text-gray-500 dark:border-dark-700 dark:text-gray-400">
      {{ t('admin.ops.openaiWarmPool.disabledHint') }}
    </div>

    <div v-else class="space-y-4">
      <div v-if="readerUnavailable" class="rounded-lg bg-amber-50 px-3 py-2 text-xs text-amber-700 dark:bg-amber-900/20 dark:text-amber-400">
        {{ t('admin.ops.openaiWarmPool.readerUnavailableHint') }}
      </div>
      <div v-else-if="bootstrappingHintVisible" class="rounded-lg bg-amber-50 px-3 py-2 text-xs text-amber-700 dark:bg-amber-900/20 dark:text-amber-400">
        {{ t('admin.ops.openaiWarmPool.bootstrappingHint') }}
      </div>

      <div class="grid grid-cols-2 gap-3 md:grid-cols-3 2xl:grid-cols-6">
        <div class="rounded-xl bg-gray-50 p-3 dark:bg-dark-900">
          <div class="text-[11px] font-medium text-gray-500 dark:text-gray-400">{{ t('admin.ops.openaiWarmPool.summary.bucketReadyAccounts') }}</div>
          <div class="mt-1 text-xl font-bold text-gray-900 dark:text-white">{{ summary.bucket_ready_account_count }}</div>
        </div>
        <div class="rounded-xl bg-gray-50 p-3 dark:bg-dark-900">
          <div class="text-[11px] font-medium text-gray-500 dark:text-gray-400">{{ t('admin.ops.openaiWarmPool.summary.globalReadyAccounts') }}</div>
          <div class="mt-1 flex items-center gap-2">
            <div class="text-xl font-bold text-green-600 dark:text-green-400">{{ summary.global_ready_account_count }}</div>
            <span
              v-if="globalStatusKey"
              class="inline-flex rounded-full px-2 py-0.5 text-[11px] font-semibold"
              :class="globalStatusClass"
            >
              {{ formatGlobalStatus(globalStatusKey) }}
            </span>
          </div>
          <div class="mt-2 flex flex-wrap gap-2">
            <button
              type="button"
              class="inline-flex items-center rounded-lg bg-green-100 px-2 py-1 text-[11px] font-semibold text-green-700 transition-colors hover:bg-green-200 dark:bg-green-900/30 dark:text-green-300 dark:hover:bg-green-900/40"
              @click="openReadyList"
            >
              {{ t('admin.ops.openaiWarmPool.viewReadyList') }}
            </button>
          </div>
        </div>
        <div class="rounded-xl bg-gray-50 p-3 dark:bg-dark-900">
          <div class="text-[11px] font-medium text-gray-500 dark:text-gray-400">{{ t('admin.ops.openaiWarmPool.summary.takeCount') }}</div>
          <div class="mt-1 text-xl font-bold text-primary-600 dark:text-primary-400">{{ summary.take_count }}</div>
        </div>
        <div class="rounded-xl bg-gray-50 p-3 dark:bg-dark-900">
          <div class="text-[11px] font-medium text-gray-500 dark:text-gray-400">{{ t('admin.ops.openaiWarmPool.summary.probingAccounts') }}</div>
          <div class="mt-1 text-xl font-bold text-blue-600 dark:text-blue-400">{{ summary.probing_account_count }}</div>
        </div>
        <div class="rounded-xl bg-gray-50 p-3 dark:bg-dark-900">
          <div class="text-[11px] font-medium text-gray-500 dark:text-gray-400">{{ t('admin.ops.openaiWarmPool.summary.coolingAccounts') }}</div>
          <div class="mt-1 text-xl font-bold text-amber-600 dark:text-amber-400">{{ summary.cooling_account_count }}</div>
        </div>
        <div class="rounded-xl bg-gray-50 p-3 dark:bg-dark-900">
          <div class="text-[11px] font-medium text-gray-500 dark:text-gray-400">{{ t('admin.ops.openaiWarmPool.summary.networkErrorAccounts') }}</div>
          <div class="mt-1 text-xl font-bold text-red-600 dark:text-red-400">{{ summary.network_error_pool_count }}</div>
          <div
            v-if="summary.network_error_pool_full"
            class="mt-2 rounded-lg bg-red-100 px-2 py-1 text-[11px] font-semibold text-red-700 dark:bg-red-900/30 dark:text-red-300"
          >
            {{ t('admin.ops.openaiWarmPool.summary.networkErrorPoolFull') }}
          </div>
        </div>
      </div>

      <div class="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-gray-500 dark:text-gray-400">
        <span>{{ t('admin.ops.openaiWarmPool.summary.trackedAccounts', { count: summary.tracked_account_count }) }}</span>
        <span>{{ t('admin.ops.openaiWarmPool.summary.activeGroups', { count: summary.active_group_count }) }}</span>
        <span>{{ t('admin.ops.openaiWarmPool.summary.targetPerGroup', { count: summary.global_target_per_active_group }) }}</span>
        <span>{{ t('admin.ops.openaiWarmPool.summary.refillPerGroup', { count: summary.global_refill_per_active_group }) }}</span>
        <span>{{ t('admin.ops.openaiWarmPool.summary.groupsBelowTarget', { count: summary.groups_below_target_count }) }}</span>
        <span>{{ t('admin.ops.openaiWarmPool.summary.groupsBelowRefill', { count: summary.groups_below_refill_count }) }}</span>
        <span>{{ t('admin.ops.openaiWarmPool.summary.lastBucketMaintenance', { time: formatTimestamp(summary.last_bucket_maintenance_at) }) }}</span>
        <span>{{ t('admin.ops.openaiWarmPool.summary.lastGlobalMaintenance', { time: formatTimestamp(summary.last_global_maintenance_at) }) }}</span>
      </div>

      <div class="overflow-hidden rounded-xl border border-gray-200 dark:border-dark-700">
        <div class="border-b border-gray-200 bg-gray-50 px-3 py-2 text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-400">
          {{ t('admin.ops.openaiWarmPool.bucketTitle') }}
        </div>
        <EmptyState
          v-if="bucketRows.length === 0"
          class="py-6"
          :title="t('common.noData')"
          :description="t('admin.ops.openaiWarmPool.empty')"
        />
        <div v-else class="max-h-[320px] overflow-auto">
          <table class="min-w-full text-left text-xs md:text-sm">
            <thead class="sticky top-0 z-10 bg-white dark:bg-dark-800">
              <tr class="border-b border-gray-200 text-gray-500 dark:border-dark-700 dark:text-gray-400">
                <th class="px-3 py-2 font-semibold">{{ t('admin.ops.openaiWarmPool.table.group') }}</th>
                <th class="px-3 py-2 font-semibold">{{ t('admin.ops.openaiWarmPool.table.ready') }}</th>
                <th class="px-3 py-2 font-semibold whitespace-nowrap">{{ t('admin.ops.openaiWarmPool.table.takeCount') }}</th>
                <th class="px-3 py-2 font-semibold">{{ t('admin.ops.openaiWarmPool.table.schedulable') }}</th>
                <th class="px-3 py-2 font-semibold">{{ t('admin.ops.openaiWarmPool.table.probing') }}</th>
                <th class="px-3 py-2 font-semibold">{{ t('admin.ops.openaiWarmPool.table.cooling') }}</th>
                <th class="px-3 py-2 font-semibold">{{ t('admin.ops.openaiWarmPool.table.status') }}</th>
                <th class="px-3 py-2 font-semibold">{{ t('admin.ops.openaiWarmPool.table.lastAccess') }}</th>
                <th class="px-3 py-2 font-semibold">{{ t('admin.ops.openaiWarmPool.table.lastRefill') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="row in bucketRows"
                :key="`${row.group_id}-${row.last_access_at || ''}-${row.last_refill_at || ''}`"
                class="border-b border-gray-100 text-gray-700 last:border-b-0 dark:border-dark-800 dark:text-gray-200"
              >
                <td class="px-3 py-2 font-medium">{{ formatBucketName(row) }}</td>
                <td class="px-3 py-2 font-semibold text-green-600 dark:text-green-400">{{ formatBucketReadyCell(row) }}</td>
                <td class="px-3 py-2 whitespace-nowrap">{{ row.take_count }}</td>
                <td class="px-3 py-2">{{ row.schedulable_accounts }}</td>
                <td class="px-3 py-2 text-blue-600 dark:text-blue-400">{{ row.probing_accounts }}</td>
                <td class="px-3 py-2 text-amber-600 dark:text-amber-400">{{ row.cooling_accounts }}</td>
                <td class="px-3 py-2">
                  <span class="inline-flex rounded-full px-2 py-0.5 text-[11px] font-semibold" :class="bucketStatusClass(row)">
                    {{ formatBucketStatus(row) }}
                  </span>
                </td>
                <td class="px-3 py-2">{{ formatTimestamp(row.last_access_at) }}</td>
                <td class="px-3 py-2">{{ formatTimestamp(row.last_refill_at) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <div v-if="showAccountTable" class="overflow-hidden rounded-xl border border-gray-200 dark:border-dark-700">
        <div class="border-b border-gray-200 bg-gray-50 px-3 py-2 text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-400">
          {{ t('admin.ops.openaiWarmPool.accountTitle') }}
        </div>
        <EmptyState
          v-if="accountRows.length === 0"
          class="py-6"
          :title="t('common.noData')"
          :description="t('admin.ops.openaiWarmPool.accountEmpty')"
        />
        <div v-else class="max-h-[320px] overflow-auto">
          <table class="min-w-full text-left text-xs md:text-sm">
            <thead class="sticky top-0 z-10 bg-white dark:bg-dark-800">
              <tr class="border-b border-gray-200 text-gray-500 dark:border-dark-700 dark:text-gray-400">
                <th class="px-3 py-2 font-semibold">{{ t('admin.ops.openaiWarmPool.accountTable.name') }}</th>
                <th class="px-3 py-2 font-semibold">{{ t('admin.ops.openaiWarmPool.accountTable.state') }}</th>
                <th class="px-3 py-2 font-semibold">{{ t('admin.ops.openaiWarmPool.accountTable.priority') }}</th>
                <th class="px-3 py-2 font-semibold">{{ t('admin.ops.openaiWarmPool.accountTable.expiresAt') }}</th>
                <th class="px-3 py-2 font-semibold">{{ t('admin.ops.openaiWarmPool.accountTable.failUntil') }}</th>
                <th class="px-3 py-2 font-semibold">{{ t('admin.ops.openaiWarmPool.accountTable.networkErrorUntil') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="row in accountRows"
                :key="row.account_id"
                class="border-b border-gray-100 text-gray-700 last:border-b-0 dark:border-dark-800 dark:text-gray-200"
              >
                <td class="px-3 py-2">
                  <div class="font-medium">{{ row.account_name || `#${row.account_id}` }}</div>
                  <div class="mt-0.5 text-[11px] text-gray-500 dark:text-gray-400">ID {{ row.account_id }} · {{ row.concurrency }}</div>
                </td>
                <td class="px-3 py-2">
                  <span class="inline-flex rounded-full px-2 py-0.5 text-[11px] font-semibold" :class="stateClass(row.state)">
                    {{ formatState(row.state) }}
                  </span>
                </td>
                <td class="px-3 py-2">{{ row.priority }}</td>
                <td class="px-3 py-2">{{ formatTimestamp(row.expires_at) }}</td>
                <td class="px-3 py-2">{{ formatTimestamp(row.fail_until) }}</td>
                <td class="px-3 py-2">{{ formatTimestamp(row.network_error_until) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <div class="overflow-hidden rounded-xl border border-gray-200 dark:border-dark-700">
        <div class="border-b border-gray-200 bg-gray-50 px-3 py-2 text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-400">
          {{ t('admin.ops.openaiWarmPool.logTitle') }}
        </div>
        <div v-if="logErrorMessage" class="border-b border-red-100 bg-red-50 px-3 py-2 text-xs text-red-600 dark:border-red-900/20 dark:bg-red-900/20 dark:text-red-400">
          {{ logErrorMessage }}
        </div>
        <div v-if="logsLoading && filteredLogRows.length === 0" class="py-6 text-center text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.ops.loadingText') }}
        </div>
        <EmptyState
          v-else-if="!logErrorMessage && filteredLogRows.length === 0"
          class="py-6"
          :title="t('common.noData')"
          :description="t('admin.ops.openaiWarmPool.logEmpty')"
        />
        <div v-else-if="filteredLogRows.length > 0" class="max-h-[320px] divide-y divide-gray-100 overflow-auto dark:divide-dark-800">
          <div v-for="row in filteredLogRows" :key="row.id" class="space-y-2 px-3 py-3 text-xs md:text-sm">
            <div class="flex flex-wrap items-center gap-2 text-[11px]">
              <span class="text-gray-500 dark:text-gray-400">{{ formatTimestamp(row.created_at) }}</span>
              <span class="inline-flex rounded-full px-2 py-0.5 font-semibold" :class="logLevelClass(row.level)">{{ row.level }}</span>
              <span v-if="getExtraString(row.extra, 'event')" class="inline-flex rounded-full bg-gray-100 px-2 py-0.5 font-medium text-gray-700 dark:bg-dark-700 dark:text-gray-300">
                {{ getExtraString(row.extra, 'event') }}
              </span>
              <span v-if="getExtraString(row.extra, 'reason')" class="inline-flex rounded-full bg-gray-100 px-2 py-0.5 font-medium text-gray-700 dark:bg-dark-700 dark:text-gray-300">
                {{ getExtraString(row.extra, 'reason') }}
              </span>
              <span v-if="formatWarmPoolLogAccountLabel(row)" class="text-gray-500 dark:text-gray-400">账号={{ formatWarmPoolLogAccountLabel(row) }}</span>
              <span v-if="getExtraString(row.extra, 'group_id')" class="text-gray-500 dark:text-gray-400">分组={{ getExtraString(row.extra, 'group_id') }}</span>
            </div>
            <div class="break-words text-gray-800 dark:text-gray-100">
              {{ formatWarmPoolLogMessage(row) }}
            </div>
          </div>
        </div>
      </div>

    </div>
  </section>

  <BaseDialog
    :show="readyListVisible"
    :title="t('admin.ops.openaiWarmPool.readyListTitle')"
    width="extra-wide"
    :close-on-click-outside="true"
    @close="closeReadyList"
  >
    <div class="space-y-4">
      <div v-if="readyListErrorMessage" class="rounded-lg bg-red-50 px-3 py-2 text-xs text-red-600 dark:bg-red-900/20 dark:text-red-400">
        {{ readyListErrorMessage }}
      </div>

      <div v-else-if="readyListLoading" class="py-8 text-center text-sm text-gray-500 dark:text-gray-400">
        {{ t('admin.ops.loadingText') }}
      </div>

      <EmptyState
        v-else-if="readyAccountRows.length === 0"
        class="py-6"
        :title="t('common.noData')"
        :description="t('admin.ops.openaiWarmPool.readyListEmpty')"
      />

      <div v-else class="max-h-[60vh] overflow-auto rounded-xl border border-gray-200 dark:border-dark-700">
        <table class="min-w-full text-left text-xs md:text-sm">
          <thead class="sticky top-0 z-10 bg-white dark:bg-dark-800">
            <tr class="border-b border-gray-200 text-gray-500 dark:border-dark-700 dark:text-gray-400">
              <th class="px-3 py-2 font-semibold">{{ t('admin.ops.openaiWarmPool.accountTable.name') }}</th>
              <th v-if="readyListShowGroupColumn" class="px-3 py-2 font-semibold">{{ t('admin.ops.openaiWarmPool.readyListTable.groups') }}</th>
              <th class="px-3 py-2 font-semibold">{{ t('admin.ops.openaiWarmPool.accountTable.priority') }}</th>
              <th class="px-3 py-2 font-semibold">{{ t('admin.ops.openaiWarmPool.readyListTable.verifiedAt') }}</th>
              <th class="px-3 py-2 font-semibold">{{ t('admin.ops.openaiWarmPool.accountTable.expiresAt') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="row in readyAccountRows"
              :key="`ready-${row.account_id}`"
              class="border-b border-gray-100 text-gray-700 last:border-b-0 dark:border-dark-800 dark:text-gray-200"
            >
              <td class="px-3 py-2">
                <div class="font-medium">{{ row.account_name || `#${row.account_id}` }}</div>
                <div class="mt-0.5 text-[11px] text-gray-500 dark:text-gray-400">ID {{ row.account_id }} · {{ row.concurrency }}</div>
              </td>
              <td v-if="readyListShowGroupColumn" class="px-3 py-2">
                <div class="flex flex-wrap gap-1">
                  <span
                    v-for="groupName in formatAccountGroups(row)"
                    :key="`${row.account_id}-${groupName}`"
                    class="inline-flex rounded-full bg-gray-100 px-2 py-0.5 text-[11px] font-medium text-gray-700 dark:bg-dark-700 dark:text-gray-300"
                  >
                    {{ groupName }}
                  </span>
                </div>
              </td>
              <td class="px-3 py-2">{{ row.priority }}</td>
              <td class="px-3 py-2">{{ formatTimestamp(row.verified_at) }}</td>
              <td class="px-3 py-2">{{ formatTimestamp(row.expires_at) }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <div v-if="readyListHasMore && readyAccountRows.length > 0" class="flex justify-center pt-1">
        <button
          type="button"
          class="inline-flex items-center rounded-lg bg-green-100 px-3 py-1.5 text-[11px] font-semibold text-green-700 transition-colors hover:bg-green-200 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-green-900/30 dark:text-green-300 dark:hover:bg-green-900/40"
          :disabled="readyListLoadingMore"
          @click="loadMoreReadyAccounts"
        >
          {{ readyListLoadingMore ? t('admin.ops.loadingText') : t('admin.ops.openaiWarmPool.loadMoreReadyList') }}
        </button>
      </div>
    </div>
  </BaseDialog>
</template>
