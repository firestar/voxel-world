package cluster

import (
	"os"
	"strings"
)

func detectRuntimeMode() runtimeMode {
	if override := strings.ToLower(strings.TrimSpace(os.Getenv("CENTRAL_CLUSTER_MODE"))); override != "" {
		switch override {
		case string(runtimeDocker):
			return runtimeDocker
		case string(runtimeKubernetes):
			return runtimeKubernetes
		case string(runtimeLocal):
			return runtimeLocal
		}
	}

	if isKubernetesEnvironment() {
		return runtimeKubernetes
	}
	if isDockerEnvironment() {
		return runtimeDocker
	}
	return runtimeLocal
}

func isDockerEnvironment() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	return false
}

func isKubernetesEnvironment() bool {
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return true
	}
	if os.Getenv("KUBERNETES_PORT") != "" {
		return true
	}
	return false
}
