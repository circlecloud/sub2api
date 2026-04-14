//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type settingUpdateRepoStub struct {
	updates          map[string]string
	values           map[string]string
	getMultipleCalls int
}

func (s *settingUpdateRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	panic("unexpected Get call")
}

func (s *settingUpdateRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	panic("unexpected GetValue call")
}

func (s *settingUpdateRepoStub) Set(ctx context.Context, key, value string) error {
	panic("unexpected Set call")
}

func (s *settingUpdateRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	s.getMultipleCalls++
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		if s.values == nil {
			continue
		}
		if value, ok := s.values[key]; ok {
			result[key] = value
		}
	}
	return result, nil
}

func (s *settingUpdateRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	s.updates = make(map[string]string, len(settings))
	for k, v := range settings {
		s.updates[k] = v
	}
	return nil
}

func (s *settingUpdateRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	result := make(map[string]string, len(s.values))
	for key, value := range s.values {
		result[key] = value
	}
	return result, nil
}

func (s *settingUpdateRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
}

type defaultSubGroupReaderStub struct {
	byID  map[int64]*Group
	errBy map[int64]error
	calls []int64
}

func (s *defaultSubGroupReaderStub) GetByID(ctx context.Context, id int64) (*Group, error) {
	s.calls = append(s.calls, id)
	if err, ok := s.errBy[id]; ok {
		return nil, err
	}
	if g, ok := s.byID[id]; ok {
		return g, nil
	}
	return nil, ErrGroupNotFound
}

func TestSettingService_UpdateSettings_DefaultSubscriptions_ValidGroup(t *testing.T) {
	repo := &settingUpdateRepoStub{}
	groupReader := &defaultSubGroupReaderStub{
		byID: map[int64]*Group{
			11: {ID: 11, SubscriptionType: SubscriptionTypeSubscription},
		},
	}
	svc := NewSettingService(repo, &config.Config{})
	svc.SetDefaultSubscriptionGroupReader(groupReader)

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		DefaultSubscriptions: []DefaultSubscriptionSetting{
			{GroupID: 11, ValidityDays: 30},
		},
	})
	require.NoError(t, err)
	require.Equal(t, []int64{11}, groupReader.calls)

	raw, ok := repo.updates[SettingKeyDefaultSubscriptions]
	require.True(t, ok)

	var got []DefaultSubscriptionSetting
	require.NoError(t, json.Unmarshal([]byte(raw), &got))
	require.Equal(t, []DefaultSubscriptionSetting{
		{GroupID: 11, ValidityDays: 30},
	}, got)
}

func TestSettingService_UpdateSettings_DefaultSubscriptions_RejectsNonSubscriptionGroup(t *testing.T) {
	repo := &settingUpdateRepoStub{}
	groupReader := &defaultSubGroupReaderStub{
		byID: map[int64]*Group{
			12: {ID: 12, SubscriptionType: SubscriptionTypeStandard},
		},
	}
	svc := NewSettingService(repo, &config.Config{})
	svc.SetDefaultSubscriptionGroupReader(groupReader)

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		DefaultSubscriptions: []DefaultSubscriptionSetting{
			{GroupID: 12, ValidityDays: 7},
		},
	})
	require.Error(t, err)
	require.Equal(t, "DEFAULT_SUBSCRIPTION_GROUP_INVALID", infraerrors.Reason(err))
	require.Nil(t, repo.updates)
}

func TestSettingService_UpdateSettings_DefaultSubscriptions_RejectsNotFoundGroup(t *testing.T) {
	repo := &settingUpdateRepoStub{}
	groupReader := &defaultSubGroupReaderStub{
		errBy: map[int64]error{
			13: ErrGroupNotFound,
		},
	}
	svc := NewSettingService(repo, &config.Config{})
	svc.SetDefaultSubscriptionGroupReader(groupReader)

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		DefaultSubscriptions: []DefaultSubscriptionSetting{
			{GroupID: 13, ValidityDays: 7},
		},
	})
	require.Error(t, err)
	require.Equal(t, "DEFAULT_SUBSCRIPTION_GROUP_INVALID", infraerrors.Reason(err))
	require.Equal(t, "13", infraerrors.FromError(err).Metadata["group_id"])
	require.Nil(t, repo.updates)
}

func TestSettingService_UpdateSettings_DefaultSubscriptions_RejectsDuplicateGroup(t *testing.T) {
	repo := &settingUpdateRepoStub{}
	groupReader := &defaultSubGroupReaderStub{
		byID: map[int64]*Group{
			11: {ID: 11, SubscriptionType: SubscriptionTypeSubscription},
		},
	}
	svc := NewSettingService(repo, &config.Config{})
	svc.SetDefaultSubscriptionGroupReader(groupReader)

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		DefaultSubscriptions: []DefaultSubscriptionSetting{
			{GroupID: 11, ValidityDays: 30},
			{GroupID: 11, ValidityDays: 60},
		},
	})
	require.Error(t, err)
	require.Equal(t, "DEFAULT_SUBSCRIPTION_GROUP_DUPLICATE", infraerrors.Reason(err))
	require.Equal(t, "11", infraerrors.FromError(err).Metadata["group_id"])
	require.Nil(t, repo.updates)
}

func TestSettingService_UpdateSettings_DefaultSubscriptions_RejectsDuplicateGroupWithoutGroupReader(t *testing.T) {
	repo := &settingUpdateRepoStub{}
	svc := NewSettingService(repo, &config.Config{})

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		DefaultSubscriptions: []DefaultSubscriptionSetting{
			{GroupID: 11, ValidityDays: 30},
			{GroupID: 11, ValidityDays: 60},
		},
	})
	require.Error(t, err)
	require.Equal(t, "DEFAULT_SUBSCRIPTION_GROUP_DUPLICATE", infraerrors.Reason(err))
	require.Equal(t, "11", infraerrors.FromError(err).Metadata["group_id"])
	require.Nil(t, repo.updates)
}

func TestSettingService_UpdateSettings_RegistrationEmailSuffixWhitelist_Normalized(t *testing.T) {
	repo := &settingUpdateRepoStub{}
	svc := NewSettingService(repo, &config.Config{})

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		RegistrationEmailSuffixWhitelist: []string{"example.com", "@EXAMPLE.com", " @foo.bar "},
	})
	require.NoError(t, err)
	require.Equal(t, `["@example.com","@foo.bar"]`, repo.updates[SettingKeyRegistrationEmailSuffixWhitelist])
}

func TestSettingService_UpdateSettings_RegistrationEmailSuffixWhitelist_Invalid(t *testing.T) {
	repo := &settingUpdateRepoStub{}
	svc := NewSettingService(repo, &config.Config{})

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		RegistrationEmailSuffixWhitelist: []string{"@invalid_domain"},
	})
	require.Error(t, err)
	require.Equal(t, "INVALID_REGISTRATION_EMAIL_SUFFIX_WHITELIST", infraerrors.Reason(err))
}

func TestSettingService_UpdateSettings_OpenAIStreamRectifier_PersistsAndRefreshesCache(t *testing.T) {
	repo := &settingUpdateRepoStub{
		values: map[string]string{
			SettingKeyEnableOpenAIStreamRectifier:                 "true",
			SettingKeyOpenAIStreamResponseHeaderRectifierTimeouts: `[8,10,12]`,
			SettingKeyOpenAIStreamFirstTokenRectifierTimeouts:     `[5,8,10]`,
		},
	}
	gatewayForwardingSF.Forget("gateway_forwarding")
	gatewayForwardingCache.Store((*cachedGatewayForwardingSettings)(nil))
	t.Cleanup(func() {
		gatewayForwardingSF.Forget("gateway_forwarding")
		gatewayForwardingCache.Store((*cachedGatewayForwardingSettings)(nil))
	})

	svc := NewSettingService(repo, &config.Config{Gateway: config.GatewayConfig{
		OpenAIStreamRectifierEnabled:                true,
		OpenAIStreamResponseHeaderRectifierTimeouts: []int{8, 10, 12},
		OpenAIStreamFirstTokenRectifierTimeouts:     []int{5, 8, 10},
	}})
	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		EnableOpenAIStreamRectifier:                 false,
		OpenAIStreamResponseHeaderRectifierTimeouts: []int{11, 13},
		OpenAIStreamFirstTokenRectifierTimeouts:     []int{7, 9},
	})
	require.NoError(t, err)
	require.Equal(t, "false", repo.updates[SettingKeyEnableOpenAIStreamRectifier])
	require.Equal(t, `[11,13]`, repo.updates[SettingKeyOpenAIStreamResponseHeaderRectifierTimeouts])
	require.Equal(t, `[7,9]`, repo.updates[SettingKeyOpenAIStreamFirstTokenRectifierTimeouts])
	require.False(t, svc.IsOpenAIStreamRectifierEnabled(context.Background()))
	header, first := svc.GetOpenAIStreamRectifierTimeouts(context.Background())
	require.Equal(t, []int{11, 13}, header)
	require.Equal(t, []int{7, 9}, first)
}

func TestSettingService_UpdateSettings_OpenAIWarmPool_PersistsAndRefreshesCache(t *testing.T) {
	repo := &settingUpdateRepoStub{
		values: map[string]string{
			SettingKeyOpenAIWarmPoolEnabled:                     "true",
			SettingKeyOpenAIWarmPoolBucketTargetSize:            "10",
			SettingKeyOpenAIWarmPoolBucketRefillBelow:           "3",
			SettingKeyOpenAIWarmPoolBucketSyncFillMin:           "1",
			SettingKeyOpenAIWarmPoolBucketEntryTTLSeconds:       "30",
			SettingKeyOpenAIWarmPoolBucketRefillCooldownSeconds: "15",
			SettingKeyOpenAIWarmPoolBucketRefillIntervalSeconds: "30",
			SettingKeyOpenAIWarmPoolGlobalTargetSize:            "30",
			SettingKeyOpenAIWarmPoolGlobalRefillBelow:           "10",
			SettingKeyOpenAIWarmPoolGlobalEntryTTLSeconds:       "300",
			SettingKeyOpenAIWarmPoolGlobalRefillCooldownSeconds: "60",
			SettingKeyOpenAIWarmPoolGlobalRefillIntervalSeconds: "300",
			SettingKeyOpenAIWarmPoolNetworkErrorPoolSize:        "3",
			SettingKeyOpenAIWarmPoolNetworkErrorEntryTTLSeconds: "120",
			SettingKeyOpenAIWarmPoolProbeMaxCandidates:          "24",
			SettingKeyOpenAIWarmPoolProbeConcurrency:            "4",
			SettingKeyOpenAIWarmPoolProbeTimeoutSeconds:         "15",
			SettingKeyOpenAIWarmPoolProbeFailureCooldownSeconds: "120",
			SettingKeyOpenAIWarmPoolStartupGroupIDs:             "5,2,5",
		},
	}
	openAIWarmPoolSF.Forget("openai_warm_pool")
	openAIWarmPoolCache.Store((*cachedOpenAIWarmPoolSettings)(nil))
	t.Cleanup(func() {
		openAIWarmPoolSF.Forget("openai_warm_pool")
		openAIWarmPoolCache.Store((*cachedOpenAIWarmPoolSettings)(nil))
	})

	svc := NewSettingService(repo, nil)
	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		OpenAIWarmPoolEnabled:                     false,
		OpenAIWarmPoolBucketTargetSize:            21,
		OpenAIWarmPoolBucketRefillBelow:           8,
		OpenAIWarmPoolBucketSyncFillMin:           4,
		OpenAIWarmPoolBucketEntryTTLSeconds:       45,
		OpenAIWarmPoolBucketRefillCooldownSeconds: 18,
		OpenAIWarmPoolBucketRefillIntervalSeconds: 36,
		OpenAIWarmPoolGlobalTargetSize:            44,
		OpenAIWarmPoolGlobalRefillBelow:           12,
		OpenAIWarmPoolGlobalEntryTTLSeconds:       600,
		OpenAIWarmPoolGlobalRefillCooldownSeconds: 90,
		OpenAIWarmPoolGlobalRefillIntervalSeconds: 420,
		OpenAIWarmPoolNetworkErrorPoolSize:        5,
		OpenAIWarmPoolNetworkErrorEntryTTLSeconds: 240,
		OpenAIWarmPoolProbeMaxCandidates:          33,
		OpenAIWarmPoolProbeConcurrency:            6,
		OpenAIWarmPoolProbeTimeoutSeconds:         19,
		OpenAIWarmPoolProbeFailureCooldownSeconds: 180,
		OpenAIWarmPoolStartupGroupIDs:             []int64{9, 3, 9},
	})
	require.NoError(t, err)

	require.Equal(t, "false", repo.updates[SettingKeyOpenAIWarmPoolEnabled])
	require.Equal(t, "21", repo.updates[SettingKeyOpenAIWarmPoolBucketTargetSize])
	require.Equal(t, "8", repo.updates[SettingKeyOpenAIWarmPoolBucketRefillBelow])
	require.Equal(t, "4", repo.updates[SettingKeyOpenAIWarmPoolBucketSyncFillMin])
	require.Equal(t, "44", repo.updates[SettingKeyOpenAIWarmPoolGlobalTargetSize])
	require.Equal(t, "12", repo.updates[SettingKeyOpenAIWarmPoolGlobalRefillBelow])
	require.Equal(t, "33", repo.updates[SettingKeyOpenAIWarmPoolProbeMaxCandidates])
	require.Equal(t, "6", repo.updates[SettingKeyOpenAIWarmPoolProbeConcurrency])
	require.Equal(t, "19", repo.updates[SettingKeyOpenAIWarmPoolProbeTimeoutSeconds])
	require.Equal(t, "180", repo.updates[SettingKeyOpenAIWarmPoolProbeFailureCooldownSeconds])
	require.Equal(t, "3,9", repo.updates[SettingKeyOpenAIWarmPoolStartupGroupIDs])

	warmPool := svc.GetOpenAIWarmPoolSettings(context.Background())
	require.Zero(t, repo.getMultipleCalls)
	require.False(t, warmPool.Enabled)
	require.Equal(t, 21, warmPool.BucketTargetSize)
	require.Equal(t, 8, warmPool.BucketRefillBelow)
	require.Equal(t, 4, warmPool.BucketSyncFillMin)
	require.Equal(t, 44, warmPool.GlobalTargetSize)
	require.Equal(t, 12, warmPool.GlobalRefillBelow)
	require.Equal(t, 600, warmPool.GlobalEntryTTLSeconds)
	require.Equal(t, 5, warmPool.NetworkErrorPoolSize)
	require.Equal(t, 240, warmPool.NetworkErrorEntryTTLSeconds)
	require.Equal(t, 33, warmPool.ProbeMaxCandidates)
	require.Equal(t, 6, warmPool.ProbeConcurrency)
	require.Equal(t, 19, warmPool.ProbeTimeoutSeconds)
	require.Equal(t, 180, warmPool.ProbeFailureCooldownSeconds)
	require.Equal(t, []int64{3, 9}, warmPool.StartupGroupIDs)
}

func TestSettingService_GetAllSettings_ParsesNotifyAndWebSearchFields(t *testing.T) {
	repo := &settingUpdateRepoStub{values: map[string]string{
		SettingKeyWebSearchEmulationConfig:    `{"enabled":true,"providers":[{"type":"brave","api_key_configured":true}]}`,
		SettingKeyBalanceLowNotifyEnabled:     "true",
		SettingKeyBalanceLowNotifyThreshold:   "12.5",
		SettingKeyBalanceLowNotifyRechargeURL: "https://example.com/recharge",
		SettingKeyAccountQuotaNotifyEnabled:   "true",
		SettingKeyAccountQuotaNotifyEmails:    `[{"email":"ops@example.com","disabled":false,"verified":true}]`,
	}}
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetAllSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.WebSearchEmulationEnabled)
	require.True(t, settings.BalanceLowNotifyEnabled)
	require.Equal(t, 12.5, settings.BalanceLowNotifyThreshold)
	require.Equal(t, "https://example.com/recharge", settings.BalanceLowNotifyRechargeURL)
	require.True(t, settings.AccountQuotaNotifyEnabled)
	require.Len(t, settings.AccountQuotaNotifyEmails, 1)
	require.Equal(t, "ops@example.com", settings.AccountQuotaNotifyEmails[0].Email)
	require.True(t, settings.AccountQuotaNotifyEmails[0].Verified)
}

func TestParseDefaultSubscriptions_NormalizesValues(t *testing.T) {
	got := parseDefaultSubscriptions(`[{"group_id":11,"validity_days":30},{"group_id":11,"validity_days":60},{"group_id":0,"validity_days":10},{"group_id":12,"validity_days":99999}]`)
	require.Equal(t, []DefaultSubscriptionSetting{
		{GroupID: 11, ValidityDays: 30},
		{GroupID: 11, ValidityDays: 60},
		{GroupID: 12, ValidityDays: MaxValidityDays},
	}, got)
}

func TestSettingService_UpdateSettings_TablePreferences(t *testing.T) {
	repo := &settingUpdateRepoStub{}
	svc := NewSettingService(repo, &config.Config{})

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		TableDefaultPageSize: 50,
		TablePageSizeOptions: []int{20, 50, 100},
	})
	require.NoError(t, err)
	require.Equal(t, "50", repo.updates[SettingKeyTableDefaultPageSize])
	require.Equal(t, "[20,50,100]", repo.updates[SettingKeyTablePageSizeOptions])

	err = svc.UpdateSettings(context.Background(), &SystemSettings{
		TableDefaultPageSize: 1000,
		TablePageSizeOptions: []int{20, 100},
	})
	require.NoError(t, err)
	require.Equal(t, "1000", repo.updates[SettingKeyTableDefaultPageSize])
	require.Equal(t, "[20,100]", repo.updates[SettingKeyTablePageSizeOptions])
}
