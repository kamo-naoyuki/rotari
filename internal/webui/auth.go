package webui

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"html"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	webLoginPath       = "/_auth/login"
	webLogoutPath      = "/_auth/logout"
	webSessionCookie   = "rotari_web_session"
	webSessionLifetime = 12 * time.Hour
)

type webSessions struct {
	mu       sync.Mutex
	expiries map[string]time.Time
}

func (sessions *webSessions) issue(writer http.ResponseWriter) bool {
	var id [32]byte
	if _, err := rand.Read(id[:]); err != nil {
		http.Error(writer, "could not create login session", http.StatusInternalServerError)
		return false
	}
	now := time.Now()
	expires := now.Add(webSessionLifetime)
	value := hex.EncodeToString(id[:])

	sessions.mu.Lock()
	for session, expiry := range sessions.expiries {
		if !now.Before(expiry) {
			delete(sessions.expiries, session)
		}
	}
	if sessions.expiries == nil {
		sessions.expiries = make(map[string]time.Time)
	}
	sessions.expiries[value] = expires
	sessions.mu.Unlock()

	http.SetCookie(writer, &http.Cookie{
		Name:     webSessionCookie,
		Value:    value,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(webSessionLifetime.Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
	return true
}

func (sessions *webSessions) accepts(request *http.Request, now time.Time) bool {
	cookie, err := request.Cookie(webSessionCookie)
	if err != nil {
		return false
	}
	sessions.mu.Lock()
	defer sessions.mu.Unlock()
	expires, ok := sessions.expiries[cookie.Value]
	if !ok {
		return false
	}
	if !now.Before(expires) {
		delete(sessions.expiries, cookie.Value)
		return false
	}
	return true
}

func (sessions *webSessions) revoke(request *http.Request) {
	cookie, err := request.Cookie(webSessionCookie)
	if err != nil {
		return
	}
	sessions.mu.Lock()
	delete(sessions.expiries, cookie.Value)
	sessions.mu.Unlock()
}

// WithAuthToken protects the Web UI with an interactive login session and
// accepts bearer or X-Rotari-Token credentials for API clients.
func WithAuthToken(next http.Handler, token string) http.Handler {
	sessions := &webSessions{}
	launchSession := launchAuth{key: []byte(rand.Text())}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == webLoginPath {
			handleWebLogin(writer, request, token, sessions)
			return
		}

		provided := request.Header.Get("X-Rotari-Token")
		if provided == "" && strings.HasPrefix(request.Header.Get("Authorization"), authBearerPrefix) {
			provided = strings.TrimPrefix(request.Header.Get("Authorization"), authBearerPrefix)
		}
		tokenAuthenticated := len(provided) == len(token) &&
			subtle.ConstantTimeCompare([]byte(provided), []byte(token)) == 1
		sessionAuthenticated := isSecureAuthRequest(request) && sessions.accepts(request, time.Now())
		launchRequest := strings.HasPrefix(request.URL.Path, launchProxyPrefix)
		launchSessionAuthenticated := launchRequest && launchSession.accepts(request, time.Now())
		authenticated := tokenAuthenticated || sessionAuthenticated || launchSessionAuthenticated

		if request.URL.Path == webLogoutPath {
			if !authenticated {
				http.Error(writer, "authentication required", http.StatusUnauthorized)
				return
			}
			handleWebLogout(writer, request, sessions)
			return
		}
		if !authenticated {
			if isHTMLNavigation(request) {
				nextPath := request.URL.RequestURI()
				noCacheAuthResponse(writer)
				http.Redirect(writer, request, webLoginPath+"?next="+url.QueryEscape(nextPath), http.StatusSeeOther)
				return
			}
			noCacheAuthResponse(writer)
			http.Error(writer, "authentication required", http.StatusUnauthorized)
			return
		}
		if (sessionAuthenticated || launchSessionAuthenticated) && requiresOrigin(request) &&
			!sameOriginFormPost(request) {
			http.Error(writer, "cross-origin request rejected", http.StatusForbidden)
			return
		}
		if launchRequest && tokenAuthenticated && !launchSessionAuthenticated {
			http.SetCookie(writer, launchSession.cookie(request, time.Now()))
		}
		next.ServeHTTP(writer, request)
	})
}

func handleWebLogin(writer http.ResponseWriter, request *http.Request, token string, sessions *webSessions) {
	noCacheAuthResponse(writer)
	if !isSecureAuthRequest(request) {
		http.Error(writer, "browser login requires HTTPS", http.StatusForbidden)
		return
	}
	nextPath := safeWebReturnPath(request.URL.Query().Get("next"))
	switch request.Method {
	case http.MethodGet, http.MethodHead:
		if sessions.accepts(request, time.Now()) {
			http.Redirect(writer, request, nextPath, http.StatusSeeOther)
			return
		}
		serveWebLoginPage(writer, nextPath, "")
	case http.MethodPost:
		if !sameOriginFormPost(request) {
			http.Error(writer, "login request must be same-origin", http.StatusForbidden)
			return
		}
		request.Body = http.MaxBytesReader(writer, request.Body, 8*1024)
		if err := request.ParseForm(); err != nil {
			serveWebLoginPageStatus(writer, nextPath, "Sign-in failed. Check your user name and token.", http.StatusBadRequest)
			return
		}
		nextPath = safeWebReturnPath(request.Form.Get("next"))
		username := request.Form.Get("username")
		password := request.Form.Get("password")
		validPassword := len(password) == len(token) &&
			subtle.ConstantTimeCompare([]byte(password), []byte(token)) == 1
		if username != "rotari" || !validPassword {
			serveWebLoginPageStatus(writer, nextPath, "Sign-in failed. Check your user name and token.", http.StatusUnauthorized)
			return
		}
		if !sessions.issue(writer) {
			return
		}
		http.Redirect(writer, request, nextPath, http.StatusSeeOther)
	default:
		writer.Header().Set("Allow", "GET, HEAD, POST")
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleWebLogout(writer http.ResponseWriter, request *http.Request, sessions *webSessions) {
	noCacheAuthResponse(writer)
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !sameOriginFormPost(request) {
		http.Error(writer, "logout request must be same-origin", http.StatusForbidden)
		return
	}
	sessions.revoke(request)
	http.SetCookie(writer, &http.Cookie{
		Name:     webSessionCookie,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(1, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
	http.SetCookie(writer, &http.Cookie{
		Name:     launchAuthCookie,
		Value:    "",
		Path:     launchProxyPrefix,
		Expires:  time.Unix(1, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
	http.Redirect(writer, request, webLoginPath, http.StatusSeeOther)
}

func sameOriginFormPost(request *http.Request) bool {
	value := request.Header.Get("Origin")
	if value == "" {
		value = request.Header.Get("Referer")
	}
	origin, err := url.Parse(value)
	if err != nil || origin.Host == "" || !strings.EqualFold(origin.Host, request.Host) {
		return false
	}
	if origin.User != nil || (origin.Scheme != "http" && origin.Scheme != "https") {
		return false
	}
	scheme := "http"
	if isSecureAuthRequest(request) {
		scheme = "https"
	}
	return origin.Scheme == scheme
}

func isSecureAuthRequest(request *http.Request) bool {
	return request.TLS != nil || strings.EqualFold(strings.TrimSpace(request.Header.Get("X-Forwarded-Proto")), "https")
}

func safeWebReturnPath(value string) string {
	parsed, err := url.Parse(value)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || !strings.HasPrefix(parsed.Path, "/") ||
		strings.HasPrefix(parsed.Path, "//") || strings.Contains(parsed.Path, "\\") {
		return "/"
	}
	return parsed.RequestURI()
}

func isHTMLNavigation(request *http.Request) bool {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		return false
	}
	path := request.URL.Path
	if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/fonts/") ||
		strings.HasSuffix(path, ".css") || strings.HasSuffix(path, ".js") ||
		strings.HasSuffix(path, ".svg") || strings.HasSuffix(path, ".woff2") {
		return false
	}
	return strings.Contains(request.Header.Get("Accept"), "text/html")
}

func requiresOrigin(request *http.Request) bool {
	return (request.Method != http.MethodGet && request.Method != http.MethodHead) || request.Header.Get("Upgrade") != ""
}

func noCacheAuthResponse(writer http.ResponseWriter) {
	writer.Header().Set("Cache-Control", "no-store")
}

func serveWebLoginPage(writer http.ResponseWriter, nextPath, message string) {
	serveWebLoginPageStatus(writer, nextPath, message, http.StatusOK)
}

func serveWebLoginPageStatus(writer http.ResponseWriter, nextPath, message string, status int) {
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.Header().Set("Referrer-Policy", "same-origin")
	noCacheAuthResponse(writer)
	writer.WriteHeader(status)
	page := strings.NewReplacer(
		"__NEXT__", html.EscapeString(nextPath),
		"__MESSAGE__", html.EscapeString(message),
	).Replace(webLoginPageHTML)
	_, _ = io.WriteString(writer, page)
}

const webLoginPageHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <meta name="referrer" content="same-origin">
  <title>Sign in · rotari</title>
  <style>
    :root { color-scheme: light dark; font: 16px system-ui, sans-serif; }
    body { min-height: 100vh; margin: 0; display: grid; place-items: center; background: Canvas; color: CanvasText; }
    main { width: min(22rem, calc(100vw - 3rem)); padding: 2rem; border: 1px solid color-mix(in srgb, CanvasText 18%, transparent); border-radius: 12px; }
    h1 { margin: 0 0 .4rem; font-size: 1.5rem; }
    p { color: GrayText; line-height: 1.5; }
    label { display: block; margin-top: 1rem; font-weight: 600; }
    input { box-sizing: border-box; width: 100%; margin-top: .35rem; padding: .65rem; border: 1px solid GrayText; border-radius: 6px; background: Canvas; color: CanvasText; font: inherit; }
    button { width: 100%; margin-top: 1.25rem; padding: .7rem; border: 0; border-radius: 6px; background: #c7423a; color: white; font: inherit; font-weight: 650; cursor: pointer; }
    .error { color: #c7423a; }
  </style>
</head>
<body>
  <main>
    <h1>Sign in to rotari</h1>
    <p>Enter the Web token to continue. The user name is <strong>rotari</strong>.</p>
    <p class="error" role="alert">__MESSAGE__</p>
    <form method="post" action="/_auth/login">
      <input type="hidden" name="next" value="__NEXT__">
      <label for="username">User name</label>
      <input id="username" name="username" value="rotari" autocomplete="username" required>
      <label for="password">Token</label>
      <input id="password" type="password" name="password" autocomplete="current-password" required autofocus>
      <button type="submit">Sign in</button>
    </form>
  </main>
</body>
</html>`
