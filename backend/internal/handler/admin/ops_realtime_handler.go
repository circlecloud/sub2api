package admin

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// GetConcurrencyStats returns real-time concurrency usage aggregated by platform/group/account.
// GET /api/v1/admin/ops/concurrency
func (h *OpsHandler) GetConcurrencyStats(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	if !h.opsService.IsRealtimeMonitoringEnabled(c.Request.Context()) {
		response.Success(c, gin.H{
			"enabled":   false,
			"platform":  map[string]*service.PlatformConcurrencyInfo{},
			"group":     map[int64]*service.GroupConcurrencyInfo{},
			"account":   map[int64]*service.AccountConcurrencyInfo{},
			"timestamp": time.Now().UTC(),
		})
		return
	}

	platformFilter := strings.TrimSpace(c.Query("platform"))
	var groupID *int64
	if v := strings.TrimSpace(c.Query("group_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid group_id")
			return
		}
		groupID = &id
	}

	includeAccount := parseOpsExplicitIncludeAccount(c.Query("include_account"))
	scopeRaw := strings.TrimSpace(c.Query("scope"))
	if scopeRaw != "" {
		switch strings.ToLower(scopeRaw) {
		case "platform", "group", "account":
		default:
			response.BadRequest(c, "Invalid scope")
			return
		}
	}
	if strings.EqualFold(scopeRaw, "account") {
		includeAccount = true
	}
	scope := normalizeOpsRealtimeScopeForHandler(scopeRaw, platformFilter, groupID, includeAccount)
	platform, group, account, collectedAt, err := h.opsService.GetConcurrencyStatsWithOptions(c.Request.Context(), platformFilter, groupID, scope)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	payload := gin.H{
		"enabled":  true,
		"platform": platform,
		"group":    group,
		"account":  account,
	}
	if collectedAt != nil {
		payload["timestamp"] = collectedAt.UTC()
	}
	response.Success(c, payload)
}

// GetUserConcurrencyStats returns real-time concurrency usage for all active users.
// GET /api/v1/admin/ops/user-concurrency
func (h *OpsHandler) GetUserConcurrencyStats(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	if !h.opsService.IsRealtimeMonitoringEnabled(c.Request.Context()) {
		response.Success(c, gin.H{
			"enabled":   false,
			"user":      map[int64]*service.UserConcurrencyInfo{},
			"timestamp": time.Now().UTC(),
		})
		return
	}

	users, collectedAt, err := h.opsService.GetUserConcurrencyStats(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	payload := gin.H{
		"enabled": true,
		"user":    users,
	}
	if collectedAt != nil {
		payload["timestamp"] = collectedAt.UTC()
	}
	response.Success(c, payload)
}

// GetAccountAvailability returns account availability statistics.
// GET /api/v1/admin/ops/account-availability
//
// Query params:
// - platform: optional
// - group_id: optional
func (h *OpsHandler) GetAccountAvailability(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	if !h.opsService.IsRealtimeMonitoringEnabled(c.Request.Context()) {
		response.Success(c, gin.H{
			"enabled":   false,
			"platform":  map[string]*service.PlatformAvailability{},
			"group":     map[int64]*service.GroupAvailability{},
			"account":   map[int64]*service.AccountAvailability{},
			"timestamp": time.Now().UTC(),
		})
		return
	}

	platform := strings.TrimSpace(c.Query("platform"))
	var groupID *int64
	if v := strings.TrimSpace(c.Query("group_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid group_id")
			return
		}
		groupID = &id
	}

	includeAccount := parseOpsExplicitIncludeAccount(c.Query("include_account"))
	scopeRaw := strings.TrimSpace(c.Query("scope"))
	if scopeRaw != "" {
		switch strings.ToLower(scopeRaw) {
		case "platform", "group", "account":
		default:
			response.BadRequest(c, "Invalid scope")
			return
		}
	}
	if strings.EqualFold(scopeRaw, "account") {
		includeAccount = true
	}
	scope := normalizeOpsRealtimeScopeForHandler(scopeRaw, platform, groupID, includeAccount)
	platformStats, groupStats, accountStats, collectedAt, err := h.opsService.GetAccountAvailabilityStatsWithOptions(c.Request.Context(), platform, groupID, scope)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	payload := gin.H{
		"enabled":  true,
		"platform": platformStats,
		"group":    groupStats,
		"account":  accountStats,
	}
	if collectedAt != nil {
		payload["timestamp"] = collectedAt.UTC()
	}
	response.Success(c, payload)
}

// GetOpenAIWarmPoolStats returns realtime OpenAI warm-pool visibility data.
// GET /api/v1/admin/ops/openai-warm-pool
//
// Query params:
// - group_id: optional
// - include_account: optional
// - accounts_only: optional
// - account_state: optional
// - page/page_size: optional, only applied for ready-list requests
func (h *OpsHandler) GetOpenAIWarmPoolStats(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	var groupID *int64
	if v := strings.TrimSpace(c.Query("group_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid group_id")
			return
		}
		groupID = &id
	}

	includeAccount := parseOpsExplicitIncludeAccount(c.Query("include_account"))
	accountState := parseOpsWarmPoolAccountState(c.Query("account_state"))
	if strings.TrimSpace(c.Query("account_state")) != "" && accountState == "" {
		response.BadRequest(c, "Invalid account_state")
		return
	}
	accountsOnly := parseOpsExplicitIncludeAccount(strings.TrimSpace(c.Query("accounts_only")))
	if accountsOnly {
		includeAccount = true
	}

	page, pageSize, paginate := shouldPaginateOpsWarmPoolReadyList(c, includeAccount, accountsOnly, accountState)
	var stats *service.OpsOpenAIWarmPoolStats
	var err error
	if paginate {
		stats, err = h.opsService.GetOpenAIWarmPoolStatsWithPage(c.Request.Context(), groupID, includeAccount, accountState, accountsOnly, page, pageSize)
	} else {
		stats, err = h.opsService.GetOpenAIWarmPoolStatsWithOptions(c.Request.Context(), groupID, includeAccount, accountState, accountsOnly)
	}
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, stats)
}

// TriggerOpenAIWarmPoolGlobalRefill manually refills the OpenAI global warm pool.
// POST /api/v1/admin/ops/openai-warm-pool/refill-global
func (h *OpsHandler) TriggerOpenAIWarmPoolGlobalRefill(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.TriggerOpenAIWarmPoolGlobalRefill(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"ok": true})
}

//nolint:unused // 预留给后续 ops 账户明细筛选接线
func parseOpsIncludeAccount(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

func parseOpsExplicitIncludeAccount(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func normalizeOpsRealtimeScopeForHandler(raw string, platformFilter string, groupIDFilter *int64, includeAccount bool) service.OpsRealtimeScope {
	scopeRaw := strings.TrimSpace(raw)
	if strings.EqualFold(scopeRaw, "account") {
		includeAccount = true
	}
	if scopeRaw == "" && groupIDFilter != nil && *groupIDFilter > 0 {
		if includeAccount {
			scopeRaw = "account"
		} else {
			scopeRaw = "group"
		}
	}
	if scopeRaw == "" && strings.TrimSpace(platformFilter) != "" {
		scopeRaw = "group"
	}
	if scopeRaw == "" {
		scopeRaw = "platform"
	}
	return service.NormalizeOpsRealtimeScope(scopeRaw, platformFilter, groupIDFilter, includeAccount)
}

func parseOpsWarmPoolAccountState(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "ready", "probing", "cooling", "network_error", "idle":
		return strings.ToLower(strings.TrimSpace(v))
	default:
		return ""
	}
}

func shouldPaginateOpsWarmPoolReadyList(c *gin.Context, includeAccount, accountsOnly bool, accountState string) (page, pageSize int, ok bool) {
	if c == nil || !includeAccount || !accountsOnly || !strings.EqualFold(strings.TrimSpace(accountState), "ready") {
		return 0, 0, false
	}
	if strings.TrimSpace(c.Query("page")) == "" && strings.TrimSpace(c.Query("page_size")) == "" && strings.TrimSpace(c.Query("limit")) == "" {
		return 0, 0, false
	}
	page, pageSize = response.ParsePagination(c)
	return page, pageSize, true
}

func parseOpsRealtimeWindow(v string) (time.Duration, string, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "1min", "1m":
		return 1 * time.Minute, "1min", true
	case "5min", "5m":
		return 5 * time.Minute, "5min", true
	case "30min", "30m":
		return 30 * time.Minute, "30min", true
	case "1h", "60m", "60min":
		return 1 * time.Hour, "1h", true
	default:
		return 0, "", false
	}
}

// GetRealtimeTrafficSummary returns QPS/TPS current/peak/avg for the selected window.
// GET /api/v1/admin/ops/realtime-traffic
//
// Query params:
// - window: 1min|5min|30min|1h (default: 1min)
// - platform: optional
// - group_id: optional
func (h *OpsHandler) GetRealtimeTrafficSummary(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	windowDur, windowLabel, ok := parseOpsRealtimeWindow(c.Query("window"))
	if !ok {
		response.BadRequest(c, "Invalid window")
		return
	}

	platform := strings.TrimSpace(c.Query("platform"))
	var groupID *int64
	if v := strings.TrimSpace(c.Query("group_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid group_id")
			return
		}
		groupID = &id
	}

	endTime := time.Now().UTC()
	startTime := endTime.Add(-windowDur)

	if !h.opsService.IsRealtimeMonitoringEnabled(c.Request.Context()) {
		disabledSummary := &service.OpsRealtimeTrafficSummary{
			Window:             windowLabel,
			StartTime:          startTime,
			EndTime:            endTime,
			Platform:           platform,
			GroupID:            groupID,
			RecentRequestCount: 0,
			RecentErrorCount:   0,
			QPS:                service.OpsRateSummary{},
			TPS:                service.OpsRateSummary{},
		}
		response.Success(c, gin.H{
			"enabled":   false,
			"summary":   disabledSummary,
			"timestamp": endTime,
		})
		return
	}

	filter := &service.OpsDashboardFilter{
		StartTime: startTime,
		EndTime:   endTime,
		Platform:  platform,
		GroupID:   groupID,
		QueryMode: service.OpsQueryModeRaw,
	}

	summary, err := h.opsService.GetRealtimeTrafficSummary(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if summary != nil {
		summary.Window = windowLabel
	}
	response.Success(c, gin.H{
		"enabled":   true,
		"summary":   summary,
		"timestamp": endTime,
	})
}
