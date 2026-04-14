package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
)

type httpUpstreamResponseHeaderTimeoutContextKey struct{}
type httpUpstreamPoolGroupIDContextKey struct{}

func WithHTTPUpstreamResponseHeaderTimeout(ctx context.Context, timeout time.Duration) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout <= 0 {
		return ctx
	}
	return context.WithValue(ctx, httpUpstreamResponseHeaderTimeoutContextKey{}, timeout)
}

func HTTPUpstreamResponseHeaderTimeoutFromContext(ctx context.Context) (time.Duration, bool) {
	if ctx == nil {
		return 0, false
	}
	timeout, ok := ctx.Value(httpUpstreamResponseHeaderTimeoutContextKey{}).(time.Duration)
	if !ok || timeout <= 0 {
		return 0, false
	}
	return timeout, true
}

func WithHTTPUpstreamPoolGroupID(ctx context.Context, groupID int64) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if groupID <= 0 {
		return ctx
	}
	return context.WithValue(ctx, httpUpstreamPoolGroupIDContextKey{}, groupID)
}

func HTTPUpstreamPoolGroupIDFromContext(ctx context.Context) (int64, bool) {
	if ctx == nil {
		return 0, false
	}
	groupID, ok := ctx.Value(httpUpstreamPoolGroupIDContextKey{}).(int64)
	if !ok || groupID <= 0 {
		return 0, false
	}
	return groupID, true
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

// HTTPUpstream 上游 HTTP 请求接口
// 用于向上游 API（Claude、OpenAI、Gemini 等）发送请求
type HTTPUpstream interface {
	// Do 执行 HTTP 请求（不启用 TLS 指纹）
	Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error)

	// DoWithTLS 执行带 TLS 指纹伪装的 HTTP 请求
	//
	// profile 参数:
	//   - nil: 不启用 TLS 指纹，行为与 Do 方法相同
	//   - non-nil: 使用指定的 Profile 进行 TLS 指纹伪装
	//
	// Profile 由调用方通过 TLSFingerprintProfileService 解析后传入，
	// 支持按账号绑定的数据库 profile 或内置默认 profile。
	DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, profile *tlsfingerprint.Profile) (*http.Response, error)
}
