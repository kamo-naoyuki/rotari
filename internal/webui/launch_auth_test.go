package webui

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWebLogoutCookiesAlwaysSecure(t *testing.T) {
	for _, test := range []struct {
		name, target, origin, forwardedProto string
	}{
		{"HTTP", "http://localhost", "http://localhost", ""},
		{"TLS", "https://localhost", "https://localhost", ""},
		{"TLS proxy", "http://localhost", "https://localhost", "https"},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := WithAuthToken(http.NotFoundHandler(), "secret")
			request := httptest.NewRequest(http.MethodPost, test.target+webLogoutPath, nil)
			request.Header.Set("X-Rotari-Token", "secret")
			request.Header.Set("Origin", test.origin)
			request.Header.Set("X-Forwarded-Proto", test.forwardedProto)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusSeeOther || recorder.Header().Get("Location") != webLoginPath {
				t.Fatalf("logout = %d, location = %q", recorder.Code, recorder.Header().Get("Location"))
			}
			cookies := recorder.Result().Cookies()
			if len(cookies) != 2 {
				t.Fatalf("logout cookies = %v, want both sessions", cookies)
			}
			paths := map[string]string{webSessionCookie: "/", launchAuthCookie: launchProxyPrefix}
			for _, cookie := range cookies {
				path, ok := paths[cookie.Name]
				if !ok || cookie.Path != path || cookie.Value != "" || cookie.MaxAge != -1 ||
					!cookie.Expires.Before(time.Now()) || !cookie.HttpOnly || !cookie.Secure ||
					cookie.SameSite != http.SameSiteStrictMode {
					t.Fatalf("unsafe logout cookie: %+v", cookie)
				}
				delete(paths, cookie.Name)
			}
			if len(paths) != 0 {
				t.Fatalf("logout did not clear cookies: %v", paths)
			}
		})
	}
}

func TestLaunchAuthenticationWithServiceAuthorization(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := WithAuthToken(next, "secret")
	request := httptest.NewRequest(http.MethodGet, "/_launch/notebook/lab", nil)
	request.Header.Set("Authorization", "Bearer secret")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("authenticated launch navigation set %d cookies, want 1", len(cookies))
	}

	cookie := cookies[0]
	if cookie.Path != launchProxyPrefix || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("unsafe launch cookie attributes: %+v", cookie)
	}
	for _, test := range []struct {
		name, path, authorization string
		cookie                    *http.Cookie
		want                      int
	}{
		{"jupyter API", "/_launch/notebook/api/contents", "token jupyter-secret", cookie, http.StatusNoContent},
		{"websocket", "/_launch/notebook/api/kernels/k/channels", "", cookie, http.StatusNoContent},
		{"no cookie", "/_launch/notebook/api/contents", "token jupyter-secret", nil, http.StatusUnauthorized},
		{"outside launches", "/api/state", "token jupyter-secret", cookie, http.StatusUnauthorized},
		{"tampered cookie", "/_launch/notebook/lab", "", &http.Cookie{Name: cookie.Name, Value: cookie.Value + "x"}, http.StatusUnauthorized},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			request.Header.Set("Authorization", test.authorization)
			if test.cookie != nil {
				request.AddCookie(test.cookie)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != test.want {
				t.Fatalf("status = %d, want %d", recorder.Code, test.want)
			}
		})
	}
	restarted := WithAuthToken(next, "secret")
	request = httptest.NewRequest(http.MethodGet, "/_launch/notebook/lab", nil)
	request.AddCookie(cookie)
	recorder = httptest.NewRecorder()
	restarted.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("cookie from previous server accepted: %d", recorder.Code)
	}
}

func TestLaunchAuthenticationCookieLifetimeAndTLS(t *testing.T) {
	auth := launchAuth{key: []byte("test-key")}
	now := time.Now()
	request := httptest.NewRequest(http.MethodGet, "https://localhost/_launch/notebook/lab", nil)
	cookie := auth.cookie(request, now)
	if !cookie.Secure || cookie.MaxAge != int(launchAuthLifetime.Seconds()) {
		t.Fatalf("unexpected cookie attributes: %+v", cookie)
	}
	request.AddCookie(cookie)
	if !auth.accepts(request, now) || auth.accepts(request, now.Add(launchAuthLifetime)) {
		t.Fatal("launch session must be valid only before its expiration")
	}
}

func TestLaunchCookieDoesNotBypassExecGuard(t *testing.T) {
	handler := WithAuthToken(WithExecGuard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), nil), "secret")
	request := httptest.NewRequest(http.MethodGet, "http://localhost/_launch/notebook/lab", nil)
	request.Header.Set("Authorization", "Bearer secret")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, want 1", len(cookies))
	}
	for _, test := range []struct {
		name, method, origin, upgrade string
		want                          int
	}{
		{"same origin POST", http.MethodPost, "http://localhost", "", http.StatusNoContent},
		{"cross origin POST", http.MethodPost, "http://other.example", "", http.StatusForbidden},
		{"missing origin POST", http.MethodPost, "", "", http.StatusForbidden},
		{"same origin websocket", http.MethodGet, "http://localhost", "websocket", http.StatusNoContent},
		{"cross origin websocket", http.MethodGet, "http://other.example", "websocket", http.StatusForbidden},
		{"missing origin websocket", http.MethodGet, "", "websocket", http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, "http://localhost/_launch/notebook/api/kernels", nil)
			request.AddCookie(cookies[0])
			request.Header.Set("Authorization", "token jupyter-secret")
			request.Header.Set("Origin", test.origin)
			request.Header.Set("Upgrade", test.upgrade)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != test.want {
				t.Fatalf("status = %d, want %d", recorder.Code, test.want)
			}
		})
	}
}
