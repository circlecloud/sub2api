package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type batchAvailableModelsAdminService struct {
	*stubAdminService
	projectionAccounts      []*service.Account
	projectionCalls         int
	heavyGetAccountsByIDsCalls int
}

func (s *batchAvailableModelsAdminService) GetAccountsByIDs(ctx context.Context, ids []int64) ([]*service.Account, error) {
	s.heavyGetAccountsByIDsCalls++
	return s.stubAdminService.GetAccountsByIDs(ctx, ids)
}

func (s *batchAvailableModelsAdminService) GetAccountsModelProjectionByIDs(_ context.Context, ids []int64) ([]*service.Account, error) {
	s.projectionCalls++
	out := make([]*service.Account, 0, len(ids))
	for _, id := range ids {
		for _, account := range s.projectionAccounts {
			if account != nil && account.ID == id {
				copyAccount := *account
				out = append(out, &copyAccount)
				break
			}
		}
	}
	return out, nil
}

func TestAccountHandlerGetBatchAvailableModels_UsesProjectionAndIntersectsSignatures(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &batchAvailableModelsAdminService{
		stubAdminService: newStubAdminService(),
		projectionAccounts: []*service.Account{
			{
				ID:       11,
				Name:     "openai-oauth-a",
				Platform: service.PlatformOpenAI,
				Type:     service.AccountTypeOAuth,
				Credentials: map[string]any{
					"model_mapping": map[string]any{
						"gpt-5.4": "gpt-5.4",
					},
				},
			},
			{
				ID:       22,
				Name:     "openai-oauth-b",
				Platform: service.PlatformOpenAI,
				Type:     service.AccountTypeOAuth,
				Credentials: map[string]any{
					"model_mapping": map[string]any{
						"gpt-5.4": "gpt-5.4",
					},
				},
			},
			{
				ID:       33,
				Name:     "openai-passthrough-a",
				Platform: service.PlatformOpenAI,
				Type:     service.AccountTypeOAuth,
				Credentials: map[string]any{
					"model_mapping": map[string]any{
						"gpt-5.4": "gpt-5.4",
					},
				},
				Extra: map[string]any{
					"openai_passthrough": true,
				},
			},
		},
	}

	handler := NewAccountHandler(svc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	method := reflect.ValueOf(handler).MethodByName("GetBatchAvailableModels")
	require.True(t, method.IsValid(), "AccountHandler should expose GetBatchAvailableModels")

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/models/batch", bytes.NewReader([]byte(`{"account_ids":[11,22,33]}`)))
	ctx.Request.Header.Set("Content-Type", "application/json")

	method.Call([]reflect.Value{reflect.ValueOf(ctx)})

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, svc.projectionCalls)
	require.Zero(t, svc.heavyGetAccountsByIDsCalls)

	var resp struct {
		Data struct {
			Count          int `json:"count"`
			SignatureCount int `json:"signature_count"`
			Models         []struct {
				ID string `json:"id"`
			} `json:"models"`
			RepresentativeAccounts []struct {
				ID       int64  `json:"id"`
				Name     string `json:"name"`
				Platform string `json:"platform"`
				Type     string `json:"type"`
			} `json:"representative_accounts"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 3, resp.Data.Count)
	require.Equal(t, 2, resp.Data.SignatureCount)
	require.Len(t, resp.Data.Models, 1)
	require.Equal(t, "gpt-5.4", resp.Data.Models[0].ID)
	require.Len(t, resp.Data.RepresentativeAccounts, 2)
	require.Equal(t, int64(11), resp.Data.RepresentativeAccounts[0].ID)
	require.Equal(t, "openai-oauth-a", resp.Data.RepresentativeAccounts[0].Name)
	require.Equal(t, service.PlatformOpenAI, resp.Data.RepresentativeAccounts[0].Platform)
	require.Equal(t, service.AccountTypeOAuth, resp.Data.RepresentativeAccounts[0].Type)
	require.Equal(t, int64(33), resp.Data.RepresentativeAccounts[1].ID)
	require.Equal(t, "openai-passthrough-a", resp.Data.RepresentativeAccounts[1].Name)
}
