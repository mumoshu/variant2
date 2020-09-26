package app

import (
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/hashicorp/hcl/v2/ext/typeexpr"
	"github.com/mumoshu/variant2/pkg/conf"
	"io/ioutil"
	"os"
	"path/filepath"
)

func (app *App) ExportFlattened(srcDir, dstDir string) error {
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return err
	}

	a, err := New(FromDir(srcDir))
	if err != nil {
		return err
	}

	f := hclwrite.NewEmptyFile()

	{
		rootBody := f.Body()

		for _, j := range a.JobByName {
			if j.Name == "" {
				continue
			}
			rootBody.AppendNewline()
			jobBlock := rootBody.AppendNewBlock("job", []string{j.Name})
			jobBody := jobBlock.Body()

			for i, o := range j.Options {
				if i != 0 {
					jobBody.AppendNewline()
				}
				optBlock := jobBody.AppendNewBlock("option", []string{o.Name})
				tpe, diagnostics := typeexpr.TypeConstraint(o.Type)
				if diagnostics.HasErrors() {
					return diagnostics
				}
				v := typeexpr.TypeConstraintVal(tpe)
				optBlock.Body().SetAttributeValue("type", v)
			}

			for i, p := range j.Parameters {
				if i != 0 {
					jobBody.AppendNewline()
				}
				paramBlock := jobBody.AppendNewBlock("parameter", []string{p.Name})
				tpe, diagnostics := typeexpr.TypeConstraint(p.Type)
				if diagnostics.HasErrors() {
					return diagnostics
				}
				v := typeexpr.TypeConstraintVal(tpe)
				paramBlock.Body().SetAttributeValue("type", v)
			}
		}
	}

	tokens := f.BuildTokens(nil)
	raw := tokens.Bytes()
	formatted := hclwrite.Format(raw)

	binName := filepath.Base(dstDir)

	cfgPath := filepath.Join(dstDir, binName+conf.VariantFileExt)

	if err := ioutil.WriteFile(cfgPath, formatted, 0644); err != nil {
		return err
	}

	return nil
}
