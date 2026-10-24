package web

import (
	"encoding/base64"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rotari-dev/rotari/conformance/support"
)

// A basedir registered after rotari web started is listed by /api/basedirs
// (the Refresh button) and by a page load, and its mount is served, without
// restarting the server.
func TestWebListsBasedirsRegisteredAfterStart(t *testing.T) {
	covers(t, "WEB-12")
	e := support.NewEnv(t)
	e.MustRotari("add", "-p", "alpha", "--", "true")
	base := e.StartWeb()
	added := filepath.Join(e.Root, "added-state")
	id := base64.RawURLEncoding.EncodeToString([]byte(filepath.Clean(added)))
	if response := e.HTTPGet(base + "/api/basedirs"); response.Status != 200 || strings.Contains(response.Body, id) {
		t.Fatalf("/api/basedirs before registering: status %d: %s", response.Status, response.Body)
	}

	e.MustRotari("add", "-b", added, "-p", "beta", "--", "true")
	if response := e.HTTPGet(base + "/api/basedirs"); response.Status != 200 || !strings.Contains(response.Body, id) {
		t.Errorf("/api/basedirs after registering: status %d, want the new basedir listed: %s", response.Status, response.Body)
	}
	if response := e.HTTPGet(base + "/"); response.Status != 200 || !strings.Contains(response.Body, id) {
		t.Errorf("page load after registering: status %d, want the new basedir in its sidebar data", response.Status)
	}
	if response := e.HTTPGet(base + "/_basedir/" + id + "/api/state"); response.Status != 200 || !strings.Contains(response.Body, `"beta"`) {
		t.Errorf("new basedir mount: status %d, want its projects: %s", response.Status, response.Body)
	}
}
