package webui

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rotari-dev/rotari/internal/launcher"
)

// launchProxyPrefix is where ready services are served, one path per
// launch. It sits outside every basedir mount: launches belong to the
// server, not to a basedir.
const launchProxyPrefix = "/_launch/"

// LaunchProxyPath is the URL path a service launch is served under; the
// service receives it as ROTARI_LAUNCHER_BASE_PATH.
func LaunchProxyPath(id string) string {
	return strings.TrimSuffix(launchProxyPrefix, "/") + "/" + id
}

// launchTransports caches one transport per ready service, so connections
// are reused while every new one is checked.
type launchTransports struct {
	mu       sync.Mutex
	byLaunch map[string]*launchTransport
}

type launchTransport struct {
	port, pgid int
	transport  *http.Transport
}

// newLaunchTransport connects only to the service's loopback port, and
// before using a new connection checks that the process listening there
// still belongs to the launch: a port the service gave up can be taken by
// another process between the service exiting and the manager noticing.
// It never uses an HTTP proxy.
func newLaunchTransport(port, pgid int) *http.Transport {
	address := net.JoinHostPort(launcher.LoopbackHost, strconv.Itoa(port))
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	return &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			conn, err := dialer.DialContext(ctx, "tcp", address)
			if err != nil {
				return nil, err
			}
			if err := launcher.VerifyListener(port, pgid); err != nil {
				conn.Close()
				return nil, err
			}
			return conn, nil
		},
		MaxIdleConnsPerHost: 8,
		IdleConnTimeout:     30 * time.Second,
	}
}

func (cache *launchTransports) get(id string, port, pgid int) *http.Transport {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.byLaunch == nil {
		cache.byLaunch = map[string]*launchTransport{}
	}
	if cached := cache.byLaunch[id]; cached != nil && cached.port == port && cached.pgid == pgid {
		return cached.transport
	}
	if cached := cache.byLaunch[id]; cached != nil {
		cached.transport.CloseIdleConnections()
	}
	transport := newLaunchTransport(port, pgid)
	cache.byLaunch[id] = &launchTransport{port: port, pgid: pgid, transport: transport}
	return transport
}

func (cache *launchTransports) drop(id string) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cached := cache.byLaunch[id]; cached != nil {
		cached.transport.CloseIdleConnections()
		delete(cache.byLaunch, id)
	}
}

// serveLaunchProxy forwards /_launch/ID/... unchanged to that ready
// service. The path keeps its prefix, which the service was told to serve
// under. The Web token never reaches the service.
func (s site) serveLaunchProxy(writer http.ResponseWriter, request *http.Request) {
	if s.launchesUnavailable(writer) {
		return
	}
	rest := strings.TrimPrefix(request.URL.Path, launchProxyPrefix)
	id, _, hasSlash := strings.Cut(rest, "/")
	if !hasSlash {
		target := LaunchProxyPath(id) + "/"
		if request.URL.RawQuery != "" {
			target += "?" + request.URL.RawQuery
		}
		http.Redirect(writer, request, target, http.StatusTemporaryRedirect)
		return
	}
	upstreamService, err := s.Launches.Manager.ServiceUpstream(id)
	if err != nil {
		s.Launches.transports.drop(id)
		status := http.StatusServiceUnavailable
		if errors.Is(err, launcher.ErrUnknownLaunch) {
			status = http.StatusNotFound
		}
		http.Error(writer, err.Error(), status)
		return
	}
	upstream := &url.URL{Scheme: "http", Host: net.JoinHostPort(launcher.LoopbackHost, strconv.Itoa(upstreamService.Port))}
	proxy := &httputil.ReverseProxy{
		Rewrite: func(out *httputil.ProxyRequest) {
			out.SetURL(upstream)
			out.Out.Header.Del("Authorization")
			out.Out.Header.Del("X-Rotari-Token")
			// Keep application cookies, but never forward Rotari's sessions.
			out.Out.Header.Del("Cookie")
			for _, cookie := range out.In.Cookies() {
				if cookie.Name != launchAuthCookie && cookie.Name != webSessionCookie {
					out.Out.AddCookie(cookie)
				}
			}
			// The service's own secret, which only this proxy presents.
			for name, value := range upstreamService.Headers {
				out.Out.Header.Set(name, value)
			}
		},
		Transport:     s.Launches.transports.get(id, upstreamService.Port, upstreamService.PGID),
		FlushInterval: -1,
		ErrorHandler: func(writer http.ResponseWriter, _ *http.Request, err error) {
			http.Error(writer, "the launched service did not answer: "+err.Error(), http.StatusBadGateway)
		},
	}
	proxy.ServeHTTP(writer, request)
}
