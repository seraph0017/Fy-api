package router

import (
	"embed"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

//go:embed web/default/dist/* web/classic/dist/* web/classic/dist/product-docs/*
var testWebAssets embed.FS

func TestSetWebRouterServesProductDocsFromDocsPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousTheme := common.GetTheme()
	common.SetTheme("classic")
	t.Cleanup(func() {
		common.SetTheme(previousTheme)
	})

	router := gin.New()
	SetWebRouter(router, ThemeAssets{
		DefaultBuildFS:   testWebAssets,
		DefaultIndexPage: []byte("<html><body>default spa shell</body></html>"),
		ClassicBuildFS:   testWebAssets,
		ClassicIndexPage: []byte("<html><body>classic spa shell</body></html>"),
	})

	for _, path := range []string{"/docs", "/docs/", "/docs/api-reference.html"} {
		t.Run(path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, path, nil)

			router.ServeHTTP(recorder, request)

			require.Equal(t, http.StatusOK, recorder.Code)
			require.Contains(t, recorder.Body.String(), "product docs marker")
			require.NotContains(t, recorder.Body.String(), "classic spa shell")
			require.Equal(t, "no-cache", recorder.Header().Get("Cache-Control"))
		})
	}
}
