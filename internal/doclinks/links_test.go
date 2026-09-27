package doclinks

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode"
)

var (
	fencePattern      = regexp.MustCompile("(?m)^\\s*(```|~~~)")
	inlineCodePattern = regexp.MustCompile("`[^`\n]*`")
	linkPattern       = regexp.MustCompile(`\]\(([^)\s]+)\)`)
	headingPattern    = regexp.MustCompile(`^#{1,6}\s+(.*?)\s*#*\s*$`)
	schemePattern     = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`)
	headingLinkText   = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
)

// TestRelativeLinks checks that every relative link in the root Markdown
// files and docs/ (except the generated docs/web-demo/) names an existing
// file or directory, and that an #anchor into a Markdown file names one of
// its headings.
func TestRelativeLinks(t *testing.T) {
	root := repoRoot(t)
	files := markdownFiles(t, root)
	if len(files) == 0 {
		t.Fatal("found no Markdown files to check")
	}
	anchors := map[string]map[string]bool{}
	for _, file := range files {
		for _, target := range links(t, file) {
			if err := checkLink(file, target, anchors); err != nil {
				rel, _ := filepath.Rel(root, file)
				t.Errorf("%s: link %q: %v", rel, target, err)
			}
		}
	}
}

func checkLink(file, target string, anchors map[string]map[string]bool) error {
	if schemePattern.MatchString(target) || strings.HasPrefix(target, "//") {
		return nil
	}
	path, anchor, _ := strings.Cut(target, "#")
	resolved := file
	if path != "" {
		resolved = filepath.Join(filepath.Dir(file), filepath.FromSlash(path))
		if _, err := os.Stat(resolved); err != nil {
			return fmt.Errorf("target does not exist")
		}
	}
	if anchor == "" || filepath.Ext(resolved) != ".md" {
		return nil
	}
	if anchors[resolved] == nil {
		data, err := os.ReadFile(resolved)
		if err != nil {
			return err
		}
		anchors[resolved] = headingAnchors(string(data))
	}
	if !anchors[resolved][anchor] {
		return fmt.Errorf("no heading with anchor #%s", anchor)
	}
	return nil
}

func links(t *testing.T, file string) []string {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var targets []string
	for _, line := range proseLines(string(data)) {
		line = inlineCodePattern.ReplaceAllString(line, "")
		for _, match := range linkPattern.FindAllStringSubmatch(line, -1) {
			targets = append(targets, match[1])
		}
	}
	return targets
}

// headingAnchors returns the anchors GitHub generates for the headings of a
// Markdown document: lowercased, punctuation dropped, spaces turned into
// hyphens, and repeats numbered -1, -2, and so on.
func headingAnchors(markdown string) map[string]bool {
	anchors := map[string]bool{}
	seen := map[string]int{}
	for _, line := range proseLines(markdown) {
		match := headingPattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		text := headingLinkText.ReplaceAllString(match[1], "$1")
		var slug strings.Builder
		for _, r := range strings.ToLower(text) {
			switch {
			case r == ' ':
				slug.WriteRune('-')
			case r == '-' || r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r):
				slug.WriteRune(r)
			}
		}
		anchor := slug.String()
		if count := seen[anchor]; count > 0 {
			anchors[fmt.Sprintf("%s-%d", anchor, count)] = true
		} else {
			anchors[anchor] = true
		}
		seen[anchor]++
	}
	return anchors
}

// proseLines returns the lines outside fenced code blocks.
func proseLines(markdown string) []string {
	var lines []string
	inFence := false
	for _, line := range strings.Split(markdown, "\n") {
		if fencePattern.MatchString(line) {
			inFence = !inFence
			continue
		}
		if !inFence {
			lines = append(lines, line)
		}
	}
	return lines
}

func markdownFiles(t *testing.T, root string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(root, "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	webDemo := filepath.Join(root, "docs", "web-demo")
	err = filepath.WalkDir(filepath.Join(root, "docs"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && path == webDemo {
			return filepath.SkipDir
		}
		if !entry.IsDir() && filepath.Ext(path) == ".md" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above the test directory")
		}
		dir = parent
	}
}
