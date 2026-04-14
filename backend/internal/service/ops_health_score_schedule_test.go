package service

import (
	"testing"
	"time"
)

func TestComputeInfraHealth_ScheduledCleanupJobUsesDailyThreshold(t *testing.T) {
	now := time.Date(2026, 4, 9, 10, 0, 0, 0, time.UTC)
	t.Run("within daily cadence stays healthy", func(t *testing.T) {
		lastSuccessAt := now.Add(-8 * time.Hour)
		score := computeInfraHealth(now, &OpsDashboardOverview{
			RequestCountTotal: 1000,
			SystemMetrics: &OpsSystemMetricsSnapshot{
				DBOK:               boolPtr(true),
				RedisOK:            boolPtr(true),
				CPUUsagePercent:    float64Ptr(30),
				MemoryUsagePercent: float64Ptr(40),
			},
			JobHeartbeats: []*OpsJobHeartbeat{{
				JobName:       opsCleanupJobName,
				LastSuccessAt: &lastSuccessAt,
			}},
		})

		if score != 100 {
			t.Fatalf("computeInfraHealth() = %.1f, want 100 for daily cleanup job within expected cadence", score)
		}
	})

	t.Run("missed daily cadence is still penalized", func(t *testing.T) {
		lastSuccessAt := now.Add(-31 * time.Hour)
		score := computeInfraHealth(now, &OpsDashboardOverview{
			RequestCountTotal: 1000,
			SystemMetrics: &OpsSystemMetricsSnapshot{
				DBOK:               boolPtr(true),
				RedisOK:            boolPtr(true),
				CPUUsagePercent:    float64Ptr(30),
				MemoryUsagePercent: float64Ptr(40),
			},
			JobHeartbeats: []*OpsJobHeartbeat{{
				JobName:       opsCleanupJobName,
				LastSuccessAt: &lastSuccessAt,
			}},
		})

		if score >= 100 {
			t.Fatalf("computeInfraHealth() = %.1f, want < 100 when cleanup job misses daily cadence", score)
		}
	})
}
