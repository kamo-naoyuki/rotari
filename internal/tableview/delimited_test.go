package tableview

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fileTarget writes content to a file in a new root and targets it.
func fileTarget(t *testing.T, name, content string) Target {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, name), content)
	return Target{Path: filepath.Join(root, name), Roots: []string{root}}
}

func cells(page Page) [][]string {
	var rows [][]string
	for _, row := range page.Rows {
		rows = append(rows, row.Cells)
	}
	return rows
}

func names(columns []Column) []string {
	var result []string
	for _, column := range columns {
		result = append(result, column.Name)
	}
	return result
}

func TestFileSplitsWithAnySeparator(t *testing.T) {
	for _, test := range []struct {
		name      string
		content   string
		separator rune
		header    bool
		columns   []string
		rows      [][]string
	}{
		{"comma", "a,b\n1,2\n", ',', true, []string{"a", "b"}, [][]string{{"1", "2"}}},
		{"tab", "a\tb\n1\t2,3\n", '\t', true, []string{"a", "b"}, [][]string{{"1", "2,3"}}},
		{"pipe without header", "x|y|z\n1|2\n", '|', false, []string{"column 1", "column 2", "column 3"}, [][]string{{"x", "y", "z"}, {"1", "2"}}},
		{"space", "utt1 /data/a.wav\nutt2 /data/b.wav\n", ' ', false, []string{"column 1", "column 2"}, [][]string{{"utt1", "/data/a.wav"}, {"utt2", "/data/b.wav"}}},
		{"multibyte", "名前；値\n甲；1\n", '；', true, []string{"名前", "値"}, [][]string{{"甲", "1"}}},
		{"quotes, line breaks, BOM, blank lines, CRLF", "\ufeffid,text\r\n1,\"a, \"\"b\"\"\nc\"\r\n\r\n2,d\r\n", ',', true, []string{"id", "text"}, [][]string{{"1", "a, \"b\"\nc"}, {"2", "d"}}},
		{"an empty header cell is named by position", "a,,c\n1,2,3\n", ',', true, []string{"a", "column 2", "c"}, [][]string{{"1", "2", "3"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			table, err := OpenFile(fileTarget(t, "data.csv", test.content), Format{Separator: test.separator, Header: test.header})
			if err != nil {
				t.Fatal(err)
			}
			page, err := table.Page(Query{})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(names(page.Columns), test.columns) || !reflect.DeepEqual(cells(page), test.rows) {
				t.Fatalf("columns %q rows %q, want %q %q", names(page.Columns), cells(page), test.columns, test.rows)
			}
			if page.Total != len(test.rows) || page.Matched != page.Total {
				t.Fatalf("total %d matched %d, want %d", page.Total, page.Matched, len(test.rows))
			}
		})
	}
}

func TestFilePagesAcrossCheckpoints(t *testing.T) {
	var content strings.Builder
	content.WriteString("n,square\n")
	for n := range 1000 {
		fmt.Fprintf(&content, "%d,%d\n", n, n*n)
	}
	target := fileTarget(t, "big.csv", content.String())
	table, err := OpenFile(target, Format{Separator: ',', Header: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, offset := range []int{0, 255, 256, 700, 998} {
		page, err := table.Page(Query{Offset: offset, Limit: 3})
		if err != nil {
			t.Fatal(err)
		}
		if page.Total != 1000 {
			t.Fatalf("total = %d", page.Total)
		}
		for index, row := range page.Rows {
			n := offset + index
			if row.Number != n+1 || row.Cells[0] != fmt.Sprint(n) || row.Cells[1] != fmt.Sprint(n*n) {
				t.Fatalf("offset %d row %d = %d %q", offset, index, row.Number, row.Cells)
			}
		}
		if want := min(3, 1000-offset); len(page.Rows) != want {
			t.Fatalf("offset %d: %d rows, want %d", offset, len(page.Rows), want)
		}
	}
	// Without a header the first record is row 1.
	table, err = OpenFile(target, Format{Separator: ','})
	if err != nil {
		t.Fatal(err)
	}
	page, err := table.Page(Query{Offset: 256, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1001 || page.Rows[0].Cells[0] != "255" {
		t.Fatalf("headerless page = %d %q", page.Total, cells(page))
	}
}

func TestFileRejectsBadRequests(t *testing.T) {
	target := fileTarget(t, "data.csv", "a,b\n1,2\n")
	for _, separator := range []rune{0, '"', '\n', '\r'} {
		if _, err := OpenFile(target, Format{Separator: separator}); err == nil {
			t.Fatalf("separator %q accepted", separator)
		}
	}
	table, err := OpenFile(target, Format{Separator: ','})
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []Query{{Offset: -1}, {Limit: -1}, {Limit: MaxLimit + 1}} {
		if _, err := table.Page(query); err == nil {
			t.Fatalf("query %+v accepted", query)
		}
	}
	if _, err := OpenFile(Target{Path: filepath.Dir(target.Path), Roots: target.Roots}, Format{Separator: ','}); err == nil {
		t.Fatal("a directory opened as a file")
	}
}

func TestFileStaysUnderItsRoot(t *testing.T) {
	outside := fileTarget(t, "secret.csv", "secret\n")
	root := t.TempDir()
	if err := os.Symlink(outside.Path, filepath.Join(root, "link.csv")); err != nil {
		t.Fatal(err)
	}
	for _, target := range []Target{
		{Path: outside.Path, Roots: []string{root}},
		{Path: filepath.Join(root, "link.csv"), Roots: []string{root}},
	} {
		if _, err := OpenFile(target, Format{Separator: ','}); err == nil {
			t.Fatalf("%s was read outside its root", target.Path)
		}
	}
}

func TestFileBoundsOneRecord(t *testing.T) {
	saved := maxRecordBytes
	maxRecordBytes = 1 << 10
	t.Cleanup(func() { maxRecordBytes = saved })
	// An unclosed quote would read the rest of the file as one cell.
	_, err := OpenFile(fileTarget(t, "bad.csv", "a,b\n\"unclosed,1\n"+strings.Repeat("x,y\n", 2000)), Format{Separator: ','})
	if !errors.Is(err, errRecordTooLong) {
		t.Fatalf("err = %v, want errRecordTooLong", err)
	}
}

func TestPageCutsLongCells(t *testing.T) {
	long := strings.Repeat("あ", cellLimit)
	table, err := OpenFile(fileTarget(t, "long.csv", "a\n"+long+"\n"), Format{Separator: ',', Header: true})
	if err != nil {
		t.Fatal(err)
	}
	page, err := table.Page(Query{})
	if err != nil {
		t.Fatal(err)
	}
	cell := page.Rows[0].Cells[0]
	if !page.Cut || len(cell) > cellLimit+len("…") || !strings.HasSuffix(cell, "…") || !strings.HasPrefix(long, strings.TrimSuffix(cell, "…")) {
		t.Fatalf("cut = %t, cell of %d bytes", page.Cut, len(cell))
	}
}
