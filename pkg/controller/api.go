package controller

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	coreGroup   = "core.variant.run"
	coreVersion = "v1beta1"
)

var reconciliationGroupVersionKind = schema.GroupVersionKind{
	Group:   coreGroup,
	Version: coreVersion,
	Kind:    "Reconciliation",
}

func newReconciliation() *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}

	obj.SetGroupVersionKind(reconciliationGroupVersionKind)

	return obj
}
