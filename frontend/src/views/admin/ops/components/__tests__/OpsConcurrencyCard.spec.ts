import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import OpsConcurrencyCard from '../OpsConcurrencyCard.vue'

const mockGetConcurrencyStats = vi.fn()
const mockGetAccountAvailabilityStats = vi.fn()
const mockGetUserConcurrencyStats = vi.fn()

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    getConcurrencyStats: (...args: any[]) => mockGetConcurrencyStats(...args),
    getAccountAvailabilityStats: (...args: any[]) => mockGetAccountAvailabilityStats(...args),
    getUserConcurrencyStats: (...args: any[]) => mockGetUserConcurrencyStats(...args)
  }
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, any>) => {
        if (params?.count !== undefined) return `${key}:${params.count}`
        return key
      }
    })
  }
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

const sampleConcurrencyResponse = {
  enabled: true,
  platform: {
    openai: {
      platform: 'openai',
      current_in_use: 2,
      max_capacity: 10,
      load_percentage: 20,
      waiting_in_queue: 1
    }
  }
}

const sampleAvailabilityResponse = {
  enabled: true,
  platform: {
    openai: {
      platform: 'openai',
      total_accounts: 3,
      available_count: 2,
      rate_limit_count: 0,
      error_count: 0
    }
  }
}

describe('OpsConcurrencyCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockGetUserConcurrencyStats.mockResolvedValue({ enabled: true, user: {} })
  })

  it('首挂只加载一次，并在无数据时优先展示局部 loading', async () => {
    const pendingConcurrency = createDeferred<typeof sampleConcurrencyResponse>()
    const pendingAvailability = createDeferred<typeof sampleAvailabilityResponse>()

    mockGetConcurrencyStats
      .mockImplementationOnce(() => pendingConcurrency.promise)
      .mockResolvedValue(sampleConcurrencyResponse)
    mockGetAccountAvailabilityStats
      .mockImplementationOnce(() => pendingAvailability.promise)
      .mockResolvedValue(sampleAvailabilityResponse)

    const wrapper = mount(OpsConcurrencyCard, {
      props: {
        refreshToken: 0
      }
    })

    await flushPromises()

    expect(mockGetConcurrencyStats).toHaveBeenCalledTimes(1)
    expect(mockGetAccountAvailabilityStats).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('admin.ops.loadingText')
    expect(wrapper.text()).not.toContain('admin.ops.concurrency.empty')

    pendingConcurrency.resolve(sampleConcurrencyResponse)
    pendingAvailability.resolve(sampleAvailabilityResponse)
    await flushPromises()

    expect(wrapper.text()).toContain('OPENAI')
    expect(wrapper.text()).toContain('admin.ops.concurrency.totalRows:1')

    await wrapper.setProps({ refreshToken: 1 })
    await flushPromises()

    expect(mockGetConcurrencyStats).toHaveBeenCalledTimes(2)
    expect(mockGetAccountAvailabilityStats).toHaveBeenCalledTimes(2)
  })
})
