package main

import (
	"errors"
	"os"

	variant "github.com/mumoshu/variant2"
)

func main() {
	m := variant.New()

	err := m.Run()

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
