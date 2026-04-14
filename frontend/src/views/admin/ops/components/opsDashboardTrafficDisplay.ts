export interface OpsTrafficDisplaySummary {
  qpsCurrent?: number | null
  recentRequestCount?: number | null
  avgRequestCount?: number | null
  avgWindowMinutes?: number | null
}

function isFiniteNumber(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value)
}

function formatLowTrafficQpm(requestCount: number, windowMinutes: number): {
  value: string
  unit: 'QPM'
} {
  const qpm = requestCount / windowMinutes
  return {
    value: qpm >= 1 ? qpm.toFixed(1) : qpm.toFixed(2),
    unit: 'QPM',
  }
}

export function formatRealtimeTrafficPrimary(summary: OpsTrafficDisplaySummary | null | undefined): {
  value: string
  unit: 'QPS' | 'QPM'
} {
  const qpsCurrent = summary?.qpsCurrent
  if (isFiniteNumber(qpsCurrent) && qpsCurrent >= 0.1) {
    return {
      value: qpsCurrent.toFixed(1),
      unit: 'QPS',
    }
  }

  const recentRequestCount = summary?.recentRequestCount
  if (isFiniteNumber(recentRequestCount) && recentRequestCount > 0) {
    return {
      value: String(Math.round(recentRequestCount)),
      unit: 'QPM',
    }
  }

  return {
    value: '0.0',
    unit: 'QPS',
  }
}

export function formatRealtimeTrafficAverage(summary: OpsTrafficDisplaySummary | null | undefined): {
  value: string
  unit: 'QPS' | 'QPM'
} {
  const avgRequestCount = summary?.avgRequestCount
  const avgWindowMinutes = summary?.avgWindowMinutes

  if (isFiniteNumber(avgRequestCount) && isFiniteNumber(avgWindowMinutes) && avgRequestCount > 0 && avgWindowMinutes > 0) {
    const avgQps = avgRequestCount / (avgWindowMinutes * 60)
    if (avgQps >= 0.1) {
      return {
        value: avgQps.toFixed(1),
        unit: 'QPS',
      }
    }
    return formatLowTrafficQpm(avgRequestCount, avgWindowMinutes)
  }

  return {
    value: '0.0',
    unit: 'QPS',
  }
}

export function isRealtimeTrafficIdle(rawRecentRequestCount: number | null | undefined, errorRate: number | null | undefined): boolean {
  const requestCount = typeof rawRecentRequestCount === 'number' && Number.isFinite(rawRecentRequestCount)
    ? rawRecentRequestCount
    : 0
  const normalizedErrorRate = typeof errorRate === 'number' && Number.isFinite(errorRate)
    ? errorRate
    : 0
  return requestCount <= 0 && normalizedErrorRate === 0
}
