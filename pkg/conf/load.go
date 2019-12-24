package conf

import (
	"os"
	"path/filepath"
)

// FindHCLFiles walks the given path and returns the files ending whose ext is .hcl
// Also, it returns the path if the path is just a file and a HCL file
func FindHCLFiles(path string) ([]string, error) {
	var (
		files []string
		err   error
	)
	fi, err := os.Stat(path)
	if err != nil {
		return files, err
	}
	if fi.IsDir() {
		return filepath.Glob(filepath.Join(path, "*.hcl"))
	}
	switch filepath.Ext(path) {
	case ".hcl":
		files = append(files, path)
	}
	return files, err
}
