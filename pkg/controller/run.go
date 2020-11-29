package controller

import (
	"os"
	"strings"

	"github.com/summerwind/whitebox-controller/config"
	"github.com/summerwind/whitebox-controller/manager"
	"github.com/summerwind/whitebox-controller/reconciler/state"
	"golang.org/x/xerrors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	// We import these here rather than in main to automate setting up cloud-provider-specific authentication strategies.
	_ "k8s.io/client-go/plugin/pkg/client/auth/azure"

	// We import these here rather than in main to automate setting up cloud-provider-specific authentication strategies.
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"

	// We import these here rather than in main to automate setting up cloud-provider-specific authentication strategies.
	kconfig "sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
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
	logf.SetLogger(zap.New())

	defer func() {
		if finalErr != nil {
			logf.Log.Error(finalErr, "Error while running controller")
		}
	}()

	kc, err := kconfig.GetConfig()
	if err != nil {
		return xerrors.Errorf("getting kubernetes client config: %w", err)
	}

	conf, err := getConfigFromEnv()
	if err != nil {
		return xerrors.Errorf("getting config from envvars: %w", err)
	}

	podName, err := os.Hostname()
	if err != nil {
		return xerrors.Errorf("getting pod name from hostname: %w", err)
	}

	ctl := &controller{
		log:            logf.Log.WithName(conf.controllerName),
		runtimeClient:  nil,
		run:            run,
		podName:        podName,
		controllerName: conf.controllerName,
	}

	handle := func(st *state.State, job string) (finalErr error) {
		return ctl.do(job, st.Object)
	}

	applyHandler := StateHandlerFunc(func(st *state.State) error {
		return handle(st, conf.jobOnApply)
	})

	destroyHandler := StateHandlerFunc(func(st *state.State) error {
		return handle(st, conf.jobOnDestroy)
	})

	whiteboxConfig := &config.Config{
		Name: conf.controllerName,
		Resources: []*config.ResourceConfig{
			{
				GroupVersionKind: schema.GroupVersionKind{
					Group:   conf.group,
					Version: conf.version,
					Kind:    conf.forKind,
				},
				Reconciler: &config.ReconcilerConfig{
					HandlerConfig: config.HandlerConfig{
						StateHandler: applyHandler,
					},
				},
				Finalizer: &config.HandlerConfig{
					StateHandler: destroyHandler,
				},
				ResyncPeriod: conf.resyncPeriod,
			},
		},
		Webhook: nil,
	}

	mgr, err := manager.New(whiteboxConfig, kc)
	if err != nil {
		return xerrors.Errorf("creating controller-manager: %w", err)
	}

	ctl.runtimeClient = mgr.GetClient()

	err = mgr.Start(signals.SetupSignalHandler())
	if err != nil {
		return xerrors.Errorf("starting controller-manager: %w", err)
	}

	return nil
}
