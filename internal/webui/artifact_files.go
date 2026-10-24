package webui

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/rotari-dev/rotari/internal/artifact"
	"github.com/rotari-dev/rotari/internal/jobstatus"
	stateinternal "github.com/rotari-dev/rotari/internal/state"
	"github.com/rotari-dev/rotari/internal/tableview"
)

// Limits on what one artifact content request returns. Files have no size
// limit: they are streamed, and text and directories are read a page at a
// time.
const (
	// artifactTextChunk is the size of one page of text.
	artifactTextChunk = 64 << 10
	// artifactDirectoryPage is how many children one directory page lists.
	artifactDirectoryPage = 200
	// artifactDirectoryScan bounds the names read from one directory.
	artifactDirectoryScan = 100000
)

// webArtifactListing is the /api/artifacts response: the listing the CLI
// prints, and whether the live server serves each entry's content.
type webArtifactListing struct {
	jobstatus.ArtifactListing
	// Previewable is left out only where nothing is served, a static export
	// without copied contents; omitzero keeps the empty list of a listing
	// without candidates, so the Web UI still offers Launch with on its
	// working directory.
	Previewable []bool `json:"previewable,omitzero"`
	// WorkingDirectoryPreviewable reports whether the working directory is
	// served as entry "wd", to be browsed and launched with like a listed
	// directory.
	WorkingDirectoryPreviewable bool `json:"working_directory_previewable,omitempty"`
	// RootsNote explains why only --artifact-root is served, when another
	// user could have chosen the listing's paths.
	RootsNote string `json:"roots_note,omitempty"`
}

// artifactRoots returns the directories whose files the live server may
// serve for a listing of a run in baseDir: every --artifact-root, and,
// when no other user could have chosen the listing's paths
// (artifactRecordsTrust), its working directory and the basedir's parent,
// unless that parent is the filesystem root. The note explains a refusal.
func (s site) artifactRoots(listing jobstatus.ArtifactListing, baseDir, projectName, runID string) ([]string, string) {
	var roots []string
	trusted, reason := artifactRecordsTrust(baseDir, projectName, runID)
	if trusted {
		if filepath.IsAbs(listing.WorkingDirectory) {
			roots = append(roots, filepath.Clean(listing.WorkingDirectory))
		}
		if parent, ok := basedirParentRoot(baseDir); ok {
			roots = append(roots, parent)
		}
	}
	for _, root := range s.ArtifactRoots {
		if filepath.IsAbs(root) {
			roots = append(roots, filepath.Clean(root))
		}
	}
	if trusted {
		return roots, ""
	}
	return roots, artifactRootsNote(reason)
}

// webArtifactListingOf adds to a listing what the live server serves of it.
func (s site) webArtifactListingOf(listing jobstatus.ArtifactListing, baseDir, projectName, runID string) webArtifactListing {
	roots, note := s.artifactRoots(listing, baseDir, projectName, runID)
	view := webArtifactListing{ArtifactListing: listing, Previewable: make([]bool, len(listing.Entries)), RootsNote: note}
	for index, entry := range listing.Entries {
		view.Previewable[index] = servable(entry, roots)
	}
	if entry, err := listingEntry(listing, artifactWorkingDirectoryEntry); err == nil {
		view.WorkingDirectoryPreviewable = servable(entry, roots)
	}
	return view
}

func servable(entry jobstatus.ArtifactEntry, roots []string) bool {
	_, _, inside := tableview.RootOf(entry.Path, roots)
	return inside && entry.Basis != artifact.BasisUnresolved &&
		(entry.Type == jobstatus.ArtifactFile || entry.Type == jobstatus.ArtifactDirectory)
}

// artifactWorkingDirectoryEntry names the listing's working directory where
// a content or launch request takes an entry index. It is not a candidate:
// it holds what a job writes without naming it, such as a tracker's
// default store, and is served under the same roots as the entries.
const artifactWorkingDirectoryEntry = "wd"

// artifactBasedirParentEntry names, in a request with a run and no job or
// attempt, the directory holding the basedir: the experiment directory,
// where a run's jobs often share one tracker store or TensorBoard log
// directory. The Web UI's Files view browses it and its children, which are
// served and launched on under the same roots as a run's entries
// (artifactRoots), so only for a trusted run or under --artifact-root.
const artifactBasedirParentEntry = "basedir_parent"

// listingEntry returns the entry a request names: an index of the listing,
// or its absolute working directory as artifactWorkingDirectoryEntry.
func listingEntry(listing jobstatus.ArtifactListing, name string) (jobstatus.ArtifactEntry, error) {
	if name == artifactWorkingDirectoryEntry {
		if !filepath.IsAbs(listing.WorkingDirectory) {
			return jobstatus.ArtifactEntry{}, fmt.Errorf("the attempt recorded no working directory")
		}
		return jobstatus.ArtifactEntry{Path: listing.WorkingDirectory, DisplayPath: ".", Basis: artifact.BasisAbsolute, Type: listing.WorkingDirectoryType}, nil
	}
	index, err := strconv.Atoi(name)
	if err != nil || index < 0 || index >= len(listing.Entries) {
		return jobstatus.ArtifactEntry{}, fmt.Errorf("entry must name an artifact candidate of the attempt, or %s for its working directory", artifactWorkingDirectoryEntry)
	}
	return listing.Entries[index], nil
}

// servedArtifact is an entry, or a child of a listed directory, that the
// live server may serve: the root it is under and its path relative to it.
type servedArtifact struct {
	root     string
	relative string
}

func (served servedArtifact) name() string {
	return filepath.Base(filepath.Join(served.root, served.relative))
}

// open opens the artifact through os.Root, which refuses a path that
// leaves the root, including through a symlink.
func (served servedArtifact) open() (*os.File, error) {
	root, err := os.OpenRoot(served.root)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return root.Open(served.relative)
}

var errNotServed = errors.New("not served")

// resolveArtifact finds what a content request names: an entry of the
// attempt's recorded listing, by index, or its recorded working directory,
// and optionally a child path inside a listed directory. There is no path
// parameter, so only recorded paths and their children can be requested,
// and only under an allowed root.
func (s site) resolveArtifact(baseDir string, query url.Values) (servedArtifact, error) {
	resolved, err := s.resolveArtifactRequest(baseDir, query)
	return resolved.servedArtifact, err
}

// resolvedArtifact is what resolveArtifact found, with the listing it came
// from and the roots it was checked against.
type resolvedArtifact struct {
	servedArtifact
	listing jobstatus.ArtifactListing
	entry   jobstatus.ArtifactEntry
	roots   []string
}

func (s site) resolveArtifactRequest(baseDir string, query url.Values) (resolvedArtifact, error) {
	var listing jobstatus.ArtifactListing
	var entry jobstatus.ArtifactEntry
	var err error
	if query.Get("entry") == artifactBasedirParentEntry {
		entry, err = basedirParentEntry(baseDir, query)
	} else {
		listing, err = webArtifacts(s.Store, baseDir, query.Get("project_name"), query.Get("run_id"), query.Get("job_id"), query.Get("attempt_id"))
		if err == nil {
			entry, err = listingEntry(listing, query.Get("entry"))
		}
	}
	if err != nil {
		return resolvedArtifact{}, err
	}
	roots, note := s.artifactRoots(listing, baseDir, query.Get("project_name"), query.Get("run_id"))
	root, relative, inside := tableview.RootOf(entry.Path, roots)
	if entry.Basis == artifact.BasisUnresolved || !inside {
		if note != "" {
			return resolvedArtifact{}, fmt.Errorf("%w: %s is not served. %s", errNotServed, entry.Path, note)
		}
		return resolvedArtifact{}, fmt.Errorf("%w: %s is outside the allowed roots (the job's working directory, the basedir's parent, and --artifact-root)", errNotServed, entry.Path)
	}
	if child := query.Get("child"); child != "" {
		if filepath.IsAbs(child) || filepath.Clean(child) != child || child == ".." || strings.HasPrefix(child, ".."+string(filepath.Separator)) || strings.ContainsRune(child, 0) {
			return resolvedArtifact{}, fmt.Errorf("child must be a relative path inside the directory")
		}
		relative = filepath.Join(relative, child)
	}
	return resolvedArtifact{servedArtifact: servedArtifact{root: root, relative: relative}, listing: listing, entry: entry, roots: roots}, nil
}

// basedirParentEntry is artifactBasedirParentEntry for an existing run of
// the basedir, as an entry resolved like a listed directory.
func basedirParentEntry(baseDir string, query url.Values) (jobstatus.ArtifactEntry, error) {
	if query.Get("job_id") != "" || query.Get("attempt_id") != "" {
		return jobstatus.ArtifactEntry{}, fmt.Errorf("entry %q names the directory holding the basedir: give project_name and run_id, and no job_id or attempt_id", artifactBasedirParentEntry)
	}
	paths, err := stateinternal.ResolveProjectPaths(baseDir, query.Get("project_name"))
	if err != nil {
		return jobstatus.ArtifactEntry{}, err
	}
	runDir, err := stateinternal.SafeJoin(paths.RunsDir, query.Get("run_id"))
	if err != nil {
		return jobstatus.ArtifactEntry{}, err
	}
	if _, err := os.Stat(runDir); err != nil {
		return jobstatus.ArtifactEntry{}, fmt.Errorf("run %s of project %s: %w", query.Get("run_id"), query.Get("project_name"), err)
	}
	parent, ok := basedirParentRoot(baseDir)
	if !ok {
		return jobstatus.ArtifactEntry{}, fmt.Errorf("%w: the basedir %s is directly under the filesystem root", errNotServed, baseDir)
	}
	return jobstatus.ArtifactEntry{Path: parent, DisplayPath: parent, Basis: artifact.BasisAbsolute, Type: jobstatus.ArtifactDirectory}, nil
}

// noStore marks an artifact content response as never cached, and keeps a
// browser from sniffing or running it.
func noStore(writer http.ResponseWriter) {
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
}

// serveArtifactFile streams a file: an image, audio, or video file inline,
// with range requests so media can seek; anything else, or any file with
// download=1, as an attachment.
func (s site) serveArtifactFile(baseDir string, writer http.ResponseWriter, request *http.Request) {
	served, err := s.resolveArtifact(baseDir, request.URL.Query())
	if err != nil {
		writeArtifactError(writer, err)
		return
	}
	file, err := served.open()
	if err != nil {
		writeArtifactError(writer, err)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		writeArtifactError(writer, err)
		return
	}
	if !info.Mode().IsRegular() {
		writeWebError(writer, fmt.Errorf("%s is not a regular file", served.name()))
		return
	}
	noStore(writer)
	// Images, audio, and video are served inline, for img, audio, and video
	// elements.
	contentType, inline := tableview.MediaTypes[strings.ToLower(filepath.Ext(served.name()))]
	if !inline || request.URL.Query().Get("download") == "1" {
		contentType = "application/octet-stream"
		writer.Header().Set("Content-Disposition", "attachment; filename="+strconv.Quote(served.name()))
	}
	writer.Header().Set(headerContentType, contentType)
	http.ServeContent(writer, request, "", info.ModTime(), file)
}

// artifactText is one page of a text file: bytes [Start, End) of Size, cut
// at line boundaries except at the ends of the file.
type artifactText struct {
	Text  string `json:"text"`
	Start int64  `json:"start"`
	End   int64  `json:"end"`
	Size  int64  `json:"size"`
}

// serveArtifactText returns one page of a text file: from=start reads
// forward from offset, from=end reads backward to offset (the file's end by
// default). A file with a NUL byte is binary and is not shown as text.
func (s site) serveArtifactText(baseDir string, writer http.ResponseWriter, request *http.Request) {
	query := request.URL.Query()
	served, err := s.resolveArtifact(baseDir, query)
	if err != nil {
		writeArtifactError(writer, err)
		return
	}
	file, err := served.open()
	if err != nil {
		writeArtifactError(writer, err)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		writeArtifactError(writer, err)
		return
	}
	if !info.Mode().IsRegular() {
		writeWebError(writer, fmt.Errorf("%s is not a regular file", served.name()))
		return
	}
	page, err := readTextPage(file, info.Size(), query.Get("from"), query.Get("offset"))
	if err != nil {
		writeWebError(writer, err)
		return
	}
	noStore(writer)
	writeWebJSON(writer, page)
}

func readTextPage(file io.ReaderAt, size int64, from, offsetText string) (artifactText, error) {
	offset := int64(-1)
	if offsetText != "" {
		value, err := strconv.ParseInt(offsetText, 10, 64)
		if err != nil || value < 0 || value > size {
			return artifactText{}, fmt.Errorf("offset must be within the file")
		}
		offset = value
	}
	var start, end int64
	switch from {
	case "", "start":
		start = max(offset, 0)
		end = min(start+artifactTextChunk, size)
	case "end":
		end = size
		if offset >= 0 {
			end = offset
		}
		start = max(end-artifactTextChunk, 0)
	default:
		return artifactText{}, fmt.Errorf("from must be start or end")
	}
	data := make([]byte, end-start)
	if _, err := file.ReadAt(data, start); err != nil && !errors.Is(err, io.EOF) {
		return artifactText{}, err
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return artifactText{}, fmt.Errorf("binary file: download it instead")
	}
	// Keep whole lines: drop a partial last line when reading forward and a
	// partial first line when reading backward, unless at the file's end.
	if from != "end" && end < size {
		if cut := bytes.LastIndexByte(data, '\n'); cut >= 0 {
			data, end = data[:cut+1], start+int64(cut+1)
		}
	}
	if from == "end" && start > 0 {
		if cut := bytes.IndexByte(data, '\n'); cut >= 0 {
			data, start = data[cut+1:], start+int64(cut+1)
		}
	}
	return artifactText{Text: string(data), Start: start, End: end, Size: size}, nil
}

// artifactChild is one entry of a directory page.
type artifactChild struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Size     int64  `json:"size,omitempty"`
	Modified string `json:"modified,omitempty"`
}

// artifactDirectory is one page of a directory's children.
type artifactDirectory struct {
	Entries []artifactChild `json:"entries"`
	Offset  int             `json:"offset"`
	Total   int             `json:"total"`
	// Truncated reports a directory with more than artifactDirectoryScan
	// children, of which only the first ones read are listed.
	Truncated bool `json:"truncated,omitempty"`
}

// serveArtifactDirectory lists one page of a directory's immediate
// children, directories first and then by name. Names are read up to
// artifactDirectoryScan; size and time only for the page.
func (s site) serveArtifactDirectory(baseDir string, writer http.ResponseWriter, request *http.Request) {
	query := request.URL.Query()
	served, err := s.resolveArtifact(baseDir, query)
	if err != nil {
		writeArtifactError(writer, err)
		return
	}
	directory, err := served.open()
	if err != nil {
		writeArtifactError(writer, err)
		return
	}
	defer directory.Close()
	offset := 0
	if value := query.Get("offset"); value != "" {
		if offset, err = strconv.Atoi(value); err != nil || offset < 0 {
			writeWebError(writer, fmt.Errorf("offset must be a non-negative integer"))
			return
		}
	}
	page, err := readDirectoryPage(directory, offset)
	if err != nil {
		writeArtifactError(writer, err)
		return
	}
	noStore(writer)
	writeWebJSON(writer, page)
}

func readDirectoryPage(directory *os.File, offset int) (artifactDirectory, error) {
	var children []fs.DirEntry
	truncated := false
	for {
		batch, err := directory.ReadDir(1024)
		children = append(children, batch...)
		if len(children) > artifactDirectoryScan {
			children, truncated = children[:artifactDirectoryScan], true
			break
		}
		if errors.Is(err, io.EOF) || (err == nil && len(batch) == 0) {
			break
		}
		if err != nil {
			return artifactDirectory{}, err
		}
	}
	slices.SortFunc(children, func(a, b fs.DirEntry) int {
		if a.IsDir() != b.IsDir() {
			if a.IsDir() {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Name(), b.Name())
	})
	page := artifactDirectory{Entries: []artifactChild{}, Offset: offset, Total: len(children), Truncated: truncated}
	for _, child := range children[min(offset, len(children)):min(offset+artifactDirectoryPage, len(children))] {
		entry := artifactChild{Name: child.Name(), Type: childType(child.Type())}
		if info, err := child.Info(); err == nil {
			if info.Mode().IsRegular() {
				entry.Size = info.Size()
			}
			entry.Modified = info.ModTime().UTC().Format(time.RFC3339)
		}
		page.Entries = append(page.Entries, entry)
	}
	return page, nil
}

func childType(mode fs.FileMode) string {
	switch {
	case mode.IsDir():
		return jobstatus.ArtifactDirectory
	case mode&fs.ModeSymlink != 0:
		return "symlink"
	case mode.IsRegular():
		return jobstatus.ArtifactFile
	}
	return jobstatus.ArtifactOther
}

// writeArtifactError answers a refused or failed content or Table viewer
// request: 403 for a path outside the allowed roots or escaping one, 404
// for a missing file.
func writeArtifactError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errNotServed), errors.Is(err, tableview.ErrOutsideRoots):
		http.Error(writer, err.Error(), http.StatusForbidden)
	case errors.Is(err, fs.ErrNotExist):
		http.Error(writer, "artifact not found", http.StatusNotFound)
	case strings.Contains(err.Error(), "path escapes from parent"):
		http.Error(writer, "the path leaves its allowed root through a symlink", http.StatusForbidden)
	default:
		writeWebError(writer, err)
	}
}
