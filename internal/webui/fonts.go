package webui

import (
	"embed"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// The Web UI's typefaces, IBM Plex Sans and Mono (assets/fonts/README.md).
// Pages name them through --font-sans and --font-mono in web_tokens.css;
// characters outside each file's range fall back to the system font.
//
//go:embed assets/fonts/*.woff2
var webFontFiles embed.FS

//go:embed assets/fonts/LICENSE.txt
var webFontLicense []byte

type webFont struct {
	file   string
	family string
	weight int
}

// IBM's unicode-range for its Latin-1 split files.
const (
	plexSansLatin1 = "U+0000, U+000D, U+0020-007E, U+00A0-00A3, U+00A4-00FF, U+0131, U+0152-0153, U+02C6, U+02DA, U+02DC, U+2013-2014, U+2018-201A, U+201C-201E, U+2020-2022, U+2026, U+2030, U+2039-203A, U+2044, U+2074, U+20AC, U+2122, U+2212, U+FB01-FB02"
	plexMonoLatin1 = "U+0020-007E, U+00A0-00FF, U+0131, U+0152-0153, U+02C6, U+02DA, U+02DC, U+2013-2014, U+2018-201A, U+201C-201E, U+2020-2022, U+2026, U+2030, U+2039-203A, U+2044, U+20AC, U+2122, U+2212, U+FB01-FB02"
)

var webFonts = []webFont{
	{"IBMPlexSans-Regular-Latin1.woff2", "IBM Plex Sans", 400},
	{"IBMPlexSans-Medium-Latin1.woff2", "IBM Plex Sans", 500},
	{"IBMPlexSans-SemiBold-Latin1.woff2", "IBM Plex Sans", 600},
	{"IBMPlexMono-Regular-Latin1.woff2", "IBM Plex Mono", 400},
	{"IBMPlexMono-Medium-Latin1.woff2", "IBM Plex Mono", 500},
}

// fontFaceCSS declares the fonts, loading them from prefix: "/fonts/" on the
// live server, or a path relative to the stylesheet in the static export.
func fontFaceCSS(prefix string) string {
	var builder strings.Builder
	for _, font := range webFonts {
		unicodeRange := plexSansLatin1
		if font.family == "IBM Plex Mono" {
			unicodeRange = plexMonoLatin1
		}
		builder.WriteString(`@font-face {
  font-family: "` + font.family + `";
  font-style: normal;
  font-weight: ` + strconv.Itoa(font.weight) + `;
  font-display: swap;
  src: url("` + prefix + font.file + `") format("woff2");
  unicode-range: ` + unicodeRange + `;
}
`)
	}
	return builder.String()
}

// serveWebFont serves one embedded font file, or the font licence, under
// /fonts/.
func serveWebFont(writer http.ResponseWriter, request *http.Request) {
	name := strings.TrimPrefix(request.URL.Path, "/fonts/")
	if name == "LICENSE.txt" {
		writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = writer.Write(webFontLicense)
		return
	}
	for _, font := range webFonts {
		if font.file != name {
			continue
		}
		data, err := webFontFiles.ReadFile(path.Join("assets/fonts", font.file))
		if err != nil {
			http.Error(writer, "font unavailable", http.StatusInternalServerError)
			return
		}
		writer.Header().Set("Content-Type", "font/woff2")
		writer.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = writer.Write(data)
		return
	}
	http.NotFound(writer, request)
}

// writeStaticFonts writes the fonts and their licence once, into fonts/ at
// the root of a static export.
func writeStaticFonts(outputDir string) error {
	directory := filepath.Join(outputDir, "fonts")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	for _, font := range webFonts {
		data, err := webFontFiles.ReadFile(path.Join("assets/fonts", font.file))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(directory, font.file), data, 0o644); err != nil {
			return err
		}
	}
	return os.WriteFile(filepath.Join(directory, "LICENSE.txt"), webFontLicense, 0o644)
}
