package web

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

func TestTableViewCommandServesOnePath(t *testing.T) {
	covers(t, "WEB-20")
	e := support.NewEnv(t)
	data := filepath.Join(e.Root, "data")
	outside := filepath.Join(e.Root, "outside")
	for path, content := range map[string]string{
		filepath.Join(data, "list.csv"):     "audio\nwav/a.wav\n" + filepath.Join(outside, "b.wav") + "\n",
		filepath.Join(data, "wav", "a.wav"): "RIFF",
		filepath.Join(outside, "b.wav"):     "RIFF",
		filepath.Join(outside, "other.csv"): "x\n1\n",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeTableViewTree(t, data)

	// A file: its directory is the only root and the base of its paths.
	base := e.StartTableView(filepath.Join(data, "list.csv"))
	if r := e.HTTPGet(base + "/"); r.Status != 200 || !strings.Contains(r.Body, "table_view.js") {
		t.Fatalf("page = (%d, %.200s)", r.Status, r.Body)
	}
	for _, asset := range []string{"/table_view.js", "/table_view_panes.js", "/table_view.css", "/web_tokens.css", "/web_fonts.css"} {
		if r := e.HTTPGet(base + asset); r.Status != 200 {
			t.Fatalf("%s = %d", asset, r.Status)
		}
	}
	if r := e.HTTPGet(base + "/api/info"); !strings.Contains(r.Body, `"kind":"file"`) || !strings.Contains(r.Body, `"bases":[{"id":"file"`) {
		t.Fatalf("info = (%d, %s)", r.Status, r.Body)
	}
	r := e.HTTPGet(base + "/api/rows?sep=,")
	if !strings.Contains(r.Body, `"ref":"wav/a.wav"`) || !strings.Contains(r.Body, `"error":"Not shown: outside the allowed roots"`) {
		t.Fatalf("rows = (%d, %s)", r.Status, r.Body)
	}
	if r := e.HTTPGet(base + "/api/media?" + url.Values{"ref": {"wav/a.wav"}}.Encode()); r.Status != 200 || r.Body != "RIFF" {
		t.Fatalf("media = (%d, %q)", r.Status, r.Body)
	}
	for _, ref := range []string{filepath.Join(outside, "b.wav"), "../outside/b.wav"} {
		if r := e.HTTPGet(base + "/api/media?" + url.Values{"ref": {ref}}.Encode()); r.Status != 403 {
			t.Fatalf("%s = (%d, %q)", ref, r.Status, r.Body)
		}
	}

	// A directory, with a token required.
	base = e.StartTableView(data, "--auth-token", "secret-token")
	if r := e.HTTPGet(base + "/api/info"); r.Status != 401 {
		t.Fatalf("without the token = %d", r.Status)
	}
	if r := e.HTTPDo("GET", base+"/api/rows?pattern=**/*.wav&expanded=wav", map[string]string{"Authorization": "Bearer secret-token"}, ""); r.Status != 200 || !strings.Contains(r.Body, `"cells":["wav/a.wav"`) || strings.Contains(r.Body, "b.wav") {
		t.Fatalf("directory rows = (%d, %s)", r.Status, r.Body)
	}
	checkTableViewExpansion(t, func(route string, extra url.Values) support.HTTPResult {
		return e.HTTPDo("GET", base+"/api/"+route+"?"+extra.Encode(), map[string]string{"Authorization": "Bearer secret-token"}, "")
	})

	if r := e.Rotari("tableview", "--port", "0", filepath.Join(e.Root, "missing.csv")); r.Code != 1 || !strings.Contains(r.Stderr, "cannot view") {
		t.Fatalf("missing path = %s", r)
	}
	if r := e.Rotari("tableview", "--port", "0"); r.Code != 1 || !strings.Contains(r.Stderr, "usage: rotari tableview") {
		t.Fatalf("no path = %s", r)
	}
}
