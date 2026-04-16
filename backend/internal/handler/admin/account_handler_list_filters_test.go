package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAccountHandlerListPassesNormalizedFiltersAndMultiSort(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adminSvc := newStubAdminService()
	router := gin.New()
	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router.GET("/api/v1/admin/accounts", handler.List)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/admin/accounts?platform=openai&type=oauth&status=active&search=keyword&group=3,1,2&group_exclude=5,4&group_match=exact&privacy_mode=blocked&last_used_filter=range&last_used_start_date=2026-01-01&last_used_end_date=2026-01-31&timezone=UTC&sort_by=last_used_at,id&sort_order=desc,asc",
		nil,
	)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	require.Equal(t, 1, adminSvc.lastListAccounts.calls)
	require.Equal(t, service.AccountListFilters{
		Platform:        service.PlatformOpenAI,
		AccountType:     service.AccountTypeOAuth,
		Status:          service.StatusActive,
		Search:          "keyword",
		GroupIDs:        "1,2,3",
		GroupExcludeIDs: "4,5",
		GroupExact:      true,
		PrivacyMode:     "blocked",
		LastUsedFilter:  "range",
		LastUsedStart:   &start,
		LastUsedEnd:     &end,
	}, adminSvc.lastListAccounts.filters)
	require.Equal(t, pagination.PaginationParams{Page: 1, PageSize: 20, SortBy: "last_used_at,id", SortOrder: "desc,asc"}, adminSvc.lastListAccounts.params)
}
