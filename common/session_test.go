package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
