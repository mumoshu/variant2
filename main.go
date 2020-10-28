package variant

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/mumoshu/variant2/pkg/controller"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
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
			buf := &bytes.Buffer{}

			outRead, outWrite := io.Pipe()
			outBuf := io.TeeReader(outRead, buf)

			errRead, errWrite := io.Pipe()
			errBuf := io.TeeReader(errRead, buf)

			eg := &errgroup.Group{}

			eg.Go(func() error {
				if _, err := io.Copy(os.Stdout, outBuf); err != nil {
					return xerrors.Errorf("copying to stdout: %w", err)
				}

				return nil
			})

			eg.Go(func() error {
				if _, err := io.Copy(os.Stderr, errBuf); err != nil {
					return xerrors.Errorf("copying to stderr: %w", err)
				}

				return nil
			})

			err := m.Run(args, RunOptions{
				Stdout:         outWrite,
				Stderr:         errWrite,
				DisableLocking: false,
			})

			outWrite.Close()
			errWrite.Close()

			if egErr := eg.Wait(); egErr != nil {
				panic(egErr)
			}

			if err != nil {
				return "", xerrors.Errorf("running command: %w", err)
			}

			return buf.String(), err
		})
	}

	return m.Run(args, RunOptions{DisableLocking: false})
}
