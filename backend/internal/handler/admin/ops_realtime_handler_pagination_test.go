package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestShouldPaginateOpsWarmPoolReadyList(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("ready list with page params", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/admin/ops/openai-warm-pool?page=2&page_size=15", nil)

		page, pageSize, ok := shouldPaginateOpsWarmPoolReadyList(c, true, true, "ready")
		require.True(t, ok)
		require.Equal(t, 2, page)
		require.Equal(t, 15, pageSize)
	})

	t.Run("missing page params", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/admin/ops/openai-warm-pool", nil)

		page, pageSize, ok := shouldPaginateOpsWarmPoolReadyList(c, true, true, "ready")
		require.False(t, ok)
		require.Equal(t, 0, page)
		require.Equal(t, 0, pageSize)
	})

	t.Run("non-ready scene does not paginate", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/admin/ops/openai-warm-pool?page=2&page_size=15", nil)

		page, pageSize, ok := shouldPaginateOpsWarmPoolReadyList(c, true, true, "probing")
		require.False(t, ok)
		require.Equal(t, 0, page)
		require.Equal(t, 0, pageSize)
	})

	t.Run("non-ready-list scene does not paginate", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/admin/ops/openai-warm-pool?page=2&page_size=15", nil)

		page, pageSize, ok := shouldPaginateOpsWarmPoolReadyList(c, false, true, "ready")
		require.False(t, ok)
		require.Equal(t, 0, page)
		require.Equal(t, 0, pageSize)
	})
}
