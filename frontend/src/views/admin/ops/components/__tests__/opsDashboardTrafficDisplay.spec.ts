import { describe, expect, it } from 'vitest'

import {
  formatRealtimeTrafficAverage,
  formatRealtimeTrafficPrimary,
  isRealtimeTrafficIdle,
} from '../opsDashboardTrafficDisplay'

describe('opsDashboardTrafficDisplay', () => {
  it('低流量时优先显示 QPM', () => {
    expect(formatRealtimeTrafficPrimary({ qpsCurrent: 0, recentRequestCount: 2 })).toEqual({
      value: '2',
      unit: 'QPM',
    })
  })

  it('高于阈值时继续显示 QPS', () => {
    expect(formatRealtimeTrafficPrimary({ qpsCurrent: 1.26, recentRequestCount: 76 })).toEqual({
      value: '1.3',
      unit: 'QPS',
    })
  })

  it('低流量的平均值会改用 QPM 展示', () => {
    expect(formatRealtimeTrafficAverage({ avgRequestCount: 2, avgWindowMinutes: 60 })).toEqual({
      value: '0.03',
      unit: 'QPM',
    })
  })

  it('平均值足够高时仍然显示 QPS', () => {
    expect(formatRealtimeTrafficAverage({ avgRequestCount: 720, avgWindowMinutes: 60 })).toEqual({
      value: '0.2',
      unit: 'QPS',
    })
  })

  it('待机判定基于原始最近请求数而不是四舍五入后的 QPS', () => {
    expect(isRealtimeTrafficIdle(2, 0)).toBe(false)
    expect(isRealtimeTrafficIdle(0, 0)).toBe(true)
    expect(isRealtimeTrafficIdle(undefined, 0.02)).toBe(false)
  })
})
