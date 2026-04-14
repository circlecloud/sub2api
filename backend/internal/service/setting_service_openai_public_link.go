package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var (
	ErrOpenAIPublicLinkNotFound = infraerrors.NotFound("OPENAI_PUBLIC_LINK_NOT_FOUND", "openai public link not found")
)

type OpenAIPublicAddLinkAccountDefaults struct {
	ProxyID            *int64         `json:"proxy_id,omitempty"`
	Concurrency        *int           `json:"concurrency,omitempty"`
	LoadFactor         *int           `json:"load_factor,omitempty"`
	Priority           *int           `json:"priority,omitempty"`
	RateMultiplier     *float64       `json:"rate_multiplier,omitempty"`
	ExpiresAt          *int64         `json:"expires_at,omitempty"`
	AutoPauseOnExpired *bool          `json:"auto_pause_on_expired,omitempty"`
	Credentials        map[string]any `json:"credentials,omitempty"`
	Extra              map[string]any `json:"extra,omitempty"`
}

type OpenAIPublicAddLink struct {
	Token           string                              `json:"token"`
	Name            string                              `json:"name"`
	GroupIDs        []int64                             `json:"group_ids"`
	AccountDefaults *OpenAIPublicAddLinkAccountDefaults `json:"account_defaults,omitempty"`
	CreatedAt       time.Time                           `json:"created_at"`
	UpdatedAt       time.Time                           `json:"updated_at"`
}

func normalizeOpenAIPublicAddLinkName(name string) string {
	return strings.TrimSpace(name)
}

func normalizeOpenAIPublicAddLinkGroupIDs(groupIDs []int64) []int64 {
	if len(groupIDs) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(groupIDs))
	result := make([]int64, 0, len(groupIDs))
	for _, id := range groupIDs {
		if id <= 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	slices.Sort(result)
	return result
}

func cloneOpenAIPublicAddLinkJSONMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}

func normalizeOpenAIPublicAddLinkAccountDefaults(defaults *OpenAIPublicAddLinkAccountDefaults) *OpenAIPublicAddLinkAccountDefaults {
	if defaults == nil {
		return nil
	}

	normalized := &OpenAIPublicAddLinkAccountDefaults{}
	if defaults.ProxyID != nil && *defaults.ProxyID > 0 {
		proxyID := *defaults.ProxyID
		normalized.ProxyID = &proxyID
	}
	if defaults.Concurrency != nil && *defaults.Concurrency > 0 {
		concurrency := *defaults.Concurrency
		normalized.Concurrency = &concurrency
	}
	if defaults.LoadFactor != nil && *defaults.LoadFactor > 0 {
		loadFactor := *defaults.LoadFactor
		normalized.LoadFactor = &loadFactor
	}
	if defaults.Priority != nil && *defaults.Priority > 0 {
		priority := *defaults.Priority
		normalized.Priority = &priority
	}
	if defaults.RateMultiplier != nil && *defaults.RateMultiplier >= 0 {
		rateMultiplier := *defaults.RateMultiplier
		normalized.RateMultiplier = &rateMultiplier
	}
	if defaults.ExpiresAt != nil && *defaults.ExpiresAt > 0 {
		expiresAt := *defaults.ExpiresAt
		normalized.ExpiresAt = &expiresAt
	}
	if defaults.AutoPauseOnExpired != nil {
		autoPauseOnExpired := *defaults.AutoPauseOnExpired
		normalized.AutoPauseOnExpired = &autoPauseOnExpired
	}
	if credentials := cloneOpenAIPublicAddLinkJSONMap(defaults.Credentials); len(credentials) > 0 {
		normalized.Credentials = credentials
	}
	if extra := cloneOpenAIPublicAddLinkJSONMap(defaults.Extra); len(extra) > 0 {
		normalized.Extra = extra
	}

	if normalized.ProxyID == nil && normalized.Concurrency == nil && normalized.LoadFactor == nil && normalized.Priority == nil && normalized.RateMultiplier == nil && normalized.ExpiresAt == nil && normalized.AutoPauseOnExpired == nil && len(normalized.Credentials) == 0 && len(normalized.Extra) == 0 {
		return nil
	}

	return normalized
}

func generateOpenAIPublicAddLinkToken() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate openai public add link token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func (s *SettingService) loadOpenAIPublicAddLinks(ctx context.Context) ([]OpenAIPublicAddLink, error) {
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyOpenAIPublicAddLinks)
	if err != nil {
		if err == ErrSettingNotFound {
			return []OpenAIPublicAddLink{}, nil
		}
		return nil, err
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return []OpenAIPublicAddLink{}, nil
	}

	var links []OpenAIPublicAddLink
	if err := json.Unmarshal([]byte(trimmed), &links); err != nil {
		return nil, infraerrors.Newf(500, "OPENAI_PUBLIC_LINKS_INVALID", "invalid openai public links config: %v", err)
	}

	for i := range links {
		links[i].Name = normalizeOpenAIPublicAddLinkName(links[i].Name)
		links[i].GroupIDs = normalizeOpenAIPublicAddLinkGroupIDs(links[i].GroupIDs)
		links[i].AccountDefaults = normalizeOpenAIPublicAddLinkAccountDefaults(links[i].AccountDefaults)
	}

	slices.SortFunc(links, func(a, b OpenAIPublicAddLink) int {
		if a.CreatedAt.Equal(b.CreatedAt) {
			return strings.Compare(a.Token, b.Token)
		}
		if a.CreatedAt.After(b.CreatedAt) {
			return -1
		}
		return 1
	})

	return links, nil
}

func (s *SettingService) saveOpenAIPublicAddLinks(ctx context.Context, links []OpenAIPublicAddLink) error {
	encoded, err := json.Marshal(links)
	if err != nil {
		return fmt.Errorf("marshal openai public add links: %w", err)
	}
	if err := s.settingRepo.Set(ctx, SettingKeyOpenAIPublicAddLinks, string(encoded)); err != nil {
		return err
	}
	if s.onUpdate != nil {
		s.onUpdate()
	}
	return nil
}

func (s *SettingService) ListOpenAIPublicAddLinks(ctx context.Context) ([]OpenAIPublicAddLink, error) {
	return s.loadOpenAIPublicAddLinks(ctx)
}

func (s *SettingService) GetOpenAIPublicAddLink(ctx context.Context, token string) (*OpenAIPublicAddLink, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrOpenAIPublicLinkNotFound
	}
	links, err := s.loadOpenAIPublicAddLinks(ctx)
	if err != nil {
		return nil, err
	}
	for i := range links {
		if links[i].Token == token {
			link := links[i]
			return &link, nil
		}
	}
	return nil, ErrOpenAIPublicLinkNotFound
}

func (s *SettingService) CreateOpenAIPublicAddLink(ctx context.Context, name string, groupIDs []int64, accountDefaults *OpenAIPublicAddLinkAccountDefaults) (*OpenAIPublicAddLink, error) {
	links, err := s.loadOpenAIPublicAddLinks(ctx)
	if err != nil {
		return nil, err
	}

	token, err := generateOpenAIPublicAddLinkToken()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	link := OpenAIPublicAddLink{
		Token:           token,
		Name:            normalizeOpenAIPublicAddLinkName(name),
		GroupIDs:        normalizeOpenAIPublicAddLinkGroupIDs(groupIDs),
		AccountDefaults: normalizeOpenAIPublicAddLinkAccountDefaults(accountDefaults),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	links = append(links, link)
	if err := s.saveOpenAIPublicAddLinks(ctx, links); err != nil {
		return nil, err
	}
	return &link, nil
}

func (s *SettingService) UpdateOpenAIPublicAddLink(ctx context.Context, token string, name string, groupIDs []int64, accountDefaults *OpenAIPublicAddLinkAccountDefaults) (*OpenAIPublicAddLink, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrOpenAIPublicLinkNotFound
	}
	links, err := s.loadOpenAIPublicAddLinks(ctx)
	if err != nil {
		return nil, err
	}
	for i := range links {
		if links[i].Token != token {
			continue
		}
		links[i].Name = normalizeOpenAIPublicAddLinkName(name)
		links[i].GroupIDs = normalizeOpenAIPublicAddLinkGroupIDs(groupIDs)
		links[i].AccountDefaults = normalizeOpenAIPublicAddLinkAccountDefaults(accountDefaults)
		links[i].UpdatedAt = time.Now().UTC()
		if err := s.saveOpenAIPublicAddLinks(ctx, links); err != nil {
			return nil, err
		}
		updated := links[i]
		return &updated, nil
	}
	return nil, ErrOpenAIPublicLinkNotFound
}

func (s *SettingService) RotateOpenAIPublicAddLink(ctx context.Context, token string) (*OpenAIPublicAddLink, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrOpenAIPublicLinkNotFound
	}
	links, err := s.loadOpenAIPublicAddLinks(ctx)
	if err != nil {
		return nil, err
	}
	for i := range links {
		if links[i].Token != token {
			continue
		}
		newToken, genErr := generateOpenAIPublicAddLinkToken()
		if genErr != nil {
			return nil, genErr
		}
		links[i].Token = newToken
		links[i].UpdatedAt = time.Now().UTC()
		if err := s.saveOpenAIPublicAddLinks(ctx, links); err != nil {
			return nil, err
		}
		updated := links[i]
		return &updated, nil
	}
	return nil, ErrOpenAIPublicLinkNotFound
}

func (s *SettingService) DeleteOpenAIPublicAddLink(ctx context.Context, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return ErrOpenAIPublicLinkNotFound
	}
	links, err := s.loadOpenAIPublicAddLinks(ctx)
	if err != nil {
		return err
	}
	filtered := make([]OpenAIPublicAddLink, 0, len(links))
	deleted := false
	for _, link := range links {
		if link.Token == token {
			deleted = true
			continue
		}
		filtered = append(filtered, link)
	}
	if !deleted {
		return ErrOpenAIPublicLinkNotFound
	}
	return s.saveOpenAIPublicAddLinks(ctx, filtered)
}

func (s *SettingService) BuildOpenAIPublicAddLinkURL(ctx context.Context, token string) string {
	trimmedToken := strings.TrimSpace(token)
	if trimmedToken == "" {
		return ""
	}
	frontendURL := strings.TrimRight(strings.TrimSpace(s.GetFrontendURL(ctx)), "/")
	if frontendURL == "" {
		return "/openai/connect/" + trimmedToken
	}
	return frontendURL + "/openai/connect/" + trimmedToken
}
