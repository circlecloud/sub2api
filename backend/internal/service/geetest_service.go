package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

var (
	ErrGeeTestVerificationFailed  = infraerrors.BadRequest("GEETEST_VERIFICATION_FAILED", "geetest verification failed")
	ErrGeeTestNotConfigured       = infraerrors.ServiceUnavailable("GEETEST_NOT_CONFIGURED", "geetest not configured")
	ErrGeeTestInvalidCaptchaToken = infraerrors.BadRequest("GEETEST_INVALID_CAPTCHA_TOKEN", "invalid geetest captcha token")
)

// GeeTestValidateRequest is the client validation payload returned by GeeTest v4.
type GeeTestValidateRequest struct {
	LotNumber     string `json:"lot_number"`
	CaptchaOutput string `json:"captcha_output"`
	PassToken     string `json:"pass_token"`
	GenTime       string `json:"gen_time"`
}

// GeeTestVerifyResponse is the GeeTest server-side verification response.
type GeeTestVerifyResponse struct {
	Result      string         `json:"result"`
	Reason      string         `json:"reason"`
	CaptchaArgs map[string]any `json:"captcha_args"`
	Status      string         `json:"status"`
	Code        string         `json:"code"`
	Msg         string         `json:"msg"`
	Desc        any            `json:"desc"`
}

// GeeTestVerifier verifies GeeTest payloads against the official API.
type GeeTestVerifier interface {
	VerifyToken(ctx context.Context, captchaID, captchaKey string, payload *GeeTestValidateRequest) (*GeeTestVerifyResponse, error)
}

// GeeTestService handles GeeTest v4 verification.
type GeeTestService struct {
	settingService *SettingService
	verifier       GeeTestVerifier
}

// NewGeeTestService creates a GeeTest verification service.
func NewGeeTestService(settingService *SettingService, verifier GeeTestVerifier) *GeeTestService {
	return &GeeTestService{
		settingService: settingService,
		verifier:       verifier,
	}
}

// ParseGeeTestValidateToken parses the serialized GeeTest client payload.
func ParseGeeTestValidateToken(token string) (*GeeTestValidateRequest, error) {
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return nil, ErrGeeTestVerificationFailed
	}

	var payload GeeTestValidateRequest
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return nil, ErrGeeTestInvalidCaptchaToken
	}
	if strings.TrimSpace(payload.LotNumber) == "" ||
		strings.TrimSpace(payload.CaptchaOutput) == "" ||
		strings.TrimSpace(payload.PassToken) == "" ||
		strings.TrimSpace(payload.GenTime) == "" {
		return nil, ErrGeeTestInvalidCaptchaToken
	}
	return &payload, nil
}

// HasValidConfig reports whether GeeTest is fully configured.
func (s *GeeTestService) HasValidConfig(ctx context.Context) bool {
	if s == nil || s.settingService == nil || s.verifier == nil {
		return false
	}
	return strings.TrimSpace(s.settingService.GetGeetestCaptchaID(ctx)) != "" &&
		strings.TrimSpace(s.settingService.GetGeetestCaptchaKey(ctx)) != ""
}

// VerifyToken verifies a serialized GeeTest v4 payload.
func (s *GeeTestService) VerifyToken(ctx context.Context, token string, remoteIP string) error {
	if s == nil || s.settingService == nil || s.verifier == nil {
		return ErrGeeTestNotConfigured
	}
	if !s.settingService.IsGeetestEnabled(ctx) {
		logger.LegacyPrintf("service.geetest", "%s", "[GeeTest] Disabled, skipping verification")
		return nil
	}

	captchaID := strings.TrimSpace(s.settingService.GetGeetestCaptchaID(ctx))
	captchaKey := strings.TrimSpace(s.settingService.GetGeetestCaptchaKey(ctx))
	if captchaID == "" || captchaKey == "" {
		logger.LegacyPrintf("service.geetest", "%s", "[GeeTest] Captcha ID or captcha key not configured")
		return ErrGeeTestNotConfigured
	}

	payload, err := ParseGeeTestValidateToken(token)
	if err != nil {
		logger.LegacyPrintf("service.geetest", "[GeeTest] Invalid captcha token for IP %s: %v", remoteIP, err)
		return err
	}

	logger.LegacyPrintf("service.geetest", "[GeeTest] Verifying captcha for IP: %s", remoteIP)
	result, err := s.verifier.VerifyToken(ctx, captchaID, captchaKey, payload)
	if err != nil {
		logger.LegacyPrintf("service.geetest", "[GeeTest] Request failed: %v", err)
		return fmt.Errorf("send request: %w", err)
	}
	if strings.EqualFold(strings.TrimSpace(result.Status), "error") {
		logger.LegacyPrintf("service.geetest", "[GeeTest] Verification API returned error status: code=%s msg=%s", result.Code, result.Msg)
		return ErrGeeTestVerificationFailed
	}
	if !strings.EqualFold(strings.TrimSpace(result.Result), "success") {
		logger.LegacyPrintf("service.geetest", "[GeeTest] Verification failed: reason=%s", result.Reason)
		return ErrGeeTestVerificationFailed
	}

	logger.LegacyPrintf("service.geetest", "%s", "[GeeTest] Verification successful")
	return nil
}

// IsEnabled checks whether GeeTest is enabled.
func (s *GeeTestService) IsEnabled(ctx context.Context) bool {
	if s == nil || s.settingService == nil {
		return false
	}
	return s.settingService.IsGeetestEnabled(ctx)
}
