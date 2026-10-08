package artifactcapture

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DirectoryWriter is an explicit raw-artifact filesystem adapter. Use it only
// through one Collector. Assembly owns directory access, retention and consent;
// lexical filenames are not a physical sandbox or a permission boundary.
type DirectoryWriter struct {
	directory        string
	maxRetainedBytes int64
	prefix           string
}

// NewDirectoryWriter explicitly owns matching regular .raw files in directory.
// An empty prefix selects all regular .raw files in a dedicated directory,
// including artifacts from older naming schemes. Foreign extensions and symlinks
// remain outside retention ownership.
func NewDirectoryWriter(directory, prefix string, maxRetainedBytes int64) (*DirectoryWriter, error) {
	if !filepath.IsAbs(directory) || (prefix != "" && !validName(prefix)) || maxRetainedBytes <= 0 {
		return nil, errors.New("absolute artifact directory, valid optional filename prefix and positive retention budget required")
	}
	if err := validateDirectory(directory); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	if err := validateDirectory(directory); err != nil {
		return nil, err
	}
	if err := os.Chmod(directory, 0700); err != nil {
		return nil, err
	}
	writer := &DirectoryWriter{directory: filepath.Clean(directory), prefix: prefix, maxRetainedBytes: maxRetainedBytes}
	if err := writer.prune(""); err != nil {
		return nil, err
	}
	return writer, nil
}
func (w *DirectoryWriter) WriteArtifact(name string, data []byte) error {
	if !strings.HasSuffix(name, ".raw") || !validName(name) || len(name) < len(w.prefix) || name[:len(w.prefix)] != w.prefix {
		return errors.New("artifact name outside configured prefix")
	}
	if int64(len(data)) > w.maxRetainedBytes {
		return errors.New("artifact exceeds directory retention budget")
	}
	path := filepath.Join(w.directory, name)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = file.Write(data)
	if err == nil {
		info, e := file.Stat()
		err = e
		if err == nil && info.Size() != int64(len(data)) {
			err = io.ErrShortWrite
		}
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(path)
		return err
	}
	if err := w.prune(name); err != nil {
		return errors.Join(err, os.Remove(path))
	}
	return nil
}
func (w *DirectoryWriter) prune(current string) error {
	entries, err := os.ReadDir(w.directory)
	if err != nil {
		return err
	}
	type artifact struct {
		name  string
		size  int64
		order int64
	}
	var files []artifact
	var total int64
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".raw") || !entry.Type().IsRegular() || len(name) < len(w.prefix) || name[:len(w.prefix)] != w.prefix {
			continue
		}
		info, e := entry.Info()
		if e != nil {
			return e
		}
		files = append(files, artifact{name, info.Size(), info.ModTime().UnixNano()})
		total += info.Size()
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].order == files[j].order {
			return files[i].name < files[j].name
		}
		return files[i].order < files[j].order
	})
	for _, file := range files {
		if total <= w.maxRetainedBytes {
			break
		}
		if file.name == current {
			continue
		}
		if err := os.Remove(filepath.Join(w.directory, file.name)); err != nil {
			return err
		}
		total -= file.size
	}
	return nil
}

// Reject symlink/non-directory ancestors at construction; this check does not
// claim protection against concurrent filesystem replacement by other actors.
func validateDirectory(directory string) error {
	for current := filepath.Clean(directory); ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err == nil && (info.Mode()&os.ModeSymlink != 0 || !info.IsDir()) {
			return errors.New("artifact directory has symlink or non-directory ancestor")
		}
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if filepath.Dir(current) == current {
			return nil
		}
	}
}
