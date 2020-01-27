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

	return generateShim(binName, files, dstDir)
}

func generateShim(variantBin string, files map[string]*hcl.File, dstDir string) error {
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

	shimData := []byte(fmt.Sprintf(`#!/usr/bin/env bash

export VARIANT_NAME=$(basename $0)
export VARIANT_DIR=$(dirname $0)

exec %s $@
`, variantBin))

	if err := ioutil.WriteFile(binPath, shimData, 0755); err != nil {
		return err
	}

	return nil
}
