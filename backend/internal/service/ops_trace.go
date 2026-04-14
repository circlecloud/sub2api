package service

import (
	"context"
	"log/slog"
	"strings"
	"time"
)

const opsTraceDefaultComponent = "service.ops"

// opsTrace 是 ops 慢链路/分支埋点的轻量结构化日志 builder。
//
// 典型用法：
//
//	trace := newOpsTrace("realtime_accounts").
//		WithPlatformFilter(platformFilter).
//		WithGroupID(groupIDFilter).
//		WithScope(string(scope)).
//		Start()
//
//	trace.WithBranch("cache_hit").WithCacheHit(true).WithLoadedAccountCount(len(accounts)).Info("ops_realtime_accounts")
//
// Start() 返回的 timer 会在最终输出日志时自动补上 duration_ms。
type opsTrace struct {
	attrs []slog.Attr
}

type opsTraceTimer struct {
	trace     opsTrace
	startedAt time.Time
}

func newOpsTrace(op string) opsTrace {
	return newOpsTraceWithComponent(opsTraceDefaultComponent, op)
}

func newOpsTraceWithComponent(component, op string) opsTrace {
	trace := opsTrace{}
	trace = trace.WithComponent(component)
	trace = trace.WithOp(op)
	return trace
}

func (t opsTrace) WithComponent(component string) opsTrace {
	return t.withString("component", component, true)
}

func (t opsTrace) WithOp(op string) opsTrace {
	return t.withString("op", op, true)
}

func (t opsTrace) WithBranch(branch string) opsTrace {
	return t.withString("branch", branch, false)
}

func (t opsTrace) WithCacheHit(hit bool) opsTrace {
	return t.WithAttrs(slog.Bool("cache_hit", hit))
}

func (t opsTrace) WithSingleflightShared(shared bool) opsTrace {
	return t.WithAttrs(slog.Bool("singleflight_shared", shared))
}

func (t opsTrace) WithGroupID(groupID *int64) opsTrace {
	if groupID == nil || *groupID <= 0 {
		return t
	}
	return t.WithAttrs(slog.Int64("group_id", *groupID))
}

func (t opsTrace) WithPlatformFilter(platformFilter string) opsTrace {
	return t.withString("platform_filter", platformFilter, false)
}

func (t opsTrace) WithScope(scope string) opsTrace {
	return t.withString("scope", scope, false)
}

func (t opsTrace) WithDuration(duration time.Duration) opsTrace {
	if duration < 0 {
		duration = 0
	}
	return t.WithAttrs(slog.Int64("duration_ms", duration.Milliseconds()))
}

func (t opsTrace) WithDurationSince(startedAt time.Time) opsTrace {
	if startedAt.IsZero() {
		return t
	}
	return t.WithDuration(time.Since(startedAt))
}

func (t opsTrace) WithLoadedAccountCount(count int) opsTrace {
	return t.WithAttrs(slog.Int("loaded_account_count", count))
}

func (t opsTrace) WithErr(err error) opsTrace {
	if err == nil {
		return t
	}
	return t.WithAttrs(slog.String("err", err.Error()))
}

func (t opsTrace) WithAttrs(attrs ...slog.Attr) opsTrace {
	if len(attrs) == 0 {
		return t
	}
	next := opsTrace{attrs: append([]slog.Attr(nil), t.attrs...)}
	for _, attr := range attrs {
		if strings.TrimSpace(attr.Key) == "" {
			continue
		}
		next.attrs = append(next.attrs, attr)
	}
	return next
}

func (t opsTrace) Attrs() []slog.Attr {
	return append([]slog.Attr(nil), t.attrs...)
}

func (t opsTrace) Start() opsTraceTimer {
	return opsTraceTimer{trace: t, startedAt: time.Now()}
}

func (t opsTrace) Log(ctx context.Context, level slog.Level, msg string) {
	t.log(ctx, level, msg)
}

func (t opsTrace) Debug(msg string) {
	t.log(context.Background(), slog.LevelDebug, msg)
}

func (t opsTrace) Info(msg string) {
	t.log(context.Background(), slog.LevelInfo, msg)
}

func (t opsTrace) Warn(msg string) {
	t.log(context.Background(), slog.LevelWarn, msg)
}

func (t opsTrace) Error(msg string) {
	t.log(context.Background(), slog.LevelError, msg)
}

func (t opsTrace) log(ctx context.Context, level slog.Level, msg string) {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		msg = "ops_trace"
	}
	if ctx == nil {
		ctx = context.Background()
	}
	slog.LogAttrs(ctx, level, msg, t.attrs...)
}

func (t opsTrace) withString(key, value string, allowDefault bool) opsTrace {
	value = strings.TrimSpace(value)
	if value == "" {
		if !allowDefault {
			return t
		}
		value = "unknown"
	}
	return t.WithAttrs(slog.String(key, value))
}

func (t opsTraceTimer) WithBranch(branch string) opsTraceTimer {
	t.trace = t.trace.WithBranch(branch)
	return t
}

func (t opsTraceTimer) WithCacheHit(hit bool) opsTraceTimer {
	t.trace = t.trace.WithCacheHit(hit)
	return t
}

func (t opsTraceTimer) WithSingleflightShared(shared bool) opsTraceTimer {
	t.trace = t.trace.WithSingleflightShared(shared)
	return t
}

func (t opsTraceTimer) WithGroupID(groupID *int64) opsTraceTimer {
	t.trace = t.trace.WithGroupID(groupID)
	return t
}

func (t opsTraceTimer) WithPlatformFilter(platformFilter string) opsTraceTimer {
	t.trace = t.trace.WithPlatformFilter(platformFilter)
	return t
}

func (t opsTraceTimer) WithScope(scope string) opsTraceTimer {
	t.trace = t.trace.WithScope(scope)
	return t
}

func (t opsTraceTimer) WithLoadedAccountCount(count int) opsTraceTimer {
	t.trace = t.trace.WithLoadedAccountCount(count)
	return t
}

func (t opsTraceTimer) WithErr(err error) opsTraceTimer {
	t.trace = t.trace.WithErr(err)
	return t
}

func (t opsTraceTimer) WithAttrs(attrs ...slog.Attr) opsTraceTimer {
	t.trace = t.trace.WithAttrs(attrs...)
	return t
}

func (t opsTraceTimer) Attrs() []slog.Attr {
	return t.trace.WithDurationSince(t.startedAt).Attrs()
}

func (t opsTraceTimer) Elapsed() time.Duration {
	if t.startedAt.IsZero() {
		return 0
	}
	return time.Since(t.startedAt)
}

func (t opsTraceTimer) Log(ctx context.Context, level slog.Level, msg string) {
	t.trace.WithDurationSince(t.startedAt).Log(ctx, level, msg)
}

func (t opsTraceTimer) Debug(msg string) {
	t.trace.WithDurationSince(t.startedAt).Debug(msg)
}

func (t opsTraceTimer) Info(msg string) {
	t.trace.WithDurationSince(t.startedAt).Info(msg)
}

func (t opsTraceTimer) Warn(msg string) {
	t.trace.WithDurationSince(t.startedAt).Warn(msg)
}

func (t opsTraceTimer) Error(msg string) {
	t.trace.WithDurationSince(t.startedAt).Error(msg)
}
