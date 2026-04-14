package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type openAIPublicLinkHandlerSettingRepoStub struct {
	mu     sync.Mutex
	values map[string]string
}

func newOpenAIPublicLinkHandlerSettingRepoStub() *openAIPublicLinkHandlerSettingRepoStub {
	return &openAIPublicLinkHandlerSettingRepoStub{values: make(map[string]string)}
}

func (s *openAIPublicLinkHandlerSettingRepoStub) Get(ctx context.Context, key string) (*service.Setting, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[key]
	if !ok {
		return nil, service.ErrSettingNotFound
	}
	return &service.Setting{Key: key, Value: value}, nil
}

func (s *openAIPublicLinkHandlerSettingRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[key]
	if !ok {
		return "", service.ErrSettingNotFound
	}
	return value, nil
}

func (s *openAIPublicLinkHandlerSettingRepoStub) Set(ctx context.Context, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = value
	return nil
}

func (s *openAIPublicLinkHandlerSettingRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			result[key] = value
		}
	}
	return result, nil
}

func (s *openAIPublicLinkHandlerSettingRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, value := range settings {
		s.values[key] = value
	}
	return nil
}

func (s *openAIPublicLinkHandlerSettingRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make(map[string]string, len(s.values))
	for key, value := range s.values {
		result[key] = value
	}
	return result, nil
}

func (s *openAIPublicLinkHandlerSettingRepoStub) Delete(ctx context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.values, key)
	return nil
}

type openAIPublicLinkAPIResponse[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Reason  string `json:"reason,omitempty"`
	Data    T      `json:"data,omitempty"`
}

type openAIPublicLinkProxyAdminService struct {
	*stubAdminService
	getProxy func(ctx context.Context, id int64) (*service.Proxy, error)
}

func (s *openAIPublicLinkProxyAdminService) GetProxy(ctx context.Context, id int64) (*service.Proxy, error) {
	if s.getProxy != nil {
		return s.getProxy(ctx, id)
	}
	return s.stubAdminService.GetProxy(ctx, id)
}

func newOpenAIPublicLinkStubAdminService() *stubAdminService {
	adminService := newStubAdminService()
	adminService.groups = []service.Group{{
		ID:        11,
		Name:      "openai-group",
		Platform:  service.PlatformOpenAI,
		Status:    service.StatusActive,
		CreatedAt: adminService.groups[0].CreatedAt,
		UpdatedAt: adminService.groups[0].UpdatedAt,
	}}
	adminService.proxies = []service.Proxy{{
		ID:        7,
		Name:      "proxy",
		Protocol:  "http",
		Host:      "127.0.0.1",
		Port:      8080,
		Status:    service.StatusActive,
		CreatedAt: adminService.proxies[0].CreatedAt,
		UpdatedAt: adminService.proxies[0].UpdatedAt,
	}}
	return adminService
}

func newOpenAIPublicLinkHandlerWithSettingService(adminService service.AdminService, settingService *service.SettingService) *OpenAIOAuthHandler {
	return &OpenAIOAuthHandler{
		openaiOAuthService: service.NewOpenAIOAuthService(nil, nil),
		adminService:       adminService,
		settingService:     settingService,
	}
}

func newOpenAIPublicLinkHandlerWithAdminService(adminService service.AdminService) *OpenAIOAuthHandler {
	repo := newOpenAIPublicLinkHandlerSettingRepoStub()
	settingService := service.NewSettingService(repo, &config.Config{Server: config.ServerConfig{FrontendURL: ""}})
	return newOpenAIPublicLinkHandlerWithSettingService(adminService, settingService)
}

func newOpenAIPublicLinkHandlerForTest() (*OpenAIOAuthHandler, *stubAdminService) {
	adminService := newOpenAIPublicLinkStubAdminService()
	return newOpenAIPublicLinkHandlerWithAdminService(adminService), adminService
}

func newOpenAIPublicLinkTestContext(t *testing.T, method, target string, body any) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	var bodyReader *bytes.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		require.NoError(t, err)
		bodyReader = bytes.NewReader(payload)
	} else {
		bodyReader = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, target, bodyReader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Origin", "https://admin.example.com")
	req.Host = "backend.internal"
	c.Request = req
	return c, recorder
}

func decodeOpenAIPublicLinkResponse[T any](t *testing.T, recorder *httptest.ResponseRecorder) openAIPublicLinkAPIResponse[T] {
	t.Helper()

	var resp openAIPublicLinkAPIResponse[T]
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp), "response body: %s", recorder.Body.String())
	return resp
}

func TestOpenAIOAuthHandler_PublicAddLinkURLUsesOriginFallback(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	repo := newOpenAIPublicLinkHandlerSettingRepoStub()
	svc := service.NewSettingService(repo, &config.Config{Server: config.ServerConfig{FrontendURL: ""}})
	h := &OpenAIOAuthHandler{settingService: svc}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodGet, "http://backend.internal/api", nil)
	req.Header.Set("Origin", "https://admin.example.com")
	req.Host = "backend.internal"
	c.Request = req

	proxyID := int64(7)
	concurrency := 5
	resp := h.toOpenAIPublicAddLinkResponse(c, &service.OpenAIPublicAddLink{
		Token: "abc",
		AccountDefaults: &service.OpenAIPublicAddLinkAccountDefaults{
			ProxyID:     &proxyID,
			Concurrency: &concurrency,
		},
	})
	require.Equal(t, "https://admin.example.com/openai/connect/abc", resp.URL)
	require.NotNil(t, resp.AccountDefaults)
	require.NotNil(t, resp.AccountDefaults.ProxyID)
	require.Equal(t, proxyID, *resp.AccountDefaults.ProxyID)
}

func TestOpenAIOAuthHandler_PublicAddLinkManagementLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h, _ := newOpenAIPublicLinkHandlerForTest()
	proxyID := int64(7)
	concurrency := 5

	createReq := openAIPublicAddLinkRequest{
		Name:     " Demo ",
		GroupIDs: []int64{11, 11, 0},
		AccountDefaults: &service.OpenAIPublicAddLinkAccountDefaults{
			ProxyID:     &proxyID,
			Concurrency: &concurrency,
		},
	}
	createCtx, createRecorder := newOpenAIPublicLinkTestContext(t, http.MethodPost, "http://backend.internal/api/v1/admin/openai/public-links", createReq)
	h.CreatePublicAddLink(createCtx)
	createResp := decodeOpenAIPublicLinkResponse[openAIPublicAddLinkResponse](t, createRecorder)
	require.Equal(t, http.StatusOK, createRecorder.Code)
	require.Equal(t, 0, createResp.Code)
	require.Equal(t, "Demo", createResp.Data.Name)
	require.Equal(t, []int64{11}, createResp.Data.GroupIDs)
	require.Equal(t, "https://admin.example.com/openai/connect/"+createResp.Data.Token, createResp.Data.URL)
	require.NotNil(t, createResp.Data.AccountDefaults)
	require.NotNil(t, createResp.Data.AccountDefaults.ProxyID)
	require.Equal(t, proxyID, *createResp.Data.AccountDefaults.ProxyID)
	require.NotNil(t, createResp.Data.AccountDefaults.Concurrency)
	require.Equal(t, concurrency, *createResp.Data.AccountDefaults.Concurrency)

	linkToken := createResp.Data.Token
	require.NotEmpty(t, linkToken)

	listCtx, listRecorder := newOpenAIPublicLinkTestContext(t, http.MethodGet, "http://backend.internal/api/v1/admin/openai/public-links", nil)
	h.ListPublicAddLinks(listCtx)
	listResp := decodeOpenAIPublicLinkResponse[[]openAIPublicAddLinkResponse](t, listRecorder)
	require.Equal(t, http.StatusOK, listRecorder.Code)
	require.Len(t, listResp.Data, 1)
	require.Equal(t, createResp.Data, listResp.Data[0])

	groupsCtx, groupsRecorder := newOpenAIPublicLinkTestContext(t, http.MethodGet, "http://backend.internal/openai/connect/"+linkToken+"/groups", nil)
	groupsCtx.Params = gin.Params{{Key: "token", Value: linkToken}}
	h.GetPublicAddLinkGroups(groupsCtx)
	groupsResp := decodeOpenAIPublicLinkResponse[[]dto.Group](t, groupsRecorder)
	require.Equal(t, http.StatusOK, groupsRecorder.Code)
	require.Len(t, groupsResp.Data, 1)
	require.Equal(t, int64(11), groupsResp.Data[0].ID)
	require.Equal(t, service.PlatformOpenAI, groupsResp.Data[0].Platform)

	priority := 2
	updateReq := openAIPublicAddLinkRequest{
		Name:     "Updated",
		GroupIDs: []int64{11},
		AccountDefaults: &service.OpenAIPublicAddLinkAccountDefaults{
			Priority: &priority,
		},
	}
	updateCtx, updateRecorder := newOpenAIPublicLinkTestContext(t, http.MethodPut, "http://backend.internal/api/v1/admin/openai/public-links/"+linkToken, updateReq)
	updateCtx.Params = gin.Params{{Key: "token", Value: linkToken}}
	h.UpdatePublicAddLink(updateCtx)
	updateResp := decodeOpenAIPublicLinkResponse[openAIPublicAddLinkResponse](t, updateRecorder)
	require.Equal(t, http.StatusOK, updateRecorder.Code)
	require.Equal(t, linkToken, updateResp.Data.Token)
	require.Equal(t, "Updated", updateResp.Data.Name)
	require.NotNil(t, updateResp.Data.AccountDefaults)
	require.Nil(t, updateResp.Data.AccountDefaults.ProxyID)
	require.NotNil(t, updateResp.Data.AccountDefaults.Priority)
	require.Equal(t, priority, *updateResp.Data.AccountDefaults.Priority)

	authCtx, authRecorder := newOpenAIPublicLinkTestContext(t, http.MethodPost, "http://backend.internal/openai/connect/"+linkToken+"/generate-auth-url", nil)
	authCtx.Params = gin.Params{{Key: "token", Value: linkToken}}
	h.GeneratePublicAddLinkAuthURL(authCtx)
	authResp := decodeOpenAIPublicLinkResponse[service.OpenAIAuthURLResult](t, authRecorder)
	require.Equal(t, http.StatusOK, authRecorder.Code)
	require.NotEmpty(t, authResp.Data.SessionID)
	require.Contains(t, authResp.Data.AuthURL, "state=")

	rotateCtx, rotateRecorder := newOpenAIPublicLinkTestContext(t, http.MethodPost, "http://backend.internal/api/v1/admin/openai/public-links/"+linkToken+"/rotate", nil)
	rotateCtx.Params = gin.Params{{Key: "token", Value: linkToken}}
	h.RotatePublicAddLink(rotateCtx)
	rotateResp := decodeOpenAIPublicLinkResponse[openAIPublicAddLinkResponse](t, rotateRecorder)
	require.Equal(t, http.StatusOK, rotateRecorder.Code)
	require.NotEqual(t, linkToken, rotateResp.Data.Token)
	require.Equal(t, "Updated", rotateResp.Data.Name)
	require.Equal(t, "https://admin.example.com/openai/connect/"+rotateResp.Data.Token, rotateResp.Data.URL)

	deleteCtx, deleteRecorder := newOpenAIPublicLinkTestContext(t, http.MethodDelete, "http://backend.internal/api/v1/admin/openai/public-links/"+rotateResp.Data.Token, nil)
	deleteCtx.Params = gin.Params{{Key: "token", Value: rotateResp.Data.Token}}
	h.DeletePublicAddLink(deleteCtx)
	deleteResp := decodeOpenAIPublicLinkResponse[map[string]string](t, deleteRecorder)
	require.Equal(t, http.StatusOK, deleteRecorder.Code)
	require.Equal(t, "OpenAI public add link deleted successfully", deleteResp.Data["message"])

	listAfterDeleteCtx, listAfterDeleteRecorder := newOpenAIPublicLinkTestContext(t, http.MethodGet, "http://backend.internal/api/v1/admin/openai/public-links", nil)
	h.ListPublicAddLinks(listAfterDeleteCtx)
	listAfterDeleteResp := decodeOpenAIPublicLinkResponse[[]openAIPublicAddLinkResponse](t, listAfterDeleteRecorder)
	require.Equal(t, http.StatusOK, listAfterDeleteRecorder.Code)
	require.Empty(t, listAfterDeleteResp.Data)
}

func TestOpenAIOAuthHandler_CreatePublicAddLinkValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("rejects invalid group", func(t *testing.T) {
		h, _ := newOpenAIPublicLinkHandlerForTest()

		createCtx, createRecorder := newOpenAIPublicLinkTestContext(t, http.MethodPost, "http://backend.internal/api/v1/admin/openai/public-links", openAIPublicAddLinkRequest{
			Name:     "demo",
			GroupIDs: []int64{999},
		})
		h.CreatePublicAddLink(createCtx)
		resp := decodeOpenAIPublicLinkResponse[map[string]any](t, createRecorder)
		require.Equal(t, http.StatusBadRequest, createRecorder.Code)
		require.Equal(t, "INVALID_OPENAI_PUBLIC_LINK_GROUP", resp.Reason)
	})

	t.Run("rejects invalid proxy default", func(t *testing.T) {
		h, _ := newOpenAIPublicLinkHandlerForTest()
		proxyID := int64(-1)

		createCtx, createRecorder := newOpenAIPublicLinkTestContext(t, http.MethodPost, "http://backend.internal/api/v1/admin/openai/public-links", openAIPublicAddLinkRequest{
			Name:     "demo",
			GroupIDs: []int64{11},
			AccountDefaults: &service.OpenAIPublicAddLinkAccountDefaults{
				ProxyID: &proxyID,
			},
		})
		h.CreatePublicAddLink(createCtx)
		resp := decodeOpenAIPublicLinkResponse[map[string]any](t, createRecorder)
		require.Equal(t, http.StatusBadRequest, createRecorder.Code)
		require.Equal(t, "OPENAI_PUBLIC_LINK_PROXY_INVALID", resp.Reason)
	})

	t.Run("maps proxy not found to bad request", func(t *testing.T) {
		proxyID := int64(99)
		proxyBaseAdminService := newOpenAIPublicLinkStubAdminService()
		adminService := &openAIPublicLinkProxyAdminService{
			stubAdminService: proxyBaseAdminService,
			getProxy: func(ctx context.Context, id int64) (*service.Proxy, error) {
				if id == proxyID {
					return nil, service.ErrProxyNotFound
				}
				return proxyBaseAdminService.GetProxy(ctx, id)
			},
		}
		h := newOpenAIPublicLinkHandlerWithAdminService(adminService)

		createCtx, createRecorder := newOpenAIPublicLinkTestContext(t, http.MethodPost, "http://backend.internal/api/v1/admin/openai/public-links", openAIPublicAddLinkRequest{
			Name:     "demo",
			GroupIDs: []int64{11},
			AccountDefaults: &service.OpenAIPublicAddLinkAccountDefaults{
				ProxyID: &proxyID,
			},
		})
		h.CreatePublicAddLink(createCtx)
		resp := decodeOpenAIPublicLinkResponse[map[string]any](t, createRecorder)
		require.Equal(t, http.StatusBadRequest, createRecorder.Code)
		require.Equal(t, "OPENAI_PUBLIC_LINK_PROXY_NOT_FOUND", resp.Reason)
	})

	t.Run("propagates non not found proxy errors during validation", func(t *testing.T) {
		proxyID := int64(99)
		upstreamErr := infraerrors.New(http.StatusServiceUnavailable, "PROXY_LOOKUP_FAILED", "proxy lookup failed")
		proxyBaseAdminService := newOpenAIPublicLinkStubAdminService()
		adminService := &openAIPublicLinkProxyAdminService{
			stubAdminService: proxyBaseAdminService,
			getProxy: func(ctx context.Context, id int64) (*service.Proxy, error) {
				if id == proxyID {
					return nil, upstreamErr
				}
				return proxyBaseAdminService.GetProxy(ctx, id)
			},
		}
		h := newOpenAIPublicLinkHandlerWithAdminService(adminService)

		createCtx, createRecorder := newOpenAIPublicLinkTestContext(t, http.MethodPost, "http://backend.internal/api/v1/admin/openai/public-links", openAIPublicAddLinkRequest{
			Name:     "demo",
			GroupIDs: []int64{11},
			AccountDefaults: &service.OpenAIPublicAddLinkAccountDefaults{
				ProxyID: &proxyID,
			},
		})
		h.CreatePublicAddLink(createCtx)
		resp := decodeOpenAIPublicLinkResponse[map[string]any](t, createRecorder)
		require.Equal(t, http.StatusServiceUnavailable, createRecorder.Code)
		require.Equal(t, "PROXY_LOOKUP_FAILED", resp.Reason)
	})
}

func TestOpenAIOAuthHandler_GeneratePublicAddLinkAuthURLPropagatesProxyLookupErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	proxyID := int64(7)
	repo := newOpenAIPublicLinkHandlerSettingRepoStub()
	settingService := service.NewSettingService(repo, &config.Config{Server: config.ServerConfig{FrontendURL: ""}})

	createAdminService := newOpenAIPublicLinkStubAdminService()
	createHandler := newOpenAIPublicLinkHandlerWithSettingService(createAdminService, settingService)
	createCtx, createRecorder := newOpenAIPublicLinkTestContext(t, http.MethodPost, "http://backend.internal/api/v1/admin/openai/public-links", openAIPublicAddLinkRequest{
		Name:     "demo",
		GroupIDs: []int64{11},
		AccountDefaults: &service.OpenAIPublicAddLinkAccountDefaults{
			ProxyID: &proxyID,
		},
	})
	createHandler.CreatePublicAddLink(createCtx)
	createResp := decodeOpenAIPublicLinkResponse[openAIPublicAddLinkResponse](t, createRecorder)
	require.Equal(t, http.StatusOK, createRecorder.Code)
	linkToken := createResp.Data.Token
	require.NotEmpty(t, linkToken)

	upstreamErr := infraerrors.New(http.StatusServiceUnavailable, "PROXY_LOOKUP_FAILED", "proxy lookup failed")
	proxyBaseAdminService := newOpenAIPublicLinkStubAdminService()
	proxyLookupCalls := 0
	errorAdminService := &openAIPublicLinkProxyAdminService{
		stubAdminService: proxyBaseAdminService,
		getProxy: func(ctx context.Context, id int64) (*service.Proxy, error) {
			proxyLookupCalls++
			if id == proxyID && proxyLookupCalls == 2 {
				return nil, upstreamErr
			}
			return proxyBaseAdminService.GetProxy(ctx, id)
		},
	}
	errorHandler := newOpenAIPublicLinkHandlerWithSettingService(errorAdminService, settingService)

	authCtx, authRecorder := newOpenAIPublicLinkTestContext(t, http.MethodPost, "http://backend.internal/openai/connect/"+linkToken+"/generate-auth-url", nil)
	authCtx.Params = gin.Params{{Key: "token", Value: linkToken}}
	errorHandler.GeneratePublicAddLinkAuthURL(authCtx)
	authResp := decodeOpenAIPublicLinkResponse[map[string]any](t, authRecorder)
	require.Equal(t, http.StatusServiceUnavailable, authRecorder.Code)
	require.Equal(t, "PROXY_LOOKUP_FAILED", authResp.Reason)
}

type openAIPublicAddAccountOAuthClientStub struct {
	exchangeResp *openai.TokenResponse
	exchangeErr  error
	refreshResp  *openai.TokenResponse
	refreshErr   error

	lastExchange struct {
		code         string
		codeVerifier string
		redirectURI  string
		proxyURL     string
		clientID     string
	}
	lastRefresh struct {
		refreshToken string
		proxyURL     string
		clientID     string
	}
}

func (s *openAIPublicAddAccountOAuthClientStub) ExchangeCode(_ context.Context, code, codeVerifier, redirectURI, proxyURL, clientID string) (*openai.TokenResponse, error) {
	s.lastExchange.code = code
	s.lastExchange.codeVerifier = codeVerifier
	s.lastExchange.redirectURI = redirectURI
	s.lastExchange.proxyURL = proxyURL
	s.lastExchange.clientID = clientID
	if s.exchangeErr != nil {
		return nil, s.exchangeErr
	}
	if s.exchangeResp != nil {
		return s.exchangeResp, nil
	}
	return &openai.TokenResponse{
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		ExpiresIn:    3600,
	}, nil
}

func (s *openAIPublicAddAccountOAuthClientStub) RefreshToken(ctx context.Context, refreshToken, proxyURL string) (*openai.TokenResponse, error) {
	return s.RefreshTokenWithClientID(ctx, refreshToken, proxyURL, "")
}

func (s *openAIPublicAddAccountOAuthClientStub) RefreshTokenWithClientID(_ context.Context, refreshToken, proxyURL string, clientID string) (*openai.TokenResponse, error) {
	s.lastRefresh.refreshToken = refreshToken
	s.lastRefresh.proxyURL = proxyURL
	s.lastRefresh.clientID = clientID
	if s.refreshErr != nil {
		return nil, s.refreshErr
	}
	if s.refreshResp != nil {
		return s.refreshResp, nil
	}
	return &openai.TokenResponse{
		AccessToken:  "refresh-access-token",
		RefreshToken: "refresh-token",
		ExpiresIn:    3600,
	}, nil
}

func newOpenAIPublicAddAccountHandlerForTest(oauthClient service.OpenAIOAuthClient) (*OpenAIOAuthHandler, *stubAdminService, *service.SettingService) {
	adminService := newOpenAIPublicLinkStubAdminService()
	repo := newOpenAIPublicLinkHandlerSettingRepoStub()
	settingService := service.NewSettingService(repo, &config.Config{Server: config.ServerConfig{FrontendURL: ""}})
	return &OpenAIOAuthHandler{
		openaiOAuthService: service.NewOpenAIOAuthService(nil, oauthClient),
		adminService:       adminService,
		settingService:     settingService,
	}, adminService, settingService
}

func createOpenAIPublicAddLinkForTest(t *testing.T, settingService *service.SettingService, defaults *service.OpenAIPublicAddLinkAccountDefaults) *service.OpenAIPublicAddLink {
	t.Helper()

	link, err := settingService.CreateOpenAIPublicAddLink(context.Background(), "demo", []int64{11}, defaults)
	require.NoError(t, err)
	require.NotEmpty(t, link.Token)
	return link
}

func createOpenAIAuthSessionForTest(t *testing.T, handler *OpenAIOAuthHandler) (string, string) {
	t.Helper()

	result, err := handler.openaiOAuthService.GenerateAuthURL(context.Background(), nil, "", service.PlatformOpenAI)
	require.NoError(t, err)
	parsed, err := url.Parse(result.AuthURL)
	require.NoError(t, err)
	state := parsed.Query().Get("state")
	require.NotEmpty(t, state)
	return result.SessionID, state
}

func TestOpenAIOAuthHandler_CreateAccountFromPublicAddLink(t *testing.T) {
	gin.SetMode(gin.TestMode)

	oauthClient := &openAIPublicAddAccountOAuthClientStub{
		exchangeResp: &openai.TokenResponse{
			AccessToken:  "public-access-token",
			RefreshToken: "public-refresh-token",
			ExpiresIn:    3600,
		},
	}
	h, adminService, settingService := newOpenAIPublicAddAccountHandlerForTest(oauthClient)

	concurrency := 6
	loadFactor := 2
	priority := 3
	rateMultiplier := 1.5
	expiresAt := int64(1710000000)
	autoPauseOnExpired := true
	link := createOpenAIPublicAddLinkForTest(t, settingService, &service.OpenAIPublicAddLinkAccountDefaults{
		Concurrency:        &concurrency,
		LoadFactor:         &loadFactor,
		Priority:           &priority,
		RateMultiplier:     &rateMultiplier,
		ExpiresAt:          &expiresAt,
		AutoPauseOnExpired: &autoPauseOnExpired,
		Credentials: map[string]any{
			"model_mapping": map[string]any{"gpt-4o": "gpt-4o-mini"},
			"ignored":       "value",
		},
		Extra: map[string]any{
			"codex_cli_only": true,
			"ignored":        "value",
		},
	})
	sessionID, state := createOpenAIAuthSessionForTest(t, h)

	ctx, recorder := newOpenAIPublicLinkTestContext(t, http.MethodPost, "http://backend.internal/openai/connect/"+link.Token+"/accounts", map[string]any{
		"session_id":   sessionID,
		"code":         "oauth-code",
		"state":        state,
		"redirect_uri": "https://callback.example.com/openai",
	})
	ctx.Params = gin.Params{{Key: "token", Value: link.Token}}
	h.CreateAccountFromPublicAddLink(ctx)

	resp := decodeOpenAIPublicLinkResponse[map[string]any](t, recorder)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, 0, resp.Code)
	require.Len(t, adminService.createdAccounts, 1)

	created := adminService.createdAccounts[0]
	require.Equal(t, "OpenAI OAuth Account", created.Name)
	require.Equal(t, service.PlatformOpenAI, created.Platform)
	require.Equal(t, service.AccountTypeOAuth, created.Type)
	require.Equal(t, []int64{11}, created.GroupIDs)
	require.Equal(t, concurrency, created.Concurrency)
	require.NotNil(t, created.LoadFactor)
	require.Equal(t, loadFactor, *created.LoadFactor)
	require.Equal(t, priority, created.Priority)
	require.NotNil(t, created.RateMultiplier)
	require.Equal(t, rateMultiplier, *created.RateMultiplier)
	require.NotNil(t, created.ExpiresAt)
	require.Equal(t, expiresAt, *created.ExpiresAt)
	require.NotNil(t, created.AutoPauseOnExpired)
	require.Equal(t, autoPauseOnExpired, *created.AutoPauseOnExpired)
	require.True(t, created.SkipDefaultGroupBind)
	require.Equal(t, "public-access-token", created.Credentials["access_token"])
	require.Equal(t, "public-refresh-token", created.Credentials["refresh_token"])
	require.Equal(t, openai.ClientID, created.Credentials["client_id"])
	require.Contains(t, created.Credentials, "model_mapping")
	require.NotContains(t, created.Credentials, "ignored")
	require.Equal(t, true, created.Extra["codex_cli_only"])
	require.NotContains(t, created.Extra, "ignored")
	require.Equal(t, "oauth-code", oauthClient.lastExchange.code)
	require.Equal(t, "https://callback.example.com/openai", oauthClient.lastExchange.redirectURI)
	require.Equal(t, openai.ClientID, oauthClient.lastExchange.clientID)
	require.Empty(t, oauthClient.lastExchange.proxyURL)
}

func TestOpenAIOAuthHandler_CreateAccountFromPublicRefreshToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	oauthClient := &openAIPublicAddAccountOAuthClientStub{
		refreshResp: &openai.TokenResponse{
			AccessToken: "refreshed-access-token",
			ExpiresIn:   7200,
		},
	}
	h, adminService, settingService := newOpenAIPublicAddAccountHandlerForTest(oauthClient)

	proxyID := int64(7)
	concurrency := 8
	link := createOpenAIPublicAddLinkForTest(t, settingService, &service.OpenAIPublicAddLinkAccountDefaults{
		ProxyID:     &proxyID,
		Concurrency: &concurrency,
		Credentials: map[string]any{
			"model_mapping": map[string]any{"gpt-4o": "gpt-4.1"},
		},
		Extra: map[string]any{
			"codex_cli_only": true,
		},
	})

	ctx, recorder := newOpenAIPublicLinkTestContext(t, http.MethodPost, "http://backend.internal/openai/connect/"+link.Token+"/refresh-token", map[string]any{
		"rt": "  link-refresh-token  ",
	})
	ctx.Params = gin.Params{{Key: "token", Value: link.Token}}
	h.CreateAccountFromPublicRefreshToken(ctx)

	resp := decodeOpenAIPublicLinkResponse[map[string]any](t, recorder)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, 0, resp.Code)
	require.Len(t, adminService.createdAccounts, 1)

	created := adminService.createdAccounts[0]
	require.NotNil(t, created.ProxyID)
	require.Equal(t, proxyID, *created.ProxyID)
	require.Equal(t, []int64{11}, created.GroupIDs)
	require.Equal(t, concurrency, created.Concurrency)
	require.True(t, created.SkipDefaultGroupBind)
	require.Equal(t, "link-refresh-token", oauthClient.lastRefresh.refreshToken)
	require.Equal(t, "http://127.0.0.1:8080", oauthClient.lastRefresh.proxyURL)
	require.Equal(t, openai.ClientID, oauthClient.lastRefresh.clientID)
	require.Equal(t, "refreshed-access-token", created.Credentials["access_token"])
	require.Equal(t, "link-refresh-token", created.Credentials["refresh_token"])
	require.Equal(t, openai.ClientID, created.Credentials["client_id"])
	require.Contains(t, created.Credentials, "model_mapping")
	require.Equal(t, true, created.Extra["codex_cli_only"])
}

func TestOpenAIOAuthHandler_CreateAccountFromPublicCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("normalizes credentials and applies defaults", func(t *testing.T) {
		h, adminService, settingService := newOpenAIPublicAddAccountHandlerForTest(nil)

		concurrency := 4
		link := createOpenAIPublicAddLinkForTest(t, settingService, &service.OpenAIPublicAddLinkAccountDefaults{
			Concurrency: &concurrency,
			Credentials: map[string]any{
				"model_mapping": map[string]any{"gpt-4o": "gpt-4o-mini"},
			},
			Extra: map[string]any{
				"openai_passthrough": true,
				"codex_cli_only":     true,
			},
		})

		ctx, recorder := newOpenAIPublicLinkTestContext(t, http.MethodPost, "http://backend.internal/openai/connect/"+link.Token+"/credentials", map[string]any{
			"credentials": map[string]any{
				"access_token":      "  direct-access-token  ",
				"email":             "  user@example.com  ",
				"account_id":        "acct_123",
				"user_id":           123,
				"poid":              "org_456",
				"chatgpt_plan_type": "  pro  ",
				"expired":           1710000000,
				"type":              "mobile",
				"clientId":          "   ",
			},
			"extra": map[string]any{
				"custom": "value",
			},
		})
		ctx.Params = gin.Params{{Key: "token", Value: link.Token}}
		h.CreateAccountFromPublicCredentials(ctx)

		resp := decodeOpenAIPublicLinkResponse[map[string]any](t, recorder)
		require.Equal(t, http.StatusOK, recorder.Code)
		require.Equal(t, 0, resp.Code)
		require.Len(t, adminService.createdAccounts, 1)

		created := adminService.createdAccounts[0]
		require.Equal(t, "user@example.com", created.Name)
		require.Equal(t, concurrency, created.Concurrency)
		require.True(t, created.SkipDefaultGroupBind)
		require.Equal(t, "direct-access-token", created.Credentials["access_token"])
		require.Equal(t, "user@example.com", created.Credentials["email"])
		require.Equal(t, "acct_123", created.Credentials["chatgpt_account_id"])
		require.Equal(t, "123", created.Credentials["chatgpt_user_id"])
		require.Equal(t, "org_456", created.Credentials["organization_id"])
		require.Equal(t, "pro", created.Credentials["plan_type"])
		require.Equal(t, time.Unix(1710000000, 0).Format(time.RFC3339), created.Credentials["expires_at"])
		require.Equal(t, openai.ClientID, created.Credentials["client_id"])
		require.NotContains(t, created.Credentials, "account_id")
		require.NotContains(t, created.Credentials, "expired")
		require.NotContains(t, created.Credentials, "expiresAt")
		require.NotContains(t, created.Credentials, "clientId")
		require.NotContains(t, created.Credentials, "model_mapping")
		require.Equal(t, true, created.Extra["openai_passthrough"])
		require.Equal(t, true, created.Extra["codex_cli_only"])
		require.Equal(t, "value", created.Extra["custom"])
	})

	t.Run("requires access token", func(t *testing.T) {
		h, adminService, settingService := newOpenAIPublicAddAccountHandlerForTest(nil)
		link := createOpenAIPublicAddLinkForTest(t, settingService, nil)

		ctx, recorder := newOpenAIPublicLinkTestContext(t, http.MethodPost, "http://backend.internal/openai/connect/"+link.Token+"/credentials", map[string]any{
			"credentials": map[string]any{
				"refresh_token": "refresh-only",
			},
		})
		ctx.Params = gin.Params{{Key: "token", Value: link.Token}}
		h.CreateAccountFromPublicCredentials(ctx)

		resp := decodeOpenAIPublicLinkResponse[map[string]any](t, recorder)
		require.Equal(t, http.StatusBadRequest, recorder.Code)
		require.Equal(t, "OPENAI_PUBLIC_LINK_ACCESS_TOKEN_REQUIRED", resp.Reason)
		require.Empty(t, adminService.createdAccounts)
	})
}
