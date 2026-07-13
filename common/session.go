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
		Secure:   SessionCookieSecure,
		SameSite: http.SameSiteStrictMode,
	}
	// Fy-api overlay: allow api/www alias domains to share browser sessions during migration.
	if SessionCookieDomain != "" {
		options.Domain = SessionCookieDomain
	}
	return options
}

// Fy-api overlay: clear host-only cookies left before SESSION_COOKIE_DOMAIN migration.
func ClearLegacyHostOnlySessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
}
