package admin

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/geminicli"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type batchAvailableModelsRequest struct {
	AccountIDs []int64 `json:"account_ids"`
}

type batchAvailableModelsResponse struct {
	Count                  int                                `json:"count"`
	SignatureCount         int                                `json:"signature_count"`
	Models                 []availableModelItem               `json:"models"`
	RepresentativeAccounts []availableModelRepresentativeItem `json:"representative_accounts"`
}

type availableModelItem struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	DisplayName string `json:"display_name"`
	CreatedAt   string `json:"created_at"`
}

type availableModelRepresentativeItem struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
	Type     string `json:"type"`
}

type accountModelProjectionReader interface {
	GetAccountsModelProjectionByIDs(ctx context.Context, ids []int64) ([]*service.Account, error)
}

func (h *AccountHandler) GetBatchAvailableModels(c *gin.Context) {
	var req batchAvailableModelsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	normalizedIDs := normalizePositiveAccountIDs(req.AccountIDs)
	if len(normalizedIDs) == 0 {
		response.BadRequest(c, "account_ids is required")
		return
	}

	accounts, err := h.getAccountsForAvailableModels(c.Request.Context(), normalizedIDs)
	if err != nil {
		response.InternalError(c, "Failed to load accounts: "+err.Error())
		return
	}
	if len(accounts) == 0 {
		response.BadRequest(c, "No valid accounts found")
		return
	}

	buckets := make([][]availableModelItem, 0)
	representatives := make([]availableModelRepresentativeItem, 0)
	seenSignatures := make(map[string]struct{}, len(accounts))
	for _, account := range accounts {
		if account == nil || account.ID <= 0 {
			continue
		}
		signature := buildAvailableModelsSignature(account)
		if _, ok := seenSignatures[signature]; ok {
			continue
		}
		seenSignatures[signature] = struct{}{}
		buckets = append(buckets, h.buildAvailableModelsForAccount(account))
		representatives = append(representatives, availableModelRepresentativeItem{
			ID:       account.ID,
			Name:     strings.TrimSpace(account.Name),
			Platform: strings.TrimSpace(account.Platform),
			Type:     strings.TrimSpace(account.Type),
		})
	}

	response.Success(c, batchAvailableModelsResponse{
		Count:                  len(accounts),
		SignatureCount:         len(buckets),
		Models:                 intersectAvailableModels(buckets),
		RepresentativeAccounts: representatives,
	})
}

func (h *AccountHandler) getAccountsForAvailableModels(ctx context.Context, ids []int64) ([]*service.Account, error) {
	if reader, ok := h.adminService.(accountModelProjectionReader); ok {
		return reader.GetAccountsModelProjectionByIDs(ctx, ids)
	}
	return h.adminService.GetAccountsByIDs(ctx, ids)
}

func normalizePositiveAccountIDs(ids []int64) []int64 {
	if len(ids) == 0 {
		return []int64{}
	}
	out := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func (h *AccountHandler) buildAvailableModelsForAccount(account *service.Account) []availableModelItem {
	if account == nil {
		return []availableModelItem{}
	}

	if account.IsOpenAI() {
		defaultModels := openai.DefaultModels
		if account.IsOAuth() {
			defaultModels = openai.DefaultOAuthModels
		}
		if account.IsOpenAIPassthroughEnabled() {
			return availableModelsFromOpenAI(defaultModels)
		}
		mapping := account.GetModelMapping()
		if len(mapping) == 0 {
			return availableModelsFromOpenAI(defaultModels)
		}
		return availableModelsFromMapping(mapping, availableModelsFromOpenAI(defaultModels))
	}

	if account.IsGemini() {
		if account.IsOAuth() {
			return availableModelsFromGemini(geminicli.DefaultModels)
		}
		mapping := account.GetModelMapping()
		if len(mapping) == 0 {
			return availableModelsFromGemini(geminicli.DefaultModels)
		}
		return availableModelsFromMapping(mapping, availableModelsFromGemini(geminicli.DefaultModels))
	}

	if account.Platform == service.PlatformAntigravity {
		return availableModelsFromAntigravity(antigravity.DefaultModels())
	}

	if account.IsOAuth() {
		return availableModelsFromClaude(claude.DefaultModels)
	}

	mapping := account.GetModelMapping()
	if len(mapping) == 0 {
		return availableModelsFromClaude(claude.DefaultModels)
	}
	return availableModelsFromMapping(mapping, availableModelsFromClaude(claude.DefaultModels))
}

func buildAvailableModelsSignature(account *service.Account) string {
	if account == nil {
		return ""
	}
	if account.Platform == service.PlatformAntigravity {
		return service.PlatformAntigravity
	}

	parts := []string{"platform=" + account.Platform}
	if account.IsOAuth() {
		parts = append(parts, "oauth_family=true")
	} else {
		parts = append(parts, "oauth_family=false")
	}

	if account.IsOpenAI() {
		parts = append(parts, "openai_passthrough="+strconv.FormatBool(account.IsOpenAIPassthroughEnabled()))
	}

	mapping := account.GetModelMapping()
	switch {
	case account.IsOpenAI():
		if !account.IsOpenAIPassthroughEnabled() {
			parts = append(parts, "mapping="+stableModelMappingSignature(mapping))
		}
	case account.IsGemini():
		if !account.IsOAuth() {
			parts = append(parts, "mapping="+stableModelMappingSignature(mapping))
		}
	default:
		if !account.IsOAuth() {
			parts = append(parts, "mapping="+stableModelMappingSignature(mapping))
		}
	}

	return strings.Join(parts, "|")
}

func stableModelMappingSignature(mapping map[string]string) string {
	if len(mapping) == 0 {
		return "empty"
	}
	keys := make([]string, 0, len(mapping))
	for key := range mapping {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", key, mapping[key]))
	}
	return strings.Join(parts, ",")
}

func availableModelsFromOpenAI(models []openai.Model) []availableModelItem {
	items := make([]availableModelItem, 0, len(models))
	for _, model := range models {
		items = append(items, availableModelItem{
			ID:          strings.TrimSpace(model.ID),
			Type:        normalizedModelType(model.Type),
			DisplayName: preferredModelLabel(model.DisplayName, model.ID),
			CreatedAt:   "",
		})
	}
	return items
}

func availableModelsFromGemini(models []geminicli.Model) []availableModelItem {
	items := make([]availableModelItem, 0, len(models))
	for _, model := range models {
		items = append(items, availableModelItem{
			ID:          strings.TrimSpace(model.ID),
			Type:        normalizedModelType(model.Type),
			DisplayName: preferredModelLabel(model.DisplayName, model.ID),
			CreatedAt:   strings.TrimSpace(model.CreatedAt),
		})
	}
	return items
}

func availableModelsFromClaude(models []claude.Model) []availableModelItem {
	items := make([]availableModelItem, 0, len(models))
	for _, model := range models {
		items = append(items, availableModelItem{
			ID:          strings.TrimSpace(model.ID),
			Type:        normalizedModelType(model.Type),
			DisplayName: preferredModelLabel(model.DisplayName, model.ID),
			CreatedAt:   strings.TrimSpace(model.CreatedAt),
		})
	}
	return items
}

func availableModelsFromAntigravity(models []antigravity.ClaudeModel) []availableModelItem {
	items := make([]availableModelItem, 0, len(models))
	for _, model := range models {
		items = append(items, availableModelItem{
			ID:          strings.TrimSpace(model.ID),
			Type:        normalizedModelType(model.Type),
			DisplayName: preferredModelLabel(model.DisplayName, model.ID),
			CreatedAt:   strings.TrimSpace(model.CreatedAt),
		})
	}
	return items
}

func availableModelsFromMapping(mapping map[string]string, defaults []availableModelItem) []availableModelItem {
	if len(mapping) == 0 {
		return defaults
	}
	defaultsByID := make(map[string]availableModelItem, len(defaults))
	for _, item := range defaults {
		defaultsByID[item.ID] = item
	}

	keys := make([]string, 0, len(mapping))
	for key := range mapping {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	items := make([]availableModelItem, 0, len(keys))
	for _, key := range keys {
		id := strings.TrimSpace(key)
		if id == "" {
			continue
		}
		if item, ok := defaultsByID[id]; ok {
			items = append(items, item)
			continue
		}
		items = append(items, availableModelItem{
			ID:          id,
			Type:        "model",
			DisplayName: id,
			CreatedAt:   "",
		})
	}
	return items
}

func intersectAvailableModels(groups [][]availableModelItem) []availableModelItem {
	if len(groups) == 0 {
		return []availableModelItem{}
	}
	result := make([]availableModelItem, 0, len(groups[0]))
	for _, item := range groups[0] {
		if strings.TrimSpace(item.ID) == "" {
			continue
		}
		presentInAll := true
		for _, group := range groups[1:] {
			if !containsAvailableModel(group, item.ID) {
				presentInAll = false
				break
			}
		}
		if presentInAll {
			result = append(result, item)
		}
	}
	return result
}

func containsAvailableModel(group []availableModelItem, id string) bool {
	for _, item := range group {
		if item.ID == id {
			return true
		}
	}
	return false
}

func normalizedModelType(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "model"
	}
	return value
}

func preferredModelLabel(label, fallback string) string {
	label = strings.TrimSpace(label)
	if label != "" {
		return label
	}
	return strings.TrimSpace(fallback)
}
