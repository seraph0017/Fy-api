package common

import (
	"net/http"

	"github.com/gin-contrib/sessions"
)

func SessionOptions() sessions.Options {
	options := sessions.Options{
		Path:     "/",
		MaxAge:   2592000, // 30 days
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteStrictMode,
	}
	// Fy-api overlay: allow api/www alias domains to share browser sessions during migration.
	if SessionCookieDomain != "" {
		options.Domain = SessionCookieDomain
	}
	return options
}
