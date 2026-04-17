package admin

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestApplyAdminOpenAISettingsUpdate_UsesRequestOverridesAndSanitizesWarmPool(t *testing.T) {
	previous := &service.SystemSettings{
		EnableOpenAIStreamRectifier:                 true,
		OpenAIStreamResponseHeaderRectifierTimeouts: []int{8, 10, 12},
		OpenAIStreamFirstTokenRectifierTimeouts:     []int{5, 8, 10},
		OpenAIUsageProbeMethod:                      "responses",
		OpenAIWarmPoolEnabled:                       true,
		OpenAIWarmPoolBucketTargetSize:              10,
		OpenAIWarmPoolBucketRefillBelow:             3,
		OpenAIWarmPoolBucketSyncFillMin:             1,
		OpenAIWarmPoolBucketEntryTTLSeconds:         30,
		OpenAIWarmPoolBucketRefillCooldownSeconds:   15,
		OpenAIWarmPoolBucketRefillIntervalSeconds:   30,
		OpenAIWarmPoolGlobalTargetSize:              30,
		OpenAIWarmPoolGlobalRefillBelow:             10,
		OpenAIWarmPoolGlobalEntryTTLSeconds:         300,
		OpenAIWarmPoolGlobalRefillCooldownSeconds:   60,
		OpenAIWarmPoolGlobalRefillIntervalSeconds:   300,
		OpenAIWarmPoolNetworkErrorPoolSize:          3,
		OpenAIWarmPoolNetworkErrorEntryTTLSeconds:   120,
		OpenAIWarmPoolProbeMaxCandidates:            24,
		OpenAIWarmPoolProbeConcurrency:              4,
		OpenAIWarmPoolProbeTimeoutSeconds:           15,
		OpenAIWarmPoolProbeFailureCooldownSeconds:   120,
		OpenAIWarmPoolStartupGroupIDs:               []int64{5, 7},
	}
	requestTimeouts := []int{11, 13}
	requestFirstToken := []int{7, 9}
	probeMethod := "WHAM"
	warmPoolEnabled := false
	warmPoolBucketTargetSize := 12
	warmPoolBucketRefillBelow := 15
	startupGroups := []int64{9, 3, 9}
	settings := &service.SystemSettings{}

	applyAdminOpenAISettingsUpdate(settings, previous, UpdateSettingsRequest{
		EnableOpenAIStreamRectifier:                 boolPtr(false),
		OpenAIStreamResponseHeaderRectifierTimeouts: &requestTimeouts,
		OpenAIStreamFirstTokenRectifierTimeouts:     &requestFirstToken,
		OpenAIUsageProbeMethod:                      &probeMethod,
		OpenAIWarmPoolEnabled:                       &warmPoolEnabled,
		OpenAIWarmPoolBucketTargetSize:              &warmPoolBucketTargetSize,
		OpenAIWarmPoolBucketRefillBelow:             &warmPoolBucketRefillBelow,
		OpenAIWarmPoolStartupGroupIDs:               &startupGroups,
	})

	require.False(t, settings.EnableOpenAIStreamRectifier)
	require.Equal(t, []int{11, 13}, settings.OpenAIStreamResponseHeaderRectifierTimeouts)
	require.Equal(t, []int{7, 9}, settings.OpenAIStreamFirstTokenRectifierTimeouts)
	require.Equal(t, "wham", settings.OpenAIUsageProbeMethod)
	require.False(t, settings.OpenAIWarmPoolEnabled)
	require.Equal(t, 12, settings.OpenAIWarmPoolBucketTargetSize)
	require.Equal(t, 11, settings.OpenAIWarmPoolBucketRefillBelow)
	require.Equal(t, []int64{3, 9}, settings.OpenAIWarmPoolStartupGroupIDs)
}

func TestApplyOpenAISettingsDTO_StartupGroupIDsAlwaysArray(t *testing.T) {
	resp := dto.SystemSettings{}
	applyOpenAISettingsDTO(&resp, &service.SystemSettings{})
	require.NotNil(t, resp.OpenAIWarmPoolStartupGroupIDs)
	require.Len(t, resp.OpenAIWarmPoolStartupGroupIDs, 0)
}

func TestAppendAdminOpenAIDiff_ReportsOnlyOpenAIChanges(t *testing.T) {
	before := &service.SystemSettings{
		EnableOpenAIStreamRectifier:                 true,
		OpenAIStreamResponseHeaderRectifierTimeouts: []int{8, 10, 12},
		OpenAIStreamFirstTokenRectifierTimeouts:     []int{5, 8, 10},
		OpenAIUsageProbeMethod:                      "responses",
		OpenAIWarmPoolStartupGroupIDs:               []int64{3},
	}
	after := &service.SystemSettings{
		EnableOpenAIStreamRectifier:                 false,
		OpenAIStreamResponseHeaderRectifierTimeouts: []int{11, 13},
		OpenAIStreamFirstTokenRectifierTimeouts:     []int{5, 8, 10},
		OpenAIUsageProbeMethod:                      "wham",
		OpenAIWarmPoolStartupGroupIDs:               []int64{3, 9},
	}

	changed := appendAdminOpenAIDiff(nil, before, after)
	require.Equal(t, []string{
		"enable_openai_stream_rectifier",
		"openai_stream_response_header_rectifier_timeouts",
		"openai_usage_probe_method",
		"openai_warm_pool_startup_group_ids",
	}, changed)
}

func boolPtr(v bool) *bool {
	return &v
}
