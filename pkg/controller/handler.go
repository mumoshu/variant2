package controller

import (
	"github.com/summerwind/whitebox-controller/handler"
	"github.com/summerwind/whitebox-controller/reconciler/state"
)

func StateHandlerFunc(f func(*state.State) error) handler.StateHandler {
	return &stateHandler{
		f: f,
	}
}

type stateHandler struct {
	f func(*state.State) error
}

func (h stateHandler) HandleState(s *state.State) error {
	return h.f(s)
}
