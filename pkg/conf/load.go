package conf

import (
	"path/filepath"

	"github.com/mumoshu/variant2/pkg/fs"
)

const (
	VariantFileExt = ".variant"
)

// FindVariantFiles walks the given path and returns the files ending whose ext is .variant
// Also, it returns the path if the path is just a file and a HCL file
func FindVariantFiles(fs *fs.FileSystem, path string) ([]string, error) {
	var (
		files []string
		err   error
	)

	fi, err := fs.Stat(path)
	if err != nil {
		return files, err
	}

	if fi.IsDir() {
		found, err := fs.Glob(filepath.Join(path, "*"+VariantFileExt+"*"))
		if err != nil {
			return nil, err
		}

		for _, f := range found {
			switch filepath.Ext(f) {
			case VariantFileExt, ".json":
			default:
				continue
			}

			info, err := fs.Stat(f)

			if err != nil {
				return nil, err
			}

			if info.IsDir() {
				continue
			}

			files = append(files, f)
		}

		return files, nil
	}

	switch filepath.Ext(path) {
	case VariantFileExt, ".json":
		files = append(files, path)
	}

	return files, err
}
