/**
 * Admin Accounts API endpoints
 * Handles AI platform account management for administrators
 */

import { apiClient } from '../client'
import type {
  Account,
  AccountPlatform,
  AccountType,
  CreateAccountRequest,
  UpdateAccountRequest,
  PaginatedResponse,
  AccountUsageInfo,
  WindowStats,
  ClaudeModel,
  AccountUsageStatsResponse,
  TempUnschedulableStatus,
  AdminDataPayload,
  AdminDataImportResult,
  CheckMixedChannelRequest,
  CheckMixedChannelResponse
} from '@/types'

export interface AccountSortParams {
  sort_by?: string
  sort_order?: string
}

export interface AccountListFilters extends BulkAccountFilters {
  lite?: string
}

export type AccountListParams = AccountListFilters & AccountSortParams

type AccountRequestOptions = {
  signal?: AbortSignal
}

type AccountListWithEtagOptions = AccountRequestOptions & {
  etag?: string | null
}

const ACCOUNT_LIST_FILTER_KEYS = [
  'platform',
  'type',
  'status',
  'group',
  'group_exclude',
  'group_match',
  'search',
  'privacy_mode',
  'lite',
  'last_used_filter',
  'last_used_start_date',
  'last_used_end_date'
] as const

const ACCOUNT_SORT_KEYS = ['sort_by', 'sort_order'] as const

const pickAccountParams = <K extends readonly string[]>(
  source: Record<string, unknown> | undefined,
  keys: K
): Record<K[number], unknown> => {
  const result: Record<string, unknown> = {}
  if (!source) return result as Record<K[number], unknown>

  keys.forEach((key) => {
    const value = source[key]
    if (value !== undefined) {
      result[key] = value
    }
  })

  return result as Record<K[number], unknown>
}

export const splitAccountListParams = (params?: AccountListParams): {
  filters: AccountListFilters
  sort: AccountSortParams
} => ({
  filters: pickAccountParams(params as Record<string, unknown> | undefined, ACCOUNT_LIST_FILTER_KEYS) as AccountListFilters,
  sort: pickAccountParams(params as Record<string, unknown> | undefined, ACCOUNT_SORT_KEYS) as AccountSortParams
})

const buildAccountListRequestParams = (
  filters?: AccountListFilters,
  sort?: AccountSortParams
): AccountListParams => ({
  ...pickAccountParams(filters as Record<string, unknown> | undefined, ACCOUNT_LIST_FILTER_KEYS),
  ...pickAccountParams(sort as Record<string, unknown> | undefined, ACCOUNT_SORT_KEYS)
}) as AccountListParams

const hasAccountSortParams = (value: unknown): value is AccountSortParams => {
  return typeof value === 'object' && value !== null && ACCOUNT_SORT_KEYS.some((key) => key in value)
}

const isAccountRequestOptions = (value: unknown): value is AccountRequestOptions => {
  return typeof value === 'object' && value !== null && 'signal' in value
}

const isAccountListWithEtagOptions = (value: unknown): value is AccountListWithEtagOptions => {
  return typeof value === 'object' && value !== null && ('signal' in value || 'etag' in value)
}

const resolveListRequestArgs = (
  paramsOrFilters?: AccountListParams | AccountListFilters,
  sortOrOptions?: AccountSortParams | AccountRequestOptions,
  options?: AccountRequestOptions
) => {
  if (hasAccountSortParams(sortOrOptions)) {
    return {
      filters: paramsOrFilters,
      sort: sortOrOptions,
      options
    }
  }

  const { filters, sort } = splitAccountListParams(paramsOrFilters as AccountListParams | undefined)
  return {
    filters,
    sort,
    options: isAccountRequestOptions(sortOrOptions) ? sortOrOptions : options
  }
}

const resolveListWithEtagRequestArgs = (
  paramsOrFilters?: AccountListParams | AccountListFilters,
  sortOrOptions?: AccountSortParams | AccountListWithEtagOptions,
  options?: AccountListWithEtagOptions
) => {
  if (hasAccountSortParams(sortOrOptions)) {
    return {
      filters: paramsOrFilters,
      sort: sortOrOptions,
      options
    }
  }

  const { filters, sort } = splitAccountListParams(paramsOrFilters as AccountListParams | undefined)
  return {
    filters,
    sort,
    options: isAccountListWithEtagOptions(sortOrOptions) ? sortOrOptions : options
  }
}

/**
 * List all accounts with pagination
 * @param page - Page number (default: 1)
 * @param pageSize - Items per page (default: 20)
 * @param filters - Optional filters
 * @returns Paginated list of accounts
 */
export async function list(
  page?: number,
  pageSize?: number,
  params?: AccountListParams,
  options?: AccountRequestOptions
): Promise<PaginatedResponse<Account>>
export async function list(
  page?: number,
  pageSize?: number,
  filters?: AccountListFilters,
  sort?: AccountSortParams,
  options?: AccountRequestOptions
): Promise<PaginatedResponse<Account>>
export async function list(
  page: number = 1,
  pageSize: number = 20,
  paramsOrFilters?: AccountListParams | AccountListFilters,
  sortOrOptions?: AccountSortParams | AccountRequestOptions,
  options?: AccountRequestOptions
): Promise<PaginatedResponse<Account>> {
  const resolved = resolveListRequestArgs(paramsOrFilters, sortOrOptions, options)
  const requestParams = buildAccountListRequestParams(resolved.filters, resolved.sort)

  const { data } = await apiClient.get<PaginatedResponse<Account>>('/admin/accounts', {
    params: {
      page,
      page_size: pageSize,
      ...requestParams
    },
    signal: resolved.options?.signal
  })
  return data
}

export interface AccountListWithEtagResult {
  notModified: boolean
  etag: string | null
  data: PaginatedResponse<Account> | null
}

export async function listWithEtag(
  page?: number,
  pageSize?: number,
  params?: AccountListParams,
  options?: AccountListWithEtagOptions
): Promise<AccountListWithEtagResult>
export async function listWithEtag(
  page?: number,
  pageSize?: number,
  filters?: AccountListFilters,
  sort?: AccountSortParams,
  options?: AccountListWithEtagOptions
): Promise<AccountListWithEtagResult>
export async function listWithEtag(
  page: number = 1,
  pageSize: number = 20,
  paramsOrFilters?: AccountListParams | AccountListFilters,
  sortOrOptions?: AccountSortParams | AccountListWithEtagOptions,
  options?: AccountListWithEtagOptions
): Promise<AccountListWithEtagResult> {
  const resolved = resolveListWithEtagRequestArgs(paramsOrFilters, sortOrOptions, options)
  const requestParams = buildAccountListRequestParams(resolved.filters, resolved.sort)
  const headers: Record<string, string> = {}
  if (resolved.options?.etag) {
    headers['If-None-Match'] = resolved.options.etag
  }

  const response = await apiClient.get<PaginatedResponse<Account>>('/admin/accounts', {
    params: {
      page,
      page_size: pageSize,
      ...requestParams
    },
    headers,
    signal: resolved.options?.signal,
    validateStatus: (status) => (status >= 200 && status < 300) || status === 304
  })

  const etagHeader = typeof response.headers?.etag === 'string' ? response.headers.etag : null
  if (response.status === 304) {
    return {
      notModified: true,
      etag: etagHeader,
      data: null
    }
  }

  return {
    notModified: false,
    etag: etagHeader,
    data: response.data
  }
}

/**
 * Get account by ID
 * @param id - Account ID
 * @returns Account details
 */
export async function getById(id: number): Promise<Account> {
  const { data } = await apiClient.get<Account>(`/admin/accounts/${id}`)
  return data
}

/**
 * Create new account
 * @param accountData - Account data
 * @returns Created account
 */
export async function create(accountData: CreateAccountRequest): Promise<Account> {
  const { data } = await apiClient.post<Account>('/admin/accounts', accountData)
  return data
}

/**
 * Update account
 * @param id - Account ID
 * @param updates - Fields to update
 * @returns Updated account
 */
export async function update(id: number, updates: UpdateAccountRequest): Promise<Account> {
  const { data } = await apiClient.put<Account>(`/admin/accounts/${id}`, updates)
  return data
}

/**
 * Check mixed-channel risk for account-group binding.
 */
export async function checkMixedChannelRisk(
  payload: CheckMixedChannelRequest
): Promise<CheckMixedChannelResponse> {
  const { data } = await apiClient.post<CheckMixedChannelResponse>('/admin/accounts/check-mixed-channel', payload)
  return data
}

/**
 * Delete account
 * @param id - Account ID
 * @returns Success confirmation
 */
export async function deleteAccount(id: number): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(`/admin/accounts/${id}`)
  return data
}

/**
 * Toggle account status
 * @param id - Account ID
 * @param status - New status
 * @returns Updated account
 */
export async function toggleStatus(id: number, status: 'active' | 'inactive'): Promise<Account> {
  return update(id, { status })
}

/**
 * Test account connectivity
 * @param id - Account ID
 * @returns Test result
 */
export async function testAccount(id: number): Promise<{
  success: boolean
  message: string
  latency_ms?: number
}> {
  const { data } = await apiClient.post<{
    success: boolean
    message: string
    latency_ms?: number
  }>(`/admin/accounts/${id}/test`)
  return data
}

/**
 * Refresh account credentials
 * @param id - Account ID
 * @returns Updated account
 */
export async function refreshCredentials(id: number): Promise<Account> {
  const { data } = await apiClient.post<Account>(`/admin/accounts/${id}/refresh`)
  return data
}

/**
 * Get account usage statistics
 * @param id - Account ID
 * @param days - Number of days (default: 30)
 * @returns Account usage statistics with history, summary, and models
 */
export async function getStats(id: number, days: number = 30): Promise<AccountUsageStatsResponse> {
  const { data } = await apiClient.get<AccountUsageStatsResponse>(`/admin/accounts/${id}/stats`, {
    params: { days }
  })
  return data
}

/**
 * Clear account error
 * @param id - Account ID
 * @returns Updated account
 */
export async function clearError(id: number): Promise<Account> {
  const { data } = await apiClient.post<Account>(`/admin/accounts/${id}/clear-error`)
  return data
}

/**
 * Get account usage information (5h/7d window)
 * @param id - Account ID
 * @returns Account usage info
 */
export async function getUsage(
  id: number,
  source?: 'passive' | 'active',
  options?: {
    force?: boolean
  }
): Promise<AccountUsageInfo> {
  const params: Record<string, string | boolean> = {}
  if (source) {
    params.source = source
  }
  if (options?.force) {
    params.force = true
  }

  const { data } = await apiClient.get<AccountUsageInfo>(`/admin/accounts/${id}/usage`, {
    params: Object.keys(params).length > 0 ? params : undefined
  })
  return data
}

/**
 * Clear account rate limit status
 * @param id - Account ID
 * @returns Updated account
 */
export async function clearRateLimit(id: number): Promise<Account> {
  const { data } = await apiClient.post<Account>(
    `/admin/accounts/${id}/clear-rate-limit`
  )
  return data
}

/**
 * Recover account runtime state in one call
 * @param id - Account ID
 * @returns Updated account
 */
export async function recoverState(id: number): Promise<Account> {
  const { data } = await apiClient.post<Account>(`/admin/accounts/${id}/recover-state`)
  return data
}

/**
 * Reset account quota usage
 * @param id - Account ID
 * @returns Updated account
 */
export async function resetAccountQuota(id: number): Promise<Account> {
  const { data } = await apiClient.post<Account>(
    `/admin/accounts/${id}/reset-quota`
  )
  return data
}

/**
 * Get temporary unschedulable status
 * @param id - Account ID
 * @returns Status with detail state if active
 */
export async function getTempUnschedulableStatus(id: number): Promise<TempUnschedulableStatus> {
  const { data } = await apiClient.get<TempUnschedulableStatus>(
    `/admin/accounts/${id}/temp-unschedulable`
  )
  return data
}

/**
 * Reset temporary unschedulable status
 * @param id - Account ID
 * @returns Success confirmation
 */
export async function resetTempUnschedulable(id: number): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(
    `/admin/accounts/${id}/temp-unschedulable`
  )
  return data
}

/**
 * Generate OAuth authorization URL
 * @param endpoint - API endpoint path
 * @param config - Proxy configuration
 * @returns Auth URL and session ID
 */
export async function generateAuthUrl(
  endpoint: string,
  config: { proxy_id?: number }
): Promise<{ auth_url: string; session_id: string }> {
  const { data } = await apiClient.post<{ auth_url: string; session_id: string }>(endpoint, config)
  return data
}

/**
 * Exchange authorization code for tokens
 * @param endpoint - API endpoint path
 * @param exchangeData - Session ID, code, and optional proxy config
 * @returns Token information
 */
export async function exchangeCode(
  endpoint: string,
  exchangeData: { session_id: string; code: string; state?: string; proxy_id?: number }
): Promise<Record<string, unknown>> {
  const { data } = await apiClient.post<Record<string, unknown>>(endpoint, exchangeData)
  return data
}

/**
 * Batch create accounts
 * @param accounts - Array of account data
 * @returns Results of batch creation
 */
export async function batchCreate(accounts: CreateAccountRequest[]): Promise<{
  success: number
  failed: number
  results: Array<{ success: boolean; account?: Account; error?: string }>
}> {
  const { data } = await apiClient.post<{
    success: number
    failed: number
    results: Array<{ success: boolean; account?: Account; error?: string }>
  }>('/admin/accounts/batch', { accounts })
  return data
}

export interface BulkAccountFilters {
  platform?: string
  type?: string
  status?: string
  group?: string
  group_exclude?: string
  group_match?: string
  search?: string
  privacy_mode?: string
  last_used_filter?: string
  last_used_start_date?: string
  last_used_end_date?: string
}

export interface BulkAccountTargetPreview {
  count: number
  platforms: AccountPlatform[]
  types: AccountType[]
}

export interface BulkAccountTargetResolution {
  count: number
  account_ids: number[]
}

export async function previewBulkUpdateTargets(filters: BulkAccountFilters): Promise<BulkAccountTargetPreview> {
  const { data } = await apiClient.post<BulkAccountTargetPreview>('/admin/accounts/bulk-update/preview', filters)
  return data
}

export async function resolveBulkUpdateTargets(filters: BulkAccountFilters): Promise<BulkAccountTargetResolution> {
  const { data } = await apiClient.post<BulkAccountTargetResolution>('/admin/accounts/bulk-update/resolve', filters)
  return data
}

/**
 * Batch update credentials fields for multiple accounts
 * @param request - Batch update request containing account IDs, field name, and value
 * @returns Results of batch update
 */
export async function batchUpdateCredentials(request: {
  account_ids: number[]
  field: string
  value: any
}): Promise<{
  success: number
  failed: number
  results: Array<{ account_id: number; success: boolean; error?: string }>
}> {
  const { data } = await apiClient.post<{
    success: number
    failed: number
    results: Array<{ account_id: number; success: boolean; error?: string }>
  }>('/admin/accounts/batch-update-credentials', request)
  return data
}

/**
 * Bulk update multiple accounts
 * @param accountIds - Array of account IDs
 * @param updates - Fields to update
 * @returns Success confirmation
 */
export async function bulkUpdate(
  target: number[] | { filters: BulkAccountFilters },
  updates: Record<string, unknown>
): Promise<{
  success: number
  failed: number
  success_ids?: number[]
  failed_ids?: number[]
  results: Array<{ account_id: number; success: boolean; error?: string }>
  }> {
  const payload = Array.isArray(target)
    ? { account_ids: target, ...updates }
    : { filters: target.filters, ...updates }

  const { data } = await apiClient.post<{
    success: number
    failed: number
    success_ids?: number[]
    failed_ids?: number[]
    results: Array<{ account_id: number; success: boolean; error?: string }>
  }>('/admin/accounts/bulk-update', payload)
  return data
}

/**
 * Get account today statistics
 * @param id - Account ID
 * @returns Today's stats (requests, tokens, cost)
 */
export async function getTodayStats(id: number): Promise<WindowStats> {
  const { data } = await apiClient.get<WindowStats>(`/admin/accounts/${id}/today-stats`)
  return data
}

export interface BatchTodayStatsResponse {
  stats: Record<string, WindowStats>
}

/**
 * 批量获取多个账号的今日统计
 * @param accountIds - 账号 ID 列表
 * @returns 以账号 ID（字符串）为键的统计映射
 */
export async function getBatchTodayStats(accountIds: number[]): Promise<BatchTodayStatsResponse> {
  const { data } = await apiClient.post<BatchTodayStatsResponse>('/admin/accounts/today-stats/batch', {
    account_ids: accountIds
  })
  return data
}

/**
 * Set account schedulable status
 * @param id - Account ID
 * @param schedulable - Whether the account should participate in scheduling
 * @returns Updated account
 */
export async function setSchedulable(id: number, schedulable: boolean): Promise<Account> {
  const { data } = await apiClient.post<Account>(`/admin/accounts/${id}/schedulable`, {
    schedulable
  })
  return data
}

/**
 * Get available models for an account
 * @param id - Account ID
 * @returns List of available models for this account
 */
export async function getAvailableModels(id: number): Promise<ClaudeModel[]> {
  const { data } = await apiClient.get<ClaudeModel[]>(`/admin/accounts/${id}/models`)
  return data
}

export interface CRSPreviewAccount {
  crs_account_id: string
  kind: string
  name: string
  platform: string
  type: string
}

export interface PreviewFromCRSResult {
  new_accounts: CRSPreviewAccount[]
  existing_accounts: CRSPreviewAccount[]
}

export async function previewFromCrs(params: {
  base_url: string
  username: string
  password: string
}): Promise<PreviewFromCRSResult> {
  const { data } = await apiClient.post<PreviewFromCRSResult>('/admin/accounts/sync/crs/preview', params)
  return data
}

export async function syncFromCrs(params: {
  base_url: string
  username: string
  password: string
  sync_proxies?: boolean
  selected_account_ids?: string[]
}): Promise<{
  created: number
  updated: number
  skipped: number
  failed: number
  items: Array<{
    crs_account_id: string
    kind: string
    name: string
    action: string
    error?: string
  }>
}> {
  const { data } = await apiClient.post<{
    created: number
    updated: number
    skipped: number
    failed: number
    items: Array<{
      crs_account_id: string
      kind: string
      name: string
      action: string
      error?: string
    }>
  }>('/admin/accounts/sync/crs', params)
  return data
}

export interface AccountExportOptions {
  ids?: number[]
  filters?: BulkAccountFilters
  sort?: AccountSortParams
  includeProxies?: boolean
}

const buildAccountExportParams = (options?: AccountExportOptions): Record<string, string> => {
  const params: Record<string, string> = {}

  if (options?.ids && options.ids.length > 0) {
    params.ids = options.ids.join(',')
  } else {
    const filters = pickAccountParams(options?.filters as Record<string, unknown> | undefined, [
      'platform',
      'type',
      'status',
      'group',
      'group_exclude',
      'group_match',
      'privacy_mode',
      'search',
      'last_used_filter',
      'last_used_start_date',
      'last_used_end_date'
    ] as const)
    const sort = pickAccountParams(options?.sort as Record<string, unknown> | undefined, ACCOUNT_SORT_KEYS)

    Object.entries({ ...filters, ...sort }).forEach(([key, value]) => {
      if (typeof value === 'string' && value !== '') {
        params[key] = value
      }
    })
  }

  if (options?.includeProxies === false) {
    params.include_proxies = 'false'
  }

  return params
}

export async function exportData(options?: AccountExportOptions): Promise<AdminDataPayload> {
  const { data } = await apiClient.get<AdminDataPayload>('/admin/accounts/data', {
    params: buildAccountExportParams(options)
  })
  return data
}

export async function importData(payload: {
  data: AdminDataPayload
  skip_default_group_bind?: boolean
}): Promise<AdminDataImportResult> {
  const { data } = await apiClient.post<AdminDataImportResult>('/admin/accounts/data', {
    data: payload.data,
    skip_default_group_bind: payload.skip_default_group_bind
  })
  return data
}

/**
 * Get Antigravity default model mapping from backend
 * @returns Default model mapping (from -> to)
 */
export async function getAntigravityDefaultModelMapping(): Promise<Record<string, string>> {
  const { data } = await apiClient.get<Record<string, string>>(
    '/admin/accounts/antigravity/default-model-mapping'
  )
  return data
}

/**
 * Refresh OpenAI token using refresh token
 * @param refreshToken - The refresh token
 * @param proxyId - Optional proxy ID
 * @returns Token information including access_token, email, etc.
 */
export async function refreshOpenAIToken(
  refreshToken: string,
  proxyId?: number | null,
  endpoint: string = '/admin/openai/refresh-token',
  clientId?: string
): Promise<Record<string, unknown>> {
  const payload: { refresh_token: string; proxy_id?: number; client_id?: string } = {
    refresh_token: refreshToken
  }
  if (proxyId) {
    payload.proxy_id = proxyId
  }
  if (clientId) {
    payload.client_id = clientId
  }
  const { data } = await apiClient.post<Record<string, unknown>>(endpoint, payload)
  return data
}

/**
 * Batch operation result type
 */
export interface BatchOperationResult {
  total: number
  success: number
  failed: number
  errors?: Array<{ account_id: number; error: string }>
  warnings?: Array<{ account_id: number; warning: string }>
}

/**
 * Batch clear account errors
 * @param accountIds - Array of account IDs
 * @returns Batch operation result
 */
export async function batchClearError(accountIds: number[]): Promise<BatchOperationResult> {
  const { data } = await apiClient.post<BatchOperationResult>('/admin/accounts/batch-clear-error', {
    account_ids: accountIds
  })
  return data
}

/**
 * Batch refresh account credentials
 * @param accountIds - Array of account IDs
 * @returns Batch operation result
 */
export async function batchRefresh(accountIds: number[]): Promise<BatchOperationResult> {
  const { data } = await apiClient.post<BatchOperationResult>('/admin/accounts/batch-refresh', {
    account_ids: accountIds,
  }, {
    timeout: 120000  // 120s timeout for large batch refreshes
  })
  return data
}

/**
 * Set privacy for an Antigravity OAuth account
 * @param id - Account ID
 * @returns Updated account
 */
export async function setPrivacy(id: number): Promise<Account> {
  const { data } = await apiClient.post<Account>(`/admin/accounts/${id}/set-privacy`)
  return data
}

export interface OpenAIPublicAddLinkAccountDefaults {
  proxy_id?: number | null
  concurrency?: number
  load_factor?: number | null
  priority?: number
  rate_multiplier?: number
  expires_at?: number | null
  auto_pause_on_expired?: boolean
  credentials?: Record<string, unknown>
  extra?: Record<string, unknown>
}

export interface OpenAIPublicAddLink {
  token: string
  name: string
  group_ids: number[]
  account_defaults?: OpenAIPublicAddLinkAccountDefaults | null
  url: string
  created_at: string
  updated_at: string
}

export async function listOpenAIPublicAddLinks(): Promise<OpenAIPublicAddLink[]> {
  const { data } = await apiClient.get<OpenAIPublicAddLink[]>('/admin/openai/public-links')
  return data
}

export async function createOpenAIPublicAddLink(payload: {
  name?: string
  group_ids: number[]
  account_defaults?: OpenAIPublicAddLinkAccountDefaults
}): Promise<OpenAIPublicAddLink> {
  const { data } = await apiClient.post<OpenAIPublicAddLink>('/admin/openai/public-links', payload)
  return data
}

export async function updateOpenAIPublicAddLink(token: string, payload: {
  name?: string
  group_ids: number[]
  account_defaults?: OpenAIPublicAddLinkAccountDefaults
}): Promise<OpenAIPublicAddLink> {
  const { data } = await apiClient.put<OpenAIPublicAddLink>(`/admin/openai/public-links/${token}`, payload)
  return data
}

export async function rotateOpenAIPublicAddLink(token: string): Promise<OpenAIPublicAddLink> {
  const { data } = await apiClient.post<OpenAIPublicAddLink>(`/admin/openai/public-links/${token}/rotate`)
  return data
}

export async function deleteOpenAIPublicAddLink(token: string): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(`/admin/openai/public-links/${token}`)
  return data
}

export const accountsAPI = {
  list,
  listWithEtag,
  getById,
  create,
  update,
  checkMixedChannelRisk,
  previewBulkUpdateTargets,
  resolveBulkUpdateTargets,
  delete: deleteAccount,
  toggleStatus,
  testAccount,
  refreshCredentials,
  getStats,
  clearError,
  getUsage,
  getTodayStats,
  getBatchTodayStats,
  clearRateLimit,
  recoverState,
  resetAccountQuota,
  getTempUnschedulableStatus,
  resetTempUnschedulable,
  setSchedulable,
  getAvailableModels,
  generateAuthUrl,
  exchangeCode,
  refreshOpenAIToken,
  batchCreate,
  batchUpdateCredentials,
  bulkUpdate,
  previewFromCrs,
  syncFromCrs,
  exportData,
  importData,
  getAntigravityDefaultModelMapping,
  batchClearError,
  batchRefresh,
  setPrivacy,
  listOpenAIPublicAddLinks,
  createOpenAIPublicAddLink,
  updateOpenAIPublicAddLink,
  rotateOpenAIPublicAddLink,
  deleteOpenAIPublicAddLink
}

export default accountsAPI
