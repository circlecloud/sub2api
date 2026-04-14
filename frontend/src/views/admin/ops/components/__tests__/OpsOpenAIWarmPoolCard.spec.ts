import { describe, it, expect, beforeEach, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import OpsOpenAIWarmPoolCard from '../OpsOpenAIWarmPoolCard.vue'

const mockGetOpenAIWarmPoolStats = vi.fn()
const mockTriggerOpenAIWarmPoolGlobalRefill = vi.fn()
const mockListSystemLogs = vi.fn()
const mockShowSuccess = vi.fn()
const mockShowError = vi.fn()

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    getOpenAIWarmPoolStats: (...args: any[]) => mockGetOpenAIWarmPoolStats(...args),
    triggerOpenAIWarmPoolGlobalRefill: (...args: any[]) => mockTriggerOpenAIWarmPoolGlobalRefill(...args),
    listSystemLogs: (...args: any[]) => mockListSystemLogs(...args),
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showSuccess: mockShowSuccess,
    showError: mockShowError,
  }),
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, any>) => {
        if (params?.count !== undefined) return `${key}:${params.count}`
        if (params?.time !== undefined) return `${key}:${params.time}`
        return key
      },
    }),
  }
})

const EmptyStateStub = defineComponent({
  name: 'EmptyState',
  props: {
    title: { type: String, default: '' },
    description: { type: String, default: '' },
  },
  template: '<div class="empty-state">{{ title }}|{{ description }}</div>',
})

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: {
    show: { type: Boolean, default: false },
    title: { type: String, default: '' },
  },
  template: '<div v-if="show" class="base-dialog"><div class="base-dialog-title">{{ title }}</div><slot /></div>',
})

const sampleLogsResponse = {
  items: [
    {
      id: 1,
      created_at: '2026-04-01T00:58:30Z',
      level: 'info',
      component: 'service.openai_warm_pool',
      message: 'warm pool probe failed because account needs reauthorization',
      account_id: 1001,
      extra: {
        event: 'probe_failed',
        reason: 'needs_reauth',
        group_id: 7,
        error_code: 'unauthenticated',
      },
    },
  ],
  total: 1,
  page: 1,
  page_size: 12,
}

const sampleResponse = {
  enabled: true,
  warm_pool_enabled: true,
  reader_ready: true,
  bootstrapping: false,
  timestamp: '2026-04-01T01:00:00Z',
  summary: {
    tracked_account_count: 3,
    bucket_ready_account_count: 2,
    global_ready_account_count: 1,
    active_group_count: 1,
    global_target_per_active_group: 10,
    global_refill_per_active_group: 6,
    groups_below_target_count: 1,
    groups_below_refill_count: 1,
    probing_account_count: 1,
    cooling_account_count: 1,
    network_error_pool_count: 1,
    take_count: 9,
    network_error_pool_full: false,
    last_bucket_maintenance_at: '2026-04-01T00:59:00Z',
    last_global_maintenance_at: '2026-04-01T00:58:00Z',
  },
  buckets: [
    {
      group_id: 7,
      group_name: 'Warm Group',
      schedulable_accounts: 3,
      bucket_ready_accounts: 1,
      bucket_target_size: 1,
      bucket_refill_below: 1,
      take_count: 4,
      probing_accounts: 1,
      cooling_accounts: 1,
      last_access_at: '2026-04-01T00:58:00Z',
      last_refill_at: '2026-04-01T00:57:00Z',
    },
  ],
  accounts: [
    {
      account_id: 1001,
      account_name: 'ready-account',
      platform: 'openai',
      schedulable: true,
      priority: 0,
      concurrency: 1,
      state: 'ready',
      groups: [
        { group_id: 7, group_name: 'Warm Group' },
      ],
      verified_at: '2026-04-01T01:00:00Z',
      expires_at: '2026-04-01T01:10:00Z',
      fail_until: null,
      network_error_until: null,
    },
  ],
  global_coverages: [
    {
      group_id: 7,
      group_name: 'Warm Group',
      coverage_count: 1,
      target_size: 10,
      refill_below: 6,
    },
  ],
  network_error_pool: {
    count: 1,
    capacity: 3,
    oldest_entered_at: '2026-04-01T00:55:00Z',
  },
}

function buildReadyAccount(accountId: number) {
  return {
    account_id: accountId,
    account_name: `ready-account-${accountId}`,
    platform: 'openai',
    schedulable: true,
    priority: accountId,
    concurrency: 1,
    state: 'ready',
    groups: [
      { group_id: 7, group_name: 'Warm Group' },
    ],
    verified_at: '2026-04-01T01:00:00Z',
    expires_at: '2026-04-01T01:10:00Z',
  }
}

function buildReadyListResponse(page: number, count: number, total: number, pageSize = 20) {
  return {
    ...sampleResponse,
    accounts: Array.from({ length: count }, (_, index) => buildReadyAccount((page - 1) * pageSize + index + 1)),
    page,
    page_size: pageSize,
    total,
    pages: Math.max(1, Math.ceil(total / pageSize)),
  }
}

function buildWarmPoolResponse(overrides: {
  bucket?: Record<string, any>
  summary?: Record<string, any>
  warm_pool_enabled?: boolean
} = {}) {
  return {
    ...sampleResponse,
    warm_pool_enabled: overrides.warm_pool_enabled ?? sampleResponse.warm_pool_enabled,
    summary: {
      ...sampleResponse.summary,
      ...(overrides.summary || {}),
    },
    buckets: [
      {
        ...sampleResponse.buckets[0],
        ...(overrides.bucket || {}),
      },
    ],
  }
}

function createDeferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

describe('OpsOpenAIWarmPoolCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockTriggerOpenAIWarmPoolGlobalRefill.mockResolvedValue({ ok: true })
    mockListSystemLogs.mockResolvedValue(sampleLogsResponse)
  })

  it('默认加载并透传 group 过滤参数', async () => {
    mockGetOpenAIWarmPoolStats.mockResolvedValue(sampleResponse)

    mount(OpsOpenAIWarmPoolCard, {
      props: {
        groupIdFilter: 7,
        refreshToken: 0,
      },
      global: {
        stubs: {
          EmptyState: EmptyStateStub,
          BaseDialog: BaseDialogStub,
        },
      },
    })

    await flushPromises()
    expect(mockGetOpenAIWarmPoolStats).toHaveBeenCalledWith(7)
    expect(mockListSystemLogs).toHaveBeenCalledWith(expect.objectContaining({
      component: 'service.openai_warm_pool',
      q: '"group_id":7',
    }))
  })

  it('全局模式展示 bucket 列表且不展示账号表', async () => {
    mockGetOpenAIWarmPoolStats.mockResolvedValue({
      ...sampleResponse,
      accounts: [],
    })

    const wrapper = mount(OpsOpenAIWarmPoolCard, {
      props: { refreshToken: 0 },
      global: {
        stubs: {
          EmptyState: EmptyStateStub,
          BaseDialog: BaseDialogStub,
        },
      },
    })

    await flushPromises()
    expect(wrapper.text()).toContain('Warm Group')
    expect(wrapper.text()).not.toContain('ready-account')
  })

  it('通用池复检日志会补充显示账号信息', async () => {
    mockGetOpenAIWarmPoolStats.mockResolvedValue({
      ...sampleResponse,
      accounts: [],
    })
    mockListSystemLogs.mockResolvedValue({
      items: [
        {
          id: 2,
          created_at: '2026-04-01T01:02:00Z',
          level: 'info',
          component: 'service.openai_warm_pool',
          message: '预热池就绪账号已过期，正在刷新前复检',
          account_id: 2001,
          extra: {
            event: 'ready_recheck_start',
            reason: 'expired',
            group_id: 0,
            account_name: 'global-ready-1',
          },
        },
      ],
      total: 1,
      page: 1,
      page_size: 12,
    })

    const wrapper = mount(OpsOpenAIWarmPoolCard, {
      props: { refreshToken: 0 },
      global: {
        stubs: {
          EmptyState: EmptyStateStub,
          BaseDialog: BaseDialogStub,
        },
      },
    })

    await flushPromises()
    expect(wrapper.text()).toContain('预热池就绪账号已过期，正在刷新前复检')
    expect(wrapper.text()).toContain('账号=global-ready-1 (#2001)')
  })

  it('不展示底部网络异常池区块', async () => {
    mockGetOpenAIWarmPoolStats.mockResolvedValue(sampleResponse)

    const wrapper = mount(OpsOpenAIWarmPoolCard, {
      props: { refreshToken: 0 },
      global: {
        stubs: {
          EmptyState: EmptyStateStub,
          BaseDialog: BaseDialogStub,
        },
      },
    })

    await flushPromises()
    expect(wrapper.text()).not.toContain('admin.ops.openaiWarmPool.networkErrorTitle')
    expect(wrapper.text()).not.toContain('admin.ops.openaiWarmPool.networkError.count')
    expect(wrapper.text()).not.toContain('admin.ops.openaiWarmPool.networkError.capacity')
  })

  it('分组模式展示账号状态明细和最近日志', async () => {
    mockGetOpenAIWarmPoolStats.mockResolvedValue(sampleResponse)

    const wrapper = mount(OpsOpenAIWarmPoolCard, {
      props: {
        groupIdFilter: 7,
        refreshToken: 0,
      },
      global: {
        stubs: {
          EmptyState: EmptyStateStub,
          BaseDialog: BaseDialogStub,
        },
      },
    })

    await flushPromises()
    expect(wrapper.text()).toContain('ready-account')
    expect(wrapper.text()).toContain('admin.ops.openaiWarmPool.state.ready')
    expect(wrapper.text()).toContain('needs_reauth')
    expect(wrapper.text()).toContain('预热池探测失败：账号需要重新授权')
  })

  it.each([
    {
      name: '正常',
      response: buildWarmPoolResponse({
        bucket: { schedulable_accounts: 3, bucket_ready_accounts: 1, bucket_refill_below: 1, probing_accounts: 0 },
        summary: { groups_below_refill_count: 0, probing_account_count: 0 },
        coverage: { coverage_count: 1, refill_below: 1 },
      }),
      bucketKey: 'admin.ops.openaiWarmPool.bucketStatus.normal',
      globalKey: 'admin.ops.openaiWarmPool.globalStatus.normal',
      colorClass: 'green',
    },
    {
      name: '待补充',
      response: buildWarmPoolResponse({
        bucket: { schedulable_accounts: 3, bucket_ready_accounts: 0, bucket_refill_below: 1, probing_accounts: 0 },
        summary: { groups_below_refill_count: 1, probing_account_count: 0 },
      }),
      bucketKey: 'admin.ops.openaiWarmPool.bucketStatus.pendingRefill',
      globalKey: 'admin.ops.openaiWarmPool.globalStatus.pendingRefill',
      colorClass: 'amber',
    },
    {
      name: '补充中',
      response: buildWarmPoolResponse({
        bucket: { schedulable_accounts: 3, bucket_ready_accounts: 0, bucket_refill_below: 1, probing_accounts: 1 },
        summary: { groups_below_refill_count: 1, probing_account_count: 1 },
      }),
      bucketKey: 'admin.ops.openaiWarmPool.bucketStatus.refilling',
      globalKey: 'admin.ops.openaiWarmPool.globalStatus.refilling',
      colorClass: 'amber',
    },
    {
      name: '全部',
      response: buildWarmPoolResponse({
        bucket: { schedulable_accounts: 0, bucket_ready_accounts: 0, bucket_refill_below: 1, probing_accounts: 0 },
        summary: { groups_below_refill_count: 1, probing_account_count: 0 },
      }),
      bucketKey: 'admin.ops.openaiWarmPool.bucketStatus.all',
      globalKey: 'admin.ops.openaiWarmPool.globalStatus.all',
      colorClass: 'blue',
    },
  ])('按规则展示 $name 状态', async ({ response, bucketKey, globalKey, colorClass }) => {
    mockGetOpenAIWarmPoolStats.mockResolvedValue(response)

    const wrapper = mount(OpsOpenAIWarmPoolCard, {
      props: { refreshToken: 0 },
      global: {
        stubs: {
          EmptyState: EmptyStateStub,
          BaseDialog: BaseDialogStub,
        },
      },
    })

    await flushPromises()
    const bucketStatus = wrapper.findAll('span').find((item) => item.text().includes(bucketKey))
    const globalStatus = wrapper.findAll('span').find((item) => item.text().includes(globalKey))
    expect(bucketStatus).toBeTruthy()
    expect(globalStatus).toBeTruthy()
    expect(bucketStatus!.classes().join(' ')).toContain(colorClass)
    expect(globalStatus!.classes().join(' ')).toContain(colorClass)
  })

  it('预热池关闭时不显示任何预热池内容', async () => {
    mockGetOpenAIWarmPoolStats.mockResolvedValue(buildWarmPoolResponse({ warm_pool_enabled: false }))

    const wrapper = mount(OpsOpenAIWarmPoolCard, {
      props: { refreshToken: 0 },
      global: {
        stubs: {
          EmptyState: EmptyStateStub,
          BaseDialog: BaseDialogStub,
        },
      },
    })

    await flushPromises()
    expect(wrapper.find('section').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('admin.ops.openaiWarmPool.title')
    expect(wrapper.text()).not.toContain('admin.ops.openaiWarmPool.featureDisabledHint')
  })

  it('可以打开 Ready 列表并按页加载账号', async () => {
    mockGetOpenAIWarmPoolStats
      .mockResolvedValueOnce(sampleResponse)
      .mockResolvedValueOnce(buildReadyListResponse(1, 20, 21))
      .mockResolvedValueOnce(buildReadyListResponse(2, 1, 21))

    const wrapper = mount(OpsOpenAIWarmPoolCard, {
      props: { refreshToken: 0 },
      global: {
        stubs: {
          EmptyState: EmptyStateStub,
          BaseDialog: BaseDialogStub,
        },
      },
    })

    await flushPromises()
    const readyListButton = wrapper.findAll('button').find((item) => item.text() === 'admin.ops.openaiWarmPool.viewReadyList')
    expect(readyListButton).toBeTruthy()
    await readyListButton!.trigger('click')
    await flushPromises()

    expect(mockGetOpenAIWarmPoolStats).toHaveBeenNthCalledWith(2, null, {
      includeAccount: true,
      accountsOnly: true,
      accountState: 'ready',
      page: 1,
      page_size: 20,
    })
    expect(wrapper.text()).toContain('admin.ops.openaiWarmPool.readyListTitle')
    expect(wrapper.text()).toContain('ready-account-1')
    expect(wrapper.text()).toContain('ready-account-20')

    const loadMoreButton = wrapper.findAll('button').find((item) => item.text() === 'admin.ops.openaiWarmPool.loadMoreReadyList')
    expect(loadMoreButton).toBeTruthy()
    await loadMoreButton!.trigger('click')
    await flushPromises()

    expect(mockGetOpenAIWarmPoolStats).toHaveBeenNthCalledWith(3, null, {
      includeAccount: true,
      accountsOnly: true,
      accountState: 'ready',
      page: 2,
      page_size: 20,
    })
    expect(wrapper.text()).toContain('ready-account-21')
    expect(wrapper.findAll('button').some((item) => item.text() === 'admin.ops.openaiWarmPool.loadMoreReadyList')).toBe(false)
  })

  it('加载更多失败时保留已加载行并弹出错误', async () => {
    mockGetOpenAIWarmPoolStats
      .mockResolvedValueOnce(sampleResponse)
      .mockResolvedValueOnce(buildReadyListResponse(1, 20, 21))
      .mockRejectedValueOnce(new Error('下一页失败'))

    const wrapper = mount(OpsOpenAIWarmPoolCard, {
      props: { refreshToken: 0 },
      global: {
        stubs: {
          EmptyState: EmptyStateStub,
          BaseDialog: BaseDialogStub,
        },
      },
    })

    await flushPromises()
    const readyListButton = wrapper.findAll('button').find((item) => item.text() === 'admin.ops.openaiWarmPool.viewReadyList')
    expect(readyListButton).toBeTruthy()
    await readyListButton!.trigger('click')
    await flushPromises()

    const loadMoreButton = wrapper.findAll('button').find((item) => item.text() === 'admin.ops.openaiWarmPool.loadMoreReadyList')
    expect(loadMoreButton).toBeTruthy()
    await loadMoreButton!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('ready-account-1')
    expect(wrapper.text()).toContain('ready-account-20')
    expect(mockShowError).toHaveBeenCalledWith('下一页失败')
    expect(wrapper.findAll('button').some((item) => item.text() === 'admin.ops.openaiWarmPool.loadMoreReadyList')).toBe(true)
  })

  it('只有一页时不展示加载更多按钮', async () => {
    mockGetOpenAIWarmPoolStats
      .mockResolvedValueOnce(sampleResponse)
      .mockResolvedValueOnce(buildReadyListResponse(1, 1, 1))

    const wrapper = mount(OpsOpenAIWarmPoolCard, {
      props: { refreshToken: 0 },
      global: {
        stubs: {
          EmptyState: EmptyStateStub,
          BaseDialog: BaseDialogStub,
        },
      },
    })

    await flushPromises()
    const readyListButton = wrapper.findAll('button').find((item) => item.text() === 'admin.ops.openaiWarmPool.viewReadyList')
    expect(readyListButton).toBeTruthy()
    await readyListButton!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('ready-account-1')
    expect(wrapper.findAll('button').some((item) => item.text() === 'admin.ops.openaiWarmPool.loadMoreReadyList')).toBe(false)
  })

  it('查看 Ready 列表时不会因自动刷新重复加载列表', async () => {
    mockGetOpenAIWarmPoolStats
      .mockResolvedValueOnce({
        ...sampleResponse,
        accounts: [],
      })
      .mockResolvedValueOnce(sampleResponse)
      .mockResolvedValueOnce({
        ...sampleResponse,
        accounts: [],
      })

    const wrapper = mount(OpsOpenAIWarmPoolCard, {
      props: { refreshToken: 0 },
      global: {
        stubs: {
          EmptyState: EmptyStateStub,
          BaseDialog: BaseDialogStub,
        },
      },
    })

    await flushPromises()
    const readyListButton = wrapper.findAll('button').find((item) => item.text() === 'admin.ops.openaiWarmPool.viewReadyList')
    expect(readyListButton).toBeTruthy()
    await readyListButton!.trigger('click')
    await flushPromises()

    await wrapper.setProps({ refreshToken: 1 })
    await flushPromises()

    expect(mockGetOpenAIWarmPoolStats).toHaveBeenCalledTimes(3)
    expect(mockGetOpenAIWarmPoolStats).toHaveBeenNthCalledWith(3, null)
  })

  it('可以手动触发补充全局池并刷新卡片', async () => {
    mockGetOpenAIWarmPoolStats
      .mockResolvedValueOnce(sampleResponse)
      .mockResolvedValueOnce({
        ...sampleResponse,
        summary: {
          ...sampleResponse.summary,
          global_ready_account_count: 2,
          last_global_maintenance_at: '2026-04-01T01:05:00Z',
        },
      })

    const wrapper = mount(OpsOpenAIWarmPoolCard, {
      props: { refreshToken: 0 },
      global: {
        stubs: {
          EmptyState: EmptyStateStub,
          BaseDialog: BaseDialogStub,
        },
      },
    })

    await flushPromises()
    const refillButton = wrapper.findAll('button').find((item) => item.text() === 'admin.ops.openaiWarmPool.triggerGlobalRefill')
    expect(refillButton).toBeTruthy()

    await refillButton!.trigger('click')
    await flushPromises()

    expect(mockTriggerOpenAIWarmPoolGlobalRefill).toHaveBeenCalledTimes(1)
    expect(mockGetOpenAIWarmPoolStats).toHaveBeenCalledTimes(2)
    expect(mockShowSuccess).toHaveBeenCalledWith('admin.ops.openaiWarmPool.triggerGlobalRefillSuccess')
    expect(wrapper.text()).toContain('2')
  })

  it('首次初始化时显示浅橙色 bootstrapping 提示', async () => {
    mockGetOpenAIWarmPoolStats.mockResolvedValue({
      ...sampleResponse,
      bootstrapping: true,
    })

    const wrapper = mount(OpsOpenAIWarmPoolCard, {
      props: { refreshToken: 0 },
      global: {
        stubs: {
          EmptyState: EmptyStateStub,
          BaseDialog: BaseDialogStub,
        },
      },
    })

    await flushPromises()
    expect(wrapper.text()).toContain('admin.ops.openaiWarmPool.bootstrappingHint')
    const hint = wrapper.findAll('div').find((item) => item.text() === 'admin.ops.openaiWarmPool.bootstrappingHint')
    expect(hint?.classes()).toContain('bg-amber-50')
    expect(hint?.classes()).toContain('text-amber-700')
    expect(wrapper.text()).not.toContain('admin.ops.openaiWarmPool.readerUnavailableHint')
  })

  it('reader 未就绪时不显示 bootstrapping 提示', async () => {
    mockGetOpenAIWarmPoolStats.mockResolvedValue({
      ...sampleResponse,
      reader_ready: false,
      bootstrapping: true,
    })

    const wrapper = mount(OpsOpenAIWarmPoolCard, {
      props: { refreshToken: 0 },
      global: {
        stubs: {
          EmptyState: EmptyStateStub,
          BaseDialog: BaseDialogStub,
        },
      },
    })

    await flushPromises()
    expect(wrapper.text()).toContain('admin.ops.openaiWarmPool.readerUnavailableHint')
    expect(wrapper.text()).not.toContain('admin.ops.openaiWarmPool.bootstrappingHint')
  })

  it('warm pool 未启用时不显示任何预热池内容', async () => {
    mockGetOpenAIWarmPoolStats.mockResolvedValue({
      ...sampleResponse,
      warm_pool_enabled: false,
      buckets: [],
      accounts: [],
    })

    const wrapper = mount(OpsOpenAIWarmPoolCard, {
      props: { refreshToken: 0 },
      global: {
        stubs: {
          EmptyState: EmptyStateStub,
          BaseDialog: BaseDialogStub,
        },
      },
    })

    await flushPromises()
    expect(wrapper.find('section').exists()).toBe(false)
    expect(wrapper.text()).toBe('')
  })

  it('日志慢于 stats 时先渲染主卡内容，不阻塞首屏主体', async () => {
    const pendingLogs = createDeferred<typeof sampleLogsResponse>()
    mockGetOpenAIWarmPoolStats.mockResolvedValue(sampleResponse)
    mockListSystemLogs.mockImplementationOnce(() => pendingLogs.promise)

    const wrapper = mount(OpsOpenAIWarmPoolCard, {
      props: { refreshToken: 0 },
      global: {
        stubs: {
          EmptyState: EmptyStateStub,
          BaseDialog: BaseDialogStub,
        },
      },
    })

    await flushPromises()
    expect(wrapper.text()).toContain('Warm Group')
    expect(wrapper.text()).toContain('admin.ops.loadingText')
    expect(wrapper.text()).not.toContain('admin.ops.openaiWarmPool.logEmpty')

    pendingLogs.resolve(sampleLogsResponse)
    await flushPromises()
    expect(wrapper.text()).toContain('needs_reauth')
  })

  it('日志接口异常时不影响主卡片展示，但会显示日志错误', async () => {
    mockGetOpenAIWarmPoolStats.mockResolvedValue(sampleResponse)
    mockListSystemLogs.mockRejectedValue(new Error('日志失败'))

    const wrapper = mount(OpsOpenAIWarmPoolCard, {
      props: { refreshToken: 0 },
      global: {
        stubs: {
          EmptyState: EmptyStateStub,
          BaseDialog: BaseDialogStub,
        },
      },
    })

    await flushPromises()
    expect(wrapper.text()).toContain('Warm Group')
    expect(wrapper.text()).toContain('日志失败')
  })

  it('接口异常时展示错误提示', async () => {
    mockGetOpenAIWarmPoolStats.mockRejectedValue(new Error('加载失败'))

    const wrapper = mount(OpsOpenAIWarmPoolCard, {
      props: { refreshToken: 0 },
      global: {
        stubs: {
          EmptyState: EmptyStateStub,
          BaseDialog: BaseDialogStub,
        },
      },
    })

    await flushPromises()
    expect(wrapper.text()).toContain('加载失败')
  })
})
