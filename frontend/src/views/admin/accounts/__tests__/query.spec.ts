import { describe, expect, it } from 'vitest'

import {
  buildActiveAccountFilters,
  buildAccountListFilters,
  buildAccountSortQuery,
  type AccountTableParams,
} from '../query'

describe('accounts query helpers', () => {
  it('buildActiveAccountFilters 只保留非空过滤字段并做 trim', () => {
    const params: Partial<AccountTableParams> = {
      platform: ' openai ',
      type: '',
      status: ' active ',
      group: ' 1,2 ',
      search: '  hello ',
      sort_by: 'id',
      sort_order: 'desc',
    }

    expect(buildActiveAccountFilters(params)).toEqual({
      platform: 'openai',
      status: 'active',
      group: '1,2',
      search: 'hello',
    })
  })

  it('buildAccountListFilters 和 buildAccountSortQuery 正确保留列表参数与排序参数', () => {
    const params: Partial<AccountTableParams> = {
      platform: 'openai',
      privacy_mode: '__unset__',
      lite: '1',
      sort_by: 'last_used_at',
      sort_order: 'asc',
    }

    expect(buildAccountListFilters(params)).toEqual({
      platform: 'openai',
      privacy_mode: '__unset__',
      lite: '1',
    })

    expect(buildAccountSortQuery(params)).toEqual({
      sort_by: 'last_used_at',
      sort_order: 'asc',
    })
  })

  it('buildAccountSortQuery 保留 usage_7d_remaining 排序键', () => {
    const params: Partial<AccountTableParams> = {
      sort_by: 'usage_7d_remaining',
      sort_order: 'desc',
    }

    expect(buildAccountSortQuery(params)).toEqual({
      sort_by: 'usage_7d_remaining',
      sort_order: 'desc',
    })
  })
})
