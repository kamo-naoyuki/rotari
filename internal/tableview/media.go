package tableview

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
)

// Properties describe an image, audio, or video file from its headers,
// without decoding it. A field is zero when the format does not record it
// or the file could not be read that far.
type Properties struct {
	Format string `json:"format"`
	Size   int64  `json:"size"`
	// Duration is in seconds.
	Duration   float64 `json:"duration,omitempty"`
	SampleRate int     `json:"sample_rate,omitempty"`
	Channels   int     `json:"channels,omitempty"`
	BitDepth   int     `json:"bit_depth,omitempty"`
	// Bitrate is in bits per second: the format's own for uncompressed
	// audio and constant-bitrate MP3, otherwise the file's size over its
	// duration.
	Bitrate int `json:"bitrate,omitempty"`
	Width   int `json:"width,omitempty"`
	Height  int `json:"height,omitempty"`
}

// Media is what a cell names: an image, audio, or video file, with its
// properties or why they are not shown. Ref and Base request the file from
// the viewer's media route.
type Media struct {
	Kind       string      `json:"kind"`
	Ref        string      `json:"ref"`
	Base       string      `json:"base,omitempty"`
	Properties *Properties `json:"properties,omitempty"`
	Error      string      `json:"error,omitempty"`
}

// mediaRefLimit bounds the cells read as media paths.
const mediaRefLimit = 4096

// mediaRef returns the path a cell names, or "" when it names no media.
func mediaRef(cell string) string {
	ref := strings.TrimSpace(cell)
	if ref == "" || len(ref) > mediaRefLimit || strings.ContainsAny(ref, "\n\r") || MediaKind(ref) == "" {
		return ""
	}
	return ref
}

// describeMedia resolves the media a cell names and reads its properties.
func (target Target) describeMedia(directory bool, base, ref string) *Media {
	media := &Media{Kind: MediaKind(ref), Ref: ref, Base: base}
	path, err := target.ResolveMedia(directory, base, ref)
	if err == nil {
		media.Properties, err = target.probe(path)
	}
	switch {
	case err == nil:
	case errors.Is(err, ErrOutsideRoots):
		media.Error = "Not shown: outside the allowed roots"
	case errors.Is(err, fs.ErrNotExist):
		media.Error = "File not found"
	case strings.Contains(err.Error(), "path escapes from parent"):
		media.Error = "Not shown: a symlink leaves the allowed roots"
	default:
		media.Error = err.Error()
	}
	return media
}

// propertiesCache keeps properties by path, size, and modification time, so
// paging back and forth does not reread headers.
var propertiesCache = struct {
	sync.Mutex
	entries map[string]Properties
}{entries: map[string]Properties{}}

// propertiesCacheSize bounds propertiesCache; it is emptied when full.
const propertiesCacheSize = 200_000

func (target Target) probe(path string) (*Properties, error) {
	file, err := target.open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("not a regular file")
	}
	key := fmt.Sprintf("%s\x00%d\x00%d", path, info.Size(), info.ModTime().UnixNano())
	propertiesCache.Lock()
	cached, ok := propertiesCache.entries[key]
	propertiesCache.Unlock()
	if ok {
		return &cached, nil
	}
	properties := Probe(file, info.Size(), filepath.Ext(path))
	propertiesCache.Lock()
	if len(propertiesCache.entries) >= propertiesCacheSize {
		propertiesCache.entries = map[string]Properties{}
	}
	propertiesCache.entries[key] = properties
	propertiesCache.Unlock()
	return &properties, nil
}

// Probe reads a media file's properties by its extension. A file whose
// headers do not parse still has its format, from the extension, and size.
func Probe(file io.ReaderAt, size int64, extension string) Properties {
	extension = strings.ToLower(extension)
	properties := Properties{Format: strings.ToUpper(strings.TrimPrefix(extension, ".")), Size: size}
	reader := headerReader{file: file, size: size}
	switch extension {
	case ".wav":
		probeWAV(reader, &properties)
	case ".flac":
		probeFLAC(reader, &properties)
	case ".mp3":
		probeMP3(reader, &properties)
	case ".ogg", ".oga", ".opus":
		probeOgg(reader, &properties)
	case ".m4a", ".mp4", ".mov":
		probeMP4(reader, &properties)
	case ".webm":
		probeWebM(reader, &properties)
	case ".png", ".jpg", ".jpeg", ".gif":
		probeImage(reader, &properties)
	case ".svg":
		probeSVG(reader, &properties)
	}
	if properties.Bitrate == 0 && properties.Duration > 0 && MediaKind(extension) != "image" {
		properties.Bitrate = int(float64(size) * 8 / properties.Duration)
	}
	return properties
}

// headerReader reads bounded pieces of a file.
type headerReader struct {
	file io.ReaderAt
	size int64
}

// maxHeaderRead bounds one read of a header.
const maxHeaderRead = 64 << 20

// read returns up to length bytes at offset, fewer at the file's end.
func (reader headerReader) read(offset, length int64) []byte {
	if offset < 0 || offset >= reader.size || length <= 0 {
		return nil
	}
	length = min(length, reader.size-offset, maxHeaderRead)
	data := make([]byte, length)
	count, _ := reader.file.ReadAt(data, offset)
	return data[:count]
}
