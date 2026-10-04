// Package artifactsource reads the configuration files that artifact
// discovery inspects. It is the only file access discovery performs: it
// reads regular files only, never blocks on a FIFO or device, and bounds
// how much it reads. Symbolic links are followed, because configuration
// files are often linked; the target must be a regular file. Reading runs as
// the job's own user, and discovery records only the path references it
// classifies, never the contents.
package artifactsource

import (
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"

	"github.com/kamo-naoyuki/rotari/internal/artifact"
)

// ErrNotRegular is returned for a directory, FIFO, device, or socket.
var ErrNotRegular = errors.New("not a regular file")

// Read returns the contents of the regular file at path, or an error when
// it is not a regular file or is larger than artifact.MaxSourceBytes.
func Read(path string) ([]byte, error) {
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
	// O_NONBLOCK keeps a file swapped for a FIFO after the Stat from
	// blocking the open; the Stat below on the open file decides.
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, ErrNotRegular
	}
	data, err := io.ReadAll(io.LimitReader(file, artifact.MaxSourceBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > artifact.MaxSourceBytes {
		return nil, tooLarge()
	}
	return data, nil
}

func tooLarge() error {
	return fmt.Errorf("larger than %d bytes", artifact.MaxSourceBytes)
}
