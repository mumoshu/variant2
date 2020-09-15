package app

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestNewImportFunc(t *testing.T) {
	type testcase struct {
		subject string
		path    string
		want    string
	}

	testcases := []testcase{
		{
			subject: "relative path",
			path:    "foo/bar",
			want:    "sub/foo/bar",
		},
		{
			subject: "git url",
			path:    "git::ssh://git@github.com/mumoshu/variant2@examples/advanced/import/foo?ref=master",
			want:    "git::ssh://git@github.com/mumoshu/variant2@examples/advanced/import/foo?ref=master",
		},
		{
			subject: "absolute path",
			path:    "/a/b/c",
			want:    "/a/b/c",
		},
	}

	for i := range testcases {
		tc := testcases[i]

		t.Run(tc.subject, func(t *testing.T) {
			a := &App{}

			f := NewImportFunc("sub", func(path string) (*App, error) {
				if d := cmp.Diff(tc.want, path); d != "" {
					t.Errorf("%s: %s", tc.subject, d)
				}

				return a, nil
			})

			r, _ := f(tc.path)

			if r != a {
				t.Fatalf("%s: unexpected result returned", tc.subject)
			}
		})
	}
}
