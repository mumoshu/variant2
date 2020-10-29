package variant

import (
	"github.com/spf13/cobra"
)

func newExportGo(r *Runner) *cobra.Command {
	return &cobra.Command{
		Use:   "go SRC_DIR DST_DIR",
		Short: "Copy and generate DST_DIR/main.go for building the single executable binary with `go build DST_DIR`",
		Example: `$ variant export go examples/simple build

$ go build -o build/simple ./build

$ build/simple -h

$ build/simple app deploy -n default
`,
		Args: cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			err := r.ap.ExportGo(args[0], args[1])
			if err != nil {
				c.SilenceUsage = true
			}

			//nolint:wrapcheck
			return err
		},
	}
}
