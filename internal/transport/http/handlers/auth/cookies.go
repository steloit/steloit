package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
)

// Cookie names and lifetimes. Declared as package-level constants so
// buildAuthCookies, buildClearAuthCookies, and any future handler that
// reads a cookie (CSRF verification, session inspection) share one
// source of truth. Changing a MaxAge here changes both the set path
// and the clear path in lockstep.
const (
	cookieNameAccess  = "access_token"
	cookieNameRefresh = "refresh_token"
	cookieNameCSRF    = "csrf_token"

	// refreshCookiePath restricts the refresh cookie to the one
	// endpoint that consumes it. Browsers will NOT send the cookie
	// to any other path, reducing the blast radius of a leak.
	refreshCookiePath = "/api/v1/auth/refresh"

	// accessMaxAgeSeconds matches the JWT exp claim issued by
	// authService.Login (15 minutes).
	accessMaxAgeSeconds = 900

	// refreshMaxAgeSeconds is 7 days — the JWT refresh token's
	// server-side TTL.
	refreshMaxAgeSeconds = 604800

	// csrfMaxAgeSeconds matches the access token so the double-
	// submit pair rotate together.
	csrfMaxAgeSeconds = 900
)

// isDevelopment returns true when APP_ENV is empty or "development".
// Controls whether the auth cookies carry the Secure attribute —
// dev browsers run over plain http://localhost and Secure cookies
// would be silently dropped.
func isDevelopment() bool {
	env := os.Getenv("APP_ENV")
	return env == "" || env == "development"
}

// generateCSRFToken produces a cryptographically random token used in
// the double-submit cookie CSRF pattern. 32 random bytes base64-
// encoded — same strength as gorilla/csrf's default.
func generateCSRFToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate CSRF token: %w", err)
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// buildAuthCookies assembles the three cookies that establish an
// authenticated session: access_token (httpOnly, Path=/), refresh_
// token (httpOnly, Path=/api/v1/auth/refresh), csrf_token (readable
// by JS so the frontend can copy it into the X-CSRF-Token header).
//
// domain lets Brokle operate across subdomains by setting e.g.
// ".brokle.com"; leave empty for single-origin deployments.
//
// Secure is determined by isDevelopment — dev runs over plain HTTP
// so the flag must be off, production runs over HTTPS so the flag
// must be on. This matches what the dashboard's cookie-setting
// middleware expects.
//
// Returns []http.Cookie (not direct w.SetCookie calls) because Huma
// operation handlers return struct-tagged output; the SetCookie
// header slice becomes the Output struct's `header:"Set-Cookie"`
// field. See internal/transport/http/handlers/auth/handlers.go.
func buildAuthCookies(access, refresh, csrf, domain string) []http.Cookie {
	secure := !isDevelopment()
	return []http.Cookie{
		{
			Name:     cookieNameAccess,
			Value:    access,
			Path:     "/",
			Domain:   domain,
			MaxAge:   accessMaxAgeSeconds,
			HttpOnly: true,
			Secure:   secure,
			SameSite: http.SameSiteLaxMode,
		},
		{
			Name:     cookieNameRefresh,
			Value:    refresh,
			Path:     refreshCookiePath,
			Domain:   domain,
			MaxAge:   refreshMaxAgeSeconds,
			HttpOnly: true,
			Secure:   secure,
			SameSite: http.SameSiteStrictMode,
		},
		{
			Name:     cookieNameCSRF,
			Value:    csrf,
			Path:     "/",
			Domain:   domain,
			MaxAge:   csrfMaxAgeSeconds,
			HttpOnly: false, // must be readable by JS — intentional
			Secure:   secure,
			SameSite: http.SameSiteLaxMode,
		},
	}
}

// buildClearAuthCookies assembles three expired cookies with the
// same Path / Domain / SameSite attributes as the set path so the
// browser reliably clears them (a mismatched attribute silently
// leaves the cookie in place on some browsers — RFC 6265 §4.1.2).
//
// MaxAge=-1 is the browser's "delete now" signal; value is set to
// "" for good measure so even if the browser retains the entry
// briefly the payload is empty.
func buildClearAuthCookies(domain string) []http.Cookie {
	secure := !isDevelopment()
	return []http.Cookie{
		{
			Name:     cookieNameAccess,
			Value:    "",
			Path:     "/",
			Domain:   domain,
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   secure,
			SameSite: http.SameSiteLaxMode,
		},
		{
			Name:     cookieNameRefresh,
			Value:    "",
			Path:     refreshCookiePath,
			Domain:   domain,
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   secure,
			SameSite: http.SameSiteStrictMode,
		},
		{
			Name:     cookieNameCSRF,
			Value:    "",
			Path:     "/",
			Domain:   domain,
			MaxAge:   -1,
			HttpOnly: false,
			Secure:   secure,
			SameSite: http.SameSiteLaxMode,
		},
	}
}
