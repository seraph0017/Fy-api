package common

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionOptionsCookieDomain(t *testing.T) {
	originalDomain := SessionCookieDomain
	t.Cleanup(func() {
		SessionCookieDomain = originalDomain
	})

	SessionCookieDomain = ""
	assert.Empty(t, SessionOptions().Domain)

	SessionCookieDomain = ".aitracenex.com"
	assert.Equal(t, ".aitracenex.com", SessionOptions().Domain)
}

func TestClearLegacyHostOnlySessionCookie(t *testing.T) {
	recorder := httptest.NewRecorder()

	ClearLegacyHostOnlySessionCookie(recorder)

	values := recorder.Header().Values("Set-Cookie")
	require.Len(t, values, 1)
	assert.True(t, strings.HasPrefix(values[0], "session=;"))
	assert.Contains(t, values[0], "Path=/")
	assert.Contains(t, values[0], "Max-Age=0")
	assert.Contains(t, values[0], "HttpOnly")
	assert.NotContains(t, values[0], "Domain=")
}
