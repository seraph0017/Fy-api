package middleware

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newInternalIdempotencyTestRouter(t *testing.T) (*gin.Engine, *int32) {
	t.Helper()

	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		gin.SetMode(oldMode)
	})

	oldDB := model.DB
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.InternalIdempotencyRecord{}))
	model.DB = db
	t.Cleanup(func() {
		model.DB = oldDB
	})

	var calls int32
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(ContextKeyInternalKeyId, c.GetHeader("X-Test-KeyId"))
		c.Next()
	})
	router.Use(InternalIdempotency())

	handler := func(c *gin.Context) {
		call := atomic.AddInt32(&calls, 1)
		body, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)

		response := fmt.Sprintf(`{"call":%d,"kid":%q,"path":%q,"body":%q}`, call, c.GetHeader("X-Test-KeyId"), c.Request.URL.Path, string(body))
		c.Data(http.StatusCreated, "application/json", []byte(response))
		require.NoError(t, SaveIdempotencyResponse(c, http.StatusCreated, response))
	}
	router.POST("/api/internal/a", handler)
	router.POST("/api/internal/b", handler)

	return router, &calls
}

func performInternalIdempotencyRequest(router http.Handler, path, kid, idemKey, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, http.NoBody)
	if body != "" {
		req = httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	}
	req.Header.Set("X-Test-KeyId", kid)
	req.Header.Set("Idempotency-Key", idemKey)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func TestInternalIdempotencyReplaysSameKeyEndpointAuthKidAndPayload(t *testing.T) {
	router, calls := newInternalIdempotencyTestRouter(t)

	first := performInternalIdempotencyRequest(router, "/api/internal/a", "kid-a", "idem-1", `{"amount":100}`)
	second := performInternalIdempotencyRequest(router, "/api/internal/a", "kid-a", "idem-1", `{"amount":100}`)

	require.Equal(t, http.StatusCreated, first.Code)
	require.Equal(t, http.StatusCreated, second.Code)
	assert.Empty(t, first.Header().Get("X-Tnb-Idempotent-Replay"))
	assert.Equal(t, "1", second.Header().Get("X-Tnb-Idempotent-Replay"))
	assert.Equal(t, first.Body.String(), second.Body.String())
	assert.EqualValues(t, 1, atomic.LoadInt32(calls))
}

func TestInternalIdempotencyScopesReplayByAuthKidAndEndpoint(t *testing.T) {
	router, calls := newInternalIdempotencyTestRouter(t)

	first := performInternalIdempotencyRequest(router, "/api/internal/a", "kid-a", "idem-1", `{"amount":100}`)
	differentEndpoint := performInternalIdempotencyRequest(router, "/api/internal/b", "kid-a", "idem-1", `{"amount":100}`)
	differentAuthKid := performInternalIdempotencyRequest(router, "/api/internal/a", "kid-b", "idem-1", `{"amount":100}`)

	require.Equal(t, http.StatusCreated, first.Code)
	require.Equal(t, http.StatusCreated, differentEndpoint.Code)
	require.Equal(t, http.StatusCreated, differentAuthKid.Code)
	assert.Empty(t, differentEndpoint.Header().Get("X-Tnb-Idempotent-Replay"))
	assert.Empty(t, differentAuthKid.Header().Get("X-Tnb-Idempotent-Replay"))
	assert.Contains(t, differentEndpoint.Body.String(), `"path":"/api/internal/b"`)
	assert.Contains(t, differentAuthKid.Body.String(), `"kid":"kid-b"`)
	assert.EqualValues(t, 3, atomic.LoadInt32(calls))
}

func TestInternalIdempotencyRejectsSameKeyEndpointAuthKidWithDifferentPayload(t *testing.T) {
	router, calls := newInternalIdempotencyTestRouter(t)

	first := performInternalIdempotencyRequest(router, "/api/internal/a", "kid-a", "idem-1", `{"amount":100}`)
	conflict := performInternalIdempotencyRequest(router, "/api/internal/a", "kid-a", "idem-1", `{"amount":200}`)

	require.Equal(t, http.StatusCreated, first.Code)
	require.Equal(t, http.StatusConflict, conflict.Code)
	assert.Contains(t, conflict.Body.String(), "idempotency key reused with different payload")
	assert.EqualValues(t, 1, atomic.LoadInt32(calls))
}
