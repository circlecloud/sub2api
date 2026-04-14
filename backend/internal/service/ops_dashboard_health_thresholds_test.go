package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type dashboardHealthThresholdRepo struct {
	*opsRepoMock
	overview *OpsDashboardOverview
}

type dashboardHealthThresholdSettingRepo struct {
	values map[string]string
}

func newDashboardHealthThresholdSettingRepo() *dashboardHealthThresholdSettingRepo {
	return &dashboardHealthThresholdSettingRepo{values: make(map[string]string)}
}

func (r *dashboardHealthThresholdSettingRepo) Get(ctx context.Context, key string) (*Setting, error) {
	if r == nil {
		return nil, ErrSettingNotFound
	}
	v, ok := r.values[key]
	if !ok {
		return nil, ErrSettingNotFound
	}
	return &Setting{Key: key, Value: v}, nil
}

func (r *dashboardHealthThresholdSettingRepo) GetValue(ctx context.Context, key string) (string, error) {
	if r == nil {
		return "", ErrSettingNotFound
	}
	v, ok := r.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return v, nil
}

func (r *dashboardHealthThresholdSettingRepo) Set(ctx context.Context, key, value string) error {
	if r == nil {
		return nil
	}
	r.values[key] = value
	return nil
}

func (r *dashboardHealthThresholdSettingRepo) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if v, ok := r.values[key]; ok {
			out[key] = v
		}
	}
	return out, nil
}

func (r *dashboardHealthThresholdSettingRepo) SetMultiple(ctx context.Context, settings map[string]string) error {
	for key, value := range settings {
		r.values[key] = value
	}
	return nil
}

func (r *dashboardHealthThresholdSettingRepo) GetAll(ctx context.Context) (map[string]string, error) {
	out := make(map[string]string, len(r.values))
	for key, value := range r.values {
		out[key] = value
	}
	return out, nil
}

func (r *dashboardHealthThresholdSettingRepo) Delete(ctx context.Context, key string) error {
	delete(r.values, key)
	return nil
}

func newDashboardHealthThresholdRepo(overview *OpsDashboardOverview) *dashboardHealthThresholdRepo {
	return &dashboardHealthThresholdRepo{
		opsRepoMock: &opsRepoMock{},
		overview:    overview,
	}
}

func (r *dashboardHealthThresholdRepo) GetDashboardOverview(ctx context.Context, filter *OpsDashboardFilter) (*OpsDashboardOverview, error) {
	if r == nil || r.overview == nil {
		return &OpsDashboardOverview{}, nil
	}
	copyOverview := *r.overview
	return &copyOverview, nil
}

func (r *dashboardHealthThresholdRepo) GetLatestSystemMetrics(ctx context.Context, windowMinutes int) (*OpsSystemMetricsSnapshot, error) {
	return nil, sql.ErrNoRows
}

func (r *dashboardHealthThresholdRepo) ListJobHeartbeats(ctx context.Context) ([]*OpsJobHeartbeat, error) {
	return nil, nil
}

func TestOpsServiceGetDashboardOverview_HealthScoreUsesConfiguredMetricThresholds(t *testing.T) {
	t.Parallel()

	strictRepo := newDashboardHealthThresholdSettingRepo()
	strictThresholds := &OpsMetricThresholds{
		SLAPercentMin:               float64Ptr(99.5),
		TTFTp99MsMax:                float64Ptr(500),
		RequestErrorRatePercentMax:  float64Ptr(5),
		UpstreamErrorRatePercentMax: float64Ptr(5),
	}
	strictRaw, err := json.Marshal(strictThresholds)
	require.NoError(t, err)
	require.NoError(t, strictRepo.Set(context.Background(), SettingKeyOpsMetricThresholds, string(strictRaw)))

	looseRepo := newDashboardHealthThresholdSettingRepo()
	looseThresholds := &OpsMetricThresholds{
		SLAPercentMin:               float64Ptr(95),
		TTFTp99MsMax:                float64Ptr(2000),
		RequestErrorRatePercentMax:  float64Ptr(10),
		UpstreamErrorRatePercentMax: float64Ptr(10),
	}
	looseRaw, err := json.Marshal(looseThresholds)
	require.NoError(t, err)
	require.NoError(t, looseRepo.Set(context.Background(), SettingKeyOpsMetricThresholds, string(looseRaw)))

	makeOverview := func() *OpsDashboardOverview {
		return &OpsDashboardOverview{
			RequestCountTotal: 100,
			RequestCountSLA:   100,
			SuccessCount:      96,
			ErrorCountTotal:   4,
			ErrorCountSLA:     4,
			SLA:               0.96,
			ErrorRate:         0.04,
			UpstreamErrorRate: 0,
			TTFT:              OpsPercentiles{P99: intPtr(1500)},
			SystemMetrics: &OpsSystemMetricsSnapshot{
				DBOK:               boolPtr(true),
				RedisOK:            boolPtr(true),
				CPUUsagePercent:    float64Ptr(30),
				MemoryUsagePercent: float64Ptr(40),
			},
		}
	}

	filter := &OpsDashboardFilter{
		StartTime: time.Now().UTC().Add(-5 * time.Minute),
		EndTime:   time.Now().UTC(),
	}

	strictSvc := NewOpsService(newDashboardHealthThresholdRepo(makeOverview()), strictRepo, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	strictOverview, err := strictSvc.GetDashboardOverview(context.Background(), filter)
	require.NoError(t, err)

	looseSvc := NewOpsService(newDashboardHealthThresholdRepo(makeOverview()), looseRepo, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	looseOverview, err := looseSvc.GetDashboardOverview(context.Background(), filter)
	require.NoError(t, err)

	require.NotNil(t, strictOverview)
	require.NotNil(t, looseOverview)
	require.Less(t, strictOverview.HealthScore, looseOverview.HealthScore)
}
