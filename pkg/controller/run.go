package controller

import (
	"fmt"
	"os"
	"strings"

	"github.com/summerwind/whitebox-controller/reconciler/state"
	"golang.org/x/xerrors"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

	podName, err := os.Hostname()
	if err != nil {
		return xerrors.Errorf("getting pod name from hostname: %w", err)
	}

	ctl := &controller{
		log:            logf.Log.WithName(controllerName),
		runtimeClient:  nil,
		run:            run,
		podName:        podName,
		controllerName: controllerName,
	}

	handle := func(st *state.State, job string) (finalErr error) {
		return ctl.do(job, st.Object)
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

	ctl.runtimeClient = mgr.GetClient()

	err = mgr.Start(signals.SetupSignalHandler())
	if err != nil {
		return xerrors.Errorf("starting controller-manager: %w", err)
	}

	return err
}
