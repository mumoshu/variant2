package fs

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/rakyll/statik/fs"
	"golang.org/x/xerrors"
)

const (
	VendorPrefix = "vendored"
)

type FileSystem struct {
	sync.Once
	fs http.FileSystem
}

type noopFS struct {
}

func (f *noopFS) Open(_ string) (http.File, error) {
	return nil, os.ErrNotExist
}

func (s *FileSystem) ReadFile(path string) ([]byte, error) {
	fs, err := s.getFS()
	if err != nil {
		return nil, err
	}

	f, err := fs.Open(s.vendored(path))
	if errors.Is(err, os.ErrNotExist) {
		return ioutil.ReadFile(path)
	}
	defer f.Close()

	bs, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("reading statik file: %w", err)
	}

	return bs, nil
}

func (s *FileSystem) Stat(path string) (os.FileInfo, error) {
	fs, err := s.getFS()
	if err != nil {
		return nil, err
	}

	f, err := fs.Open(s.vendored(path))
	if errors.Is(err, os.ErrNotExist) {
		return os.Stat(path)
	}
	defer f.Close()

	return f.Stat()
}

func (s *FileSystem) Glob(pattern string) ([]string, error) {
	fs, err := s.getFS()
	if err != nil {
		return nil, err
	}

	dir, _ := filepath.Split(s.vendored(pattern))

	found, err := glob(fs, dir, s.vendored(pattern))
	if err != nil {
		return nil, fmt.Errorf("glob using statik: %w", err)
	}

	if len(found) > 0 {
		return found, nil
	}

	found, err = filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob using filepath: %w", err)
	}

	return found, nil
}

func (s *FileSystem) getFS() (http.FileSystem, error) {
	var err error

	s.Once.Do(func() {
		s.fs, err = fs.New()
		if err != nil {
			s.fs = &noopFS{}
			err = nil
		}
	})

	if err != nil {
		return nil, err
	}

	return s.fs, nil
}

func (s *FileSystem) vendored(path string) string {
	return filepath.Join(string(filepath.Separator), path)
}

func glob(fs http.FileSystem, dir, pattern string) ([]string, error) {
	d, err := fs.Open(dir)
	if err != nil {
		return nil, nil
	}
	defer d.Close()

	fi, err := d.Stat()
	if err != nil {
		return nil, nil
	}

	if !fi.IsDir() {
		return nil, nil
	}

	entries, err := d.Readdir(-1)
	if err != nil {
		return nil, fmt.Errorf("readdir: %w", err)
	}

	var names []string

	for _, ent := range entries {
		if ent.IsDir() {
			subEntries, err := glob(fs, "/"+ent.Name(), pattern)
			if err != nil {
				return nil, err
			}

			names = append(names, subEntries...)
		} else {
			names = append(names, filepath.Join(dir, ent.Name()))
		}
	}

	sort.Strings(names)

	var m []string

	for _, n := range names {
		matched, err := filepath.Match(pattern, n)
		if err != nil {
			return m, xerrors.Errorf("matching pattern %s against %s: %w", pattern, n, err)
		}

		if matched {
			m = append(m, n)
		}
	}

	return m, nil
}
