import type { BulkAccountFilters, AccountListFilters, AccountSortParams } from '@/api/admin/accounts'

export type AccountTableParams = {
  platform: string
  type: string
  status: string
  privacy_mode: string
  group: string
  group_exclude: string
  group_match: string
  search: string
  last_used_filter: string
  last_used_start_date: string
  last_used_end_date: string
  sort_by: string
  sort_order: string
  lite?: string
}

export const ACCOUNT_FILTER_PARAM_KEYS = [
  'platform',
  'type',
  'status',
  'privacy_mode',
  'group',
  'group_exclude',
  'group_match',
  'search',
  'last_used_filter',
  'last_used_start_date',
  'last_used_end_date'
] as const

export const buildActiveAccountFilters = (source?: Partial<AccountTableParams>): BulkAccountFilters => {
  const currentParams = (source ?? {}) as Record<string, string | undefined>
  const filters: BulkAccountFilters = {}

  ACCOUNT_FILTER_PARAM_KEYS.forEach((key) => {
    const value = currentParams[key]
    if (typeof value === 'string' && value.trim() !== '') {
      filters[key] = value.trim()
    }
  })

  return filters
}

export const buildAccountListFilters = (source: Partial<AccountTableParams>): AccountListFilters => {
  const currentParams = source as Record<string, string | undefined>
  const filters: AccountListFilters = {}

  ACCOUNT_FILTER_PARAM_KEYS.forEach((key) => {
    const value = currentParams[key]
    if (typeof value === 'string') {
      filters[key] = value
    }
  })

  if (typeof currentParams.lite === 'string') {
    filters.lite = currentParams.lite
  }

  return filters
}

export const buildAccountSortQuery = (source?: Partial<AccountTableParams>): AccountSortParams => {
  const currentParams = (source ?? {}) as Record<string, string | undefined>
  const sort: AccountSortParams = {}

  if (typeof currentParams.sort_by === 'string') {
    sort.sort_by = currentParams.sort_by
  }
  if (typeof currentParams.sort_order === 'string') {
    sort.sort_order = currentParams.sort_order
  }

  return sort
}
