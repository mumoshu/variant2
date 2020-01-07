package app

import (
	"os"
	"testing"
)

func TestExampleComplex(t *testing.T) {
	app, err := New("../../examples/complex")
	app.Stdout = os.Stdout
	app.Stderr = os.Stderr

	if err != nil {
		app.ExitWithError(err)
	}

	testcases := []struct {
		cmd  string
		args map[string]interface{}
		opts map[string]interface{}
	}{
		{
			cmd: "",
			args: map[string]interface{}{
				"param1": "param1v",
			},
			opts: map[string]interface{}{
				"opt1": "opt1",
			},
		},
		{
			cmd: "cmd1",
			args: map[string]interface{}{
				"param1": "param1v",
				"param2": "param2",
				"param3": "param3",
			},
			opts: map[string]interface{}{
				"opt1": "opt1",
			},
		},
		{
			cmd: "bar baz",
			args: map[string]interface{}{
				"param1": "param1v",
				"param2": "param2",
				"param3": "param3",
			},
			opts: map[string]interface{}{
				"opt1": "opt1",
			},
		},
	}

	for _, tc := range testcases {
		cmd := tc.cmd
		args := tc.args
		opts := tc.opts

		if _, err := app.Run(cmd, args, opts); err != nil {
			app.ExitWithError(err)
		}
	}
}
