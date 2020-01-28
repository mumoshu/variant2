package app

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/hashicorp/hcl/v2"

	"github.com/mumoshu/hcl2test/pkg/conf"
)

func (app *App) ExportShim(srcDir, dstDir string) error {
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return err
	}

	files, _, err := newConfigFromDir(srcDir)
	if err != nil {
		return err
	}

	var binName string
	if app.BinName != "" {
		binName = app.BinName
	} else {
		binName = "variant"
	}

	return exportWithShim(binName, files, dstDir)
}

func exportWithShim(variantBin string, files map[string]*hcl.File, dstDir string) error {
	binName := filepath.Base(dstDir)

	binPath := filepath.Join(dstDir, binName)
	cfgPath := filepath.Join(dstDir, binName+conf.VariantFileExt)

	buf := bytes.Buffer{}

	for _, file := range files {
		buf.Write(file.Bytes)
		buf.Write([]byte("\n"))
	}

	if err := ioutil.WriteFile(cfgPath, buf.Bytes(), 0644); err != nil {
		return err
	}

	return generateShim(variantBin, binPath)
}

func GenerateShim(variantBin, dir string) error {
	binName := filepath.Base(dir)
	binPath := filepath.Join(dir, binName)

	return generateShim(variantBin, binPath)
}

func generateShim(variantBin string, path string) error {
	shimData := []byte(fmt.Sprintf(`#!/usr/bin/env %s

import = "."
`, variantBin))

	return ioutil.WriteFile(path, shimData, 0755)
}
