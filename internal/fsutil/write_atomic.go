// Package fsutil contains file-system helper utilities.
package fsutil

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// WriteFileAtomic writes a complete file by first writing into a temporary file
// in the same directory and then replacing the destination path.
func WriteFileAtomic(path string, data []byte, perm fs.FileMode) (retErr error) {
	cleanPath := filepath.Clean(path)
	dir := filepath.Dir(cleanPath)
	base := filepath.Base(cleanPath)

	tmp, err := os.CreateTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	tmpPath := tmp.Name()

	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	err = tmp.Chmod(perm)
	if err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}

	_, err = tmp.Write(data)
	if err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}

	err = tmp.Sync()
	if err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temp file: %w", err)
	}

	err = tmp.Close()
	if err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	err = os.Rename(tmpPath, cleanPath)
	if err != nil {
		return fmt.Errorf("replace destination file: %w", err)
	}

	cleanup = false

	return nil
}
