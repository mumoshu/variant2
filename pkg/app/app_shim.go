package app

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"

	"github.com/mumoshu/variant2/pkg/conf"
)

func (app *App) ExportBinary(srcDir, dstFile string) error {
	tmpDir, err := ioutil.TempDir("", "variant-"+filepath.Base(srcDir))
	if err != nil {
		return err
	}

	defer os.RemoveAll(tmpDir)

	if err := app.ExportGo(srcDir, tmpDir); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(dstFile), 0755); err != nil {
		return err
	}

	absDstFile, err := filepath.Abs(dstFile)

	if err != nil {
		return err
	}

	_, err = app.execCmd(
		Command{
			Name: "sh",
			Args: []string{"-c", fmt.Sprintf("cd %s; go mod init %s && go build -o %s %s", tmpDir, filepath.Base(srcDir), absDstFile, tmpDir)},
			Env:  map[string]string{},
		},
		true,
	)

	return err
}

func (app *App) ExportGo(srcDir, dstDir string) error {
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return err
	}

	fs, err := findVariantFiles(srcDir)
	if err != nil {
		return err
	}

	srcs, err := loadFiles(fs...)
	if err != nil {
		return err
	}

	files, _, err := newConfigFromSources(srcs)
	if err != nil {
		return err
	}

	merged, err := merge(files)
	if err != nil {
		return err
	}

	backquote := "<<<backquote>>>"

	code := []byte(fmt.Sprintf(`package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	variant "github.com/mumoshu/variant2"
)

func main() {
	source := strings.Replace(%s, "`+backquote+`", "`+"`"+`", -1)

	var args []string

	if len(os.Args) > 1 {
		args = os.Args[1:]
	}

	bin := filepath.Base(os.Args[0])

	err := variant.MustLoad(variant.FromSource(bin, source)).Run(args)

	var verr variant.Error

	var code int

	if err != nil {
		if ok := errors.As(err, &verr); ok {
			code = verr.ExitCode
		} else {
			code = 1
		}
	} else {
		code = 0
	}

	os.Exit(code)
}
`, "`"+strings.Replace(string(merged)+"\n", "`", backquote, -1)+"`"))

	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return err
	}

	exportDir := filepath.Join(dstDir, "main.go")

	if err := ioutil.WriteFile(exportDir, code, 0644); err != nil {
		return err
	}

	return nil
}

func (app *App) ExportShim(srcDir, dstDir string) error {
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return err
	}

	fs, err := findVariantFiles(srcDir)
	if err != nil {
		return err
	}

	srcs, err := loadFiles(fs...)
	if err != nil {
		return err
	}

	files, _, err := newConfigFromSources(srcs)
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

func merge(files map[string]*hcl.File) ([]byte, error) {
	buf := bytes.Buffer{}

	for _, file := range files {
		if _, err := buf.Write(file.Bytes); err != nil {
			return nil, err
		}

		if _, err := buf.Write([]byte("\n")); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

func exportWithShim(variantBin string, files map[string]*hcl.File, dstDir string) error {
	binName := filepath.Base(dstDir)

	binPath := filepath.Join(dstDir, binName)
	cfgPath := filepath.Join(dstDir, binName+conf.VariantFileExt)

	bs, err := merge(files)
	if err != nil {
		return err
	}

	if err := ioutil.WriteFile(cfgPath, bs, 0644); err != nil {
		return err
	}

	return generateShim(variantBin, binPath)
}

func GenerateShim(variantBin, dir string) error {
	var err error

	dir, err = filepath.Abs(dir)

	if err != nil {
		return err
	}

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
