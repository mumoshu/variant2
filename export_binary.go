package variant

import (
	"github.com/spf13/cobra"
)

func newExportBinary(r *Runner) *cobra.Command {
	return &cobra.Command{
		Use:   "binary SRC_DIR DST_FILE",
		Short: "Builds the single executable binary of the Variant command defined in SRC_DIR. Basically `variant export go SRC_DIR TMP_DIR && go build -o DST_FILE TMP_DIR`",
		Example: `$ variant export binary examples/simple build/simple

$ build/simple -h

$ build/simple app deploy -n default
`,
		Args: cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			err := r.ap.ExportBinary(args[0], args[1])
			if err != nil {
				c.SilenceUsage = true
			}
			return err
		},
	}
}
