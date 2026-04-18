import type { BulkAccountFilters } from '@/api/admin/accounts'
import type { Account } from '@/types'

export const ACCOUNT_UNGROUPED_GROUP_QUERY_VALUE = 'ungrouped'
export const ACCOUNT_PRIVACY_MODE_UNSET_QUERY_VALUE = '__unset__'

export const accountMatchesLastUsedFilter = (account: Account, filters: BulkAccountFilters) => {
  if (filters.last_used_filter === 'unused') {
    return !account.last_used_at
  }
  if (filters.last_used_filter === 'range') {
    if (!account.last_used_at || !filters.last_used_start_date || !filters.last_used_end_date) return false
    const lastUsedAt = new Date(account.last_used_at).getTime()
    const startAt = new Date(`${filters.last_used_start_date}T00:00:00`).getTime()
    const endExclusive = new Date(`${filters.last_used_end_date}T00:00:00`)
    endExclusive.setDate(endExclusive.getDate() + 1)
    const endAt = endExclusive.getTime()
    if (!Number.isFinite(lastUsedAt) || !Number.isFinite(startAt) || !Number.isFinite(endAt)) return false
    return lastUsedAt >= startAt && lastUsedAt < endAt
  }
  return true
}

export const hasFutureTimestamp = (value: string | null | undefined) => {
  if (!value) return false
  const ts = new Date(value).getTime()
  return Number.isFinite(ts) && ts > Date.now()
}

export const isAutoPausedExpiredAccount = (account: Account) => {
  if (!account.auto_pause_on_expired || !account.expires_at) return false
  return account.expires_at * 1000 <= Date.now()
}

export const isNormallyActiveAccount = (account: Account) => {
  return account.status === 'active'
    && account.schedulable
    && !hasFutureTimestamp(account.rate_limit_reset_at)
    && !hasFutureTimestamp(account.overload_until)
    && !hasFutureTimestamp(account.temp_unschedulable_until)
    && !isAutoPausedExpiredAccount(account)
}

export const parseGroupFilterIds = (value: string | undefined) => {
  if (!value) return []
  return Array.from(new Set(
    value
      .split(',')
      .map(item => Number(item.trim()))
      .filter(id => Number.isInteger(id) && id > 0)
  )).sort((left, right) => left - right)
}

export const getAccountGroupIds = (account: Account) => {
  const rawGroupIds = account.group_ids ?? account.groups?.map((group) => group.id) ?? []
  return Array.from(new Set(
    rawGroupIds
      .map(item => Number(item))
      .filter(id => Number.isInteger(id) && id > 0)
  )).sort((left, right) => left - right)
}

export const accountMatchesGroupFilters = (account: Account, filters: BulkAccountFilters) => {
  const groupIds = getAccountGroupIds(account)
  if (filters.group) {
    if (filters.group === ACCOUNT_UNGROUPED_GROUP_QUERY_VALUE) {
      if (groupIds.length > 0) return false
    } else {
      const includeGroupIds = parseGroupFilterIds(filters.group)
      if (includeGroupIds.length > 0) {
        if (filters.group_match === 'exact') {
          if (groupIds.length !== includeGroupIds.length) return false
          if (!includeGroupIds.every((groupId, index) => groupIds[index] === groupId)) return false
        } else if (!includeGroupIds.some((groupId) => groupIds.includes(groupId))) {
          return false
        }
      }
    }
  }

  const excludeGroupIds = parseGroupFilterIds(filters.group_exclude)
  if (excludeGroupIds.length > 0 && excludeGroupIds.some((groupId) => groupIds.includes(groupId))) {
    return false
  }

  return true
}

const parseSearchAccountId = (rawSearch: string) => {
  const trimmed = rawSearch.trim()
  if (trimmed.length <= 3 || trimmed.slice(0, 3).toLowerCase() !== 'id:') return null

  const candidate = trimmed.slice(3).trim()
  if (!candidate || !/^[+-]?\d+$/.test(candidate)) return null

  const id = Number(candidate)
  return Number.isSafeInteger(id) && id > 0 ? id : null
}

export const accountMatchesCurrentFilters = (account: Account, filters: BulkAccountFilters) => {
  if (filters.platform && account.platform !== filters.platform) return false
  if (filters.type && account.type !== filters.type) return false
  if (filters.status) {
    if (filters.status === 'rate_limited') {
      if (!hasFutureTimestamp(account.rate_limit_reset_at)) return false
    } else if (filters.status === 'active') {
      if (!isNormallyActiveAccount(account)) return false
    } else if (filters.status === 'temp_unschedulable') {
      if (!hasFutureTimestamp(account.temp_unschedulable_until)) return false
    } else if (filters.status === 'unschedulable') {
      if (account.status !== 'active' || account.schedulable || hasFutureTimestamp(account.rate_limit_reset_at) || hasFutureTimestamp(account.temp_unschedulable_until)) return false
    } else if (account.status !== filters.status) {
      return false
    }
  }
  if (!accountMatchesGroupFilters(account, filters)) return false
  const privacyMode = typeof account.extra?.privacy_mode === 'string' ? account.extra.privacy_mode : ''
  if (filters.privacy_mode) {
    if (filters.privacy_mode === ACCOUNT_PRIVACY_MODE_UNSET_QUERY_VALUE) {
      if (privacyMode.trim() !== '') return false
    } else if (privacyMode !== filters.privacy_mode) {
      return false
    }
  }
  if (!accountMatchesLastUsedFilter(account, filters)) return false

  const search = String(filters.search || '').trim()
  if (!search) return true

  const searchAccountId = parseSearchAccountId(search)
  if (searchAccountId !== null) return account.id === searchAccountId

  return account.name.toLowerCase().includes(search.toLowerCase())
}

export const mergeRuntimeFields = (oldAccount: Account, updatedAccount: Account): Account => ({
  ...updatedAccount,
  current_concurrency: updatedAccount.current_concurrency ?? oldAccount.current_concurrency,
  current_window_cost: updatedAccount.current_window_cost ?? oldAccount.current_window_cost,
  active_sessions: updatedAccount.active_sessions ?? oldAccount.active_sessions
})
