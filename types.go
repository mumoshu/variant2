package variant

import (
	"context"
	"io"
)

type State struct {
	Stdin  io.Reader
	Args   map[string]interface{}
	Stdout io.WriteCloser
	Stderr io.WriteCloser
}

type Job struct {
	Name        string
	Description string
	Options     map[string]Option
	Run         func(context.Context, State) error
}

type Option struct {
	Type        string
	Description string
}

type JobOptions struct {
	Stdout io.Writer
	Stderr io.Writer
	Args   map[string]interface{}
}

type JobRun struct {
	Job     Job
	Options JobOptions
}

func (j *JobRun) Run(ctx context.Context) error {
	return nil
}
