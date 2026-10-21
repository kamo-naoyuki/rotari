package tableview

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// mediaTree writes files under a new root: WAV files of 0.5 s from
// testdata, and others with the text given.
func mediaTree(t *testing.T, files map[string]string) string {
	t.Helper()
	wav, err := os.ReadFile(filepath.Join("testdata", "media", "pcm16.wav"))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for name, content := range files {
		if content == "wav" {
			content = string(wav)
		}
		writeFile(t, filepath.Join(root, filepath.FromSlash(name)), content)
	}
	return root
}

func openListing(t *testing.T, root string, listing Listing) *Table {
	t.Helper()
	table, err := OpenDirectory(Target{Path: root, Roots: []string{root}}, listing)
	if err != nil {
		t.Fatal(err)
	}
	return table
}

func column(page Page, index int) []string {
	var values []string
	for _, row := range page.Rows {
		values = append(values, row.Cells[index])
	}
	return values
}

func TestDirectoryListsFilesWithProperties(t *testing.T) {
	root := mediaTree(t, map[string]string{"ref/a.wav": "wav", "ref/b.wav": "wav", "notes.txt": "hello", "deep/x/y.wav": "wav"})
	table := openListing(t, root, Listing{})
	page, err := table.Page(Query{})
	if err != nil {
		t.Fatal(err)
	}
	if got := column(page, 0); !reflect.DeepEqual(got, []string{"deep/x/y.wav", "notes.txt", "ref/a.wav", "ref/b.wav"}) {
		t.Fatalf("paths = %q", got)
	}
	notes := page.Rows[1]
	if notes.Cells[1] != "" || notes.Cells[8] != "5" || notes.Cells[9] == "" || notes.Media != nil {
		t.Fatalf("notes.txt = %q %+v", notes.Cells, notes.Media)
	}
	wav := page.Rows[2]
	if !reflect.DeepEqual(wav.Cells[1:8], []string{"WAV PCM", "0.5", "16000", "1", "256000", "", ""}) {
		t.Fatalf("a.wav = %q", wav.Cells)
	}
	if media := wav.Media[0]; media == nil || media.Ref != "ref/a.wav" || media.Base != "" || media.Properties == nil {
		t.Fatalf("a.wav media = %+v", media)
	}
	if page.Columns[2].Type != TypeDuration || page.Columns[8].Type != TypeBytes || page.Columns[0].Type != TypePath {
		t.Fatalf("columns = %+v", page.Columns)
	}

	// Filters read each column's units; Describe totals durations.
	for _, test := range []struct {
		filter Condition
		want   []string
	}{
		{Condition{Column: 2, Op: OpGreaterEqual, Value: "0:00.5"}, []string{"deep/x/y.wav", "ref/a.wav", "ref/b.wav"}},
		{Condition{Column: 8, Op: OpLess, Value: "1K"}, []string{"notes.txt"}},
		{Condition{Column: 8, Op: OpGreater, Value: "0.01MiB"}, []string{"deep/x/y.wav", "ref/a.wav", "ref/b.wav"}},
		{Condition{Column: 9, Op: OpGreater, Value: "2000-01-01"}, []string{"deep/x/y.wav", "notes.txt", "ref/a.wav", "ref/b.wav"}},
		{Condition{Column: 9, Op: OpLess, Value: "2000-01-01"}, nil},
	} {
		page, err := table.Page(Query{Filters: []Condition{test.filter}})
		if err != nil {
			t.Fatal(err)
		}
		if got := column(page, 0); !reflect.DeepEqual(got, test.want) {
			t.Errorf("%+v: %q, want %q", test.filter, got, test.want)
		}
	}
	description, err := table.Describe(nil)
	if err != nil {
		t.Fatal(err)
	}
	if duration := description.Columns[2]; duration.Numeric != 3 || *duration.Sum != 1.5 {
		t.Fatalf("duration = %+v", duration)
	}
	// A sort by size puts the text file first.
	page, err = table.Page(Query{Sort: &Sort{Column: 8}})
	if err != nil {
		t.Fatal(err)
	}
	if page.Rows[0].Cells[0] != "notes.txt" {
		t.Fatalf("by size = %q", column(page, 0))
	}
}

func TestDirectoryPatternsChooseFiles(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.wav": "x", "b.png": "x", "c.txt": "x", "sub/d.wav": "x", "sub/deeper/e.wav": "x"})
	for pattern, want := range map[string]string{
		"":                "a.wav b.png c.txt sub/d.wav sub/deeper/e.wav",
		"*":               "a.wav b.png c.txt",
		"*.wav":           "a.wav",
		"**/*.wav":        "a.wav sub/d.wav sub/deeper/e.wav",
		"sub/**":          "sub/d.wav sub/deeper/e.wav",
		"**/*.{wav,png}":  "a.wav b.png sub/d.wav sub/deeper/e.wav",
		"sub/*/*.wav":     "sub/deeper/e.wav",
		"nothing/**/*.go": "",
	} {
		page, err := openListing(t, root, Listing{Pattern: pattern}).Page(Query{})
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(column(page, 0), " "); got != want {
			t.Errorf("%q = %q, want %q", pattern, got, want)
		}
	}
	for _, pattern := range []string{"[", "a//b", "{wav"} {
		if _, err := OpenDirectory(Target{Path: root, Roots: []string{root}}, Listing{Pattern: pattern}); err == nil {
			t.Errorf("pattern %q accepted", pattern)
		}
	}
}

func TestDirectoryGroupsStems(t *testing.T) {
	nested := mediaTree(t, map[string]string{
		"ref/u1.wav": "wav", "ref/u2.wav": "wav", "ref/u3.wav": "wav",
		"hyp/u1.wav": "wav", "hyp/u3.wav": "wav",
		"plots/u1.png": "x", "plots/u2.png": "x",
		"readme.txt": "x",
	})
	for _, test := range []struct {
		name    string
		root    string
		listing Listing
		columns []string
		rows    [][]string
	}{
		{"first-level directories by default", nested, Listing{View: ViewStems},
			[]string{"stem", "./", "hyp/", "plots/", "ref/"},
			[][]string{{"readme", "readme.txt", "", "", ""}, {"u1", "", "hyp/u1.wav", "plots/u1.png", "ref/u1.wav"}, {"u2", "", "", "plots/u2.png", "ref/u2.wav"}, {"u3", "", "hyp/u3.wav", "", "ref/u3.wav"}}},
		{"only incomplete stems, of the files a pattern chooses", nested, Listing{View: ViewStems, Pattern: "*/*.wav", Incomplete: true},
			[]string{"stem", "hyp/", "ref/"},
			[][]string{{"u2", "", "ref/u2.wav"}}},
		{"parent directories", mediaTree(t, map[string]string{"a/x/u1.wav": "x", "b/x/u1.wav": "x", "a/y/u1.wav": "x"}), Listing{View: ViewStems, Group: GroupParent},
			[]string{"stem", "a/x/", "a/y/", "b/x/"},
			[][]string{{"u1", "a/x/u1.wav", "a/y/u1.wav", "b/x/u1.wav"}}},
		{"extensions, chosen for a flat directory", mediaTree(t, map[string]string{"a.wav": "x", "a.png": "x", "b.wav": "x", "c": "x"}), Listing{View: ViewStems},
			[]string{"stem", "(none)", ".png", ".wav"},
			[][]string{{"a", "", "a.png", "a.wav"}, {"b", "", "", "b.wav"}, {"c", "c", "", ""}}},
		{"suffixes after a separator", mediaTree(t, map[string]string{"a_ref.wav": "x", "a_hyp.wav": "x", "b_ref.wav": "x", "plain.wav": "x"}), Listing{View: ViewStems, Group: GroupSuffix, SuffixSeparator: "_"},
			[]string{"stem", "(none)", "hyp", "ref"},
			[][]string{{"a", "", "a_hyp.wav", "a_ref.wav"}, {"b", "", "", "b_ref.wav"}, {"plain", "plain.wav", "", ""}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			page, err := openListing(t, test.root, test.listing).Page(Query{})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(names(page.Columns), test.columns) || !reflect.DeepEqual(cells(page), test.rows) {
				t.Fatalf("columns %q rows %q\nwant %q %q", names(page.Columns), cells(page), test.columns, test.rows)
			}
		})
	}
	// A stems view's media paths are relative to the directory; a missing
	// file's cell is blank and names nothing.
	page, err := openListing(t, nested, Listing{View: ViewStems, Pattern: "*/*.wav"}).Page(Query{Filters: []Condition{{Column: 1, Op: OpEmpty}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Rows) != 1 || page.Rows[0].Media[1] != nil || page.Rows[0].Media[2] == nil || page.Rows[0].Media[2].Properties.Duration != 0.5 {
		t.Fatalf("incomplete row = %+v", page.Rows)
	}
}

func TestDirectoryRefusesBadListings(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.wav": "x"})
	target := Target{Path: root, Roots: []string{root}}
	for _, listing := range []Listing{
		{View: "tree"},
		{View: ViewStems, Group: "color"},
		{View: ViewStems, Group: GroupSuffix},
		{View: ViewStems, Group: GroupSuffix, SuffixSeparator: "/"},
		{View: ViewStems, Group: GroupTop, SuffixSeparator: "_"},
		{View: ViewFiles, Group: GroupTop},
		{View: ViewFiles, Incomplete: true},
	} {
		if _, err := OpenDirectory(target, listing); err == nil {
			t.Errorf("%+v accepted", listing)
		}
	}
	if _, err := OpenDirectory(Target{Path: filepath.Join(root, "a.wav"), Roots: []string{root}}, Listing{}); err == nil {
		t.Error("a file listed as a directory")
	}
	table := openListing(t, root, Listing{})
	if _, err := table.Page(Query{Base: FileBase}); err == nil {
		t.Error("a file's base accepted for a directory")
	}
}

func TestDirectoryStaysUnderItsRoot(t *testing.T) {
	outside := mediaTree(t, map[string]string{"secret.wav": "wav", "d/inner.wav": "wav"})
	root := mediaTree(t, map[string]string{"in.wav": "wav"})
	if err := os.Symlink(filepath.Join(outside, "secret.wav"), filepath.Join(root, "link.wav")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "d"), filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	page, err := openListing(t, root, Listing{}).Page(Query{})
	if err != nil {
		t.Fatal(err)
	}
	// A directory symlink is not followed; a file symlink is listed, but
	// what it points to outside the root is not read.
	if got := strings.Join(column(page, 0), " "); got != "in.wav link.wav linked" {
		t.Fatalf("paths = %q", got)
	}
	if link := page.Rows[1]; link.Cells[1] != "" || link.Media[0] == nil || link.Media[0].Error != "Not shown: a symlink leaves the allowed roots" {
		t.Fatalf("link.wav = %q %+v", link.Cells, link.Media[0])
	}
	if _, err := OpenDirectory(Target{Path: filepath.Join(root, "linked"), Roots: []string{root}}, Listing{}); err == nil {
		t.Error("a directory symlink out of the root was listed")
	}
}
