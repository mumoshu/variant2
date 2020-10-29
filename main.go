package variant

import (
	"fmt"
	"github.com/mumoshu/variant2/pkg/controller"
	"io"
)

func RunMain(env Env, opts ...Option) error {
	cmd, path, args := GetPathAndArgsFromEnv(env)

	m, err := Load(FromPath(path, func(m *Main) {
		m.Command = cmd

		for _, o := range opts {
			o(m)
		}
	}))
	if err != nil {
		return fmt.Errorf("loading command: %w", err)
	}

	if controller.RunRequested() {
		return controller.Run(func(args []string) (string, error) {
			out, err := controller.CaptureOutput(func(stdout, stderr io.Writer) error {
				return m.Run(args, RunOptions{
					Stdout:         stdout,
					Stderr:         stdout,
					DisableLocking: false,
				})
			})

			return out, err
		})
	}

	return m.Run(args, RunOptions{DisableLocking: false})
}
