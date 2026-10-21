package webui

import (
	_ "embed"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/kamo-naoyuki/rotari/internal/tableview"
)

// The Table viewer shows a delimited file as a paged table. rotari web
// offers it under Launch with for an artifact and serves it under
// /tableview/. The page's own query names the source, for an artifact as a
// content request does; the page passes it through to its API requests and
// adds its own parameters (sep, header, offset, limit).

//go:embed assets/table_view.html
var tableViewHTML string

//go:embed assets/table_view.js
var tableViewJS string

//go:embed assets/table_view.css
var tableViewCSS string

// tableViewSource is what a viewer request names.
type tableViewSource struct {
	target    tableview.Target
	directory bool
	// name titles the page and trail places it, outermost first.
	name  string
	trail []string
	// download links the file's download, relative to the page; "" offers
	// none.
	download string
}

// tableView serves the viewer for the sources resolve finds.
type tableView struct {
	resolve func(url.Values) (tableViewSource, error)
	// stylesheets are linked before the viewer's own styles: the fonts and
	// design tokens the rest of the Web UI uses.
	stylesheets []string
}

func (view tableView) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(writer http.ResponseWriter, request *http.Request) {
		if _, err := view.resolve(request.URL.Query()); err != nil {
			writeArtifactError(writer, err)
			return
		}
		writer.Header().Set(headerContentType, "text/html; charset=utf-8")
		_, _ = writer.Write([]byte(view.page()))
	})
	mux.HandleFunc("GET /table_view.js", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set(headerContentType, "text/javascript; charset=utf-8")
		_, _ = writer.Write([]byte(tableViewJS))
	})
	mux.HandleFunc("GET /table_view.css", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set(headerContentType, "text/css; charset=utf-8")
		_, _ = writer.Write([]byte(tableViewCSS))
	})
	mux.HandleFunc("GET /api/info", func(writer http.ResponseWriter, request *http.Request) {
		source, err := view.resolve(request.URL.Query())
		if err != nil {
			writeArtifactError(writer, err)
			return
		}
		info, err := tableViewInfoOf(source)
		if err != nil {
			writeArtifactError(writer, err)
			return
		}
		noStore(writer)
		writeWebJSON(writer, info)
	})
	mux.HandleFunc("GET /api/rows", func(writer http.ResponseWriter, request *http.Request) {
		query := request.URL.Query()
		source, err := view.resolve(query)
		if err != nil {
			writeArtifactError(writer, err)
			return
		}
		table, err := openTableView(source, query)
		if err != nil {
			writeArtifactError(writer, err)
			return
		}
		page, err := table.Page(tableview.Query{Offset: intParam(query, "offset"), Limit: intParam(query, "limit")})
		if err != nil {
			writeArtifactError(writer, err)
			return
		}
		noStore(writer)
		writeWebJSON(writer, page)
	})
	return mux
}

func (view tableView) page() string {
	var links strings.Builder
	for _, href := range view.stylesheets {
		links.WriteString(`<link rel="stylesheet" href="` + html.EscapeString(href) + `" />`)
	}
	return strings.Replace(tableViewHTML, "__ROTARI_STYLESHEETS__", links.String(), 1)
}

// tableViewInfo is the /api/info response: what the page shows before any
// rows.
type tableViewInfo struct {
	Name     string   `json:"name"`
	Trail    []string `json:"trail,omitempty"`
	Kind     string   `json:"kind"`
	Size     int64    `json:"size"`
	Download string   `json:"download,omitempty"`
	// Separator is the one a file's extension suggests.
	Separator string           `json:"separator,omitempty"`
	Bases     []tableview.Base `json:"bases"`
}

func tableViewInfoOf(source tableViewSource) (tableViewInfo, error) {
	info, err := source.target.Stat()
	if err != nil {
		return tableViewInfo{}, err
	}
	view := tableViewInfo{Name: source.name, Trail: source.trail, Kind: "file", Size: info.Size(),
		Download: source.download, Bases: source.target.AllBases(source.directory)}
	if source.directory {
		view.Kind, view.Size = "directory", 0
		return view, nil
	}
	view.Separator = suggestedSeparator(source.target.Path)
	return view, nil
}

// suggestedSeparator is the separator a delimited file's extension implies.
func suggestedSeparator(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".tsv", ".tab":
		return "\t"
	case ".psv":
		return "|"
	case ".scp", ".lst", ".list":
		return " "
	}
	return ","
}

// openTableView reads a source as its query asks.
func openTableView(source tableViewSource, query url.Values) (*tableview.Table, error) {
	if source.directory {
		return nil, fmt.Errorf("%s is a directory; the viewer reads delimited files", source.name)
	}
	separator, size := utf8.DecodeRuneInString(query.Get("sep"))
	if size == 0 || size != len(query.Get("sep")) {
		return nil, fmt.Errorf("sep must be one character")
	}
	header := true
	switch query.Get("header") {
	case "", "1":
	case "0":
		header = false
	default:
		return nil, fmt.Errorf("header must be 1 or 0")
	}
	return tableview.OpenFile(source.target, tableview.Format{Separator: separator, Header: header})
}

// intParam is an integer query parameter; a malformed one is -1, which
// every caller rejects.
func intParam(query url.Values, name string) int {
	text := query.Get(name)
	if text == "" {
		return 0
	}
	value, err := strconv.Atoi(text)
	if err != nil {
		return -1
	}
	return value
}

// artifactTableView resolves a viewer request for an artifact: the query
// names it as a content request does. Media paths in its cells resolve
// from the basedir by default, or the job's working directory.
func (s site) artifactTableView(baseDir string, query url.Values) (tableViewSource, error) {
	resolved, err := s.resolveArtifactRequest(baseDir, query)
	if err != nil {
		return tableViewSource{}, err
	}
	target := tableview.Target{Path: filepath.Join(resolved.root, resolved.relative), Roots: resolved.roots}
	if absolute, err := filepath.Abs(baseDir); err == nil {
		target.Bases = append(target.Bases, tableview.Base{ID: "basedir", Label: "basedir", Path: absolute})
	}
	if filepath.IsAbs(resolved.listing.WorkingDirectory) {
		target.Bases = append(target.Bases, tableview.Base{ID: "wd", Label: "job's working directory", Path: resolved.listing.WorkingDirectory})
	}
	info, err := target.Stat()
	if err != nil {
		return tableViewSource{}, err
	}
	name := resolved.entry.DisplayPath
	if child := query.Get("child"); child != "" {
		name = strings.TrimSuffix(name, "/") + "/" + child
	}
	source := tableViewSource{target: target, directory: info.IsDir(), name: name,
		trail: []string{query.Get("project_name"), query.Get("run_id"), query.Get("job_id")}}
	if !source.directory {
		download := url.Values{"download": {"1"}}
		for _, key := range []string{"project_name", "run_id", "job_id", "attempt_id", "entry", "child"} {
			if value := query.Get(key); value != "" {
				download.Set(key, value)
			}
		}
		source.download = "../api/artifact-file?" + download.Encode()
	}
	return source, nil
}
