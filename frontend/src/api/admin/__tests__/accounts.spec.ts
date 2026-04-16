import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { AxiosInstance } from 'axios'

vi.mock('@/i18n', () => ({
  getLocale: () => 'zh-CN',
}))

const buildSuccessResponse = (data: unknown) => ({
  status: 200,
  data: {
    code: 0,
    data,
  },
  headers: {},
  config: {},
  statusText: 'OK',
})

describe('admin accounts api query boundaries', () => {
  let apiClient: AxiosInstance
  let list: typeof import('../accounts').list
  let listWithEtag: typeof import('../accounts').listWithEtag
  let exportData: typeof import('../accounts').exportData

  beforeEach(async () => {
    vi.resetModules()
    const clientModule = await import('@/api/client')
    apiClient = clientModule.apiClient
    const accountsModule = await import('../accounts')
    list = accountsModule.list
    listWithEtag = accountsModule.listWithEtag
    exportData = accountsModule.exportData
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('builds list params from separate filters and sort arguments', async () => {
    const adapter = vi.fn().mockResolvedValue(buildSuccessResponse({
      items: [],
      total: 0,
      pages: 0,
    }))
    apiClient.defaults.adapter = adapter

    await list(
      2,
      50,
      {
        platform: 'openai',
        group: '1,2,3',
        group_exclude: '4,5',
        group_match: 'exact',
        search: 'keyword',
      },
      {
        sort_by: 'priority,name',
        sort_order: 'desc,asc',
      }
    )

    expect(adapter).toHaveBeenCalledTimes(1)
    const config = adapter.mock.calls[0][0]
    expect(config.params).toMatchObject({
      page: 2,
      page_size: 50,
      platform: 'openai',
      group: '1,2,3',
      group_exclude: '4,5',
      group_match: 'exact',
      search: 'keyword',
      sort_by: 'priority,name',
      sort_order: 'desc,asc',
    })
  })

  it('keeps combined list params backwards compatible with request options', async () => {
    const adapter = vi.fn().mockResolvedValue(buildSuccessResponse({
      items: [],
      total: 0,
      pages: 0,
    }))
    apiClient.defaults.adapter = adapter
    const controller = new AbortController()

    await list(
      1,
      20,
      {
        search: 'keyword',
        sort_by: 'id',
        sort_order: 'desc',
      },
      {
        signal: controller.signal,
      }
    )

    expect(adapter).toHaveBeenCalledTimes(1)
    const config = adapter.mock.calls[0][0]
    expect(config.params).toMatchObject({
      page: 1,
      page_size: 20,
      search: 'keyword',
      sort_by: 'id',
      sort_order: 'desc',
    })
    expect(config.signal).toBe(controller.signal)
  })

  it('builds listWithEtag params from separate filters and sort arguments', async () => {
    const adapter = vi.fn().mockResolvedValue({
      status: 304,
      data: {},
      headers: {
        etag: 'W/"accounts-etag"',
      },
      config: {},
      statusText: 'Not Modified',
    })
    apiClient.defaults.adapter = adapter

    const result = await listWithEtag(
      3,
      25,
      {
        status: 'active',
        last_used_filter: 'range',
        last_used_start_date: '2026-01-01',
        last_used_end_date: '2026-01-31',
      },
      {
        sort_by: 'expires_at',
        sort_order: 'asc',
      },
      {
        etag: 'W/"previous-etag"',
      }
    )

    expect(result).toEqual({
      notModified: true,
      etag: 'W/"accounts-etag"',
      data: null,
    })

    expect(adapter).toHaveBeenCalledTimes(1)
    const config = adapter.mock.calls[0][0]
    expect(config.params).toMatchObject({
      page: 3,
      page_size: 25,
      status: 'active',
      last_used_filter: 'range',
      last_used_start_date: '2026-01-01',
      last_used_end_date: '2026-01-31',
      sort_by: 'expires_at',
      sort_order: 'asc',
    })
    expect(config.headers).toMatchObject({
      'If-None-Match': 'W/"previous-etag"',
    })
  })

  it('passes export filters and sort as separate request inputs', async () => {
    const adapter = vi.fn().mockResolvedValue(buildSuccessResponse({
      exported_at: '2026-01-31T00:00:00Z',
      proxies: [],
      accounts: [],
    }))
    apiClient.defaults.adapter = adapter

    await exportData({
      filters: {
        platform: 'openai',
        type: 'oauth',
        status: 'active',
        group: '1,2,3',
        group_exclude: '4,5',
        group_match: 'exact',
        privacy_mode: 'training_set_cf_blocked',
        search: 'keyword',
        last_used_filter: 'range',
        last_used_start_date: '2026-01-01',
        last_used_end_date: '2026-01-31',
      },
      sort: {
        sort_by: 'priority,name',
        sort_order: 'desc,asc',
      },
    })

    expect(adapter).toHaveBeenCalledTimes(1)
    const config = adapter.mock.calls[0][0]
    expect(config.params).toMatchObject({
      platform: 'openai',
      type: 'oauth',
      status: 'active',
      group: '1,2,3',
      group_exclude: '4,5',
      group_match: 'exact',
      privacy_mode: 'training_set_cf_blocked',
      search: 'keyword',
      last_used_filter: 'range',
      last_used_start_date: '2026-01-01',
      last_used_end_date: '2026-01-31',
      sort_by: 'priority,name',
      sort_order: 'desc,asc',
    })
  })
})
