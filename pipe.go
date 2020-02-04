package variant

import (
	"bytes"
	"errors"
	"io"
	"io/ioutil"
)

func Pipe() (func() (*bytes.Buffer, error), io.WriteCloser) {
	r, w := io.Pipe()

	out := &bytes.Buffer{}

	outDone := make(chan error, 1)

	go func() {
		bs, err := ioutil.ReadAll(r)

		if err != nil {
			outDone <- err
			return
		}

		if _, err := out.Write(bs); err != nil {
			outDone <- err
			return
		}

		outDone <- nil
	}()

	return func() (*bytes.Buffer, error) {
		err := <-outDone

		if !errors.Is(err, io.EOF) {
			return nil, err
		}

		return out, nil
	}, w
}
