package controller

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/summerwind/whitebox-controller/handler"
	"github.com/summerwind/whitebox-controller/reconciler/state"
	"golang.org/x/xerrors"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	kconfig "sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"

	_ "k8s.io/client-go/plugin/pkg/client/auth/azure"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"

	"github.com/summerwind/whitebox-controller/config"
	"github.com/summerwind/whitebox-controller/manager"
)

const (
	EnvPrefix = "VARIANT_CONTROLLER_"
)

func RunRequested() bool {
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, EnvPrefix) {
			return true
		}
	}

	return false
}

const (
	coreGroup   = "core.variant.run"
	coreVersion = "v1beta1"
)

var (
	reconcilationGroupVersionKind = schema.GroupVersionKind{
		Group:   coreGroup,
		Version: coreVersion,
		Kind:    "Reconcilation",
	}
)

func Run(run func([]string) (string, error)) (finalErr error) {
	logf.SetLogger(logf.ZapLogger(false))

	defer func() {
		if finalErr != nil {
			logf.Log.Error(finalErr, "Error while running controller")
		}
	}()

	kc, err := kconfig.GetConfig()
	if err != nil {
		return xerrors.Errorf("getting kubernetes client config: %w", err)
	}

	getEnv := func(n string) (string, string) {
		name := EnvPrefix + n
		value := os.Getenv(name)

		return name, value
	}

	controllerNameEnv, controllerName := getEnv("NAME")
	if controllerName == "" {
		return fmt.Errorf("missing required environment variable: %s", controllerNameEnv)
	}

	log := logf.Log.WithName(controllerName)

	_, forAPIVersion := getEnv("FOR_API_VERSION")
	if forAPIVersion == "" {
		forAPIVersion = coreGroup + "/" + coreVersion
	}

	_, forKind := getEnv("FOR_KIND")
	if forKind == "" {
		forKind = "Resource"
	}

	_, resyncPeriod := getEnv("RESYNC_PERIOD")

	groupVersion := strings.Split(forAPIVersion, "/")
	group := groupVersion[0]
	version := groupVersion[1]

	jobOnApplyEnv, jobOnApply := getEnv("JOB_ON_APPLY")
	if jobOnApply == "" {
		return fmt.Errorf("missing required environment variable: %s", jobOnApplyEnv)
	}

	jobOnDestroyEnv, jobOnDestroy := getEnv("JOB_ON_DESTROY")
	if jobOnDestroy == "" {
		return fmt.Errorf("missing required environment variable: %s", jobOnDestroyEnv)
	}

	var runtimeClient client.Client

	podName, err := os.Hostname()
	if err != nil {
		return xerrors.Errorf("getting pod name from hostname: %w", err)
	}

	logReconcilation := func(orig *unstructured.Unstructured, job, combinedLogs string) error {
		name := orig.GetName()
		namespace := orig.GetNamespace()

		st := &unstructured.Unstructured{}
		st.SetGroupVersionKind(orig.GroupVersionKind())

		if err := runtimeClient.Get(context.TODO(), client.ObjectKey{Namespace: namespace, Name: name}, st); err != nil {
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

		obj := newReconcilation()

		var update bool

		getErr := runtimeClient.Get(context.TODO(), client.ObjectKey{Namespace: namespace, Name: reconName}, obj)
		if getErr != nil {
			if !errors.IsNotFound(getErr) {
				return fmt.Errorf("getting reconcilation object: %w", err)
			}
		} else {
			update = true
		}

		// Use of GenerateName results in 404
		//obj.SetGenerateName(name + "-")
		obj.SetName(reconName)
		// Missing Namespace results in 404
		obj.SetNamespace(namespace)
		obj.SetLabels(map[string]string{
			"core.variant.run/event":      "apply",
			"core.variant.run/controller": controllerName,
			"core.variant.run/pod":        podName,
		})
		spec, ok, err := unstructured.NestedMap(st.Object, "spec")
		if !ok {
			return fmt.Errorf("missing Resource.Spec: %w", err)
		}

		if err != nil {
			return xerrors.Errorf("calling unstructured.NestedMap: %w", err)
		}

		unstructured.SetNestedField(obj.Object, job, "spec", "job")
		unstructured.SetNestedMap(obj.Object, spec, "spec", "resource")
		unstructured.SetNestedField(obj.Object, combinedLogs, "spec", "combinedLogs", "data")

		if update {
			if err := runtimeClient.Update(context.TODO(), obj); err != nil {
				return fmt.Errorf("updating reconcilation object: %w", err)
			}
		} else {
			if err := runtimeClient.Create(context.TODO(), obj); err != nil {
				return fmt.Errorf("creating reconcilation object: %w", err)
			}
		}

		return nil
	}

	handle := func(st *state.State, job string) (finalErr error) {
		args := strings.Split(job, " ")
		m, found, err := unstructured.NestedMap(st.Object.Object, "spec")
		if !found {
			return fmt.Errorf(`"spec" field not found: %v`, st.Object.Object)
		}

		if err != nil {
			return xerrors.Errorf("getting nested map from the object: %w", err)
		}

		for k, v := range m {
			args = append(args, "--"+k, fmt.Sprintf("%v", v))
		}

		log.Info("Running Variant", "args", strings.Join(args, " "))

		args = append([]string{"run"}, args...)

		combinedLogs, err := run(args)
		if err != nil {
			return xerrors.Errorf("executing %v: %w", args, err)
		}

		if err := logReconcilation(st.Object, job, combinedLogs); err != nil {
			return xerrors.Errorf("logging result of %q: %w", job, err)
		}

		return nil
	}

	applyHandler := StateHandlerFunc(func(st *state.State) error {
		return handle(st, jobOnApply)
	})

	destroyHandler := StateHandlerFunc(func(st *state.State) error {
		return handle(st, jobOnDestroy)
	})

	c := &config.Config{
		Name: controllerName,
		Resources: []*config.ResourceConfig{
			{
				GroupVersionKind: schema.GroupVersionKind{
					Group:   group,
					Version: version,
					Kind:    forKind,
				},
				Reconciler: &config.ReconcilerConfig{
					HandlerConfig: config.HandlerConfig{
						StateHandler: applyHandler,
					},
				},
				Finalizer: &config.HandlerConfig{
					StateHandler: destroyHandler,
				},
				ResyncPeriod: resyncPeriod,
			},
		},
		Webhook: nil,
	}

	mgr, err := manager.New(c, kc)
	if err != nil {
		return xerrors.Errorf("creating controller-manager: %w", err)
	}

	runtimeClient = mgr.GetClient()

	err = mgr.Start(signals.SetupSignalHandler())
	if err != nil {
		return xerrors.Errorf("starting controller-manager: %w", err)
	}

	return err
}

func newReconcilation() *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}

	obj.SetGroupVersionKind(reconcilationGroupVersionKind)

	return obj
}

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
