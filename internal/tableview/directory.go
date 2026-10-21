package tableview

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Column types a directory listing's columns carry; a delimited file's
// columns are untyped text.
const (
	// TypePath cells name a file relative to the directory.
	TypePath = "path"
	// TypeDuration cells are seconds.
	TypeDuration = "duration"
	// TypeBytes cells are a size in bytes; a filter's value may carry a
	// binary suffix, as 10M or 1.5G.
	TypeBytes = "bytes"
	// TypeTime cells are RFC 3339 times in UTC, which sort as text.
	TypeTime   = "time"
	TypeNumber = "number"
)

// The views of a directory.
const (
	// ViewFiles lists one file per row.
	ViewFiles = "files"
	// ViewStems lists one stem per row and one group per column.
	ViewStems = "stems"
)

// How the stems view splits a file's path into a group and a stem.
const (
	// GroupAuto is GroupExtension when every file is directly in the
	// directory, and GroupTop otherwise.
	GroupAuto = ""
	// GroupTop groups by the first directory in the path; the stem is the
	// rest without its extension.
	GroupTop = "top"
	// GroupParent groups by the file's directory; the stem is its name
	// without its extension.
	GroupParent = "parent"
	// GroupExtension groups by extension; the stem is the path without it.
	GroupExtension = "extension"
	// GroupSuffix groups by the end of the name after the last
	// SuffixSeparator; the stem is the path before it.
	GroupSuffix = "suffix"
)

// Listing is how a directory becomes a table.
type Listing struct {
	// Pattern chooses files by their path relative to the directory, with
	// "/" between names: "*" matches within a name, "**" any number of
	// directories, and {a,b} either alternative. "" is "**/*".
	Pattern string
	View    string
	Group   string
	// SuffixSeparator splits a name for GroupSuffix.
	SuffixSeparator string
	// Incomplete keeps only the stems missing a file in some group.
	Incomplete bool
}

// listScanLimit bounds the entries a listing reads, as an artifact
// directory page does.
const listScanLimit = 100_000

// listingAge is how long one listing of a directory is reused, so paging
// does not walk it again while a job is still writing files into it.
const listingAge = 10 * time.Second

// listedFile is a file a directory listing found.
type listedFile struct {
	path     string
	size     int64
	modified time.Time
}

// directoryListing is the files of one directory a pattern matches.
type directoryListing struct {
	files     []listedFile
	truncated bool
}

// directoryTable is a directory listed as files or stems.
type directoryTable struct {
	target  Target
	listing Listing
	id      string
	files   []listedFile
	// stems view: the groups, as columns, and for each stem its file in
	// each group, an index into files or -1.
	groups  []string
	stems   []string
	members [][]int
	notes   []string
}

// OpenDirectory lists target, a directory, as listing asks.
func OpenDirectory(target Target, listing Listing) (*Table, error) {
	if listing.Pattern == "" {
		listing.Pattern = "**/*"
	}
	switch listing.View {
	case "", ViewFiles:
		listing.View = ViewFiles
		if listing.Group != "" || listing.SuffixSeparator != "" || listing.Incomplete {
			return nil, fmt.Errorf("groups apply to the stems view")
		}
	case ViewStems:
		switch listing.Group {
		case GroupAuto, GroupTop, GroupParent, GroupExtension:
			if listing.SuffixSeparator != "" {
				return nil, fmt.Errorf("a suffix separator applies to suffix groups")
			}
		case GroupSuffix:
			if listing.SuffixSeparator == "" || strings.Contains(listing.SuffixSeparator, "/") {
				return nil, fmt.Errorf("suffix groups need a separator other than /")
			}
		default:
			return nil, fmt.Errorf("group must be top, parent, extension, or suffix")
		}
	default:
		return nil, fmt.Errorf("view must be files or stems")
	}
	patterns, err := expandPattern(listing.Pattern)
	if err != nil {
		return nil, err
	}
	info, err := target.Stat()
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", target.Path)
	}
	bucket := time.Now().UnixNano() / int64(listingAge)
	listKey := fmt.Sprintf("listing\x00%s\x00%s\x00%d", target.Path, listing.Pattern, bucket)
	value, err := results.get(listKey, func() (any, error) { return listDirectory(target, patterns) })
	if err != nil {
		return nil, err
	}
	found := value.(*directoryListing)
	table := &directoryTable{target: target, listing: listing, files: found.files,
		id: fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%t", listKey, listing.View, listing.Group, listing.SuffixSeparator, listing.Incomplete)}
	if found.truncated {
		table.notes = append(table.notes, fmt.Sprintf("Only the first %d entries of the directory were read.", listScanLimit))
	}
	if listing.View == ViewStems {
		table.groupStems()
	}
	return &Table{source: table, target: target, directory: true}, nil
}

// expandPattern splits a pattern's {a,b} alternatives into patterns, and
// checks each.
func expandPattern(pattern string) ([]string, error) {
	open := strings.IndexByte(pattern, '{')
	if open < 0 {
		for _, part := range strings.Split(pattern, "/") {
			if _, err := path.Match(part, ""); err != nil || part == "" {
				return nil, fmt.Errorf("invalid pattern %q", pattern)
			}
		}
		return []string{pattern}, nil
	}
	end := strings.IndexByte(pattern[open:], '}')
	if end < 0 {
		return nil, fmt.Errorf("invalid pattern %q: unclosed {", pattern)
	}
	var patterns []string
	for _, choice := range strings.Split(pattern[open+1:open+end], ",") {
		expanded, err := expandPattern(pattern[:open] + choice + pattern[open+end+1:])
		if err != nil {
			return nil, err
		}
		patterns = append(patterns, expanded...)
	}
	return patterns, nil
}

// matchPath matches a slash-separated path against a pattern whose "**"
// parts match any number of names.
func matchPath(pattern, name string) bool {
	return matchParts(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

func matchParts(pattern, name []string) bool {
	for len(pattern) > 0 {
		if pattern[0] == "**" {
			for skip := 0; skip <= len(name); skip++ {
				if matchParts(pattern[1:], name[skip:]) {
					return true
				}
			}
			return false
		}
		if len(name) == 0 {
			return false
		}
		if matched, _ := path.Match(pattern[0], name[0]); !matched {
			return false
		}
		pattern, name = pattern[1:], name[1:]
	}
	return len(name) == 0
}

// listDirectory walks the directory through its root, without following
// directory symlinks, and keeps the files and file symlinks a pattern
// matches, by path.
func listDirectory(target Target, patterns []string) (*directoryListing, error) {
	rootPath, relative, ok := RootOf(target.Path, target.Roots)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrOutsideRoots, target.Path)
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	start := path.Clean(strings.ReplaceAll(relative, string(os.PathSeparator), "/"))
	listing := &directoryListing{}
	entries := 0
	err = fs.WalkDir(root.FS(), start, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			if name == start {
				return err
			}
			return nil
		}
		if entries++; entries > listScanLimit {
			listing.truncated = true
			return fs.SkipAll
		}
		if entry.IsDir() {
			return nil
		}
		inside := strings.TrimPrefix(strings.TrimPrefix(name, start), "/")
		if start == "." {
			inside = name
		}
		if !slices.ContainsFunc(patterns, func(pattern string) bool { return matchPath(pattern, inside) }) {
			return nil
		}
		file := listedFile{path: inside}
		if info, err := entry.Info(); err == nil {
			file.size, file.modified = info.Size(), info.ModTime()
		}
		listing.files = append(listing.files, file)
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(listing.files, func(a, b listedFile) int { return strings.Compare(a.path, b.path) })
	return listing, nil
}

// split returns a file's group and stem for the listing's grouping.
func (table *directoryTable) split(file string, group string) (string, string) {
	directory, name := path.Split(file)
	extension := path.Ext(name)
	bare := strings.TrimSuffix(file, extension)
	switch group {
	case GroupTop:
		top, rest, nested := strings.Cut(file, "/")
		if !nested {
			return ".", bare
		}
		return top, strings.TrimSuffix(rest, extension)
	case GroupParent:
		return strings.TrimSuffix(directory, "/"), strings.TrimSuffix(name, extension)
	case GroupExtension:
		return extension, bare
	default:
		cut := strings.LastIndex(bare, table.listing.SuffixSeparator)
		if cut < len(directory) {
			return "", bare
		}
		return bare[cut+len(table.listing.SuffixSeparator):], bare[:cut]
	}
}

// groupStems builds the stems view: one row per stem, one column per
// group, with the first file of a stem in a group kept.
func (table *directoryTable) groupStems() {
	group := table.listing.Group
	if group == GroupAuto {
		group = GroupExtension
		if slices.ContainsFunc(table.files, func(file listedFile) bool { return strings.Contains(file.path, "/") }) {
			group = GroupTop
		}
	}
	type split struct{ group, stem string }
	splits := make([]split, len(table.files))
	groups := map[string]int{}
	stems := map[string]int{}
	for index, file := range table.files {
		splits[index].group, splits[index].stem = table.split(file.path, group)
		groups[splits[index].group] = 0
		stems[splits[index].stem] = 0
	}
	table.groups = slices.Sorted(func(yield func(string) bool) {
		for name := range groups {
			if !yield(name) {
				return
			}
		}
	})
	for index, name := range table.groups {
		groups[name] = index
	}
	table.stems = slices.Sorted(func(yield func(string) bool) {
		for name := range stems {
			if !yield(name) {
				return
			}
		}
	})
	for index, name := range table.stems {
		stems[name] = index
	}
	table.members = make([][]int, len(table.stems))
	for index := range table.members {
		table.members[index] = slices.Repeat([]int{-1}, len(table.groups))
	}
	duplicates := 0
	for index, split := range splits {
		member := &table.members[stems[split.stem]][groups[split.group]]
		if *member >= 0 {
			duplicates++
			continue
		}
		*member = index
	}
	if duplicates > 0 {
		table.notes = append(table.notes, fmt.Sprintf("%d files share a stem and group with another and are not shown.", duplicates))
	}
	if table.listing.Incomplete {
		var stemsKept []string
		var membersKept [][]int
		for index, members := range table.members {
			if slices.Contains(members, -1) {
				stemsKept = append(stemsKept, table.stems[index])
				membersKept = append(membersKept, members)
			}
		}
		table.stems, table.members = stemsKept, membersKept
	}
	table.listing.Group = group
}

func (table *directoryTable) key() string { return table.id }

func (table *directoryTable) notesOf() []string { return table.notes }

// fileColumns are the files view's columns, in the order fileCells fills.
var fileColumns = []Column{
	{Name: "path", Type: TypePath}, {Name: "format"}, {Name: "duration", Type: TypeDuration},
	{Name: "sample rate", Type: TypeNumber}, {Name: "channels", Type: TypeNumber}, {Name: "bitrate", Type: TypeNumber},
	{Name: "width", Type: TypeNumber}, {Name: "height", Type: TypeNumber},
	{Name: "size", Type: TypeBytes}, {Name: "modified", Type: TypeTime},
}

func (table *directoryTable) columns() []Column {
	if table.listing.View == ViewFiles {
		return slices.Clone(fileColumns)
	}
	columns := []Column{{Name: "stem"}}
	for _, group := range table.groups {
		name := group
		switch {
		case name == "":
			name = "(none)"
		case table.listing.Group == GroupTop || table.listing.Group == GroupParent:
			name += "/"
		}
		columns = append(columns, Column{Name: name, Type: TypePath})
	}
	return columns
}

func (table *directoryTable) len() int {
	if table.listing.View == ViewFiles {
		return len(table.files)
	}
	return len(table.stems)
}

func (table *directoryTable) row(number int) []string {
	if table.listing.View == ViewStems {
		cells := []string{table.stems[number]}
		for _, member := range table.members[number] {
			if member < 0 {
				cells = append(cells, "")
			} else {
				cells = append(cells, table.files[member].path)
			}
		}
		return cells
	}
	file := table.files[number]
	cells := make([]string, len(fileColumns))
	cells[0], cells[8] = file.path, strconv.FormatInt(file.size, 10)
	if !file.modified.IsZero() {
		cells[9] = file.modified.UTC().Format(time.RFC3339)
	}
	if MediaKind(file.path) == "" {
		return cells
	}
	properties, err := table.target.probe(filepath.Join(table.target.Path, filepath.FromSlash(file.path)))
	if err != nil {
		return cells
	}
	count := func(value int) string {
		if value == 0 {
			return ""
		}
		return strconv.Itoa(value)
	}
	cells[1] = properties.Format
	if properties.Duration > 0 {
		cells[2] = strconv.FormatFloat(properties.Duration, 'f', -1, 64)
	}
	cells[3], cells[4], cells[5] = count(properties.SampleRate), count(properties.Channels), count(properties.Bitrate)
	cells[6], cells[7] = count(properties.Width), count(properties.Height)
	return cells
}

func (table *directoryTable) scan(visit func(number int, cells []string) bool) error {
	for number := range table.len() {
		if !visit(number, table.row(number)) {
			return nil
		}
	}
	return nil
}

func (table *directoryTable) rows(numbers []int) ([][]string, error) {
	rows := make([][]string, len(numbers))
	for index, number := range numbers {
		rows[index] = table.row(number)
	}
	return rows, nil
}
