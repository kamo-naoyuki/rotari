package webui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Limits on the artifact contents a static export copies with
// --static-artifact-contents: an export's size is its readers' download.
const (
	staticArtifactFileLimit  = 10 << 20
	staticArtifactTotalLimit = 100 << 20
	staticArtifactTextLimit  = 1 << 20
	// staticArtifactDirectory is where copied files go in the export.
	staticArtifactDirectory = "artifact-files"
)

// staticArtifactData is what a static export embeds so artifact previews
// work without a server: each content route's first answer, the copied file
// of an entry, and an entry's size and time for a file without a preview.
// Keys are staticArtifactEntryKey values.
type staticArtifactData struct {
	Pages map[string]json.RawMessage    `json:"pages,omitempty"`
	Files map[string]string             `json:"files,omitempty"`
	Meta  map[string]staticArtifactMeta `json:"meta,omitempty"`
}

type staticArtifactMeta struct {
	Size     int64  `json:"size"`
	Modified string `json:"modified"`
}

// staticArtifactEntryKey identifies one entry of an attempt's listing,
// matching staticArtifactEntryKey in web_static_bootstrap.js.
func staticArtifactEntryKey(projectName, runID, jobID, attemptID string, entry int) string {
	return staticArtifactsKey(projectName, runID, jobID, attemptID) + "/" + strconv.Itoa(entry) + "/"
}

// staticArtifactCollector gathers artifact contents for a static export
// under the live server's rules, with the job's working directory as the
// only root.
type staticArtifactCollector struct {
	site    site
	baseDir string
	data    staticArtifactData
	// copies maps an export path to what it copies; byPath reuses one copy
	// for a file several attempts list.
	copies map[string]servedArtifact
	byPath map[string]string
	bytes  int64
}

func newStaticArtifactCollector(s site, baseDir string) *staticArtifactCollector {
	return &staticArtifactCollector{site: s, baseDir: baseDir, copies: map[string]servedArtifact{}, byPath: map[string]string{},
		data: staticArtifactData{Pages: map[string]json.RawMessage{}, Files: map[string]string{}, Meta: map[string]staticArtifactMeta{}}}
}

// collect embeds what previews of one attempt's listing need and returns
// which entries the export can preview.
func (c *staticArtifactCollector) collect(projectName, runID, jobID, attemptID string, listing webArtifactListing) []bool {
	previewable := make([]bool, len(listing.Entries))
	for index := range listing.Entries {
		query := url.Values{"project_name": {projectName}, "run_id": {runID}, "job_id": {jobID}, "attempt_id": {attemptID}, "entry": {strconv.Itoa(index)}}
		served, err := c.site.resolveArtifact(c.baseDir, query)
		if err != nil {
			continue
		}
		previewable[index] = c.collectEntry(staticArtifactEntryKey(projectName, runID, jobID, attemptID, index), served)
	}
	return previewable
}

func (c *staticArtifactCollector) collectEntry(key string, served servedArtifact) bool {
	file, err := served.open()
	if err != nil {
		return false
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return false
	}
	if info.IsDir() {
		page, err := readDirectoryPage(file, 0)
		return err == nil && c.page(key, "directory", page)
	}
	if !info.Mode().IsRegular() {
		return false
	}
	c.data.Meta[key] = staticArtifactMeta{Size: info.Size(), Modified: info.ModTime().UTC().Format(time.RFC1123)}
	source := filepath.Join(served.root, served.relative)
	copied, ok := c.byPath[source]
	if !ok {
		if info.Size() > staticArtifactFileLimit || c.bytes+info.Size() > staticArtifactTotalLimit {
			return false
		}
		copied = fmt.Sprintf("%s/%d%s", staticArtifactDirectory, len(c.copies), strings.ToLower(filepath.Ext(source)))
		c.copies[copied] = served
		c.byPath[source] = copied
		c.bytes += info.Size()
	}
	c.data.Files[key] = copied
	switch strings.ToLower(filepath.Ext(source)) {
	case ".npy":
		if array, err := readNumpyArray(file); err == nil {
			c.page(key, "array", artifactArrays{Arrays: []artifactArray{array}})
		}
	case ".npz":
		if arrays, err := readNumpyArchive(file, info.Size()); err == nil {
			c.page(key, "array", arrays)
		}
	}
	if info.Size() <= staticArtifactTextLimit {
		data := make([]byte, info.Size())
		if _, err := file.ReadAt(data, 0); err == nil || err == io.EOF {
			if bytes.IndexByte(data, 0) < 0 {
				c.page(key, "text", artifactText{Text: string(data), Start: 0, End: info.Size(), Size: info.Size()})
			}
		}
	}
	return true
}

func (c *staticArtifactCollector) page(key, route string, value any) bool {
	data, err := json.Marshal(value)
	if err != nil {
		return false
	}
	c.data.Pages[key+route] = data
	return true
}

// write copies the collected files into the export through os.Root, as the
// live server reads them, and returns how many files and bytes it copied.
func (c *staticArtifactCollector) write(outputDir string) (int, int64, error) {
	var total int64
	for copied, served := range c.copies {
		source, err := served.open()
		if err != nil {
			return 0, 0, err
		}
		target := filepath.Join(outputDir, filepath.FromSlash(copied))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			source.Close()
			return 0, 0, err
		}
		output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
		if err != nil {
			source.Close()
			return 0, 0, err
		}
		written, err := io.Copy(output, io.LimitReader(source, staticArtifactFileLimit))
		source.Close()
		if closeErr := output.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return 0, 0, err
		}
		total += written
	}
	return len(c.copies), total, nil
}
