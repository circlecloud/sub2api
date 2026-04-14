import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, nextTick } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import OpsAlertEventsCard from '../OpsAlertEventsCard.vue'
import OpsSystemLogTable from '../OpsSystemLogTable.vue'

const mockListAlertEvents = vi.fn()
const mockListSystemLogs = vi.fn()
const mockGetSystemLogSinkHealth = vi.fn()
const mockGetRuntimeLogConfig = vi.fn()
const mockShowError = vi.fn()
const mockShowSuccess = vi.fn()

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    listAlertEvents: (...args: any[]) => mockListAlertEvents(...args),
    listSystemLogs: (...args: any[]) => mockListSystemLogs(...args),
    getSystemLogSinkHealth: (...args: any[]) => mockGetSystemLogSinkHealth(...args),
    getRuntimeLogConfig: (...args: any[]) => mockGetRuntimeLogConfig(...args),
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: mockShowError,
    showSuccess: mockShowSuccess,
  }),
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError: mockShowError,
    showSuccess: mockShowSuccess,
  }),
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

const SelectStub = defineComponent({
  name: 'SelectControlStub',
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
  emits: ['change', 'update:modelValue'],
  template: `
    <select
      class="select-stub"
      :value="modelValue"
      @change="$emit('change', $event.target.value); $emit('update:modelValue', $event.target.value)"
    >
      <option v-for="option in options" :key="String(option.value)" :value="String(option.value)">
        {{ option.label }}
      </option>
    </select>
  `,
})

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: {
    show: {
      type: Boolean,
      default: false,
    },
  },
  template: '<div v-if="show" class="base-dialog"><slot /></div>',
})

const PaginationStub = defineComponent({
  name: 'Pagination',
  template: '<div class="pagination-stub" />',
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

const alertEventsResponse = [
  {
    id: 101,
    rule_id: 7,
    title: 'High error rate',
    description: 'error spike detected',
    severity: 'P1',
    status: 'firing',
    email_sent: true,
    created_at: '2026-04-01T00:00:00Z',
    fired_at: '2026-04-01T00:00:00Z',
    resolved_at: null,
    dimensions: {
      platform: 'openai',
      group_id: 3,
      region: 'us',
    },
  },
]

const systemLogsResponse = {
  items: [
    {
      id: 201,
      created_at: '2026-04-01T00:00:00Z',
      level: 'info',
      component: 'service.ops',
      message: 'service ready',
      request_id: 'req-1',
      client_request_id: 'client-1',
      user_id: 1,
      account_id: 2,
      platform: 'openai',
      model: 'gpt-4o-mini',
      extra: {
        status_code: 200,
        latency_ms: 123,
        phase: 'response_header',
        timeout_ms: 8000,
        attempt: 1,
        header_attempt: 1,
        first_token_attempt: 1,
        retry_count: 1,
        retry_limit: 2,
        scheduler_bucket: '7:openai:single',
      },
    },
  ],
  total: 1,
  page: 1,
  page_size: 20,
}

const sinkHealthResponse = {
  queue_depth: 0,
  queue_capacity: 100,
  dropped_count: 0,
  write_failed_count: 0,
  written_count: 12,
  avg_write_delay_ms: 1,
}

const runtimeConfigResponse = {
  level: 'info',
  enable_sampling: false,
  sampling_initial: 100,
  sampling_thereafter: 100,
  caller: true,
  stacktrace_level: 'error',
  retention_days: 30,
}

describe('ops 列表刷新稳定性', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockGetSystemLogSinkHealth.mockResolvedValue(sinkHealthResponse)
    mockGetRuntimeLogConfig.mockResolvedValue(runtimeConfigResponse)
  })

  it('告警事件切换筛选时保留现有表格，避免 loading 替换内容区', async () => {
    const pendingReload = createDeferred<typeof alertEventsResponse>()
    mockListAlertEvents
      .mockResolvedValueOnce(alertEventsResponse)
      .mockImplementationOnce(() => pendingReload.promise)

    const wrapper = mount(OpsAlertEventsCard, {
      global: {
        stubs: {
          Select: SelectStub,
          BaseDialog: BaseDialogStub,
          Icon: true,
        },
      },
    })

    await flushPromises()

    expect(wrapper.find('table').exists()).toBe(true)
    expect(wrapper.text()).toContain('High error rate')

    const selects = wrapper.findAll('.select-stub')
    await selects[0].setValue('6h')
    await nextTick()

    expect(mockListAlertEvents).toHaveBeenCalledTimes(2)
    expect(wrapper.find('table').exists()).toBe(true)
    expect(wrapper.text()).toContain('High error rate')

    pendingReload.resolve(alertEventsResponse)
    await flushPromises()
  })

  it('系统日志查询时保留现有表格，避免 loading 替换内容区', async () => {
    const pendingReload = createDeferred<typeof systemLogsResponse>()
    mockListSystemLogs
      .mockResolvedValueOnce(systemLogsResponse)
      .mockImplementationOnce(() => pendingReload.promise)

    const wrapper = mount(OpsSystemLogTable, {
      global: {
        stubs: {
          Select: SelectStub,
          Pagination: PaginationStub,
        },
      },
    })

    await flushPromises()

    expect(wrapper.find('table').exists()).toBe(true)
    expect(wrapper.text()).toContain('service ready')

    const queryButton = wrapper.findAll('button').find(button => button.text() === '查询')
    expect(queryButton).toBeTruthy()
    await queryButton!.trigger('click')
    await nextTick()

    expect(mockListSystemLogs).toHaveBeenCalledTimes(2)
    expect(wrapper.find('table').exists()).toBe(true)
    expect(wrapper.text()).toContain('service ready')

    pendingReload.resolve(systemLogsResponse)
    await flushPromises()
  })

  it('系统日志会补充展示 rectifier 相关字段', async () => {
    mockListSystemLogs.mockResolvedValue(systemLogsResponse)

    const wrapper = mount(OpsSystemLogTable, {
      global: {
        stubs: {
          Select: SelectStub,
          Pagination: PaginationStub,
        },
      },
    })

    await flushPromises()

    const text = wrapper.text()
    expect(text).toContain('phase=response_header')
    expect(text).toContain('timeout_ms=8000')
    expect(text).toContain('attempt=1')
    expect(text).toContain('header_attempt=1')
    expect(text).toContain('first_token_attempt=1')
    expect(text).toContain('retry_count=1')
    expect(text).toContain('retry_limit=2')
    expect(text).toContain('scheduler_bucket=7:openai:single')
  })
})
