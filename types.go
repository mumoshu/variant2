package variant

import (
	"context"
	"io"
)

type State struct {
	Stdin      io.Reader
	Parameters map[string]interface{}
	Options    map[string]interface{}
	Stdout     io.WriteCloser
	Stderr     io.WriteCloser
}

type Job struct {
	Name        string
	Description string
	Options     map[string]Variable
	Parameters  map[string]Variable
	Run         func(context.Context, State) error
}

type Type int

const (
	String Type = iota
	Int
	StringSlice
	IntSlice
	Bool
)

type Variable struct {
	Type        Type
	Description string
}

type JobRun func(context.Context) error
