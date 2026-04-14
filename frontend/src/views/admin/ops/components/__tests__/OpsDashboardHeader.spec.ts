import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import OpsDashboardHeader from '../OpsDashboardHeader.vue'

const i18nState = vi.hoisted(() => ({
  locale: 'zh' as 'zh' | 'en',
}))

const mockGetAllGroups = vi.fn()
const mockGetRealtimeTrafficSummary = vi.fn()
const mockSetOpsRealtimeMonitoringEnabledLocal = vi.fn()

vi.mock('@/api', () => ({
  adminAPI: {
    groups: {
      getAll: (...args: any[]) => mockGetAllGroups(...args),
    },
  },
}))

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    getRealtimeTrafficSummary: (...args: any[]) => mockGetRealtimeTrafficSummary(...args),
  },
}))

vi.mock('@/stores', () => ({
  useAdminSettingsStore: () => ({
    opsRealtimeMonitoringEnabled: true,
    setOpsRealtimeMonitoringEnabledLocal: mockSetOpsRealtimeMonitoringEnabledLocal,
  }),
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  const zhLocale = (await import('@/i18n/locales/zh')).default
  const enLocale = (await import('@/i18n/locales/en')).default

  const getMessage = (source: Record<string, any>, key: string) => key
    .split('.')
    .reduce<any>((acc, segment) => acc?.[segment], source)

  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, any>) => {
        const source = i18nState.locale === 'zh' ? zhLocale : enLocale
        const value = getMessage(source as Record<string, any>, key)
        if (typeof value !== 'string') return key
        if (!params) return value
        return value.replace(/\{(\w+)\}/g, (_, name: string) => String(params[name] ?? `{${name}}`))
      },
    }),
  }
})

const SelectStub = defineComponent({
  name: 'SelectStub',
  props: {
    modelValue: {
      type: [String, Number, Boolean],
      default: '',
    },
    options: {
      type: Array,
      default: () => [],
    },
  },
  emits: ['update:modelValue'],
  template: '<div class="select-stub" />',
})

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: {
    show: {
      type: Boolean,
      default: false,
    },
    title: {
      type: String,
      default: '',
    },
  },
  template: '<div v-if="show" class="base-dialog"><slot /></div>',
})

function createOverview(overrides: Record<string, any> = {}) {
  return {
    start_time: '2026-04-01T00:00:00Z',
    end_time: '2026-04-01T01:00:00Z',
    platform: 'openai',
    group_id: null,
    health_score: 97,
    system_metrics: null,
    job_heartbeats: [],
    success_count: 118,
    error_count_total: 5,
    business_limited_count: 3,
    error_count_sla: 2,
    request_count_total: 123,
    request_count_sla: 120,
    token_consumed: 4567,
    sla: 0.98,
    error_rate: 0.016,
    upstream_error_rate: 0.008,
    upstream_error_count_excl_429_529: 1,
    upstream_429_count: 1,
    upstream_529_count: 0,
    qps: {
      current: 2.5,
      peak: 4.2,
      avg: 2.1,
    },
    tps: {
      current: 18.4,
      peak: 24.6,
      avg: 16.2,
    },
    duration: {
      p50_ms: 100,
      p90_ms: 180,
      p95_ms: 220,
      p99_ms: 350,
      avg_ms: 140,
      max_ms: 500,
    },
    ttft: {
      p50_ms: 50,
      p90_ms: 75,
      p95_ms: 90,
      p99_ms: 120,
      avg_ms: 60,
      max_ms: 150,
    },
    ...overrides,
  }
}

function createRealtimeSummary(overrides: Record<string, any> = {}) {
  const qps = {
    current: 0.1,
    peak: 0.3,
    avg: 0.2,
    ...(overrides.qps ?? {}),
  }
  const tps = {
    current: 1.1,
    peak: 1.5,
    avg: 1.2,
    ...(overrides.tps ?? {}),
  }

  return {
    enabled: true,
    summary: {
      window: '1min',
      start_time: '2026-04-01T00:59:00Z',
      end_time: '2026-04-01T01:00:00Z',
      platform: 'openai',
      group_id: null,
      recent_request_count: 8,
      recent_error_count: 0,
      request_count_total: 8,
      qps,
      tps,
      ...overrides,
    },
  }
}

function mountHeader(
  locale: 'zh' | 'en',
  overviewOverrides: Record<string, any> = {},
  realtimeOverrides: Record<string, any> = {},
) {
  i18nState.locale = locale
  mockGetRealtimeTrafficSummary.mockResolvedValue(createRealtimeSummary(realtimeOverrides))

  return mount(OpsDashboardHeader, {
    props: {
      overview: createOverview(overviewOverrides),
      platform: 'openai',
      groupId: null,
      timeRange: '1h',
      queryMode: 'auto',
      loading: false,
      lastUpdated: new Date('2026-04-01T01:00:00Z'),
      autoRefreshEnabled: false,
      fullscreen: false,
      customStartTime: null,
      customEndTime: null,
    },
    global: {
      stubs: {
        Select: SelectStub,
        HelpTooltip: true,
        BaseDialog: BaseDialogStub,
        Icon: true,
      },
    },
  })
}

function findCard(wrapper: ReturnType<typeof mount>, cardTitle: string) {
  const card = wrapper
    .findAll('div.rounded-2xl')
    .find((candidate) => candidate.text().includes(cardTitle))

  expect(card, `missing card for ${cardTitle}`).toBeTruthy()
  return card!
}

function findMetricRowTextInCard(wrapper: ReturnType<typeof mount>, cardTitle: string, label: string) {
  const row = findCard(wrapper, cardTitle)
    .findAll('div.flex.justify-between')
    .find((candidate) => candidate.findAll('span')[0]?.text() === `${label}:`)

  expect(row, `missing metric row for ${label} in ${cardTitle}`).toBeTruthy()
  return row!.text()
}

function expectMetricAbsentInCard(wrapper: ReturnType<typeof mount>, cardTitle: string, label: string) {
  const row = findCard(wrapper, cardTitle)
    .findAll('div.flex.justify-between')
    .find((candidate) => candidate.findAll('span')[0]?.text() === `${label}:`)

  expect(row, `unexpected metric row for ${label} in ${cardTitle}`).toBeUndefined()
}

describe('OpsDashboardHeader', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockGetAllGroups.mockResolvedValue([])
    mockGetRealtimeTrafficSummary.mockResolvedValue(createRealtimeSummary())
  })

  it('中文下把整流器重试展示到上游错误卡片，且业务限制只有一个冒号', async () => {
    const wrapper = mountHeader('zh', {
      rectifier_retry_count: 7,
    })

    await flushPromises()

    expect(findMetricRowTextInCard(wrapper, '上游错误', '整流器重试')).toBe('整流器重试:7')
    expect(findMetricRowTextInCard(wrapper, '上游错误', '429/529')).toBe('429/529:1')
    expect(findMetricRowTextInCard(wrapper, '请求错误', '业务限制')).toBe('业务限制:3')
    expectMetricAbsentInCard(wrapper, '请求错误', '整流器重试')
    expect(wrapper.text()).not.toContain('业务限制：:')
  })

  it('英文下在缺少 rectifier_retry_count 时回退为 0，且 Business Limited 只有一个冒号', async () => {
    const wrapper = mountHeader('en')

    await flushPromises()

    expect(findMetricRowTextInCard(wrapper, 'Upstream Errors', 'Rectifier retries')).toBe('Rectifier retries:0')
    expect(findMetricRowTextInCard(wrapper, 'Upstream Errors', '429/529')).toBe('429/529:1')
    expect(findMetricRowTextInCard(wrapper, 'Request Errors', 'Business Limited')).toBe('Business Limited:3')
    expectMetricAbsentInCard(wrapper, 'Request Errors', 'Rectifier retries')
    expect(wrapper.text()).not.toContain('business_limited::')
  })

  it('近 1 小时有请求但最近 1 分钟无请求时，不应误判为待机', async () => {
    const wrapper = mountHeader(
      'zh',
      {
        error_rate: 0,
        upstream_error_rate: 0,
        error_count_total: 0,
        error_count_sla: 0,
        request_count_total: 42,
        request_count_sla: 42,
        success_count: 42,
      },
      {
        recent_request_count: 0,
        qps: {
          current: 0,
          peak: 0,
          avg: 0,
        },
        tps: {
          current: 0,
          peak: 0,
          avg: 0,
        },
      },
    )

    await flushPromises()

    expect(wrapper.text()).not.toContain('待机')
    expect(wrapper.text()).toContain('42')
  })

  it('低流量下平均请求速率会改用 QPM 展示', async () => {
    const wrapper = mountHeader(
      'zh',
      {
        error_rate: 0,
        upstream_error_rate: 0,
        error_count_total: 0,
        error_count_sla: 0,
        request_count_total: 2,
        request_count_sla: 2,
        success_count: 2,
      },
      {
        recent_request_count: 0,
        qps: {
          current: 0,
          peak: 0,
          avg: 0,
        },
        tps: {
          current: 0,
          peak: 0,
          avg: 0,
        },
      },
    )

    await flushPromises()

    const rowText = findMetricRowTextInCard(wrapper, '请求', '平均 QPS')
    expect(rowText).toContain('0.03')
    expect(rowText).toContain('QPM')
  })
})
