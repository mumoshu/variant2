package app

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"

	"github.com/mumoshu/variant2/pkg/conf"
	"github.com/mumoshu/variant2/pkg/fs"
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

	if err := os.MkdirAll(filepath.Dir(dstFile), 0o755); err != nil {
		return err
	}

	absDstFile, err := filepath.Abs(dstFile)
	if err != nil {
		return err
	}

	_, err = app.execCmd(
		nil,
		Command{
			Name: "sh",
			Args: []string{"-c", fmt.Sprintf("cd %s; go build -o %s %s", tmpDir, absDstFile, tmpDir)},
			Env:  map[string]string{},
		},
		true,
	)

	return err
}

func (app *App) ExportGo(srcDir, dstDir string) error {
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}

	dstVendorDir := filepath.Join(dstDir, fs.VendorPrefix)

	cacheDir := filepath.Join(dstVendorDir, DefaultCacheDir)

	a, err := New(FromDir(srcDir), WithCacheDir(cacheDir))
	if err != nil {
		return err
	}

	merged, err := merge(a.Files)
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
	_ "${MODULE_NAME}/statik"
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

	moduleName := app.moduleName(srcDir)

	replaced := strings.ReplaceAll(string(code), "${MODULE_NAME}", moduleName)
	code = []byte(replaced)

	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}

	exportDir := filepath.Join(dstDir, "main.go")

	//nolint:gosec
	if err := ioutil.WriteFile(exportDir, code, 0o644); err != nil {
		return err
	}

	if err := copyFiles(srcDir, dstVendorDir); err != nil {
		return fmt.Errorf("copy files: %w", err)
	}

	_, err = app.execCmd(
		nil,
		Command{
			Name: "sh",
			Args: []string{"-c", fmt.Sprintf("cd %s; go mod init %s && go get github.com/rakyll/statik && statik -src=%s", dstDir, moduleName, fs.VendorPrefix)},
			Env:  map[string]string{},
		},
		true,
	)
	if err != nil {
		return err
	}

	variantVer := os.Getenv("VARIANT_BUILD_VER")
	if variantVer != "" {
		_, err = app.execCmd(
			nil,
			Command{
				Name: "sh",
				Args: []string{"-c", fmt.Sprintf("cd %s; go mod edit -require=github.com/mumoshu/variant2@%s", dstDir, variantVer)},
				Env:  map[string]string{},
			},
			true,
		)
		if err != nil {
			return err
		}
	}

	variantReplace := os.Getenv("VARIANT_BUILD_VARIANT_REPLACE")
	if variantReplace != "" {
		_, err = app.execCmd(
			nil,
			Command{
				Name: "sh",
				Args: []string{"-c", fmt.Sprintf("cd %s; go mod edit -replace github.com/mumoshu/variant2@%s=%s", dstDir, variantVer, variantReplace)},
				Env:  map[string]string{},
			},
			true,
		)
		if err != nil {
			return err
		}
	}

	var modReplaces []string

	modReplace := os.Getenv("VARIANT_BUILD_MOD_REPLACE")

	if modReplace != "" {
		reps := strings.Split(modReplace, ",")

		modReplaces = append(modReplaces, reps...)
	}

	// Required until https://github.com/summerwind/whitebox-controller/pull/8 is merged
	modReplaces = append(modReplaces, "github.com/summerwind/whitebox-controller@v0.7.1=github.com/mumoshu/whitebox-controller@v0.5.1-0.20201028130131-ac7a0743254b")

	// Required to fix go mod issue that k8s.io/client-go is somehow "updated" to invalid "v10.0.0+incompatible" on build
	modReplaces = append(modReplaces, "k8s.io/client-go@v10.0.0+incompatible=k8s.io/client-go@v0.18.9")

	for _, modReplace := range modReplaces {
		_, err = app.execCmd(
			nil,
			Command{
				Name: "sh",
				Args: []string{"-c", fmt.Sprintf("cd %s; go mod edit -replace %s", dstDir, modReplace)},
				Env:  map[string]string{},
			},
			true,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

func copyFiles(srcDir string, dstDir string) error {
	walkErr := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("walking into %s: %w", path, err)
		}

		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return fmt.Errorf("computing path of %s relative to %s: %w", path, srcDir, err)
		}

		abs := filepath.Join(dstDir, rel)

		if strings.Contains(rel, DefaultCacheDir) {
			fmt.Fprintf(os.Stderr, "Skipping %s\n", rel)

			return nil
		}

		if info.IsDir() {
			return os.MkdirAll(abs, 0o755)
		}

		return copyFile(path, abs)
	})
	if walkErr != nil {
		return fmt.Errorf("copying files from %s: %w", srcDir, walkErr)
	}

	return nil
}

func copyFile(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return
	}

	defer func() {
		cerr := out.Close()

		if err == nil {
			err = cerr
		}
	}()

	if _, err = io.Copy(out, in); err != nil {
		return
	}

	err = out.Sync()

	return
}

func (app *App) ExportShim(srcDir, dstDir string) error {
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}

	a, err := New(FromDir(srcDir))
	if err != nil {
		return err
	}

	var binName string
	if app.BinName != "" {
		binName = app.BinName
	} else {
		binName = "variant"
	}

	return exportWithShim(binName, a.Files, dstDir)
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

	//nolint:gosec
	if err := ioutil.WriteFile(cfgPath, bs, 0o644); err != nil {
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

	//nolint:gosec
	return ioutil.WriteFile(path, shimData, 0o755)
}
