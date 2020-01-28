package app

import (
	"bytes"
	"fmt"
	"github.com/hashicorp/hcl/v2"
	"github.com/mumoshu/hcl2test/pkg/conf"
	"io/ioutil"
	"os"
	"path/filepath"
)

func (a *App) ExportShim(srcDir, dstDir string) error {
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return err
	}

	files, _, _, err := newConfigFromDir(srcDir)
	if err != nil {
		return err
	}

	var binName string
	if a.BinName != "" {
		binName = a.BinName
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

func generateShim(variantBin, path string) error {
	shimData := []byte(fmt.Sprintf(`#!/usr/bin/env variant

import = "."
`))

	return ioutil.WriteFile(path, shimData, 0755)
}
