package artifactsource

import (
	"os"
	"sync"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/artifact"
)

// maxCacheEntries bounds how many file versions one Cache remembers. Files
// beyond it are still parsed, only not remembered.
const maxCacheEntries = 1024

// Cache parses each configuration file once per version for one run, so
// the tasks of an array, the members of a matrix, and retries that name the
// same file share one parse. A version is the file's path, size, and
// modification time; a file that changes during the run is parsed again.
// Parse results are cached with their errors. Read errors are not, since
// the stat that finds them is cheap. A Cache is safe for concurrent use.
type Cache struct {
	mutex   sync.Mutex
	entries map[cacheKey]cacheEntry
}

type cacheKey struct {
	path     string
	size     int64
	modified time.Time
}

type cacheEntry struct {
	references []artifact.ConfigReference
	err        error
}

// NewCache returns an empty Cache.
func NewCache() *Cache {
	return &Cache{entries: map[cacheKey]cacheEntry{}}
}

// References returns the references in the configuration file at path,
// parsing it only when this version has not been parsed before. A file that
// is not regular or is larger than artifact.MaxSourceBytes is rejected from
// its stat, without being read. It is an artifact.ConfigReader.
func (cache *Cache) References(path string) ([]artifact.ConfigReference, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, ErrNotRegular
	}
	if info.Size() > artifact.MaxSourceBytes {
		return nil, tooLarge()
	}
	key := cacheKey{path: path, size: info.Size(), modified: info.ModTime()}
	cache.mutex.Lock()
	entry, ok := cache.entries[key]
	cache.mutex.Unlock()
	if ok {
		return entry.references, entry.err
	}
	data, err := Read(path)
	if err != nil {
		return nil, err
	}
	entry.references, entry.err = artifact.ConfigReferences(artifact.ConfigFormat(path), data)
	cache.mutex.Lock()
	if len(cache.entries) < maxCacheEntries {
		cache.entries[key] = entry
	}
	cache.mutex.Unlock()
	return entry.references, entry.err
}
