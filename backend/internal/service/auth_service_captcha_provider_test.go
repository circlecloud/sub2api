//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type geeTestVerifierSpy struct {
	called         int
	lastCaptchaID  string
	lastCaptchaKey string
	lastPayload    *GeeTestValidateRequest
	result         *GeeTestVerifyResponse
	err            error
}

func (s *geeTestVerifierSpy) VerifyToken(_ context.Context, captchaID, captchaKey string, payload *GeeTestValidateRequest) (*GeeTestVerifyResponse, error) {
	s.called++
	s.lastCaptchaID = captchaID
	s.lastCaptchaKey = captchaKey
	s.lastPayload = payload
	if s.err != nil {
		return nil, s.err
	}
	if s.result != nil {
		return s.result, nil
	}
	return &GeeTestVerifyResponse{Result: "success"}, nil
}

func newAuthServiceForCaptchaProviderTest(settings map[string]string, turnstileVerifier TurnstileVerifier, geeTestVerifier GeeTestVerifier, cfg *config.Config) *AuthService {
	if cfg == nil {
		cfg = &config.Config{
			Server:    config.ServerConfig{Mode: "release"},
			Turnstile: config.TurnstileConfig{Required: false},
			Geetest:   config.GeetestConfig{Required: false},
		}
	}

	settingService := NewSettingService(&settingRepoStub{values: settings}, cfg)

	var turnstileService *TurnstileService
	if turnstileVerifier != nil {
		turnstileService = NewTurnstileService(settingService, turnstileVerifier)
	}

	var geeTestService *GeeTestService
	if geeTestVerifier != nil {
		geeTestService = NewGeeTestService(settingService, geeTestVerifier)
	}

	return NewAuthService(
		nil,
		&userRepoStub{},
		nil,
		nil,
		cfg,
		settingService,
		nil,
		turnstileService,
		geeTestService,
		nil,
		nil,
		nil,
	)
}

func TestAuthService_VerifyCaptcha_PrefersGeeTestWhenConfigured(t *testing.T) {
	turnstileVerifier := &turnstileVerifierSpy{}
	geeTestVerifier := &geeTestVerifierSpy{}
	service := newAuthServiceForCaptchaProviderTest(map[string]string{
		SettingKeyGeetestEnabled:     "true",
		SettingKeyGeetestCaptchaID:   "captcha-id",
		SettingKeyGeetestCaptchaKey:  "captcha-key",
		SettingKeyTurnstileEnabled:   "true",
		SettingKeyTurnstileSecretKey: "turnstile-secret",
	}, turnstileVerifier, geeTestVerifier, nil)

	err := service.VerifyCaptcha(context.Background(), `{"lot_number":"lot-123","captcha_output":"output","pass_token":"pass","gen_time":"1710000000"}`, "127.0.0.1")
	require.NoError(t, err)
	require.Equal(t, 1, geeTestVerifier.called)
	require.Equal(t, 0, turnstileVerifier.called)
	require.Equal(t, "captcha-id", geeTestVerifier.lastCaptchaID)
	require.Equal(t, "captcha-key", geeTestVerifier.lastCaptchaKey)
	require.NotNil(t, geeTestVerifier.lastPayload)
	require.Equal(t, "lot-123", geeTestVerifier.lastPayload.LotNumber)
}

func TestAuthService_VerifyCaptcha_FallsBackToTurnstileWhenGeeTestIncomplete(t *testing.T) {
	turnstileVerifier := &turnstileVerifierSpy{}
	geeTestVerifier := &geeTestVerifierSpy{}
	service := newAuthServiceForCaptchaProviderTest(map[string]string{
		SettingKeyGeetestEnabled:     "true",
		SettingKeyGeetestCaptchaID:   "captcha-id",
		SettingKeyTurnstileEnabled:   "true",
		SettingKeyTurnstileSecretKey: "turnstile-secret",
	}, turnstileVerifier, geeTestVerifier, nil)

	err := service.VerifyCaptcha(context.Background(), "turnstile-token", "127.0.0.1")
	require.NoError(t, err)
	require.Equal(t, 0, geeTestVerifier.called)
	require.Equal(t, 1, turnstileVerifier.called)
	require.Equal(t, "turnstile-token", turnstileVerifier.lastToken)
}

func TestAuthService_VerifyCaptcha_GeeTestRequiredReturnsNotConfigured(t *testing.T) {
	turnstileVerifier := &turnstileVerifierSpy{}
	service := newAuthServiceForCaptchaProviderTest(map[string]string{
		SettingKeyTurnstileEnabled:   "true",
		SettingKeyTurnstileSecretKey: "turnstile-secret",
	}, turnstileVerifier, nil, &config.Config{
		Server:    config.ServerConfig{Mode: "release"},
		Turnstile: config.TurnstileConfig{Required: false},
		Geetest:   config.GeetestConfig{Required: true},
	})

	err := service.VerifyCaptcha(context.Background(), "turnstile-token", "127.0.0.1")
	require.ErrorIs(t, err, ErrGeeTestNotConfigured)
	require.Equal(t, 0, turnstileVerifier.called)
}
