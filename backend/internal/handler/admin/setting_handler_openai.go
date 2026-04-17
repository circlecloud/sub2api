package admin

import (
	"slices"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func applyOpenAISettingsDTO(target *dto.SystemSettings, settings *service.SystemSettings) {
	if target == nil || settings == nil {
		return
	}

	target.EnableOpenAIStreamRectifier = settings.EnableOpenAIStreamRectifier
	target.OpenAIStreamResponseHeaderRectifierTimeouts = settings.OpenAIStreamResponseHeaderRectifierTimeouts
	target.OpenAIStreamFirstTokenRectifierTimeouts = settings.OpenAIStreamFirstTokenRectifierTimeouts
	target.OpenAIUsageProbeMethod = settings.OpenAIUsageProbeMethod
	target.OpenAIWarmPoolEnabled = settings.OpenAIWarmPoolEnabled
	target.OpenAIWarmPoolBucketTargetSize = settings.OpenAIWarmPoolBucketTargetSize
	target.OpenAIWarmPoolBucketRefillBelow = settings.OpenAIWarmPoolBucketRefillBelow
	target.OpenAIWarmPoolBucketSyncFillMin = settings.OpenAIWarmPoolBucketSyncFillMin
	target.OpenAIWarmPoolBucketEntryTTLSeconds = settings.OpenAIWarmPoolBucketEntryTTLSeconds
	target.OpenAIWarmPoolBucketRefillCooldownSeconds = settings.OpenAIWarmPoolBucketRefillCooldownSeconds
	target.OpenAIWarmPoolBucketRefillIntervalSeconds = settings.OpenAIWarmPoolBucketRefillIntervalSeconds
	target.OpenAIWarmPoolGlobalTargetSize = settings.OpenAIWarmPoolGlobalTargetSize
	target.OpenAIWarmPoolGlobalRefillBelow = settings.OpenAIWarmPoolGlobalRefillBelow
	target.OpenAIWarmPoolGlobalEntryTTLSeconds = settings.OpenAIWarmPoolGlobalEntryTTLSeconds
	target.OpenAIWarmPoolGlobalRefillCooldownSeconds = settings.OpenAIWarmPoolGlobalRefillCooldownSeconds
	target.OpenAIWarmPoolGlobalRefillIntervalSeconds = settings.OpenAIWarmPoolGlobalRefillIntervalSeconds
	target.OpenAIWarmPoolNetworkErrorPoolSize = settings.OpenAIWarmPoolNetworkErrorPoolSize
	target.OpenAIWarmPoolNetworkErrorEntryTTLSeconds = settings.OpenAIWarmPoolNetworkErrorEntryTTLSeconds
	target.OpenAIWarmPoolProbeMaxCandidates = settings.OpenAIWarmPoolProbeMaxCandidates
	target.OpenAIWarmPoolProbeConcurrency = settings.OpenAIWarmPoolProbeConcurrency
	target.OpenAIWarmPoolProbeTimeoutSeconds = settings.OpenAIWarmPoolProbeTimeoutSeconds
	target.OpenAIWarmPoolProbeFailureCooldownSeconds = settings.OpenAIWarmPoolProbeFailureCooldownSeconds
	target.OpenAIWarmPoolStartupGroupIDs = ensureInt64SliceForJSON(settings.OpenAIWarmPoolStartupGroupIDs)
}

func applyAdminOpenAISettingsUpdate(target *service.SystemSettings, previous *service.SystemSettings, req UpdateSettingsRequest) {
	if target == nil {
		return
	}
	if previous == nil {
		previous = &service.SystemSettings{}
	}

	target.EnableOpenAIStreamRectifier = resolveBoolSetting(req.EnableOpenAIStreamRectifier, previous.EnableOpenAIStreamRectifier)
	if req.OpenAIStreamResponseHeaderRectifierTimeouts != nil {
		target.OpenAIStreamResponseHeaderRectifierTimeouts = append([]int(nil), (*req.OpenAIStreamResponseHeaderRectifierTimeouts)...)
	} else {
		target.OpenAIStreamResponseHeaderRectifierTimeouts = append([]int(nil), previous.OpenAIStreamResponseHeaderRectifierTimeouts...)
	}
	if req.OpenAIStreamFirstTokenRectifierTimeouts != nil {
		target.OpenAIStreamFirstTokenRectifierTimeouts = append([]int(nil), (*req.OpenAIStreamFirstTokenRectifierTimeouts)...)
	} else {
		target.OpenAIStreamFirstTokenRectifierTimeouts = append([]int(nil), previous.OpenAIStreamFirstTokenRectifierTimeouts...)
	}
	target.OpenAIUsageProbeMethod = previous.OpenAIUsageProbeMethod
	if req.OpenAIUsageProbeMethod != nil {
		target.OpenAIUsageProbeMethod = service.NormalizeOpenAIUsageProbeMethod(*req.OpenAIUsageProbeMethod)
	}

	warmPoolDefaults := adminOpenAIWarmPoolSettingsFromSystemSettings(previous)
	warmPoolSettings := sanitizeAdminOpenAIWarmPoolSettings(service.OpenAIWarmPoolSettings{
		Enabled:                     resolveBoolSetting(req.OpenAIWarmPoolEnabled, warmPoolDefaults.Enabled),
		BucketTargetSize:            resolveIntSetting(req.OpenAIWarmPoolBucketTargetSize, warmPoolDefaults.BucketTargetSize),
		BucketRefillBelow:           resolveIntSetting(req.OpenAIWarmPoolBucketRefillBelow, warmPoolDefaults.BucketRefillBelow),
		BucketSyncFillMin:           resolveIntSetting(req.OpenAIWarmPoolBucketSyncFillMin, warmPoolDefaults.BucketSyncFillMin),
		BucketEntryTTLSeconds:       resolveIntSetting(req.OpenAIWarmPoolBucketEntryTTLSeconds, warmPoolDefaults.BucketEntryTTLSeconds),
		BucketRefillCooldownSeconds: resolveIntSetting(req.OpenAIWarmPoolBucketRefillCooldownSeconds, warmPoolDefaults.BucketRefillCooldownSeconds),
		BucketRefillIntervalSeconds: resolveIntSetting(req.OpenAIWarmPoolBucketRefillIntervalSeconds, warmPoolDefaults.BucketRefillIntervalSeconds),
		GlobalTargetSize:            resolveIntSetting(req.OpenAIWarmPoolGlobalTargetSize, warmPoolDefaults.GlobalTargetSize),
		GlobalRefillBelow:           resolveIntSetting(req.OpenAIWarmPoolGlobalRefillBelow, warmPoolDefaults.GlobalRefillBelow),
		GlobalEntryTTLSeconds:       resolveIntSetting(req.OpenAIWarmPoolGlobalEntryTTLSeconds, warmPoolDefaults.GlobalEntryTTLSeconds),
		GlobalRefillCooldownSeconds: resolveIntSetting(req.OpenAIWarmPoolGlobalRefillCooldownSeconds, warmPoolDefaults.GlobalRefillCooldownSeconds),
		GlobalRefillIntervalSeconds: resolveIntSetting(req.OpenAIWarmPoolGlobalRefillIntervalSeconds, warmPoolDefaults.GlobalRefillIntervalSeconds),
		NetworkErrorPoolSize:        resolveIntSetting(req.OpenAIWarmPoolNetworkErrorPoolSize, warmPoolDefaults.NetworkErrorPoolSize),
		NetworkErrorEntryTTLSeconds: resolveIntSetting(req.OpenAIWarmPoolNetworkErrorEntryTTLSeconds, warmPoolDefaults.NetworkErrorEntryTTLSeconds),
		ProbeMaxCandidates:          resolveIntSetting(req.OpenAIWarmPoolProbeMaxCandidates, warmPoolDefaults.ProbeMaxCandidates),
		ProbeConcurrency:            resolveIntSetting(req.OpenAIWarmPoolProbeConcurrency, warmPoolDefaults.ProbeConcurrency),
		ProbeTimeoutSeconds:         resolveIntSetting(req.OpenAIWarmPoolProbeTimeoutSeconds, warmPoolDefaults.ProbeTimeoutSeconds),
		ProbeFailureCooldownSeconds: resolveIntSetting(req.OpenAIWarmPoolProbeFailureCooldownSeconds, warmPoolDefaults.ProbeFailureCooldownSeconds),
		StartupGroupIDs:             resolveInt64SliceSetting(req.OpenAIWarmPoolStartupGroupIDs, warmPoolDefaults.StartupGroupIDs),
	}, warmPoolDefaults)

	target.OpenAIWarmPoolEnabled = warmPoolSettings.Enabled
	target.OpenAIWarmPoolBucketTargetSize = warmPoolSettings.BucketTargetSize
	target.OpenAIWarmPoolBucketRefillBelow = warmPoolSettings.BucketRefillBelow
	target.OpenAIWarmPoolBucketSyncFillMin = warmPoolSettings.BucketSyncFillMin
	target.OpenAIWarmPoolBucketEntryTTLSeconds = warmPoolSettings.BucketEntryTTLSeconds
	target.OpenAIWarmPoolBucketRefillCooldownSeconds = warmPoolSettings.BucketRefillCooldownSeconds
	target.OpenAIWarmPoolBucketRefillIntervalSeconds = warmPoolSettings.BucketRefillIntervalSeconds
	target.OpenAIWarmPoolGlobalTargetSize = warmPoolSettings.GlobalTargetSize
	target.OpenAIWarmPoolGlobalRefillBelow = warmPoolSettings.GlobalRefillBelow
	target.OpenAIWarmPoolGlobalEntryTTLSeconds = warmPoolSettings.GlobalEntryTTLSeconds
	target.OpenAIWarmPoolGlobalRefillCooldownSeconds = warmPoolSettings.GlobalRefillCooldownSeconds
	target.OpenAIWarmPoolGlobalRefillIntervalSeconds = warmPoolSettings.GlobalRefillIntervalSeconds
	target.OpenAIWarmPoolNetworkErrorPoolSize = warmPoolSettings.NetworkErrorPoolSize
	target.OpenAIWarmPoolNetworkErrorEntryTTLSeconds = warmPoolSettings.NetworkErrorEntryTTLSeconds
	target.OpenAIWarmPoolProbeMaxCandidates = warmPoolSettings.ProbeMaxCandidates
	target.OpenAIWarmPoolProbeConcurrency = warmPoolSettings.ProbeConcurrency
	target.OpenAIWarmPoolProbeTimeoutSeconds = warmPoolSettings.ProbeTimeoutSeconds
	target.OpenAIWarmPoolProbeFailureCooldownSeconds = warmPoolSettings.ProbeFailureCooldownSeconds
	target.OpenAIWarmPoolStartupGroupIDs = warmPoolSettings.StartupGroupIDs
}

func appendAdminOpenAIDiff(changed []string, before *service.SystemSettings, after *service.SystemSettings) []string {
	if before == nil || after == nil {
		return changed
	}
	if before.EnableOpenAIStreamRectifier != after.EnableOpenAIStreamRectifier {
		changed = append(changed, "enable_openai_stream_rectifier")
	}
	if !slices.Equal(before.OpenAIStreamResponseHeaderRectifierTimeouts, after.OpenAIStreamResponseHeaderRectifierTimeouts) {
		changed = append(changed, "openai_stream_response_header_rectifier_timeouts")
	}
	if !slices.Equal(before.OpenAIStreamFirstTokenRectifierTimeouts, after.OpenAIStreamFirstTokenRectifierTimeouts) {
		changed = append(changed, "openai_stream_first_token_rectifier_timeouts")
	}
	if before.OpenAIUsageProbeMethod != after.OpenAIUsageProbeMethod {
		changed = append(changed, "openai_usage_probe_method")
	}
	if before.OpenAIWarmPoolEnabled != after.OpenAIWarmPoolEnabled {
		changed = append(changed, "openai_warm_pool_enabled")
	}
	if before.OpenAIWarmPoolBucketTargetSize != after.OpenAIWarmPoolBucketTargetSize {
		changed = append(changed, "openai_warm_pool_bucket_target_size")
	}
	if before.OpenAIWarmPoolBucketRefillBelow != after.OpenAIWarmPoolBucketRefillBelow {
		changed = append(changed, "openai_warm_pool_bucket_refill_below")
	}
	if before.OpenAIWarmPoolBucketSyncFillMin != after.OpenAIWarmPoolBucketSyncFillMin {
		changed = append(changed, "openai_warm_pool_bucket_sync_fill_min")
	}
	if before.OpenAIWarmPoolBucketEntryTTLSeconds != after.OpenAIWarmPoolBucketEntryTTLSeconds {
		changed = append(changed, "openai_warm_pool_bucket_entry_ttl_seconds")
	}
	if before.OpenAIWarmPoolBucketRefillCooldownSeconds != after.OpenAIWarmPoolBucketRefillCooldownSeconds {
		changed = append(changed, "openai_warm_pool_bucket_refill_cooldown_seconds")
	}
	if before.OpenAIWarmPoolBucketRefillIntervalSeconds != after.OpenAIWarmPoolBucketRefillIntervalSeconds {
		changed = append(changed, "openai_warm_pool_bucket_refill_interval_seconds")
	}
	if before.OpenAIWarmPoolGlobalTargetSize != after.OpenAIWarmPoolGlobalTargetSize {
		changed = append(changed, "openai_warm_pool_global_target_size")
	}
	if before.OpenAIWarmPoolGlobalRefillBelow != after.OpenAIWarmPoolGlobalRefillBelow {
		changed = append(changed, "openai_warm_pool_global_refill_below")
	}
	if before.OpenAIWarmPoolGlobalEntryTTLSeconds != after.OpenAIWarmPoolGlobalEntryTTLSeconds {
		changed = append(changed, "openai_warm_pool_global_entry_ttl_seconds")
	}
	if before.OpenAIWarmPoolGlobalRefillCooldownSeconds != after.OpenAIWarmPoolGlobalRefillCooldownSeconds {
		changed = append(changed, "openai_warm_pool_global_refill_cooldown_seconds")
	}
	if before.OpenAIWarmPoolGlobalRefillIntervalSeconds != after.OpenAIWarmPoolGlobalRefillIntervalSeconds {
		changed = append(changed, "openai_warm_pool_global_refill_interval_seconds")
	}
	if before.OpenAIWarmPoolNetworkErrorPoolSize != after.OpenAIWarmPoolNetworkErrorPoolSize {
		changed = append(changed, "openai_warm_pool_network_error_pool_size")
	}
	if before.OpenAIWarmPoolNetworkErrorEntryTTLSeconds != after.OpenAIWarmPoolNetworkErrorEntryTTLSeconds {
		changed = append(changed, "openai_warm_pool_network_error_entry_ttl_seconds")
	}
	if before.OpenAIWarmPoolProbeMaxCandidates != after.OpenAIWarmPoolProbeMaxCandidates {
		changed = append(changed, "openai_warm_pool_probe_max_candidates")
	}
	if before.OpenAIWarmPoolProbeConcurrency != after.OpenAIWarmPoolProbeConcurrency {
		changed = append(changed, "openai_warm_pool_probe_concurrency")
	}
	if before.OpenAIWarmPoolProbeTimeoutSeconds != after.OpenAIWarmPoolProbeTimeoutSeconds {
		changed = append(changed, "openai_warm_pool_probe_timeout_seconds")
	}
	if before.OpenAIWarmPoolProbeFailureCooldownSeconds != after.OpenAIWarmPoolProbeFailureCooldownSeconds {
		changed = append(changed, "openai_warm_pool_probe_failure_cooldown_seconds")
	}
	if !slices.Equal(before.OpenAIWarmPoolStartupGroupIDs, after.OpenAIWarmPoolStartupGroupIDs) {
		changed = append(changed, "openai_warm_pool_startup_group_ids")
	}
	return changed
}

func adminOpenAIWarmPoolSettingsFromSystemSettings(settings *service.SystemSettings) service.OpenAIWarmPoolSettings {
	if settings == nil {
		return service.OpenAIWarmPoolSettings{}
	}
	return service.OpenAIWarmPoolSettings{
		Enabled:                     settings.OpenAIWarmPoolEnabled,
		BucketTargetSize:            settings.OpenAIWarmPoolBucketTargetSize,
		BucketRefillBelow:           settings.OpenAIWarmPoolBucketRefillBelow,
		BucketSyncFillMin:           settings.OpenAIWarmPoolBucketSyncFillMin,
		BucketEntryTTLSeconds:       settings.OpenAIWarmPoolBucketEntryTTLSeconds,
		BucketRefillCooldownSeconds: settings.OpenAIWarmPoolBucketRefillCooldownSeconds,
		BucketRefillIntervalSeconds: settings.OpenAIWarmPoolBucketRefillIntervalSeconds,
		GlobalTargetSize:            settings.OpenAIWarmPoolGlobalTargetSize,
		GlobalRefillBelow:           settings.OpenAIWarmPoolGlobalRefillBelow,
		GlobalEntryTTLSeconds:       settings.OpenAIWarmPoolGlobalEntryTTLSeconds,
		GlobalRefillCooldownSeconds: settings.OpenAIWarmPoolGlobalRefillCooldownSeconds,
		GlobalRefillIntervalSeconds: settings.OpenAIWarmPoolGlobalRefillIntervalSeconds,
		NetworkErrorPoolSize:        settings.OpenAIWarmPoolNetworkErrorPoolSize,
		NetworkErrorEntryTTLSeconds: settings.OpenAIWarmPoolNetworkErrorEntryTTLSeconds,
		ProbeMaxCandidates:          settings.OpenAIWarmPoolProbeMaxCandidates,
		ProbeConcurrency:            settings.OpenAIWarmPoolProbeConcurrency,
		ProbeTimeoutSeconds:         settings.OpenAIWarmPoolProbeTimeoutSeconds,
		ProbeFailureCooldownSeconds: settings.OpenAIWarmPoolProbeFailureCooldownSeconds,
		StartupGroupIDs:             ensureInt64SliceForJSON(settings.OpenAIWarmPoolStartupGroupIDs),
	}
}

func ensureInt64SliceForJSON(values []int64) []int64 {
	if len(values) == 0 {
		return make([]int64, 0)
	}
	return append([]int64(nil), values...)
}

func resolveBoolSetting(value *bool, fallback bool) bool {
	if value != nil {
		return *value
	}
	return fallback
}

func resolveIntSetting(value *int, fallback int) int {
	if value != nil {
		return *value
	}
	return fallback
}

func resolveInt64SliceSetting(value *[]int64, fallback []int64) []int64 {
	if value == nil {
		return append([]int64(nil), fallback...)
	}
	result := make([]int64, 0, len(*value))
	seen := make(map[int64]struct{}, len(*value))
	for _, item := range *value {
		if item <= 0 {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	slices.Sort(result)
	return result
}

func sanitizeAdminOpenAIWarmPoolSettings(input service.OpenAIWarmPoolSettings, fallback service.OpenAIWarmPoolSettings) service.OpenAIWarmPoolSettings {
	result := input
	if result.BucketTargetSize <= 0 {
		result.BucketTargetSize = fallback.BucketTargetSize
	}
	if result.BucketTargetSize <= 0 {
		result.BucketTargetSize = 10
	}
	if result.BucketRefillBelow <= 0 {
		result.BucketRefillBelow = fallback.BucketRefillBelow
	}
	if result.BucketRefillBelow <= 0 {
		result.BucketRefillBelow = 3
	}
	if result.BucketRefillBelow >= result.BucketTargetSize {
		result.BucketRefillBelow = result.BucketTargetSize - 1
	}
	if result.BucketRefillBelow <= 0 {
		result.BucketRefillBelow = 1
	}
	if result.BucketSyncFillMin < 0 {
		result.BucketSyncFillMin = fallback.BucketSyncFillMin
	}
	if result.BucketSyncFillMin < 0 {
		result.BucketSyncFillMin = 0
	}
	if result.BucketSyncFillMin > result.BucketTargetSize {
		result.BucketSyncFillMin = result.BucketTargetSize
	}
	if result.BucketEntryTTLSeconds <= 0 {
		result.BucketEntryTTLSeconds = fallback.BucketEntryTTLSeconds
	}
	if result.BucketEntryTTLSeconds <= 0 {
		result.BucketEntryTTLSeconds = 30
	}
	if result.BucketRefillCooldownSeconds < 0 {
		result.BucketRefillCooldownSeconds = fallback.BucketRefillCooldownSeconds
	}
	if result.BucketRefillCooldownSeconds < 0 {
		result.BucketRefillCooldownSeconds = 15
	}
	if result.BucketRefillIntervalSeconds < 0 {
		result.BucketRefillIntervalSeconds = fallback.BucketRefillIntervalSeconds
	}
	if result.BucketRefillIntervalSeconds < 0 {
		result.BucketRefillIntervalSeconds = 30
	}
	if result.GlobalTargetSize <= 0 {
		result.GlobalTargetSize = fallback.GlobalTargetSize
	}
	if result.GlobalTargetSize <= 0 {
		result.GlobalTargetSize = 30
	}
	if result.GlobalRefillBelow <= 0 {
		result.GlobalRefillBelow = fallback.GlobalRefillBelow
	}
	if result.GlobalRefillBelow <= 0 {
		result.GlobalRefillBelow = 10
	}
	if result.GlobalRefillBelow > result.GlobalTargetSize {
		result.GlobalRefillBelow = result.GlobalTargetSize
	}
	if result.GlobalEntryTTLSeconds <= 0 {
		result.GlobalEntryTTLSeconds = fallback.GlobalEntryTTLSeconds
	}
	if result.GlobalEntryTTLSeconds <= 0 {
		result.GlobalEntryTTLSeconds = 300
	}
	if result.GlobalRefillCooldownSeconds < 0 {
		result.GlobalRefillCooldownSeconds = fallback.GlobalRefillCooldownSeconds
	}
	if result.GlobalRefillCooldownSeconds < 0 {
		result.GlobalRefillCooldownSeconds = 60
	}
	if result.GlobalRefillIntervalSeconds < 0 {
		result.GlobalRefillIntervalSeconds = fallback.GlobalRefillIntervalSeconds
	}
	if result.GlobalRefillIntervalSeconds < 0 {
		result.GlobalRefillIntervalSeconds = 300
	}
	if result.NetworkErrorPoolSize < 0 {
		result.NetworkErrorPoolSize = fallback.NetworkErrorPoolSize
	}
	if result.NetworkErrorPoolSize < 0 {
		result.NetworkErrorPoolSize = 3
	}
	if result.NetworkErrorEntryTTLSeconds < 0 {
		result.NetworkErrorEntryTTLSeconds = fallback.NetworkErrorEntryTTLSeconds
	}
	if result.NetworkErrorEntryTTLSeconds < 0 {
		result.NetworkErrorEntryTTLSeconds = 120
	}
	if result.ProbeMaxCandidates <= 0 {
		result.ProbeMaxCandidates = fallback.ProbeMaxCandidates
	}
	if result.ProbeMaxCandidates <= 0 {
		result.ProbeMaxCandidates = 24
	}
	if result.ProbeConcurrency <= 0 {
		result.ProbeConcurrency = fallback.ProbeConcurrency
	}
	if result.ProbeConcurrency <= 0 {
		result.ProbeConcurrency = 4
	}
	if result.ProbeTimeoutSeconds <= 0 {
		result.ProbeTimeoutSeconds = fallback.ProbeTimeoutSeconds
	}
	if result.ProbeTimeoutSeconds <= 0 {
		result.ProbeTimeoutSeconds = 15
	}
	if result.ProbeFailureCooldownSeconds < 0 {
		result.ProbeFailureCooldownSeconds = fallback.ProbeFailureCooldownSeconds
	}
	if result.ProbeFailureCooldownSeconds < 0 {
		result.ProbeFailureCooldownSeconds = 120
	}
	return result
}
