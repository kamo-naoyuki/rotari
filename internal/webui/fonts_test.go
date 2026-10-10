package webui

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/joblist"
	"github.com/kamo-naoyuki/rotari/internal/model"
	stateinternal "github.com/kamo-naoyuki/rotari/internal/state"
)

var fontURL = regexp.MustCompile(`url\("([^"]+)"\)`)

func TestLiveServerServesEmbeddedFonts(t *testing.T) {
	handler := Handler(testOptions(t.TempDir(), false))
	get := func(target string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
		return response
	}
	css := get("/web_fonts.css")
	if css.Code != http.StatusOK || css.Header().Get("Content-Type") != "text/css; charset=utf-8" {
		t.Fatalf("GET /web_fonts.css = %d (%q)", css.Code, css.Header().Get("Content-Type"))
	}
	urls := fontURL.FindAllStringSubmatch(css.Body.String(), -1)
	if len(urls) != len(webFonts) {
		t.Fatalf("web_fonts.css declares %d fonts, want %d", len(urls), len(webFonts))
	}
	for _, match := range urls {
		response := get(match[1])
		want, err := webFontFiles.ReadFile(path.Join("assets/fonts", path.Base(match[1])))
		if err != nil {
			t.Fatal(err)
		}
		if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "font/woff2" || !bytes.Equal(response.Body.Bytes(), want) {
			t.Fatalf("GET %s = %d (%q), want the embedded font", match[1], response.Code, response.Header().Get("Content-Type"))
		}
	}
	if license := get("/fonts/LICENSE.txt"); license.Code != http.StatusOK || !strings.Contains(license.Body.String(), "SIL Open Font License") {
		t.Fatalf("GET /fonts/LICENSE.txt = %d, want the font licence", license.Code)
	}
	for _, target := range []string{"/fonts/", "/fonts/missing.woff2", "/fonts/../web_styles.css", "/fonts/README.md"} {
		if response := get(target); response.Code != http.StatusNotFound && response.Code != http.StatusMovedPermanently {
			t.Errorf("GET %s = %d, want only the embedded fonts served", target, response.Code)
		}
	}
	if !strings.Contains(composeWebHTML(nil, true, ""), `<link rel="stylesheet" href="/web_fonts.css" />`) {
		t.Error("web page does not link the font stylesheet")
	}
	jobs := jobsHTML("/", nil, nil, joblist.DefaultSinceText, true, true)
	if !strings.Contains(jobs, `src: url("/fonts/IBMPlexSans-Regular-Latin1.woff2")`) {
		t.Error("Job activity page does not declare the fonts")
	}
}

// Each static page's web_fonts.css must reach the single fonts/ directory at
// the export root, however deep the page is.
func TestStaticExportWritesFontsOnce(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := stateinternal.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := stateinternal.WriteJSON(paths.QueueFile, model.Queue{}); err != nil {
		t.Fatal(err)
	}
	if err := stateinternal.WriteJSON(filepath.Join(paths.RunsDir, "run-1", "summary.json"), model.RunSummary{RunID: "run-1", Status: "finished"}); err != nil {
		t.Fatal(err)
	}
	outputDir := filepath.Join(t.TempDir(), "web")
	if err := siteFor(baseDir).generateStaticWeb(outputDir); err != nil {
		t.Fatal(err)
	}
	for _, font := range webFonts {
		if _, err := os.Stat(filepath.Join(outputDir, "fonts", font.file)); err != nil {
			t.Fatalf("static export lacks %s: %v", font.file, err)
		}
	}
	if license, err := os.ReadFile(filepath.Join(outputDir, "fonts", "LICENSE.txt")); err != nil || !bytes.Equal(license, webFontLicense) {
		t.Fatalf("static export lacks the font licence: %v", err)
	}
	checked := 0
	err = filepath.WalkDir(outputDir, func(file string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || entry.Name() != "web_fonts.css" {
			return walkErr
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		for _, match := range fontURL.FindAllStringSubmatch(string(data), -1) {
			target := filepath.Join(filepath.Dir(file), filepath.FromSlash(match[1]))
			if _, err := os.Stat(target); err != nil {
				t.Errorf("%s loads %s, which does not resolve: %v", file, match[1], err)
			}
		}
		checked++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked < 4 {
		t.Fatalf("checked %d web_fonts.css files, want one beside each page (root, search, jobs, project, run)", checked)
	}
	runPage, err := os.ReadFile(filepath.Join(outputDir, "project", "default", "run", "run-1", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(runPage), `href="web_fonts.css"`) {
		t.Fatal("static run page does not link its relative font stylesheet")
	}
	if matches, _ := filepath.Glob(filepath.Join(outputDir, "project", "*", "*.woff2")); len(matches) > 0 {
		t.Fatalf("fonts are copied beside pages: %v", matches)
	}
}

// Font families come from --font-sans and --font-mono, so one change of
// typeface reaches every page.
func TestStylesheetsTakeFontsFromTokens(t *testing.T) {
	family := regexp.MustCompile(`^\s*font(-family)?:\s*(.*)$`)
	for name, stylesheet := range map[string]string{
		"web_styles.css":         webStylesCSS,
		"web_sidebar_styles.css": webSidebarStylesCSS,
		"web_info_styles.css":    webInfoStylesCSS,
	} {
		for number, line := range strings.Split(stylesheet, "\n") {
			match := family.FindStringSubmatch(line)
			if match == nil || match[2] == "" || strings.Contains(match[2], "var(--font-") || strings.HasPrefix(match[2], "inherit") {
				continue
			}
			t.Errorf("%s:%d names a font family instead of a token: %s", name, number+1, strings.TrimSpace(line))
		}
	}
	for _, token := range []string{"--font-sans:", "--font-mono:", `"IBM Plex Sans"`, `"IBM Plex Mono"`} {
		if !strings.Contains(webTokensCSS, token) {
			t.Errorf("web_tokens.css lacks %s", token)
		}
	}
}
