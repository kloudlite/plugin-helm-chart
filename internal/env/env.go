package env

import "github.com/codingconcepts/env"

type Env struct {
	MaxConcurrentReconciles int    `env:"MAX_CONCURRENT_RECONCILES" default:"1"`
	HelmJobImage            string `env:"HELM_JOB_IMAGE" default:"ghcr.io/kloudlite/kloudlite/operator/workers/helm-job-runner:v1.1.4"`
}

func LoadEnv() (*Env, error) {
	var ev Env
	if err := env.Set(&ev); err != nil {
		return nil, err
	}
	return &ev, nil
}
