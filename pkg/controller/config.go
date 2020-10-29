package controller

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	controllerName string
	resyncPeriod   string
	forKind        string
	group          string
	version        string
	jobOnApply     string
	jobOnDestroy   string
}

func getConfigFromEnv() (*Config, error) {
	getEnv := func(n string) (string, string) {
		name := EnvPrefix + n
		value := os.Getenv(name)

		return name, value
	}

	controllerNameEnv, controllerName := getEnv("NAME")
	if controllerName == "" {
		return nil, fmt.Errorf("missing required environment variable: %s", controllerNameEnv)
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
		return nil, fmt.Errorf("missing required environment variable: %s", jobOnApplyEnv)
	}

	jobOnDestroyEnv, jobOnDestroy := getEnv("JOB_ON_DESTROY")
	if jobOnDestroy == "" {
		return nil, fmt.Errorf("missing required environment variable: %s", jobOnDestroyEnv)
	}

	return &Config{
		controllerName: controllerName,
		resyncPeriod:   resyncPeriod,
		forKind:        forKind,
		group:          group,
		version:        version,
		jobOnApply:     jobOnApply,
		jobOnDestroy:   jobOnDestroy,
	}, nil
}
