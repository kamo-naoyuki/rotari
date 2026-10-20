package webui

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const launchAuthCookie = "rotari_launch_auth"
const launchAuthLifetime = 12 * time.Hour

// Launch pages need their own credential because an application's
// Authorization header replaces the browser's Basic authentication.
type launchAuth struct {
	key []byte
}

func (auth launchAuth) signature(value string) string {
	mac := hmac.New(sha256.New, auth.key)
	mac.Write([]byte(value))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (auth launchAuth) cookie(request *http.Request, now time.Time) *http.Cookie {
	expires := now.Add(launchAuthLifetime)
	value := strconv.FormatInt(expires.Unix(), 10)
	return &http.Cookie{
		Name:     launchAuthCookie,
		Value:    value + "." + auth.signature(value),
		Path:     launchProxyPrefix,
		Expires:  expires,
		MaxAge:   int(launchAuthLifetime.Seconds()),
		HttpOnly: true,
		Secure:   request.TLS != nil,
		SameSite: http.SameSiteStrictMode,
	}
}

func (auth launchAuth) accepts(request *http.Request, now time.Time) bool {
	cookie, err := request.Cookie(launchAuthCookie)
	if err != nil {
		return false
	}
	value, signature, ok := strings.Cut(cookie.Value, ".")
	if !ok || !hmac.Equal([]byte(signature), []byte(auth.signature(value))) {
		return false
	}
	expires, err := strconv.ParseInt(value, 10, 64)
	return err == nil && now.Unix() < expires
}
