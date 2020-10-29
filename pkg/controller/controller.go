package controller

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"golang.org/x/xerrors"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type controller struct {
	// controllerName is the name of this controller that is shown in the logs
	controllerName string

	// podName is the hostname of where the controller is running on.
	// Stored in Reconciliation objects so that the operator can track which controller in which pod has done the
	// reconciliation.
	podName string

	// Kubernetes client to be used for querying target objects and managing Reconciliation objects
	runtimeClient client.Client

	// Runs `variant run <args>` and returns combined output and/or error
	run func([]string) (string, error)

	log logr.Logger
}

func (c *controller) do(job string, obj *unstructured.Unstructured) error {
	args := strings.Split(job, " ")

	m, found, err := unstructured.NestedMap(obj.Object, "spec")
	if !found {
		return fmt.Errorf(`"spec" field not found: %v`, obj.Object)
	}

	if err != nil {
		return xerrors.Errorf("getting nested map from the object: %w", err)
	}

	for k, v := range m {
		args = append(args, "--"+k, fmt.Sprintf("%v", v))
	}

	c.log.Info("Running Variant", "args", strings.Join(args, " "))

	args = append([]string{"run"}, args...)

	combinedLogs, err := c.run(args)
	if err != nil {
		return xerrors.Errorf("executing %v: %w", args, err)
	}

	if err := c.logReconciliation(obj, job, combinedLogs); err != nil {
		return xerrors.Errorf("logging result of %q: %w", job, err)
	}

	return nil
}

func (c *controller) logReconciliation(orig *unstructured.Unstructured, job, combinedLogs string) error {
	name := orig.GetName()
	namespace := orig.GetNamespace()

	st := &unstructured.Unstructured{}
	st.SetGroupVersionKind(orig.GroupVersionKind())

	if err := c.runtimeClient.Get(context.TODO(), client.ObjectKey{Namespace: namespace, Name: name}, st); err != nil {
		return fmt.Errorf("getting object %q: %w", name, err)
	}

	gen, ok, err := unstructured.NestedInt64(st.Object, "metadata", "generation")
	if !ok {
		return fmt.Errorf("missing Resource.Generation: %w", err)
	}

	if err != nil {
		return xerrors.Errorf("getting metadata.generation from %s: %w", name, err)
	}

	reconName := name + "-" + fmt.Sprintf("%d", gen)

	obj := newReconciliation()

	var update bool

	getErr := c.runtimeClient.Get(context.TODO(), client.ObjectKey{Namespace: namespace, Name: reconName}, obj)
	if getErr != nil {
		if !errors.IsNotFound(getErr) {
			return fmt.Errorf("getting reconciliation object: %w", err)
		}
	} else {
		update = true
	}

	// Use of GenerateName results in 404
	// obj.SetGenerateName(name + "-")
	obj.SetName(reconName)
	// Missing Namespace results in 404
	obj.SetNamespace(namespace)
	obj.SetLabels(map[string]string{
		"core.variant.run/event":      "apply",
		"core.variant.run/controller": c.controllerName,
		"core.variant.run/pod":        c.podName,
	})

	spec, ok, err := unstructured.NestedMap(st.Object, "spec")
	if !ok {
		return fmt.Errorf("missing Resource.Spec: %w", err)
	}

	if err != nil {
		return xerrors.Errorf("calling unstructured.NestedMap: %w", err)
	}

	if err := unstructured.SetNestedField(obj.Object, job, "spec", "job"); err != nil {
		return xerrors.Errorf("setting nested field spec.job: %w", err)
	}

	if err := unstructured.SetNestedMap(obj.Object, spec, "spec", "resource"); err != nil {
		return xerrors.Errorf("setting nested map spec.resource: %w", err)
	}

	if err := unstructured.SetNestedField(obj.Object, combinedLogs, "spec", "combinedLogs", "data"); err != nil {
		return xerrors.Errorf("setting nested field spec.combinedLogs.data: %w", err)
	}

	if update {
		if err := c.runtimeClient.Update(context.TODO(), obj); err != nil {
			return fmt.Errorf("updating reconciliation object: %w", err)
		}
	} else {
		if err := c.runtimeClient.Create(context.TODO(), obj); err != nil {
			return fmt.Errorf("creating reconciliation object: %w", err)
		}
	}

	return nil
}
