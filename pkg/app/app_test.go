package app

import (
	"fmt"
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
		err  string
	}{
		{
			cmd: "",
			args: map[string]interface{}{
				"param1": "param1v",
			},
			opts: map[string]interface{}{
				"opt1": "opt1",
			},
			err: "nothing to run",
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

	for i := range testcases {
		tc := testcases[i]

		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			cmd := tc.cmd
			args := tc.args
			opts := tc.opts

			_, err := app.Run(cmd, args, opts, false)
			if err != nil {
				if tc.err == "" {
					t.Errorf("unexpected error: %v", err)
				} else if tc.err != err.Error() {
					t.Errorf("unexpected error: want %q, got %q", tc.err, err.Error())
				}
			} else if tc.err != "" {
				t.Errorf("expected error did not occur: want %q, got none", tc.err)
			}
		})
	}
}
