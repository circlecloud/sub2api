package service

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

func appendOpenAISettingsUpdates(updates map[string]string, settings *SystemSettings) error {
	if settings == nil {
		return nil
	}

	updates[SettingKeyEnableFingerprintUnification] = fmtBool(settings.EnableFingerprintUnification)
	updates[SettingKeyEnableMetadataPassthrough] = fmtBool(settings.EnableMetadataPassthrough)
	updates[SettingKeyEnableCCHSigning] = fmtBool(settings.EnableCCHSigning)
	updates[SettingKeyEnableOpenAIStreamRectifier] = fmtBool(settings.EnableOpenAIStreamRectifier)
	updates[SettingKeyOpenAIUsageProbeMethod] = normalizeOpenAIUsageProbeMethod(settings.OpenAIUsageProbeMethod)

	responseHeaderRectifierTimeoutsJSON, err := json.Marshal(settings.OpenAIStreamResponseHeaderRectifierTimeouts)
	if err != nil {
		return fmt.Errorf("marshal openai response header rectifier timeouts: %w", err)
	}
	updates[SettingKeyOpenAIStreamResponseHeaderRectifierTimeouts] = string(responseHeaderRectifierTimeoutsJSON)

	firstTokenRectifierTimeoutsJSON, err := json.Marshal(settings.OpenAIStreamFirstTokenRectifierTimeouts)
	if err != nil {
		return fmt.Errorf("marshal openai first token rectifier timeouts: %w", err)
	}
	updates[SettingKeyOpenAIStreamFirstTokenRectifierTimeouts] = string(firstTokenRectifierTimeoutsJSON)

	updates[SettingKeyOpenAIWarmPoolEnabled] = fmtBool(settings.OpenAIWarmPoolEnabled)
	updates[SettingKeyOpenAIWarmPoolBucketTargetSize] = fmtInt(settings.OpenAIWarmPoolBucketTargetSize)
	updates[SettingKeyOpenAIWarmPoolBucketRefillBelow] = fmtInt(settings.OpenAIWarmPoolBucketRefillBelow)
	updates[SettingKeyOpenAIWarmPoolBucketSyncFillMin] = fmtInt(settings.OpenAIWarmPoolBucketSyncFillMin)
	updates[SettingKeyOpenAIWarmPoolBucketEntryTTLSeconds] = fmtInt(settings.OpenAIWarmPoolBucketEntryTTLSeconds)
	updates[SettingKeyOpenAIWarmPoolBucketRefillCooldownSeconds] = fmtInt(settings.OpenAIWarmPoolBucketRefillCooldownSeconds)
	updates[SettingKeyOpenAIWarmPoolBucketRefillIntervalSeconds] = fmtInt(settings.OpenAIWarmPoolBucketRefillIntervalSeconds)
	updates[SettingKeyOpenAIWarmPoolGlobalTargetSize] = fmtInt(settings.OpenAIWarmPoolGlobalTargetSize)
	updates[SettingKeyOpenAIWarmPoolGlobalRefillBelow] = fmtInt(settings.OpenAIWarmPoolGlobalRefillBelow)
	updates[SettingKeyOpenAIWarmPoolGlobalEntryTTLSeconds] = fmtInt(settings.OpenAIWarmPoolGlobalEntryTTLSeconds)
	updates[SettingKeyOpenAIWarmPoolGlobalRefillCooldownSeconds] = fmtInt(settings.OpenAIWarmPoolGlobalRefillCooldownSeconds)
	updates[SettingKeyOpenAIWarmPoolGlobalRefillIntervalSeconds] = fmtInt(settings.OpenAIWarmPoolGlobalRefillIntervalSeconds)
	updates[SettingKeyOpenAIWarmPoolNetworkErrorPoolSize] = fmtInt(settings.OpenAIWarmPoolNetworkErrorPoolSize)
	updates[SettingKeyOpenAIWarmPoolNetworkErrorEntryTTLSeconds] = fmtInt(settings.OpenAIWarmPoolNetworkErrorEntryTTLSeconds)
	updates[SettingKeyOpenAIWarmPoolProbeMaxCandidates] = fmtInt(settings.OpenAIWarmPoolProbeMaxCandidates)
	updates[SettingKeyOpenAIWarmPoolProbeConcurrency] = fmtInt(settings.OpenAIWarmPoolProbeConcurrency)
	updates[SettingKeyOpenAIWarmPoolProbeTimeoutSeconds] = fmtInt(settings.OpenAIWarmPoolProbeTimeoutSeconds)
	updates[SettingKeyOpenAIWarmPoolProbeFailureCooldownSeconds] = fmtInt(settings.OpenAIWarmPoolProbeFailureCooldownSeconds)
	updates[SettingKeyOpenAIWarmPoolStartupGroupIDs] = encodePositiveInt64CSV(settings.OpenAIWarmPoolStartupGroupIDs)

	return nil
}

func refreshOpenAISettingsCaches(settings *SystemSettings, cfg *config.Config) {
	if settings == nil {
		return
	}

	gatewayForwardingSF.Forget("gateway_forwarding")
	gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{
		fingerprintUnification:                  settings.EnableFingerprintUnification,
		metadataPassthrough:                     settings.EnableMetadataPassthrough,
		cchSigning:                              settings.EnableCCHSigning,
		openAIStreamRectifier:                   settings.EnableOpenAIStreamRectifier,
		openAIStreamResponseHeaderTimeouts:      cloneIntSlice(settings.OpenAIStreamResponseHeaderRectifierTimeouts),
		openAIStreamFirstTokenRectifierTimeouts: cloneIntSlice(settings.OpenAIStreamFirstTokenRectifierTimeouts),
		expiresAt:                               time.Now().Add(gatewayForwardingCacheTTL).UnixNano(),
	})

	openAIWarmPoolSF.Forget("openai_warm_pool")
	openAIWarmPoolCache.Store(&cachedOpenAIWarmPoolSettings{
		settings: sanitizeOpenAIWarmPoolSettings(
			openAIWarmPoolSettingsFromSystemSettings(settings),
			defaultOpenAIWarmPoolSettingsFromConfig(cfg),
		),
		expiresAt: time.Now().Add(openAIWarmPoolCacheTTL).UnixNano(),
	})
}

func applyOpenAISettingsFromMap(result *SystemSettings, settings map[string]string, cfg *config.Config) {
	if result == nil {
		return
	}

	if v, ok := settings[SettingKeyEnableFingerprintUnification]; ok && v != "" {
		result.EnableFingerprintUnification = v == "true"
	} else {
		result.EnableFingerprintUnification = true
	}
	result.EnableMetadataPassthrough = settings[SettingKeyEnableMetadataPassthrough] == "true"
	result.EnableCCHSigning = settings[SettingKeyEnableCCHSigning] == "true"
	result.EnableOpenAIStreamRectifier = parseBoolSettingOrDefault(settings, SettingKeyEnableOpenAIStreamRectifier, true)
	result.OpenAIStreamResponseHeaderRectifierTimeouts = parseOpenAIStreamRectifierTimeoutsSettingOrDefault(
		settings,
		SettingKeyOpenAIStreamResponseHeaderRectifierTimeouts,
		defaultOpenAIResponseHeaderRectifierTimeoutsFromConfig(cfg),
	)
	result.OpenAIStreamFirstTokenRectifierTimeouts = parseOpenAIStreamRectifierTimeoutsSettingOrDefault(
		settings,
		SettingKeyOpenAIStreamFirstTokenRectifierTimeouts,
		defaultOpenAIFirstTokenRectifierTimeoutsFromConfig(cfg),
	)
	result.OpenAIUsageProbeMethod = normalizeOpenAIUsageProbeMethod(settings[SettingKeyOpenAIUsageProbeMethod])

	warmPoolDefaults := defaultOpenAIWarmPoolSettingsFromConfig(cfg)
	result.OpenAIWarmPoolEnabled = parseBoolSettingOrDefault(settings, SettingKeyOpenAIWarmPoolEnabled, warmPoolDefaults.Enabled)
	result.OpenAIWarmPoolBucketTargetSize = parseIntSettingOrDefault(settings, SettingKeyOpenAIWarmPoolBucketTargetSize, warmPoolDefaults.BucketTargetSize)
	result.OpenAIWarmPoolBucketRefillBelow = parseIntSettingOrDefault(settings, SettingKeyOpenAIWarmPoolBucketRefillBelow, warmPoolDefaults.BucketRefillBelow)
	result.OpenAIWarmPoolBucketSyncFillMin = parseIntSettingOrDefault(settings, SettingKeyOpenAIWarmPoolBucketSyncFillMin, warmPoolDefaults.BucketSyncFillMin)
	result.OpenAIWarmPoolBucketEntryTTLSeconds = parseIntSettingOrDefault(settings, SettingKeyOpenAIWarmPoolBucketEntryTTLSeconds, warmPoolDefaults.BucketEntryTTLSeconds)
	result.OpenAIWarmPoolBucketRefillCooldownSeconds = parseIntSettingOrDefault(settings, SettingKeyOpenAIWarmPoolBucketRefillCooldownSeconds, warmPoolDefaults.BucketRefillCooldownSeconds)
	result.OpenAIWarmPoolBucketRefillIntervalSeconds = parseIntSettingOrDefault(settings, SettingKeyOpenAIWarmPoolBucketRefillIntervalSeconds, warmPoolDefaults.BucketRefillIntervalSeconds)
	result.OpenAIWarmPoolGlobalTargetSize = parseIntSettingOrDefault(settings, SettingKeyOpenAIWarmPoolGlobalTargetSize, warmPoolDefaults.GlobalTargetSize)
	result.OpenAIWarmPoolGlobalRefillBelow = parseIntSettingOrDefault(settings, SettingKeyOpenAIWarmPoolGlobalRefillBelow, warmPoolDefaults.GlobalRefillBelow)
	result.OpenAIWarmPoolGlobalEntryTTLSeconds = parseIntSettingOrDefault(settings, SettingKeyOpenAIWarmPoolGlobalEntryTTLSeconds, warmPoolDefaults.GlobalEntryTTLSeconds)
	result.OpenAIWarmPoolGlobalRefillCooldownSeconds = parseIntSettingOrDefault(settings, SettingKeyOpenAIWarmPoolGlobalRefillCooldownSeconds, warmPoolDefaults.GlobalRefillCooldownSeconds)
	result.OpenAIWarmPoolGlobalRefillIntervalSeconds = parseIntSettingOrDefault(settings, SettingKeyOpenAIWarmPoolGlobalRefillIntervalSeconds, warmPoolDefaults.GlobalRefillIntervalSeconds)
	result.OpenAIWarmPoolNetworkErrorPoolSize = parseIntSettingOrDefault(settings, SettingKeyOpenAIWarmPoolNetworkErrorPoolSize, warmPoolDefaults.NetworkErrorPoolSize)
	result.OpenAIWarmPoolNetworkErrorEntryTTLSeconds = parseIntSettingOrDefault(settings, SettingKeyOpenAIWarmPoolNetworkErrorEntryTTLSeconds, warmPoolDefaults.NetworkErrorEntryTTLSeconds)
	result.OpenAIWarmPoolProbeMaxCandidates = parseIntSettingOrDefault(settings, SettingKeyOpenAIWarmPoolProbeMaxCandidates, warmPoolDefaults.ProbeMaxCandidates)
	result.OpenAIWarmPoolProbeConcurrency = parseIntSettingOrDefault(settings, SettingKeyOpenAIWarmPoolProbeConcurrency, warmPoolDefaults.ProbeConcurrency)
	result.OpenAIWarmPoolProbeTimeoutSeconds = parseIntSettingOrDefault(settings, SettingKeyOpenAIWarmPoolProbeTimeoutSeconds, warmPoolDefaults.ProbeTimeoutSeconds)
	result.OpenAIWarmPoolProbeFailureCooldownSeconds = parseIntSettingOrDefault(settings, SettingKeyOpenAIWarmPoolProbeFailureCooldownSeconds, warmPoolDefaults.ProbeFailureCooldownSeconds)
	result.OpenAIWarmPoolStartupGroupIDs = parsePositiveInt64CSV(settings[SettingKeyOpenAIWarmPoolStartupGroupIDs])
	applyOpenAIWarmPoolSanitizationToSystemSettings(result, warmPoolDefaults)
}

func openAIWarmPoolSettingsFromSystemSettings(settings *SystemSettings) OpenAIWarmPoolSettings {
	if settings == nil {
		return OpenAIWarmPoolSettings{}
	}
	return OpenAIWarmPoolSettings{
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
		StartupGroupIDs:             cloneInt64Slice(settings.OpenAIWarmPoolStartupGroupIDs),
	}
}

func fmtBool(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func fmtInt(value int) string {
	return fmt.Sprintf("%d", value)
}
