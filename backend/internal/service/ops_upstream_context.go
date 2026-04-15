package service

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// Gin context keys used by Ops error logger for capturing upstream error details.
// These keys are set by gateway services and consumed by handler/ops_error_logger.go.
const (
	OpsUpstreamStatusCodeKey   = "ops_upstream_status_code"
	OpsUpstreamErrorMessageKey = "ops_upstream_error_message"
	OpsUpstreamErrorDetailKey  = "ops_upstream_error_detail"
	OpsUpstreamErrorsKey       = "ops_upstream_errors"

	// Best-effort capture of the current upstream request body so ops can
	// retry the specific upstream attempt (not just the client request).
	// This value is sanitized+trimmed before being persisted.
	OpsUpstreamRequestBodyKey = "ops_upstream_request_body"

	// Optional stage latencies (milliseconds) for troubleshooting and alerting.
	OpsAuthLatencyMsKey             = "ops_auth_latency_ms"
	OpsRoutingLatencyMsKey          = "ops_routing_latency_ms"
	OpsGatewayPrepareLatencyMsKey   = "ops_gateway_prepare_latency_ms"
	OpsUpstreamLatencyMsKey         = "ops_upstream_latency_ms"
	OpsResponseLatencyMsKey         = "ops_response_latency_ms"
	OpsTimeToFirstTokenMsKey        = "ops_time_to_first_token_ms"
	OpsStreamFirstEventLatencyMsKey = "ops_stream_first_event_latency_ms"
	// OpenAI WS 关键观测字段
	OpsOpenAIWSQueueWaitMsKey = "ops_openai_ws_queue_wait_ms"
	OpsOpenAIWSConnPickMsKey  = "ops_openai_ws_conn_pick_ms"
	OpsOpenAIWSConnReusedKey  = "ops_openai_ws_conn_reused"
	OpsOpenAIWSConnIDKey      = "ops_openai_ws_conn_id"

	// OpsSkipPassthroughKey 由 applyErrorPassthroughRule 在命中 skip_monitoring=true 的规则时设置。
	// ops_error_logger 中间件检查此 key，为 true 时跳过错误记录。
	OpsSkipPassthroughKey = "ops_skip_passthrough"
	// OpsRectifierTimeoutExhaustedKey 标记请求最终因为 OpenAI 整流器 exhausted 返回错误。
	// ops_error_logger 会结合 Ops 高级设置决定是否忽略该类错误记录。
	OpsRectifierTimeoutExhaustedKey = "ops_rectifier_timeout_exhausted"
	// OpsRectifierRetryCountKey 保存单个请求内真正发生的 rectifier retry 总次数。
	// 仅在 advance 分支累加，不包含 reconnect / failover reset 等其他重试语义。
	OpsRectifierRetryCountKey = "ops_rectifier_retry_count"
	// OpsOpenAIAccountSelectFailureKey 保存 OpenAI 账号选择失败的结构化排障信息。
	OpsOpenAIAccountSelectFailureKey = "ops_openai_account_select_failure"
)

func setOpsUpstreamRequestBody(c *gin.Context, body []byte) {
	if c == nil || len(body) == 0 {
		return
	}
	// 热路径避免 string(body) 额外分配，按需在落库前再转换。
	c.Set(OpsUpstreamRequestBodyKey, body)
}

func SetOpsLatencyMs(c *gin.Context, key string, value int64) {
	if c == nil || strings.TrimSpace(key) == "" || value < 0 {
		return
	}
	c.Set(key, value)
}

//nolint:unused // 预留给后续上下文指标序列化
func getContextLatencyIntPtr(c *gin.Context, key string) *int {
	if c == nil || strings.TrimSpace(key) == "" {
		return nil
	}
	v, ok := c.Get(key)
	if !ok {
		return nil
	}
	var ms int64
	switch t := v.(type) {
	case int:
		ms = int64(t)
	case int32:
		ms = int64(t)
	case int64:
		ms = t
	case float64:
		ms = int64(t)
	default:
		return nil
	}
	if ms < 0 {
		return nil
	}
	value := int(ms)
	return &value
}

//nolint:unused // 预留给后续上下文指标序列化
func int64ToIntPtr(value int64) *int {
	if value < 0 {
		return nil
	}
	v := int(value)
	return &v
}

// SetOpsUpstreamError is the exported wrapper for setOpsUpstreamError, used by
// handler-layer code (e.g. failover-exhausted paths) that needs to record the
// original upstream status code before mapping it to a client-facing code.
func SetOpsUpstreamError(c *gin.Context, upstreamStatusCode int, upstreamMessage, upstreamDetail string) {
	setOpsUpstreamError(c, upstreamStatusCode, upstreamMessage, upstreamDetail)
}

func MarkOpsRectifierTimeoutExhausted(c *gin.Context) {
	if c == nil {
		return
	}
	c.Set(OpsRectifierTimeoutExhaustedKey, true)
}

func IsOpsRectifierTimeoutExhausted(c *gin.Context) bool {
	if c == nil {
		return false
	}
	v, ok := c.Get(OpsRectifierTimeoutExhaustedKey)
	if !ok {
		return false
	}
	flag, _ := v.(bool)
	return flag
}

func GetOpsRectifierRetryCount(c *gin.Context) int {
	if c == nil {
		return 0
	}
	v, ok := c.Get(OpsRectifierRetryCountKey)
	if !ok {
		return 0
	}
	switch value := v.(type) {
	case int:
		if value > 0 {
			return value
		}
	case int32:
		if value > 0 {
			return int(value)
		}
	case int64:
		if value > 0 {
			return int(value)
		}
	case float64:
		if value > 0 {
			return int(value)
		}
	}
	return 0
}

func SetOpsRectifierRetryCount(c *gin.Context, count int) {
	if c == nil || count < 0 {
		return
	}
	c.Set(OpsRectifierRetryCountKey, count)
}

func IncrementOpsRectifierRetryCount(c *gin.Context) int {
	count := GetOpsRectifierRetryCount(c) + 1
	SetOpsRectifierRetryCount(c, count)
	return count
}

func setOpsUpstreamError(c *gin.Context, upstreamStatusCode int, upstreamMessage, upstreamDetail string) {
	if c == nil {
		return
	}
	if upstreamStatusCode > 0 {
		c.Set(OpsUpstreamStatusCodeKey, upstreamStatusCode)
	}
	if msg := strings.TrimSpace(upstreamMessage); msg != "" {
		c.Set(OpsUpstreamErrorMessageKey, msg)
	}
	if detail := strings.TrimSpace(upstreamDetail); detail != "" {
		c.Set(OpsUpstreamErrorDetailKey, detail)
	}
}

type OpsOpenAIAccountSelectFailure struct {
	Layer                  string  `json:"layer,omitempty"`
	FailureReason          string  `json:"failure_reason,omitempty"`
	FailureDetail          string  `json:"failure_detail,omitempty"`
	Error                  string  `json:"error,omitempty"`
	CandidateCount         int     `json:"candidate_count,omitempty"`
	TopK                   int     `json:"top_k,omitempty"`
	LoadSkew               float64 `json:"load_skew,omitempty"`
	WarmPoolTried          bool    `json:"warm_pool_tried,omitempty"`
	WarmPoolCandidateCount int     `json:"warm_pool_candidate_count,omitempty"`
	ExcludedAccountCount   int     `json:"excluded_account_count,omitempty"`
}

func SetOpsOpenAIAccountSelectFailure(c *gin.Context, decision OpenAIAccountScheduleDecision, err error, excludedAccountCount int) {
	if c == nil {
		return
	}
	failure := OpsOpenAIAccountSelectFailure{
		Layer:                  strings.TrimSpace(decision.Layer),
		FailureReason:          strings.TrimSpace(decision.FailureReason),
		FailureDetail:          strings.TrimSpace(decision.FailureDetail),
		CandidateCount:         decision.CandidateCount,
		TopK:                   decision.TopK,
		LoadSkew:               decision.LoadSkew,
		WarmPoolTried:          decision.WarmPoolTried,
		WarmPoolCandidateCount: decision.WarmPoolCandidateCount,
		ExcludedAccountCount:   excludedAccountCount,
	}
	if err != nil {
		failure.Error = strings.TrimSpace(err.Error())
	}
	if failure.FailureReason == "" && failure.FailureDetail == "" && failure.Error == "" {
		return
	}
	c.Set(OpsOpenAIAccountSelectFailureKey, failure)
}

func GetOpsOpenAIAccountSelectFailure(c *gin.Context) *OpsOpenAIAccountSelectFailure {
	if c == nil {
		return nil
	}
	v, ok := c.Get(OpsOpenAIAccountSelectFailureKey)
	if !ok {
		return nil
	}
	failure, ok := v.(OpsOpenAIAccountSelectFailure)
	if !ok {
		return nil
	}
	copied := failure
	return &copied
}

// OpsUpstreamErrorEvent describes one upstream error attempt during a single gateway request.
// It is stored in ops_error_logs.upstream_errors as a JSON array.
type OpsUpstreamErrorEvent struct {
	AtUnixMs int64 `json:"at_unix_ms,omitempty"`

	// Passthrough 表示本次请求是否命中“原样透传（仅替换认证）”分支。
	// 该字段用于排障与灰度评估；存入 JSON，不涉及 DB schema 变更。
	Passthrough bool `json:"passthrough,omitempty"`

	// Context
	Platform    string `json:"platform,omitempty"`
	AccountID   int64  `json:"account_id,omitempty"`
	AccountName string `json:"account_name,omitempty"`

	// Outcome
	UpstreamStatusCode int    `json:"upstream_status_code,omitempty"`
	UpstreamRequestID  string `json:"upstream_request_id,omitempty"`

	// UpstreamURL is the actual upstream URL that was called (host + path, query/fragment stripped).
	// Helps debug 404/routing errors by showing which endpoint was targeted.
	UpstreamURL string `json:"upstream_url,omitempty"`

	// Best-effort upstream request capture (sanitized+trimmed).
	// Required for retrying a specific upstream attempt.
	UpstreamRequestBody string `json:"upstream_request_body,omitempty"`

	// Best-effort upstream response capture (sanitized+trimmed).
	UpstreamResponseBody string `json:"upstream_response_body,omitempty"`

	// Kind: http_error | request_error | retry_exhausted | failover
	Kind string `json:"kind,omitempty"`

	Message string `json:"message,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

func appendOpsUpstreamError(c *gin.Context, ev OpsUpstreamErrorEvent) {
	if c == nil {
		return
	}
	if ev.AtUnixMs <= 0 {
		ev.AtUnixMs = time.Now().UnixMilli()
	}
	ev.Platform = strings.TrimSpace(ev.Platform)
	ev.UpstreamRequestID = strings.TrimSpace(ev.UpstreamRequestID)
	ev.UpstreamRequestBody = strings.TrimSpace(ev.UpstreamRequestBody)
	ev.UpstreamResponseBody = strings.TrimSpace(ev.UpstreamResponseBody)
	ev.Kind = strings.TrimSpace(ev.Kind)
	ev.UpstreamURL = strings.TrimSpace(ev.UpstreamURL)
	ev.Message = strings.TrimSpace(ev.Message)
	ev.Detail = strings.TrimSpace(ev.Detail)
	if ev.Message != "" {
		ev.Message = sanitizeUpstreamErrorMessage(ev.Message)
	}

	// If the caller didn't explicitly pass upstream request body but the gateway
	// stored it on the context, attach it so ops can retry this specific attempt.
	if ev.UpstreamRequestBody == "" {
		if v, ok := c.Get(OpsUpstreamRequestBodyKey); ok {
			switch raw := v.(type) {
			case string:
				ev.UpstreamRequestBody = strings.TrimSpace(raw)
			case []byte:
				ev.UpstreamRequestBody = strings.TrimSpace(string(raw))
			}
		}
	}

	var existing []*OpsUpstreamErrorEvent
	if v, ok := c.Get(OpsUpstreamErrorsKey); ok {
		if arr, ok := v.([]*OpsUpstreamErrorEvent); ok {
			existing = arr
		}
	}

	evCopy := ev
	existing = append(existing, &evCopy)
	c.Set(OpsUpstreamErrorsKey, existing)

	checkSkipMonitoringForUpstreamEvent(c, &evCopy)
}

// checkSkipMonitoringForUpstreamEvent checks whether the upstream error event
// matches a passthrough rule with skip_monitoring=true and, if so, sets the
// OpsSkipPassthroughKey on the context.  This ensures intermediate retry /
// failover errors (which never go through the final applyErrorPassthroughRule
// path) can still suppress ops_error_logs recording.
func checkSkipMonitoringForUpstreamEvent(c *gin.Context, ev *OpsUpstreamErrorEvent) {
	if ev.UpstreamStatusCode == 0 {
		return
	}

	svc := getBoundErrorPassthroughService(c)
	if svc == nil {
		return
	}

	// Use the best available body representation for keyword matching.
	// Even when body is empty, MatchRule can still match rules that only
	// specify ErrorCodes (no Keywords), so we always call it.
	body := ev.Detail
	if body == "" {
		body = ev.Message
	}

	rule := svc.MatchRule(ev.Platform, ev.UpstreamStatusCode, []byte(body))
	if rule != nil && rule.SkipMonitoring {
		c.Set(OpsSkipPassthroughKey, true)
	}
}

func marshalOpsUpstreamErrors(events []*OpsUpstreamErrorEvent) *string {
	if len(events) == 0 {
		return nil
	}
	// Ensure we always store a valid JSON value.
	raw, err := json.Marshal(events)
	if err != nil || len(raw) == 0 {
		return nil
	}
	s := string(raw)
	return &s
}

func ParseOpsUpstreamErrors(raw string) ([]*OpsUpstreamErrorEvent, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []*OpsUpstreamErrorEvent{}, nil
	}
	var out []*OpsUpstreamErrorEvent
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}

// safeUpstreamURL returns scheme + host + path from a URL, stripping query/fragment
// to avoid leaking sensitive query parameters (e.g. OAuth tokens).
func safeUpstreamURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	if idx := strings.IndexByte(rawURL, '?'); idx >= 0 {
		rawURL = rawURL[:idx]
	}
	if idx := strings.IndexByte(rawURL, '#'); idx >= 0 {
		rawURL = rawURL[:idx]
	}
	return rawURL
}
