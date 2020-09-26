package variant

import (
	"github.com/spf13/cobra"
)

func newExportFlattened(r *Runner) *cobra.Command {
	return &cobra.Command{
		Use:   "flattened SRC_DIR DST_DIR",
		Short: "Process all the imports and export the \"flattened\" variant command to DST_DIR",
		Example: `$ variant export embedded examples/simple flattened

$ cd flattened

$ variant run -h
`,
		Args: cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			err := r.ap.ExportFlattened(args[0], args[1])
			if err != nil {
				c.SilenceUsage = true
			}
			return err
		},
	}
}
