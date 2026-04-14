package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type groupCapacitySummaryServiceStub struct {
	results []service.GroupCapacitySummary
	err     error
	calls   int
}

func (s *groupCapacitySummaryServiceStub) GetAllGroupCapacity(ctx context.Context) ([]service.GroupCapacitySummary, error) {
	s.calls++
	return s.results, s.err
}

func setupGroupCapacitySummaryRouter(capacitySvc groupCapacitySummaryService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewGroupHandler(newStubAdminService(), nil, capacitySvc)
	router.GET("/api/v1/admin/groups/capacity-summary", handler.GetCapacitySummary)
	return router
}

func TestGroupHandlerGetCapacitySummary_Success(t *testing.T) {
	capacitySvc := &groupCapacitySummaryServiceStub{
		results: []service.GroupCapacitySummary{
			{
				GroupID:         11,
				ConcurrencyUsed: 2,
				ConcurrencyMax:  5,
				SessionsUsed:    3,
				SessionsMax:     8,
				RPMUsed:         50,
				RPMMax:          120,
			},
			{
				GroupID:         12,
				ConcurrencyUsed: 1,
				ConcurrencyMax:  4,
				SessionsUsed:    0,
				SessionsMax:     6,
				RPMUsed:         25,
				RPMMax:          90,
			},
		},
	}
	router := setupGroupCapacitySummaryRouter(capacitySvc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/groups/capacity-summary", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, capacitySvc.calls)

	var resp struct {
		Code    int                            `json:"code"`
		Message string                         `json:"message"`
		Data    []service.GroupCapacitySummary `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Equal(t, "success", resp.Message)
	require.Equal(t, capacitySvc.results, resp.Data)
}

func TestGroupHandlerGetCapacitySummary_ServiceError(t *testing.T) {
	capacitySvc := &groupCapacitySummaryServiceStub{err: errors.New("boom")}
	router := setupGroupCapacitySummaryRouter(capacitySvc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/groups/capacity-summary", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Equal(t, 1, capacitySvc.calls)

	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, http.StatusInternalServerError, resp.Code)
	require.Equal(t, "Failed to get group capacity summary", resp.Message)
}
