package bugreport

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"time"
)

// WriteTarGz writes bundle to a new file at target as a gzip-compressed tar
// archive under a top directory named root. The summary is root/README.md.
// An existing file is not replaced.
func WriteTarGz(target, root string, bundle Bundle, modified time.Time) (err error) {
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			_ = os.Remove(target)
		}
	}()
	compressed := gzip.NewWriter(file)
	archive := tar.NewWriter(compressed)
	entries := append([]File{{Name: "README.md", Data: []byte(bundle.Summary)}}, bundle.Files...)
	for _, entry := range entries {
		name, err := archiveName(root, entry.Name)
		if err != nil {
			return err
		}
		header := &tar.Header{Name: name, Mode: 0o600, Size: int64(len(entry.Data)), ModTime: modified, Typeflag: tar.TypeReg, Format: tar.FormatPAX}
		if err := archive.WriteHeader(header); err != nil {
			return err
		}
		if _, err := archive.Write(entry.Data); err != nil {
			return err
		}
	}
	return errors.Join(archive.Close(), compressed.Close())
}

func archiveName(root, name string) (string, error) {
	cleaned := path.Clean(root + "/" + name)
	if strings.Contains(name, "\\") || path.IsAbs(name) || !strings.HasPrefix(cleaned, root+"/") {
		return "", fmt.Errorf("invalid bundle entry name %q", name)
	}
	return cleaned, nil
}
