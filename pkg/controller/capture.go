package controller

import (
	"bytes"
	"io"
	"os"

	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
)

func CaptureOutput(f func(io.Writer, io.Writer) error) (string, error) {
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

	err := f(outWrite, errWrite)

	outWrite.Close()
	errWrite.Close()

	if egErr := eg.Wait(); egErr != nil {
		panic(egErr)
	}

	if err != nil {
		return "", xerrors.Errorf("running command: %w", err)
	}

	return buf.String(), nil
}
