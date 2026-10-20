package webui

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// loopbackHostNames are always accepted as Host while launching is enabled,
// with any port: SSH forwarding can use a different local port, and DNS
// rebinding needs an attacker's name, not these.
var loopbackHostNames = []string{"localhost", "127.0.0.1", "::1"}

// WithExecGuard protects a server that can launch commands from requests a
// browser makes on behalf of another site:
//
//   - Host must be a loopback name or one of allowedHosts (DNS rebinding).
//   - A request that is not GET or HEAD must carry an Origin equal to its own
//     scheme and Host, or, without Origin, an X-Rotari-Token or bearer
//     Authorization header, which a browser never adds by itself. A missing
//     Origin is never taken as proof that the request is same-origin.
//
// Authentication itself is WithAuthToken's job.
func WithExecGuard(next http.Handler, allowedHosts []string) http.Handler {
	allowed := map[string]bool{}
	for _, name := range append(append([]string{}, loopbackHostNames...), allowedHosts...) {
		allowed[strings.ToLower(strings.Trim(name, "[]"))] = true
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		host := requestHostName(request.Host)
		if !allowed[host] {
			http.Error(writer, "Host "+request.Host+" is not allowed while launching is enabled; open the server as localhost, or restart rotari web with --allowed-host "+host, http.StatusMisdirectedRequest)
			return
		}
		if request.Method != http.MethodGet && request.Method != http.MethodHead {
			if message := crossOriginRejection(request); message != "" {
				http.Error(writer, message, http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(writer, request)
	})
}

// requestHostName is Host without its port, lowercased and unbracketed.
func requestHostName(hostPort string) string {
	host, _, err := net.SplitHostPort(hostPort)
	if err != nil {
		host = hostPort
	}
	return strings.ToLower(strings.Trim(host, "[]"))
}

func crossOriginRejection(request *http.Request) string {
	origin := request.Header.Get("Origin")
	if origin == "" {
		if request.Header.Get("X-Rotari-Token") != "" || strings.HasPrefix(request.Header.Get("Authorization"), authBearerPrefix) {
			return ""
		}
		return "a request without Origin must authenticate with X-Rotari-Token or Authorization: Bearer while launching is enabled"
	}
	parsed, err := url.Parse(origin)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || !strings.EqualFold(parsed.Host, request.Host) {
		return "cross-origin request from " + origin + " rejected while launching is enabled"
	}
	return ""
}
