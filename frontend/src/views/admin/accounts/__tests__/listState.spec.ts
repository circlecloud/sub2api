import { describe, expect, it } from 'vitest'

import {
  ACCOUNT_PRIVACY_MODE_UNSET_QUERY_VALUE,
  ACCOUNT_UNGROUPED_GROUP_QUERY_VALUE,
  accountMatchesCurrentFilters,
  mergeRuntimeFields,
  parseGroupFilterIds,
} from '../listState'

const buildAccount = (overrides: Record<string, unknown> = {}) => ({
  id: 101,
  name: 'Account 101',
  platform: 'openai',
  type: 'oauth',
  status: 'active',
  schedulable: true,
  group_ids: [],
  groups: [],
  extra: {},
  ...overrides,
}) as any

describe('accounts list state helpers', () => {
  it('parseGroupFilterIds 会去重、过滤非法值并排序', () => {
    expect(parseGroupFilterIds('3, 2, foo, 3, 1')).toEqual([1, 2, 3])
  })

  it('accountMatchesCurrentFilters 支持 ungrouped 与 privacy unset 过滤', () => {
    const account = buildAccount()

    expect(accountMatchesCurrentFilters(account, {
      group: ACCOUNT_UNGROUPED_GROUP_QUERY_VALUE,
      privacy_mode: ACCOUNT_PRIVACY_MODE_UNSET_QUERY_VALUE,
    })).toBe(true)

    expect(accountMatchesCurrentFilters(account, {
      group: '1,2',
    })).toBe(false)
  })

  it('accountMatchesCurrentFilters 支持 id: 精确匹配搜索', () => {
    const account = buildAccount({ id: 101, name: 'Another Account' })

    expect(accountMatchesCurrentFilters(account, {
      search: 'id:101',
    })).toBe(true)

    expect(accountMatchesCurrentFilters(account, {
      search: 'id:999',
    })).toBe(false)
  })

  it('accountMatchesCurrentFilters 非法 id: 搜索会回退为名称包含匹配', () => {
    const account = buildAccount({ name: 'Fallback id:abc account' })

    expect(accountMatchesCurrentFilters(account, {
      search: 'id:abc',
    })).toBe(true)
  })

  it('accountMatchesCurrentFilters 空搜索与普通名称搜索保持现有行为', () => {
    const account = buildAccount({ name: 'Alpha Account' })

    expect(accountMatchesCurrentFilters(account, {
      search: '',
    })).toBe(true)

    expect(accountMatchesCurrentFilters(account, {
      search: 'alpha',
    })).toBe(true)

    expect(accountMatchesCurrentFilters(account, {
      search: 'beta',
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
