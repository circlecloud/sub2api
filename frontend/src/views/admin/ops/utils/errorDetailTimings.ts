import type { OpsErrorDetail } from '@/api/admin/ops'

export type OpsErrorTimingKey =
  | 'auth'
  | 'routing'
  | 'prepare'
  | 'upstream'
  | 'response'
  | 'ttft'
  | 'firstEvent'

export interface OpsErrorTimingItem {
  key: OpsErrorTimingKey
  value: number
}

function normalizeTiming(value: number | null | undefined): number | null {
  if (typeof value !== 'number' || !Number.isFinite(value) || value < 0) {
    return null
  }
  return value
}

export function extractErrorDetailTimings(detail: OpsErrorDetail | null | undefined): OpsErrorTimingItem[] {
  if (!detail) return []

  const candidates: Array<[OpsErrorTimingKey, number | null | undefined]> = [
    ['auth', detail.auth_latency_ms],
    ['routing', detail.routing_latency_ms],
    ['prepare', detail.gateway_prepare_latency_ms],
    ['upstream', detail.upstream_latency_ms],
    ['response', detail.response_latency_ms],
    ['ttft', detail.time_to_first_token_ms],
    ['firstEvent', detail.stream_first_event_latency_ms]
  ]

  return candidates.flatMap(([key, value]) => {
    const normalized = normalizeTiming(value)
    return normalized === null ? [] : [{ key, value: normalized }]
  })
}
