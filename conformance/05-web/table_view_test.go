package web

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

func TestWebTableViewerReadsArtifacts(t *testing.T) {
	covers(t, "WEB-19")
	e := support.NewEnv(t)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.tsv"), []byte("secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// a.wav is one second of 16 kHz mono 16-bit silence, beside a table
	// that names it and a file outside every root.
	header := []byte("RIFF\x24\x7d\x00\x00WAVEfmt \x10\x00\x00\x00\x01\x00\x01\x00\x80\x3e\x00\x00\x00\x7d\x00\x00\x02\x00\x10\x00data\x00\x7d\x00\x00")
	if err := os.WriteFile(filepath.Join(e.Root, "a.wav"), append(header, make([]byte, 32000)...), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.wav"), header, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.Root, "media.csv"), []byte("audio\na.wav\n"+filepath.Join(outside, "secret.wav")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	script := `printf 'id\tscore\n' > data.tsv; for n in $(seq 1 300); do printf '%d\t"a\tb"\n' "$n" >> data.tsv; done; ln -s "$1" link.tsv`
	jobID := support.AddedJobID(t, e.MustRotari("add", "-p", "tv", "--", "sh", "-c", script, "sh",
		filepath.Join(outside, "secret.tsv"), "data.tsv", "link.tsv", "media.csv"))
	e.MustRotari("run", "-p", "tv", "--quiet")
	var shown struct {
		RunID string `json:"run_id"`
		Jobs  []struct {
			Artifacts struct {
				Entries []struct {
					DisplayPath string `json:"display_path"`
				} `json:"entries"`
			} `json:"artifacts"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal([]byte(e.MustRotari("show", "-p", "tv", "-j", jobID, "--json").Stdout), &shown); err != nil {
		t.Fatal(err)
	}
	var run struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal([]byte(e.MustRotari("show", "-p", "tv", "--json").Stdout), &run); err != nil {
		t.Fatal(err)
	}
	entries := map[string]int{}
	for index, entry := range shown.Jobs[0].Artifacts.Entries {
		entries[filepath.Base(entry.DisplayPath)] = index
	}
	base := e.StartWeb()
	get := func(route, name string, extra url.Values) support.HTTPResult {
		query := url.Values{"project_name": {"tv"}, "run_id": {run.RunID}, "job_id": {jobID}, "entry": {strconv.Itoa(entries[name])}}
		for key, values := range extra {
			query[key] = values
		}
		return e.HTTPGet(base + route + "?" + query.Encode())
	}

	// Launch with lists the viewer for a table file without --allow-exec.
	var launchers struct {
		Available bool `json:"available"`
		Viewers   []struct {
			Name string `json:"name"`
			Path string `json:"path"`
		} `json:"viewers"`
	}
	r := e.HTTPGet(base + "/api/launchers?kind=file&name=data.tsv")
	if err := json.Unmarshal([]byte(r.Body), &launchers); err != nil || launchers.Available || len(launchers.Viewers) != 1 || launchers.Viewers[0].Path != "/tableview/" {
		t.Fatalf("launchers = (%d, %s)", r.Status, r.Body)
	}
	if r := e.HTTPGet(base + "/api/launchers?kind=directory&name=out"); !strings.Contains(r.Body, `"viewers":[{"name":"Table viewer"`) {
		t.Fatalf("a directory was not offered the viewer: %s", r.Body)
	}
	if r := e.HTTPGet(base + "/api/launchers?kind=file&name=run.log"); strings.Contains(r.Body, "viewers") {
		t.Fatalf("a log was offered the viewer: %s", r.Body)
	}

	if r := get("/tableview/", "data.tsv", nil); r.Status != 200 || !strings.Contains(r.Body, "table_view.js") {
		t.Fatalf("page = (%d, %.200s)", r.Status, r.Body)
	}
	if r := get("/tableview/api/info", "data.tsv", nil); r.Status != 200 || !strings.Contains(r.Body, `"separator":"\t"`) {
		t.Fatalf("info = (%d, %s)", r.Status, r.Body)
	}
	var page struct {
		Columns []struct {
			Name string `json:"name"`
		} `json:"columns"`
		Rows []struct {
			Number int      `json:"number"`
			Cells  []string `json:"cells"`
		} `json:"rows"`
		Total int `json:"total"`
	}
	r = get("/tableview/api/rows", "data.tsv", url.Values{"sep": {"\t"}, "offset": {"290"}, "limit": {"20"}})
	if err := json.Unmarshal([]byte(r.Body), &page); err != nil {
		t.Fatalf("rows = (%d, %s)", r.Status, r.Body)
	}
	if page.Total != 300 || len(page.Columns) != 2 || page.Columns[1].Name != "score" || len(page.Rows) != 10 ||
		page.Rows[0].Number != 291 || !reflect.DeepEqual(page.Rows[0].Cells, []string{"291", "a\tb"}) {
		t.Fatalf("rows = %+v", page)
	}
	if r := get("/tableview/api/rows", "data.tsv", url.Values{"sep": {"\t"}, "header": {"0"}, "limit": {"1"}}); !strings.Contains(r.Body, `"cells":["id","score"]`) {
		t.Fatalf("headerless rows = (%d, %s)", r.Status, r.Body)
	}
	// Filters, a sort, and Describe cover the whole file, not one page.
	filters := `[{"column":0,"op":">","value":"295"}]`
	r = get("/tableview/api/rows", "data.tsv", url.Values{"sep": {"\t"}, "filters": {filters}, "sort": {"0"}, "desc": {"1"}, "limit": {"2"}})
	if !strings.Contains(r.Body, `"matched":5`) || !strings.Contains(r.Body, `"number":300,"cells":["300"`) || !strings.Contains(r.Body, `"number":299`) {
		t.Fatalf("filtered and sorted rows = (%d, %s)", r.Status, r.Body)
	}
	r = get("/tableview/api/describe", "data.tsv", url.Values{"sep": {"\t"}, "filters": {filters}})
	if !strings.Contains(r.Body, `"count":5,"unique":5,"numeric":5,"mean":298`) || !strings.Contains(r.Body, `"sum":1490`) || !strings.Contains(r.Body, `"matched":5,"total":300`) {
		t.Fatalf("describe = (%d, %s)", r.Status, r.Body)
	}
	// A cell naming a media file under the roots gets its properties and
	// the file is served; one outside them gets neither.
	r = get("/tableview/api/rows", "media.csv", url.Values{"sep": {","}, "base": {"wd"}})
	if !strings.Contains(r.Body, `"format":"WAV PCM","size":32044,"duration":1,"sample_rate":16000,"channels":1,"bit_depth":16,"bitrate":256000`) ||
		!strings.Contains(r.Body, `"error":"Not shown: outside the allowed roots"`) {
		t.Fatalf("media rows = (%d, %s)", r.Status, r.Body)
	}
	if r := get("/tableview/api/media", "media.csv", url.Values{"ref": {"a.wav"}, "base": {"wd"}}); r.Status != 200 || len(r.Body) != 32044 {
		t.Fatalf("media = (%d, %d bytes)", r.Status, len(r.Body))
	}
	for _, ref := range []string{filepath.Join(outside, "secret.wav"), "media.csv", "data.tsv"} {
		if r := get("/tableview/api/media", "media.csv", url.Values{"ref": {ref}, "base": {"wd"}}); r.Status == 200 {
			t.Fatalf("%s was served as media", ref)
		}
	}
	// The working directory lists its files, with media properties, and as
	// stems grouped by extension when it has no subdirectories in the
	// pattern.
	wd := func(route string, extra url.Values) support.HTTPResult {
		query := url.Values{"project_name": {"tv"}, "run_id": {run.RunID}, "job_id": {jobID}, "entry": {"wd"}}
		for key, values := range extra {
			query[key] = values
		}
		return e.HTTPGet(base + route + "?" + query.Encode())
	}
	if r := wd("/tableview/api/rows", url.Values{"pattern": {"*.wav"}}); !strings.Contains(r.Body, `"cells":["a.wav","WAV PCM","1","16000","1","256000","","","32044"`) {
		t.Fatalf("working directory files = (%d, %s)", r.Status, r.Body)
	}
	if r := wd("/tableview/api/rows", url.Values{"view": {"stems"}, "pattern": {"{a.wav,data.tsv,link.tsv}"}}); !strings.Contains(r.Body, `"columns":[{"name":"stem"},{"name":".tsv","type":"path"},{"name":".wav","type":"path"}]`) ||
		!strings.Contains(r.Body, `"cells":["a","","a.wav"]`) || !strings.Contains(r.Body, `"cells":["data","data.tsv",""]`) {
		t.Fatalf("working directory stems = (%d, %s)", r.Status, r.Body)
	}
	if r := wd("/tableview/api/rows", url.Values{"sep": {","}}); r.Status == 200 {
		t.Fatalf("a file's separator was accepted for a directory: %s", r.Body)
	}
	for _, refused := range []struct {
		name  string
		extra url.Values
	}{
		{"secret.tsv", url.Values{"sep": {"\t"}}},                                                      // outside every allowed root
		{"link.tsv", url.Values{"sep": {"\t"}}},                                                        // a symlink leaving the root
		{"data.tsv", url.Values{"sep": {"\t,"}}},                                                       // not one character
		{"data.tsv", url.Values{"sep": {"\t"}, "filters": {`[{"column":0,"op":">","value":"high"}]`}}}, // an ordering needs a number
	} {
		if r := get("/tableview/api/rows", refused.name, refused.extra); r.Status == 200 {
			t.Fatalf("%s %v was read: %s", refused.name, refused.extra, r.Body)
		}
	}
}
