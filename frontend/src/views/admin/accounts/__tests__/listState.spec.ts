import { describe, expect, it } from 'vitest'

import {
  ACCOUNT_PRIVACY_MODE_UNSET_QUERY_VALUE,
  ACCOUNT_UNGROUPED_GROUP_QUERY_VALUE,
  accountMatchesCurrentFilters,
  mergeRuntimeFields,
  parseGroupFilterIds,
} from '../listState'

describe('accounts list state helpers', () => {
  it('parseGroupFilterIds 会去重、过滤非法值并排序', () => {
    expect(parseGroupFilterIds('3, 2, foo, 3, 1')).toEqual([1, 2, 3])
  })

  it('accountMatchesCurrentFilters 支持 ungrouped 与 privacy unset 过滤', () => {
    const account = {
      id: 101,
      name: 'Account 101',
      platform: 'openai',
      type: 'oauth',
      status: 'active',
      schedulable: true,
      group_ids: [],
      groups: [],
      extra: {},
    } as any

    expect(accountMatchesCurrentFilters(account, {
      group: ACCOUNT_UNGROUPED_GROUP_QUERY_VALUE,
      privacy_mode: ACCOUNT_PRIVACY_MODE_UNSET_QUERY_VALUE,
    })).toBe(true)

    expect(accountMatchesCurrentFilters(account, {
      group: '1,2',
    })).toBe(false)
  })

  it('mergeRuntimeFields 会保留旧行缺失的运行时字段', () => {
    const oldAccount = {
      current_concurrency: 3,
      current_window_cost: 12,
      active_sessions: 2,
    } as any

    const updatedAccount = {
      id: 101,
      name: 'Updated',
      current_concurrency: undefined,
      current_window_cost: 20,
      active_sessions: undefined,
    } as any

    expect(mergeRuntimeFields(oldAccount, updatedAccount)).toMatchObject({
      id: 101,
      name: 'Updated',
      current_concurrency: 3,
      current_window_cost: 20,
      active_sessions: 2,
    })
  })
})
