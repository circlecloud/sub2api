package service

import (
	"math"
	"time"
)

const (
	opsHealthErrorHealthyRatio = 0.2
	opsHealthErrorZeroRatio    = 2.0
	opsHealthTTFTHealthyRatio  = 2.0
	opsHealthTTFTZeroRatio     = 6.0
	opsHealthSLAHealthyBudget  = 10.0
	opsHealthSLAZeroBudget     = 20.0
)

// computeDashboardHealthScore computes a 0-100 health score from the metrics returned by the dashboard overview.
//
// Design goals:
// - Backend-owned scoring (UI only displays).
// - Layered scoring: Business Health (70%) + Infrastructure Health (30%)
// - Avoids double-counting (e.g., DB failure affects both infra and business metrics)
// - Conservative + stable: penalize clear degradations; avoid overreacting to missing/idle data.
//
//nolint:unused // 预留给后续 dashboard 统一健康分入口
func computeDashboardHealthScore(now time.Time, overview *OpsDashboardOverview) int {
	return computeDashboardHealthScoreWithThresholds(now, overview, defaultOpsMetricThresholds())
}

func computeDashboardHealthScoreWithThresholds(now time.Time, overview *OpsDashboardOverview, thresholds *OpsMetricThresholds) int {
	if overview == nil {
		return 0
	}

	// Idle/no-data: avoid showing a "bad" score when there is no traffic.
	// UI can still render a gray/idle state based on QPS + error rate.
	if overview.RequestCountSLA <= 0 && overview.RequestCountTotal <= 0 && overview.ErrorCountTotal <= 0 {
		return 100
	}

	businessHealth := computeBusinessHealthWithThresholds(overview, thresholds)
	infraHealth := computeInfraHealth(now, overview)

	// Weighted combination: 70% business + 30% infrastructure
	score := businessHealth*0.7 + infraHealth*0.3
	return int(math.Round(clampFloat64(score, 0, 100)))
}

// computeBusinessHealth calculates business health score (0-100)
// Components: Availability/Error (50%) + TTFT (50%)
//
//nolint:unused // 预留给后续 dashboard 业务健康分入口
func computeBusinessHealth(overview *OpsDashboardOverview) float64 {
	return computeBusinessHealthWithThresholds(overview, defaultOpsMetricThresholds())
}

func computeBusinessHealthWithThresholds(overview *OpsDashboardOverview, thresholds *OpsMetricThresholds) float64 {
	if overview == nil {
		return 0
	}
	cfg := resolveOpsMetricThresholds(thresholds)

	availabilityScore := 100.0
	if overview.RequestCountSLA > 0 {
		slaPercent := clampFloat64(overview.SLA*100, 0, 100)
		availabilityScore = scoreLowerIsWorseByThreshold(slaPercent, cfg.slaPercentMin, opsHealthSLAHealthyBudget, opsHealthSLAZeroBudget)
	}

	requestErrorScore := scoreHigherIsWorseByThreshold(
		clampFloat64(overview.ErrorRate*100, 0, 100),
		cfg.requestErrorRatePercentMax,
		opsHealthErrorHealthyRatio,
		opsHealthErrorZeroRatio,
	)
	upstreamErrorScore := scoreHigherIsWorseByThreshold(
		clampFloat64(overview.UpstreamErrorRate*100, 0, 100),
		cfg.upstreamErrorRatePercentMax,
		opsHealthErrorHealthyRatio,
		opsHealthErrorZeroRatio,
	)
	availabilityScore = math.Min(availabilityScore, math.Min(requestErrorScore, upstreamErrorScore))

	// TTFT score: scale against configured threshold so the dashboard tracks the same runtime settings as diagnostics/alerts.
	ttftScore := 100.0
	if overview.TTFT.P99 != nil {
		ttftScore = scoreHigherIsWorseByThreshold(
			float64(*overview.TTFT.P99),
			cfg.ttftp99MsMax,
			opsHealthTTFTHealthyRatio,
			opsHealthTTFTZeroRatio,
		)
	}

	// Weighted combination: 50% availability/error + 50% TTFT
	return availabilityScore*0.5 + ttftScore*0.5
}

// computeInfraHealth calculates infrastructure health score (0-100)
// Components: Storage (40%) + Compute Resources (30%) + Background Jobs (30%)
func computeInfraHealth(now time.Time, overview *OpsDashboardOverview) float64 {
	// Storage score: DB critical, Redis less critical
	storageScore := 100.0
	if overview.SystemMetrics != nil {
		if overview.SystemMetrics.DBOK != nil && !*overview.SystemMetrics.DBOK {
			storageScore = 0 // DB failure is critical
		} else if overview.SystemMetrics.RedisOK != nil && !*overview.SystemMetrics.RedisOK {
			storageScore = 50 // Redis failure is degraded but not critical
		}
	}

	// Compute resources score: CPU + Memory
	computeScore := 100.0
	if overview.SystemMetrics != nil {
		cpuScore := 100.0
		if overview.SystemMetrics.CPUUsagePercent != nil {
			cpuPct := clampFloat64(*overview.SystemMetrics.CPUUsagePercent, 0, 100)
			if cpuPct > 80 {
				if cpuPct <= 100 {
					cpuScore = (100 - cpuPct) / 20 * 100
				} else {
					cpuScore = 0
				}
			}
		}

		memScore := 100.0
		if overview.SystemMetrics.MemoryUsagePercent != nil {
			memPct := clampFloat64(*overview.SystemMetrics.MemoryUsagePercent, 0, 100)
			if memPct > 85 {
				if memPct <= 100 {
					memScore = (100 - memPct) / 15 * 100
				} else {
					memScore = 0
				}
			}
		}

		computeScore = (cpuScore + memScore) / 2
	}

	// Background jobs score
	jobScore := 100.0
	failedJobs := 0
	totalJobs := 0
	for _, hb := range overview.JobHeartbeats {
		if hb == nil {
			continue
		}
		totalJobs++
		if isOpsJobHeartbeatFailed(now, hb) {
			failedJobs++
		}
	}
	if totalJobs > 0 && failedJobs > 0 {
		jobScore = (1 - float64(failedJobs)/float64(totalJobs)) * 100
	}

	// Weighted combination
	return storageScore*0.4 + computeScore*0.3 + jobScore*0.3
}

type resolvedOpsMetricThresholds struct {
	slaPercentMin               float64
	ttftp99MsMax                float64
	requestErrorRatePercentMax  float64
	upstreamErrorRatePercentMax float64
}

func resolveOpsMetricThresholds(thresholds *OpsMetricThresholds) resolvedOpsMetricThresholds {
	cfg := resolvedOpsMetricThresholds{}
	if defaults := defaultOpsMetricThresholds(); defaults != nil {
		if defaults.SLAPercentMin != nil {
			cfg.slaPercentMin = *defaults.SLAPercentMin
		}
		if defaults.TTFTp99MsMax != nil {
			cfg.ttftp99MsMax = *defaults.TTFTp99MsMax
		}
		if defaults.RequestErrorRatePercentMax != nil {
			cfg.requestErrorRatePercentMax = *defaults.RequestErrorRatePercentMax
		}
		if defaults.UpstreamErrorRatePercentMax != nil {
			cfg.upstreamErrorRatePercentMax = *defaults.UpstreamErrorRatePercentMax
		}
	}
	if thresholds == nil {
		return cfg
	}
	if thresholds.SLAPercentMin != nil && *thresholds.SLAPercentMin >= 0 && *thresholds.SLAPercentMin <= 100 {
		cfg.slaPercentMin = *thresholds.SLAPercentMin
	}
	if thresholds.TTFTp99MsMax != nil && *thresholds.TTFTp99MsMax >= 0 {
		cfg.ttftp99MsMax = *thresholds.TTFTp99MsMax
	}
	if thresholds.RequestErrorRatePercentMax != nil && *thresholds.RequestErrorRatePercentMax >= 0 && *thresholds.RequestErrorRatePercentMax <= 100 {
		cfg.requestErrorRatePercentMax = *thresholds.RequestErrorRatePercentMax
	}
	if thresholds.UpstreamErrorRatePercentMax != nil && *thresholds.UpstreamErrorRatePercentMax >= 0 && *thresholds.UpstreamErrorRatePercentMax <= 100 {
		cfg.upstreamErrorRatePercentMax = *thresholds.UpstreamErrorRatePercentMax
	}
	return cfg
}

func scoreHigherIsWorseByThreshold(value float64, threshold float64, healthyRatio float64, zeroRatio float64) float64 {
	value = math.Max(value, 0)
	threshold = math.Max(threshold, 0)
	if threshold == 0 {
		if value <= 0 {
			return 100
		}
		return 0
	}
	healthy := threshold * healthyRatio
	zero := threshold * zeroRatio
	if zero <= healthy {
		if value <= healthy {
			return 100
		}
		return 0
	}
	if value <= healthy {
		return 100
	}
	if value >= zero {
		return 0
	}
	return ((zero - value) / (zero - healthy)) * 100
}

func scoreLowerIsWorseByThreshold(value float64, threshold float64, healthyBudgetMultiplier float64, zeroBudgetMultiplier float64) float64 {
	value = clampFloat64(value, 0, 100)
	threshold = clampFloat64(threshold, 0, 100)
	failureBudget := math.Max(100-threshold, 0.1)
	healthyDeficit := failureBudget * healthyBudgetMultiplier
	zeroDeficit := failureBudget * zeroBudgetMultiplier
	currentDeficit := 100 - value
	if currentDeficit <= healthyDeficit {
		return 100
	}
	if currentDeficit >= zeroDeficit {
		return 0
	}
	return ((zeroDeficit - currentDeficit) / (zeroDeficit - healthyDeficit)) * 100
}

func isOpsJobHeartbeatFailed(now time.Time, hb *OpsJobHeartbeat) bool {
	if hb == nil {
		return false
	}
	if hb.LastErrorAt != nil && (hb.LastSuccessAt == nil || hb.LastErrorAt.After(*hb.LastSuccessAt)) {
		return true
	}
	if hb.LastSuccessAt != nil {
		return now.Sub(*hb.LastSuccessAt) > opsJobHeartbeatStaleThreshold(hb.JobName)
	}
	return false
}

func opsJobHeartbeatStaleThreshold(jobName string) time.Duration {
	switch jobName {
	case opsCleanupJobName:
		return 30 * time.Hour
	case opsAggDailyJobName:
		return 3 * opsAggDailyInterval
	case opsAggHourlyJobName:
		return 3 * opsAggHourlyInterval
	default:
		return 15 * time.Minute
	}
}

func clampFloat64(v float64, min float64, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
