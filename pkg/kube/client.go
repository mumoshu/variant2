package kube

import (
	sourcev1beta1 "github.com/fluxcd/source-controller/api/v1beta1"
	"golang.org/x/xerrors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewClient() (client.Client, error) {
	cfg, err := NewRestConfig()
	if err != nil {
		return nil, err
	}

	scheme := runtime.NewScheme()

	if err := sourcev1beta1.AddToScheme(scheme); err != nil {
		return nil, xerrors.Errorf("adding sourcev1beta1: %w", err)
	}

	client, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}

	return client, nil
}
