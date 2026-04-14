import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, reactive } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import OpsDashboard from '../OpsDashboard.vue'

const mockAdminSettingsFetch = vi.fn()
const mockRouterReplace = vi.fn()
const mockShowError = vi.fn()
const mockGetDashboardSnapshotV2 = vi.fn()
const mockGetThroughputTrend = vi.fn()
const mockGetDashboardOverview = vi.fn()
const mockGetErrorTrend = vi.fn()
const mockGetLatencyHistogram = vi.fn()
const mockGetErrorDistribution = vi.fn()
const mockGetAdvancedSettings = vi.fn()
const mockGetMetricThresholds = vi.fn()

const routeState = reactive({
  query: {} as Record<string, any>
})

const adminSettingsStoreState = reactive({
  opsMonitoringEnabled: true,
  opsQueryModeDefault: 'auto',
  fetch: (...args: any[]) => mockAdminSettingsFetch(...args)
})

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
  useRouter: () => ({
    replace: (...args: any[]) => mockRouterReplace(...args)
  })
}))

vi.mock('@/stores', () => ({
  useAdminSettingsStore: () => adminSettingsStoreState,
  useAppStore: () => ({
    showError: mockShowError
  })
}))

vi.mock('@/api/admin/ops', () => {
  const mockedOpsAPI = {
    getDashboardSnapshotV2: (...args: any[]) => mockGetDashboardSnapshotV2(...args),
    getThroughputTrend: (...args: any[]) => mockGetThroughputTrend(...args),
    getDashboardOverview: (...args: any[]) => mockGetDashboardOverview(...args),
    getErrorTrend: (...args: any[]) => mockGetErrorTrend(...args),
    getLatencyHistogram: (...args: any[]) => mockGetLatencyHistogram(...args),
    getErrorDistribution: (...args: any[]) => mockGetErrorDistribution(...args),
    getAdvancedSettings: (...args: any[]) => mockGetAdvancedSettings(...args),
    getMetricThresholds: (...args: any[]) => mockGetMetricThresholds(...args)
  }

  return {
    default: mockedOpsAPI,
    opsAPI: mockedOpsAPI
  }
})

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

const AppLayoutStub = defineComponent({
  name: 'AppLayout',
  template: '<div class="app-layout-stub"><slot /></div>'
})

const OpsDashboardSkeletonStub = defineComponent({
  name: 'OpsDashboardSkeleton',
  template: '<div class="ops-dashboard-skeleton">skeleton</div>'
})

const OpsDashboardHeaderStub = defineComponent({
  name: 'OpsDashboardHeader',
  emits: ['refresh'],
  template: '<button class="ops-dashboard-header" @click="$emit(\'refresh\')">header</button>'
})

const OpsConcurrencyCardStub = defineComponent({
  name: 'OpsConcurrencyCard',
  props: {
    refreshToken: {
      type: Number,
      default: 0
    }
  },
  template: '<div class="ops-concurrency-card" :data-refresh-token="refreshToken">concurrency</div>'
})

const OpsOpenAIWarmPoolCardStub = defineComponent({
  name: 'OpsOpenAIWarmPoolCard',
  props: {
    refreshToken: {
      type: Number,
      default: 0
    }
  },
  template: '<div class="ops-openai-warm-pool-card" :data-refresh-token="refreshToken">warm-pool</div>'
})

function createDeferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

const snapshotResponse = {
  generated_at: '2026-04-01T01:00:00Z',
  overview: {
    start_time: '2026-04-01T00:00:00Z',
    end_time: '2026-04-01T01:00:00Z',
    platform: 'openai',
    group_id: null,
    success_count: 100,
    error_count_total: 2,
    business_limited_count: 1,
    rectifier_retry_count: 0,
    error_count_sla: 1,
    request_count_total: 102,
    request_count_sla: 101,
    token_consumed: 2048,
    sla: 0.99,
    error_rate: 0.01,
    upstream_error_rate: 0.005,
    upstream_error_count_excl_429_529: 1,
    upstream_429_count: 0,
    upstream_529_count: 0,
    qps: {
      current: 1,
      peak: 2,
      avg: 1.5
    },
    tps: {
      current: 10,
      peak: 20,
      avg: 15
    },
    duration: {
      p50_ms: 100,
      p90_ms: 200,
      p95_ms: 250,
      p99_ms: 300,
      avg_ms: 150,
      max_ms: 400
    },
    ttft: {
      p50_ms: 50,
      p90_ms: 80,
      p95_ms: 90,
      p99_ms: 100,
      avg_ms: 60,
      max_ms: 120
    }
  },
  throughput_trend: {
    bucket: '1m',
    points: []
  },
  error_trend: {
    bucket: '1m',
    points: []
  }
}

const switchTrendResponse = {
  bucket: '1m',
  points: []
}

const advancedSettingsResponse = {
  display_alert_events: true,
  display_openai_token_stats: false,
  auto_refresh_enabled: false,
  auto_refresh_interval_seconds: 30
}

function mountDashboard() {
  return mount(OpsDashboard, {
    global: {
      stubs: {
        AppLayout: AppLayoutStub,
        BaseDialog: true,
        OpsDashboardSkeleton: OpsDashboardSkeletonStub,
        OpsDashboardHeader: OpsDashboardHeaderStub,
        OpsConcurrencyCard: OpsConcurrencyCardStub,
        OpsOpenAIWarmPoolCard: OpsOpenAIWarmPoolCardStub,
        OpsSwitchRateTrendChart: true,
        OpsThroughputTrendChart: true,
        OpsLatencyChart: true,
        OpsErrorDistributionChart: true,
        OpsErrorTrendChart: true,
        OpsAlertEventsCard: true,
        OpsOpenAITokenStatsCard: true,
        OpsSystemLogTable: true,
        OpsErrorDetailsModal: true,
        OpsErrorDetailModal: true,
        OpsRequestDetailsModal: true,
        OpsSettingsDialog: true,
        OpsAlertRulesCard: true
      }
    }
  })
}

describe('OpsDashboard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    routeState.query = {}
    adminSettingsStoreState.opsMonitoringEnabled = true
    adminSettingsStoreState.opsQueryModeDefault = 'auto'
    mockAdminSettingsFetch.mockResolvedValue(undefined)
    mockGetMetricThresholds.mockResolvedValue({})
    mockGetDashboardOverview.mockResolvedValue(snapshotResponse.overview)
    mockGetErrorTrend.mockResolvedValue(snapshotResponse.error_trend)
    mockGetLatencyHistogram.mockResolvedValue({
      start_time: '2026-04-01T00:00:00Z',
      end_time: '2026-04-01T01:00:00Z',
      platform: 'openai',
      total_requests: 0,
      buckets: []
    })
    mockGetErrorDistribution.mockResolvedValue({
      total: 0,
      items: []
    })
  })

  it('确认 ops 已开启后并行启动首刷，并避免首刷对子卡再打一次 refresh token', async () => {
    const pendingAdvancedSettings = createDeferred<typeof advancedSettingsResponse>()
    const pendingSnapshot = createDeferred<typeof snapshotResponse>()
    const pendingSwitchTrend = createDeferred<typeof switchTrendResponse>()

    mockGetAdvancedSettings.mockImplementationOnce(() => pendingAdvancedSettings.promise)
    mockGetDashboardSnapshotV2
      .mockImplementationOnce(() => pendingSnapshot.promise)
      .mockResolvedValue(snapshotResponse)
    mockGetThroughputTrend
      .mockImplementationOnce(() => pendingSwitchTrend.promise)
      .mockResolvedValue(switchTrendResponse)

    const wrapper = mountDashboard()
    await flushPromises()

    expect(mockAdminSettingsFetch).toHaveBeenCalledTimes(1)
    expect(mockGetAdvancedSettings).toHaveBeenCalledTimes(1)
    expect(mockGetDashboardSnapshotV2).toHaveBeenCalledTimes(1)
    expect(mockGetThroughputTrend).toHaveBeenCalledTimes(1)
    expect(wrapper.find('.ops-dashboard-skeleton').exists()).toBe(true)
    expect(wrapper.find('.ops-concurrency-card').exists()).toBe(true)
    expect(wrapper.find('.ops-openai-warm-pool-card').exists()).toBe(true)

    pendingAdvancedSettings.resolve(advancedSettingsResponse)
    pendingSnapshot.resolve(snapshotResponse)
    pendingSwitchTrend.resolve(switchTrendResponse)
    await flushPromises()

    expect(wrapper.find('.ops-dashboard-skeleton').exists()).toBe(false)
    expect(wrapper.find('.ops-dashboard-header').exists()).toBe(true)
    expect(wrapper.find('.ops-concurrency-card').attributes('data-refresh-token')).toBe('0')
    expect(wrapper.find('.ops-openai-warm-pool-card').attributes('data-refresh-token')).toBe('0')

    await wrapper.find('.ops-dashboard-header').trigger('click')
    await flushPromises()

    expect(mockGetDashboardSnapshotV2).toHaveBeenCalledTimes(2)
    expect(mockGetThroughputTrend).toHaveBeenCalledTimes(2)
    expect(wrapper.find('.ops-concurrency-card').attributes('data-refresh-token')).toBe('1')
    expect(wrapper.find('.ops-openai-warm-pool-card').attributes('data-refresh-token')).toBe('1')
  })
})
