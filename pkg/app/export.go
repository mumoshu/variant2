package app

import "path/filepath"

func (app *App) moduleName(srcDir string) string {
	return "example.com/" + filepath.Base(srcDir)
}
