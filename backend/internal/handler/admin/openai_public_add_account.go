package admin

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

func (h *OpenAIOAuthHandler) CreateAccountFromPublicAddLink(c *gin.Context) {
	link, err := h.requireOpenAIPublicAddLink(c)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	var req struct {
		SessionID   string `json:"session_id" binding:"required"`
		Code        string `json:"code" binding:"required"`
		State       string `json:"state" binding:"required"`
		RedirectURI string `json:"redirect_uri"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	defaults, err := h.resolveOpenAIPublicLinkAccountDefaults(c.Request.Context(), link)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	account, err := h.createOAuthAccount(c.Request.Context(), service.PlatformOpenAI, &service.OpenAIExchangeCodeInput{
		SessionID:   req.SessionID,
		Code:        req.Code,
		State:       req.State,
		RedirectURI: req.RedirectURI,
		ProxyID:     defaults.ProxyID,
	}, openAIAccountCreateOptions{
		GroupIDs:             defaults.GroupIDs,
		ProxyID:              defaults.ProxyID,
		Concurrency:          defaults.Concurrency,
		LoadFactor:           defaults.LoadFactor,
		Priority:             defaults.Priority,
		RateMultiplier:       defaults.RateMultiplier,
		ExpiresAt:            defaults.ExpiresAt,
		AutoPauseOnExpired:   defaults.AutoPauseOnExpired,
		CredentialDefaults:   defaults.CredentialDefaults,
		Extra:                defaults.Extra,
		SkipDefaultGroupBind: true,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.AccountFromService(account))
}

func (h *OpenAIOAuthHandler) CreateAccountFromPublicRefreshToken(c *gin.Context) {
	link, err := h.requireOpenAIPublicAddLink(c)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	var req struct {
		RefreshToken string `json:"refresh_token"`
		RT           string `json:"rt"`
		ClientID     string `json:"client_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	refreshToken := strings.TrimSpace(req.RefreshToken)
	if refreshToken == "" {
		refreshToken = strings.TrimSpace(req.RT)
	}
	if refreshToken == "" {
		response.BadRequest(c, "refresh_token is required")
		return
	}
	defaults, err := h.resolveOpenAIPublicLinkAccountDefaults(c.Request.Context(), link)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	clientID := strings.TrimSpace(req.ClientID)
	if clientID == "" {
		clientID = openai.ClientID
	}
	tokenInfo, err := h.openaiOAuthService.RefreshTokenWithClientID(c.Request.Context(), refreshToken, defaults.ProxyURL, clientID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	credentials := h.buildCredentialsFromTokenInfoWithFallbacks(tokenInfo, refreshToken, clientID)
	account, err := h.createAccountWithCredentials(c.Request.Context(), service.PlatformOpenAI, "", credentials, nil, openAIAccountCreateOptions{
		GroupIDs:             defaults.GroupIDs,
		ProxyID:              defaults.ProxyID,
		Concurrency:          defaults.Concurrency,
		LoadFactor:           defaults.LoadFactor,
		Priority:             defaults.Priority,
		RateMultiplier:       defaults.RateMultiplier,
		ExpiresAt:            defaults.ExpiresAt,
		AutoPauseOnExpired:   defaults.AutoPauseOnExpired,
		CredentialDefaults:   defaults.CredentialDefaults,
		Extra:                defaults.Extra,
		SkipDefaultGroupBind: true,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.AccountFromService(account))
}

func (h *OpenAIOAuthHandler) CreateAccountFromPublicCredentials(c *gin.Context) {
	link, err := h.requireOpenAIPublicAddLink(c)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	var req struct {
		Credentials map[string]any `json:"credentials" binding:"required"`
		Extra       map[string]any `json:"extra"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	defaults, err := h.resolveOpenAIPublicLinkAccountDefaults(c.Request.Context(), link)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	credentials := normalizePublicOpenAICredentials(req.Credentials)
	if extractCredentialString(credentials, "access_token") == "" {
		response.ErrorFrom(c, infraerrors.BadRequest("OPENAI_PUBLIC_LINK_ACCESS_TOKEN_REQUIRED", "access_token is required for direct token import"))
		return
	}
	account, err := h.createAccountWithCredentials(c.Request.Context(), service.PlatformOpenAI, "", credentials, req.Extra, openAIAccountCreateOptions{
		GroupIDs:             defaults.GroupIDs,
		ProxyID:              defaults.ProxyID,
		Concurrency:          defaults.Concurrency,
		LoadFactor:           defaults.LoadFactor,
		Priority:             defaults.Priority,
		RateMultiplier:       defaults.RateMultiplier,
		ExpiresAt:            defaults.ExpiresAt,
		AutoPauseOnExpired:   defaults.AutoPauseOnExpired,
		CredentialDefaults:   defaults.CredentialDefaults,
		Extra:                defaults.Extra,
		SkipDefaultGroupBind: true,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.AccountFromService(account))
}

type openAIAccountCreateOptions struct {
	PreferredName        string
	GroupIDs             []int64
	ProxyID              *int64
	Concurrency          int
	LoadFactor           *int
	Priority             int
	RateMultiplier       *float64
	ExpiresAt            *int64
	AutoPauseOnExpired   *bool
	CredentialDefaults   map[string]any
	Extra                map[string]any
	SkipDefaultGroupBind bool
}

func (h *OpenAIOAuthHandler) createOAuthAccount(
	ctx context.Context,
	platform string,
	exchangeInput *service.OpenAIExchangeCodeInput,
	options openAIAccountCreateOptions,
) (*service.Account, error) {
	tokenInfo, err := h.openaiOAuthService.ExchangeCode(ctx, exchangeInput)
	if err != nil {
		return nil, err
	}
	return h.createOAuthAccountFromTokenInfo(ctx, platform, tokenInfo, options)
}

func (h *OpenAIOAuthHandler) createOAuthAccountFromTokenInfo(
	ctx context.Context,
	platform string,
	tokenInfo *service.OpenAITokenInfo,
	options openAIAccountCreateOptions,
) (*service.Account, error) {
	credentials := h.buildCredentialsFromTokenInfoWithFallbacks(tokenInfo, "", "")
	return h.createAccountWithCredentials(ctx, platform, options.PreferredName, credentials, nil, options)
}

func (h *OpenAIOAuthHandler) buildCredentialsFromTokenInfoWithFallbacks(tokenInfo *service.OpenAITokenInfo, fallbackRefreshToken, fallbackClientID string) map[string]any {
	credentials := h.openaiOAuthService.BuildAccountCredentials(tokenInfo)
	fallbackRefreshToken = strings.TrimSpace(fallbackRefreshToken)
	if fallbackRefreshToken != "" {
		if _, exists := credentials["refresh_token"]; !exists {
			credentials["refresh_token"] = fallbackRefreshToken
		}
	}
	fallbackClientID = strings.TrimSpace(fallbackClientID)
	if fallbackClientID != "" {
		if _, exists := credentials["client_id"]; !exists {
			credentials["client_id"] = fallbackClientID
		}
	}
	return credentials
}

func (h *OpenAIOAuthHandler) createAccountWithCredentials(
	ctx context.Context,
	platform string,
	preferredName string,
	credentials map[string]any,
	extra map[string]any,
	options openAIAccountCreateOptions,
) (*service.Account, error) {
	if options.Concurrency <= 0 {
		options.Concurrency = 10
	}
	if options.Priority <= 0 {
		options.Priority = 1
	}
	finalCredentials := mergeOpenAIPublicLinkJSONMap(credentials, options.CredentialDefaults)
	finalExtra := mergeOpenAIPublicLinkJSONMap(extra, options.Extra)
	name := buildDefaultOpenAIOAuthAccountName(platform, preferredName, extractCredentialString(finalCredentials, "email"))
	return h.adminService.CreateAccount(ctx, &service.CreateAccountInput{
		Name:                 name,
		Platform:             platform,
		Type:                 "oauth",
		Credentials:          finalCredentials,
		Extra:                finalExtra,
		ProxyID:              options.ProxyID,
		Concurrency:          options.Concurrency,
		LoadFactor:           options.LoadFactor,
		Priority:             options.Priority,
		RateMultiplier:       options.RateMultiplier,
		GroupIDs:             options.GroupIDs,
		ExpiresAt:            options.ExpiresAt,
		AutoPauseOnExpired:   options.AutoPauseOnExpired,
		SkipDefaultGroupBind: options.SkipDefaultGroupBind,
	})
}

func buildDefaultOpenAIOAuthAccountName(platform, preferredName, email string) string {
	name := strings.TrimSpace(preferredName)
	if name != "" {
		return name
	}
	if strings.TrimSpace(email) != "" {
		return strings.TrimSpace(email)
	}
	return "OpenAI OAuth Account"
}

func normalizePublicOpenAICredentials(raw map[string]any) map[string]any {
	credentials := make(map[string]any, len(raw))
	for key, value := range raw {
		credentials[key] = value
	}
	if accessToken := extractCredentialString(credentials, "access_token"); accessToken != "" {
		credentials["access_token"] = accessToken
	}
	if refreshToken := extractCredentialString(credentials, "refresh_token"); refreshToken != "" {
		credentials["refresh_token"] = refreshToken
	}
	if idToken := extractCredentialString(credentials, "id_token"); idToken != "" {
		credentials["id_token"] = idToken
	}
	if email := extractCredentialString(credentials, "email"); email != "" {
		credentials["email"] = email
	}
	if accountID := extractCredentialString(credentials, "chatgpt_account_id", "account_id"); accountID != "" {
		credentials["chatgpt_account_id"] = accountID
	}
	if userID := extractCredentialString(credentials, "chatgpt_user_id", "user_id"); userID != "" {
		credentials["chatgpt_user_id"] = userID
	}
	if organizationID := extractCredentialString(credentials, "organization_id", "poid"); organizationID != "" {
		credentials["organization_id"] = organizationID
	}
	if planType := extractCredentialString(credentials, "plan_type", "chatgpt_plan_type"); planType != "" {
		credentials["plan_type"] = planType
	}
	if expiresAt := extractExpiryCredential(credentials, "expires_at", "expired", "expiresAt"); expiresAt != "" {
		credentials["expires_at"] = expiresAt
	}
	if clientID := extractCredentialString(credentials, "client_id", "clientId"); clientID != "" {
		credentials["client_id"] = clientID
	} else {
		credentials["client_id"] = inferPublicOpenAIClientID(extractCredentialString(credentials, "type"))
	}
	delete(credentials, "account_id")
	delete(credentials, "expired")
	delete(credentials, "expiresAt")
	delete(credentials, "clientId")
	return credentials
}

func extractCredentialString(credentials map[string]any, keys ...string) string {
	for _, key := range keys {
		value, exists := credentials[key]
		if !exists || value == nil {
			continue
		}
		switch typed := value.(type) {
		case string:
			if trimmed := strings.TrimSpace(typed); trimmed != "" {
				return trimmed
			}
		case float64:
			if typed > 0 {
				return strconv.FormatInt(int64(typed), 10)
			}
		case int64:
			if typed > 0 {
				return strconv.FormatInt(typed, 10)
			}
		case int:
			if typed > 0 {
				return strconv.Itoa(typed)
			}
		}
	}
	return ""
}

func extractExpiryCredential(credentials map[string]any, keys ...string) string {
	for _, key := range keys {
		value, exists := credentials[key]
		if !exists || value == nil {
			continue
		}
		switch typed := value.(type) {
		case string:
			if trimmed := strings.TrimSpace(typed); trimmed != "" {
				return trimmed
			}
		case float64:
			if typed > 0 {
				return time.Unix(int64(typed), 0).Format(time.RFC3339)
			}
		case int64:
			if typed > 0 {
				return time.Unix(typed, 0).Format(time.RFC3339)
			}
		case int:
			if typed > 0 {
				return time.Unix(int64(typed), 0).Format(time.RFC3339)
			}
		}
	}
	return ""
}

func inferPublicOpenAIClientID(typeValue string) string {
	normalized := strings.ToLower(strings.TrimSpace(typeValue))
	if strings.Contains(normalized, "mobile") || strings.Contains(normalized, "ios") || strings.Contains(normalized, "android") {
		return openai.ClientID
	}
	return openai.ClientID
}

type openAIPublicLinkResolvedAccountDefaults struct {
	GroupIDs           []int64
	ProxyID            *int64
	ProxyURL           string
	Concurrency        int
	LoadFactor         *int
	Priority           int
	RateMultiplier     *float64
	ExpiresAt          *int64
	AutoPauseOnExpired *bool
	CredentialDefaults map[string]any
	Extra              map[string]any
}

func cloneOpenAIPublicLinkJSONMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}

func sanitizeOpenAIPublicLinkCredentialDefaults(raw map[string]any) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	allowed := map[string]struct{}{
		"model_mapping": {},
	}
	result := make(map[string]any)
	for key, value := range raw {
		if _, ok := allowed[key]; !ok {
			continue
		}
		result[key] = value
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func sanitizeOpenAIPublicLinkExtraDefaults(raw map[string]any) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	allowed := map[string]struct{}{
		"openai_passthrough":                           {},
		"openai_oauth_responses_websockets_v2_mode":    {},
		"openai_oauth_responses_websockets_v2_enabled": {},
		"codex_cli_only":                               {},
	}
	result := make(map[string]any)
	for key, value := range raw {
		if _, ok := allowed[key]; !ok {
			continue
		}
		result[key] = value
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func mergeOpenAIPublicLinkJSONMap(base map[string]any, defaults map[string]any) map[string]any {
	if len(base) == 0 && len(defaults) == 0 {
		return nil
	}
	merged := make(map[string]any, len(base)+len(defaults))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range defaults {
		merged[key] = value
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
}

func mapOpenAIPublicLinkProxyLookupError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, service.ErrProxyNotFound) {
		return infraerrors.BadRequest("OPENAI_PUBLIC_LINK_PROXY_NOT_FOUND", "proxy not found")
	}
	return err
}

func (h *OpenAIOAuthHandler) requireOpenAIPublicLinkProxy(ctx context.Context, proxyID int64) (*service.Proxy, error) {
	proxy, err := h.adminService.GetProxy(ctx, proxyID)
	if err != nil {
		return nil, mapOpenAIPublicLinkProxyLookupError(err)
	}
	if proxy == nil {
		return nil, infraerrors.BadRequest("OPENAI_PUBLIC_LINK_PROXY_NOT_FOUND", "proxy not found")
	}
	return proxy, nil
}

func (h *OpenAIOAuthHandler) validateOpenAIPublicLinkAccountDefaults(ctx context.Context, defaults *service.OpenAIPublicAddLinkAccountDefaults) (*service.OpenAIPublicAddLinkAccountDefaults, error) {
	if defaults == nil {
		return nil, nil
	}

	normalized := &service.OpenAIPublicAddLinkAccountDefaults{}
	if defaults.ProxyID != nil {
		if *defaults.ProxyID <= 0 {
			return nil, infraerrors.BadRequest("OPENAI_PUBLIC_LINK_PROXY_INVALID", "proxy_id must be a positive integer")
		}
		if _, err := h.requireOpenAIPublicLinkProxy(ctx, *defaults.ProxyID); err != nil {
			return nil, err
		}
		proxyID := *defaults.ProxyID
		normalized.ProxyID = &proxyID
	}
	if defaults.Concurrency != nil {
		if *defaults.Concurrency < 1 {
			return nil, infraerrors.BadRequest("OPENAI_PUBLIC_LINK_CONCURRENCY_INVALID", "concurrency must be at least 1")
		}
		concurrency := *defaults.Concurrency
		normalized.Concurrency = &concurrency
	}
	if defaults.LoadFactor != nil {
		if *defaults.LoadFactor < 1 {
			return nil, infraerrors.BadRequest("OPENAI_PUBLIC_LINK_LOAD_FACTOR_INVALID", "load_factor must be at least 1")
		}
		loadFactor := *defaults.LoadFactor
		normalized.LoadFactor = &loadFactor
	}
	if defaults.Priority != nil {
		if *defaults.Priority < 1 {
			return nil, infraerrors.BadRequest("OPENAI_PUBLIC_LINK_PRIORITY_INVALID", "priority must be at least 1")
		}
		priority := *defaults.Priority
		normalized.Priority = &priority
	}
	if defaults.RateMultiplier != nil {
		if *defaults.RateMultiplier < 0 {
			return nil, infraerrors.BadRequest("OPENAI_PUBLIC_LINK_RATE_MULTIPLIER_INVALID", "rate_multiplier must be greater than or equal to 0")
		}
		rateMultiplier := *defaults.RateMultiplier
		normalized.RateMultiplier = &rateMultiplier
	}
	if defaults.ExpiresAt != nil {
		if *defaults.ExpiresAt <= 0 {
			return nil, infraerrors.BadRequest("OPENAI_PUBLIC_LINK_EXPIRES_AT_INVALID", "expires_at must be a positive unix timestamp")
		}
		expiresAt := *defaults.ExpiresAt
		normalized.ExpiresAt = &expiresAt
	}
	if defaults.AutoPauseOnExpired != nil {
		autoPauseOnExpired := *defaults.AutoPauseOnExpired
		normalized.AutoPauseOnExpired = &autoPauseOnExpired
	}
	if credentials := sanitizeOpenAIPublicLinkCredentialDefaults(cloneOpenAIPublicLinkJSONMap(defaults.Credentials)); len(credentials) > 0 {
		normalized.Credentials = credentials
	}
	if extra := sanitizeOpenAIPublicLinkExtraDefaults(cloneOpenAIPublicLinkJSONMap(defaults.Extra)); len(extra) > 0 {
		normalized.Extra = extra
	}
	if normalized.Extra != nil {
		if passthrough, ok := normalized.Extra["openai_passthrough"].(bool); ok && passthrough {
			delete(normalized.Credentials, "model_mapping")
			if len(normalized.Credentials) == 0 {
				normalized.Credentials = nil
			}
		}
	}
	if normalized.ProxyID == nil && normalized.Concurrency == nil && normalized.LoadFactor == nil && normalized.Priority == nil && normalized.RateMultiplier == nil && normalized.ExpiresAt == nil && normalized.AutoPauseOnExpired == nil && len(normalized.Credentials) == 0 && len(normalized.Extra) == 0 {
		return nil, nil
	}
	return normalized, nil
}

func (h *OpenAIOAuthHandler) resolveOpenAIPublicLinkAccountDefaults(ctx context.Context, link *service.OpenAIPublicAddLink) (*openAIPublicLinkResolvedAccountDefaults, error) {
	groupIDs, err := h.resolveOpenAIPublicLinkGroupIDs(ctx, link)
	if err != nil {
		return nil, err
	}
	validatedDefaults, err := h.validateOpenAIPublicLinkAccountDefaults(ctx, link.AccountDefaults)
	if err != nil {
		return nil, err
	}
	resolved := &openAIPublicLinkResolvedAccountDefaults{
		GroupIDs:    groupIDs,
		Concurrency: 10,
		Priority:    1,
	}
	if validatedDefaults == nil {
		return resolved, nil
	}
	resolved.ProxyID = validatedDefaults.ProxyID
	if validatedDefaults.ProxyID != nil {
		proxy, err := h.requireOpenAIPublicLinkProxy(ctx, *validatedDefaults.ProxyID)
		if err != nil {
			return nil, err
		}
		resolved.ProxyURL = proxy.URL()
	}
	if validatedDefaults.Concurrency != nil {
		resolved.Concurrency = *validatedDefaults.Concurrency
	}
	if validatedDefaults.LoadFactor != nil {
		loadFactor := *validatedDefaults.LoadFactor
		resolved.LoadFactor = &loadFactor
	}
	if validatedDefaults.Priority != nil {
		resolved.Priority = *validatedDefaults.Priority
	}
	if validatedDefaults.RateMultiplier != nil {
		rateMultiplier := *validatedDefaults.RateMultiplier
		resolved.RateMultiplier = &rateMultiplier
	}
	if validatedDefaults.ExpiresAt != nil {
		expiresAt := *validatedDefaults.ExpiresAt
		resolved.ExpiresAt = &expiresAt
	}
	if validatedDefaults.AutoPauseOnExpired != nil {
		autoPauseOnExpired := *validatedDefaults.AutoPauseOnExpired
		resolved.AutoPauseOnExpired = &autoPauseOnExpired
	}
	resolved.CredentialDefaults = sanitizeOpenAIPublicLinkCredentialDefaults(cloneOpenAIPublicLinkJSONMap(validatedDefaults.Credentials))
	resolved.Extra = sanitizeOpenAIPublicLinkExtraDefaults(cloneOpenAIPublicLinkJSONMap(validatedDefaults.Extra))
	return resolved, nil
}

func (h *OpenAIOAuthHandler) requireOpenAIPublicAddLink(c *gin.Context) (*service.OpenAIPublicAddLink, error) {
	token := strings.TrimSpace(c.Param("token"))
	if token == "" {
		return nil, service.ErrOpenAIPublicLinkNotFound
	}
	return h.settingService.GetOpenAIPublicAddLink(c.Request.Context(), token)
}

func (h *OpenAIOAuthHandler) validateOpenAIPublicLinkGroupIDs(ctx context.Context, groupIDs []int64) ([]int64, error) {
	groups, err := h.adminService.GetAllGroupsByPlatform(ctx, service.PlatformOpenAI)
	if err != nil {
		return nil, err
	}
	available := make(map[int64]struct{}, len(groups))
	for _, group := range groups {
		available[group.ID] = struct{}{}
	}
	seen := make(map[int64]struct{}, len(groupIDs))
	normalized := make([]int64, 0, len(groupIDs))
	for _, groupID := range groupIDs {
		if groupID <= 0 {
			continue
		}
		if _, exists := seen[groupID]; exists {
			continue
		}
		seen[groupID] = struct{}{}
		if _, ok := available[groupID]; !ok {
			return nil, infraerrors.BadRequest("INVALID_OPENAI_PUBLIC_LINK_GROUP", "group must be an active openai group")
		}
		normalized = append(normalized, groupID)
	}
	if len(normalized) == 0 {
		return nil, infraerrors.BadRequest("OPENAI_PUBLIC_LINK_GROUP_REQUIRED", "at least one active openai group is required")
	}
	return normalized, nil
}

func (h *OpenAIOAuthHandler) resolveOpenAIPublicLinkGroupIDs(ctx context.Context, link *service.OpenAIPublicAddLink) ([]int64, error) {
	return h.validateOpenAIPublicLinkGroupIDs(ctx, link.GroupIDs)
}

func (h *OpenAIOAuthHandler) listAllowedOpenAIGroupDTOs(ctx context.Context, allowedGroupIDs []int64) ([]*dto.Group, error) {
	groups, err := h.adminService.GetAllGroupsByPlatform(ctx, service.PlatformOpenAI)
	if err != nil {
		return nil, err
	}
	groupMap := make(map[int64]*service.Group, len(groups))
	for i := range groups {
		group := groups[i]
		groupMap[group.ID] = &group
	}
	result := make([]*dto.Group, 0, len(allowedGroupIDs))
	for _, groupID := range allowedGroupIDs {
		group, ok := groupMap[groupID]
		if !ok {
			continue
		}
		result = append(result, dto.GroupFromService(group))
	}
	return result, nil
}
