package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	OpsGatewayPrepareLatencyMsKey   = "ops_gateway_prepare_latency_ms"
	OpsStreamFirstEventLatencyMsKey = "ops_stream_first_event_latency_ms"
)

type openAIStreamFirstTokenRectifierContextValue struct {
	HeaderAttempt         int
	FirstTokenAttempt     int
	ResponseHeaderTimeout time.Duration
	FirstTokenTimeout     time.Duration
}

type UpstreamResponseHeaderTimeoutError struct {
	Timeout time.Duration
	Err     error
}

func (e *UpstreamResponseHeaderTimeoutError) Error() string {
	if e == nil {
		return "upstream response header timeout"
	}
	if e.Err == nil {
		return fmt.Sprintf("upstream response header timeout after %s", e.Timeout)
	}
	return e.Err.Error()
}

func (e *UpstreamResponseHeaderTimeoutError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func AsUpstreamResponseHeaderTimeoutError(err error) (*UpstreamResponseHeaderTimeoutError, bool) {
	var target *UpstreamResponseHeaderTimeoutError
	if err == nil || !errors.As(err, &target) || target == nil {
		return nil, false
	}
	return target, true
}

func getOpenAIStreamFirstTokenRectifier(ctx context.Context) (openAIStreamFirstTokenRectifierContextValue, bool) {
	_ = ctx
	return openAIStreamFirstTokenRectifierContextValue{}, false
}

func (s *OpenAIGatewayService) EnsureOpenAIStreamFirstTokenRectifierContext(ctx context.Context, stream bool, headerAttempt, firstTokenAttempt int) context.Context {
	_ = s
	_ = stream
	_ = headerAttempt
	_ = firstTokenAttempt
	return ctx
}

func (s *OpenAIGatewayService) ApplyOpenAIStreamResponseHeaderRectifierContext(ctx context.Context, stream bool) context.Context {
	_ = s
	_ = stream
	return ctx
}

func (s *OpenAIGatewayService) logOpenAIStreamFirstTokenRectifierEnabled(ctx context.Context, account *Account, rectifier openAIStreamFirstTokenRectifierContextValue) {
	_ = s
	_ = ctx
	_ = account
	_ = rectifier
}

func (s *OpenAIGatewayService) logOpenAIStreamResponseHeaderRectifierEnabled(ctx context.Context, account *Account, rectifier openAIStreamFirstTokenRectifierContextValue) {
	_ = s
	_ = ctx
	_ = account
	_ = rectifier
}

func (s *OpenAIGatewayService) logOpenAIStreamFirstTokenRectifierObserved(ctx context.Context, account *Account, rectifier openAIStreamFirstTokenRectifierContextValue, endToEndFirstTokenMs int, streamFirstEventMs int) {
	_ = s
	_ = ctx
	_ = account
	_ = rectifier
	_ = endToEndFirstTokenMs
	_ = streamFirstEventMs
}

func (s *OpenAIGatewayService) newOpenAIStreamFirstTokenRectifierTimeoutError(ctx context.Context, c *gin.Context, account *Account, rectifier openAIStreamFirstTokenRectifierContextValue, phase string) error {
	_ = s
	_ = ctx
	_ = c
	_ = account
	_ = rectifier
	phase = strings.TrimSpace(strings.ToLower(phase))
	if phase == "" {
		phase = "first_token"
	}
	return fmt.Errorf("openai stream %s timeout", phase)
}
