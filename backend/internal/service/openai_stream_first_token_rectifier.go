package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type openAIStreamFirstTokenRectifierContextKey struct{}
type openAIStreamRectifierPolicyContextKey struct{}

type openAIStreamFirstTokenRectifierContextValue struct {
	HeaderAttempt         int
	FirstTokenAttempt     int
	ResponseHeaderTimeout time.Duration
	FirstTokenTimeout     time.Duration
}

type OpenAIStreamRectifierPolicy struct {
	Enabled                bool
	ResponseHeaderTimeouts []time.Duration
	FirstTokenTimeouts     []time.Duration
}

func cloneDurationSlice(values []time.Duration) []time.Duration {
	if len(values) == 0 {
		return nil
	}
	cloned := make([]time.Duration, len(values))
	copy(cloned, values)
	return cloned
}

func durationsFromPositiveSeconds(values []int) []time.Duration {
	if len(values) == 0 {
		return nil
	}
	result := make([]time.Duration, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			return nil
		}
		result = append(result, time.Duration(value)*time.Second)
	}
	return result
}

func defaultOpenAIStreamRectifierPolicy() OpenAIStreamRectifierPolicy {
	return OpenAIStreamRectifierPolicy{
		Enabled:                true,
		ResponseHeaderTimeouts: durationsFromPositiveSeconds([]int{8, 10, 12}),
		FirstTokenTimeouts:     durationsFromPositiveSeconds([]int{5, 8, 10}),
	}
}

func (p OpenAIStreamRectifierPolicy) ResponseHeaderTimeoutForAttempt(attempt int) time.Duration {
	if attempt <= 0 || attempt > len(p.ResponseHeaderTimeouts) {
		return 0
	}
	return p.ResponseHeaderTimeouts[attempt-1]
}

func (p OpenAIStreamRectifierPolicy) FirstTokenTimeoutForAttempt(attempt int) time.Duration {
	if attempt <= 0 || attempt > len(p.FirstTokenTimeouts) {
		return 0
	}
	return p.FirstTokenTimeouts[attempt-1]
}

func (p OpenAIStreamRectifierPolicy) MaxAttemptsForPhase(phase string) int {
	switch strings.TrimSpace(strings.ToLower(phase)) {
	case "response_header":
		return len(p.ResponseHeaderTimeouts)
	default:
		return len(p.FirstTokenTimeouts)
	}
}

func WithOpenAIStreamRectifierPolicy(ctx context.Context, policy OpenAIStreamRectifierPolicy) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	policy.ResponseHeaderTimeouts = cloneDurationSlice(policy.ResponseHeaderTimeouts)
	policy.FirstTokenTimeouts = cloneDurationSlice(policy.FirstTokenTimeouts)
	return context.WithValue(ctx, openAIStreamRectifierPolicyContextKey{}, policy)
}

func getOpenAIStreamRectifierPolicyFromContext(ctx context.Context) (OpenAIStreamRectifierPolicy, bool) {
	if ctx == nil {
		return OpenAIStreamRectifierPolicy{}, false
	}
	policy, ok := ctx.Value(openAIStreamRectifierPolicyContextKey{}).(OpenAIStreamRectifierPolicy)
	if !ok {
		return OpenAIStreamRectifierPolicy{}, false
	}
	policy.ResponseHeaderTimeouts = cloneDurationSlice(policy.ResponseHeaderTimeouts)
	policy.FirstTokenTimeouts = cloneDurationSlice(policy.FirstTokenTimeouts)
	return policy, true
}

func WithOpenAIStreamFirstTokenRectifier(ctx context.Context, headerAttempt, firstTokenAttempt int, responseHeaderTimeout, firstTokenTimeout time.Duration) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if (headerAttempt <= 0 && firstTokenAttempt <= 0) || (responseHeaderTimeout <= 0 && firstTokenTimeout <= 0) {
		return ctx
	}
	if headerAttempt <= 0 {
		headerAttempt = 1
	}
	if firstTokenAttempt <= 0 {
		firstTokenAttempt = 1
	}
	return context.WithValue(ctx, openAIStreamFirstTokenRectifierContextKey{}, openAIStreamFirstTokenRectifierContextValue{
		HeaderAttempt:         headerAttempt,
		FirstTokenAttempt:     firstTokenAttempt,
		ResponseHeaderTimeout: responseHeaderTimeout,
		FirstTokenTimeout:     firstTokenTimeout,
	})
}

func getOpenAIStreamFirstTokenRectifier(ctx context.Context) (openAIStreamFirstTokenRectifierContextValue, bool) {
	if ctx == nil {
		return openAIStreamFirstTokenRectifierContextValue{}, false
	}
	value, ok := ctx.Value(openAIStreamFirstTokenRectifierContextKey{}).(openAIStreamFirstTokenRectifierContextValue)
	if !ok || (value.HeaderAttempt <= 0 && value.FirstTokenAttempt <= 0) || (value.ResponseHeaderTimeout <= 0 && value.FirstTokenTimeout <= 0) {
		return openAIStreamFirstTokenRectifierContextValue{}, false
	}
	return value, true
}

func (s *OpenAIGatewayService) ResolveOpenAIStreamRectifierPolicy(ctx context.Context) OpenAIStreamRectifierPolicy {
	if policy, ok := getOpenAIStreamRectifierPolicyFromContext(ctx); ok {
		return policy
	}
	policy := defaultOpenAIStreamRectifierPolicy()
	if s != nil && s.settingService != nil {
		policy.Enabled = s.settingService.IsOpenAIStreamRectifierEnabled(ctx)
		responseHeaderTimeouts, firstTokenTimeouts := s.settingService.GetOpenAIStreamRectifierTimeouts(ctx)
		if values := durationsFromPositiveSeconds(responseHeaderTimeouts); len(values) > 0 {
			policy.ResponseHeaderTimeouts = values
		}
		if values := durationsFromPositiveSeconds(firstTokenTimeouts); len(values) > 0 {
			policy.FirstTokenTimeouts = values
		}
		return policy
	}
	if s != nil && s.cfg != nil {
		policy.Enabled = s.cfg.Gateway.OpenAIStreamRectifierEnabled
		if values := durationsFromPositiveSeconds(s.cfg.Gateway.OpenAIStreamResponseHeaderRectifierTimeouts); len(values) > 0 {
			policy.ResponseHeaderTimeouts = values
		}
		if values := durationsFromPositiveSeconds(s.cfg.Gateway.OpenAIStreamFirstTokenRectifierTimeouts); len(values) > 0 {
			policy.FirstTokenTimeouts = values
		}
	}
	return policy
}

func (s *OpenAIGatewayService) PrepareOpenAIStreamFirstTokenRectifierContext(ctx context.Context, stream bool, headerAttempt, firstTokenAttempt int) context.Context {
	if !stream {
		return ctx
	}
	if headerAttempt <= 0 {
		headerAttempt = 1
	}
	if firstTokenAttempt <= 0 {
		firstTokenAttempt = 1
	}
	policy := s.ResolveOpenAIStreamRectifierPolicy(ctx)
	ctx = WithOpenAIStreamRectifierPolicy(ctx, policy)
	if !policy.Enabled {
		return ctx
	}
	responseHeaderTimeout := policy.ResponseHeaderTimeoutForAttempt(headerAttempt)
	firstTokenTimeout := policy.FirstTokenTimeoutForAttempt(firstTokenAttempt)
	if responseHeaderTimeout <= 0 && firstTokenTimeout <= 0 {
		return ctx
	}
	return WithOpenAIStreamFirstTokenRectifier(ctx, headerAttempt, firstTokenAttempt, responseHeaderTimeout, firstTokenTimeout)
}

func (s *OpenAIGatewayService) EnsureOpenAIStreamFirstTokenRectifierContext(ctx context.Context, stream bool, headerAttempt, firstTokenAttempt int) context.Context {
	if !stream {
		return ctx
	}
	if _, ok := getOpenAIStreamFirstTokenRectifier(ctx); ok {
		return ctx
	}
	if headerAttempt <= 0 {
		headerAttempt = 1
	}
	if firstTokenAttempt <= 0 {
		firstTokenAttempt = 1
	}
	return s.PrepareOpenAIStreamFirstTokenRectifierContext(ctx, stream, headerAttempt, firstTokenAttempt)
}

func (s *OpenAIGatewayService) IsOpenAIStreamRectifierEnabled(ctx context.Context) bool {
	if s != nil && s.settingService != nil {
		return s.settingService.IsOpenAIStreamRectifierEnabled(ctx)
	}
	if s != nil && s.cfg != nil {
		return s.cfg.Gateway.OpenAIStreamRectifierEnabled
	}
	return true
}

func openAIStreamRectifierTimeoutForPhase(rectifier openAIStreamFirstTokenRectifierContextValue, phase string) time.Duration {
	switch strings.TrimSpace(strings.ToLower(phase)) {
	case "response_header":
		return rectifier.ResponseHeaderTimeout
	default:
		return rectifier.FirstTokenTimeout
	}
}

func openAIStreamRectifierAttemptForPhase(rectifier openAIStreamFirstTokenRectifierContextValue, phase string) int {
	switch strings.TrimSpace(strings.ToLower(phase)) {
	case "response_header":
		return rectifier.HeaderAttempt
	default:
		return rectifier.FirstTokenAttempt
	}
}

func openAIStreamRectifierLogFields(account *Account, rectifier openAIStreamFirstTokenRectifierContextValue, timeout time.Duration, phase string) []zap.Field {
	fields := []zap.Field{
		zap.Int("attempt", openAIStreamRectifierAttemptForPhase(rectifier, phase)),
		zap.Int("header_attempt", rectifier.HeaderAttempt),
		zap.Int("first_token_attempt", rectifier.FirstTokenAttempt),
		zap.Duration("timeout", timeout),
		zap.Int64("timeout_ms", timeout.Milliseconds()),
		zap.String("phase", phase),
	}
	if account == nil {
		return fields
	}
	fields = append(fields, zap.Int64("account_id", account.ID))
	if account.Name != "" {
		fields = append(fields, zap.String("account_name", account.Name))
	}
	return fields
}

func (s *OpenAIGatewayService) logOpenAIStreamFirstTokenRectifierEnabled(ctx context.Context, account *Account, rectifier openAIStreamFirstTokenRectifierContextValue) {
	timeout := rectifier.FirstTokenTimeout
	if timeout <= 0 {
		return
	}
	logger.FromContext(ctx).Info("openai.stream_first_token_rectifier_enabled", openAIStreamRectifierLogFields(account, rectifier, timeout, "first_token")...)
}

func (s *OpenAIGatewayService) logOpenAIStreamResponseHeaderRectifierEnabled(ctx context.Context, account *Account, rectifier openAIStreamFirstTokenRectifierContextValue) {
	timeout := rectifier.ResponseHeaderTimeout
	if timeout <= 0 {
		return
	}
	logger.FromContext(ctx).Info("openai.stream_response_header_rectifier_enabled", openAIStreamRectifierLogFields(account, rectifier, timeout, "response_header")...)
}

func (s *OpenAIGatewayService) logOpenAIStreamFirstTokenRectifierObserved(ctx context.Context, account *Account, rectifier openAIStreamFirstTokenRectifierContextValue, endToEndFirstTokenMs int, streamFirstEventMs int) {
	timeout := rectifier.FirstTokenTimeout
	if timeout <= 0 {
		return
	}
	fields := append(openAIStreamRectifierLogFields(account, rectifier, timeout, "first_token"),
		zap.Int("first_token_ms", endToEndFirstTokenMs),
		zap.Int("stream_first_event_ms", streamFirstEventMs),
		zap.Bool("within_timeout", int64(streamFirstEventMs) <= timeout.Milliseconds()),
	)
	logger.FromContext(ctx).Info("openai.stream_first_token_rectifier_first_token", fields...)
}

func (s *OpenAIGatewayService) ApplyOpenAIStreamResponseHeaderRectifierContext(ctx context.Context, stream bool) context.Context {
	if !stream {
		return ctx
	}
	rectifier, ok := getOpenAIStreamFirstTokenRectifier(ctx)
	if !ok || rectifier.ResponseHeaderTimeout <= 0 {
		return ctx
	}
	return WithHTTPUpstreamResponseHeaderTimeout(ctx, rectifier.ResponseHeaderTimeout)
}

type OpenAIRectifierTimeoutError struct {
	StatusCode        int
	Phase             string
	Attempt           int
	HeaderAttempt     int
	FirstTokenAttempt int
	Timeout           time.Duration
	Message           string
}

func (e *OpenAIRectifierTimeoutError) Error() string {
	if e == nil {
		return "openai rectifier timeout"
	}
	if strings.TrimSpace(e.Message) != "" {
		return e.Message
	}
	phase := strings.TrimSpace(e.Phase)
	if phase == "" {
		phase = "first_token"
	}
	return fmt.Sprintf("openai %s timeout after %s on attempt %d", phase, e.Timeout, e.Attempt)
}

func AsOpenAIRectifierTimeoutError(err error) (*OpenAIRectifierTimeoutError, bool) {
	var target *OpenAIRectifierTimeoutError
	if err == nil || !errors.As(err, &target) || target == nil {
		return nil, false
	}
	return target, true
}

func openAIRectifierTimeoutOpsDetail(timeoutErr *OpenAIRectifierTimeoutError) string {
	if timeoutErr == nil {
		return ""
	}
	return fmt.Sprintf(
		"phase=%s timeout_ms=%d attempt=%d header_attempt=%d first_token_attempt=%d",
		strings.TrimSpace(timeoutErr.Phase),
		timeoutErr.Timeout.Milliseconds(),
		timeoutErr.Attempt,
		timeoutErr.HeaderAttempt,
		timeoutErr.FirstTokenAttempt,
	)
}

func recordOpenAIRectifierTimeoutOpsContext(c *gin.Context, account *Account, timeoutErr *OpenAIRectifierTimeoutError) {
	if c == nil || timeoutErr == nil {
		return
	}
	message := sanitizeUpstreamErrorMessage(timeoutErr.Message)
	detail := openAIRectifierTimeoutOpsDetail(timeoutErr)
	setOpsUpstreamError(c, timeoutErr.StatusCode, message, detail)

	event := OpsUpstreamErrorEvent{
		Platform:           PlatformOpenAI,
		UpstreamStatusCode: timeoutErr.StatusCode,
		Kind:               "request_error:rectifier_timeout",
		Message:            message,
		Detail:             detail,
	}
	if account != nil {
		if account.ID > 0 {
			event.AccountID = account.ID
		}
		event.AccountName = strings.TrimSpace(account.Name)
		if platform := strings.TrimSpace(account.Platform); platform != "" {
			event.Platform = platform
		}
	}
	appendOpsUpstreamError(c, event)
}

func (s *OpenAIGatewayService) newOpenAIStreamFirstTokenRectifierTimeoutError(ctx context.Context, c *gin.Context, account *Account, rectifier openAIStreamFirstTokenRectifierContextValue, phase string) error {
	phase = strings.TrimSpace(strings.ToLower(phase))
	if phase == "" {
		phase = "first_token"
	}
	timeout := openAIStreamRectifierTimeoutForPhase(rectifier, phase)
	if timeout <= 0 {
		timeout = rectifier.FirstTokenTimeout
	}
	attempt := openAIStreamRectifierAttemptForPhase(rectifier, phase)
	message := fmt.Sprintf("OpenAI stream first token timeout after %ds on first-token attempt %d", int(timeout/time.Second), attempt)
	if phase == "response_header" {
		message = fmt.Sprintf("OpenAI response header timeout after %ds on header attempt %d", int(timeout/time.Second), attempt)
	}
	timeoutErr := &OpenAIRectifierTimeoutError{
		StatusCode:        http.StatusBadGateway,
		Phase:             phase,
		Attempt:           attempt,
		HeaderAttempt:     rectifier.HeaderAttempt,
		FirstTokenAttempt: rectifier.FirstTokenAttempt,
		Timeout:           timeout,
		Message:           message,
	}
	recordOpenAIRectifierTimeoutOpsContext(c, account, timeoutErr)
	return timeoutErr
}

//nolint:unused // 预留给后续从配置直接构造整流器策略
func openAIStreamRectifierPolicyFromConfig(cfg *config.Config) OpenAIStreamRectifierPolicy {
	policy := defaultOpenAIStreamRectifierPolicy()
	if cfg == nil {
		return policy
	}
	policy.Enabled = cfg.Gateway.OpenAIStreamRectifierEnabled
	if values := durationsFromPositiveSeconds(cfg.Gateway.OpenAIStreamResponseHeaderRectifierTimeouts); len(values) > 0 {
		policy.ResponseHeaderTimeouts = values
	}
	if values := durationsFromPositiveSeconds(cfg.Gateway.OpenAIStreamFirstTokenRectifierTimeouts); len(values) > 0 {
		policy.FirstTokenTimeouts = values
	}
	return policy
}
