package env

import "github.com/codingconcepts/env"

type Env struct {
	// HelmJobImage string `env:"HELM_JOB_IMAGE" required:"true"`
	HelmJobImage string `env:"HELM_JOB_IMAGE" default:"ghcr.io/kloudlite/kloudlite/operator/workers/helm-job-runner:v1.1.4"`
}

func LoadEnv() (*Env, error) {
	var ev Env
	if err := env.Set(&ev); err != nil {
		return nil, err
	}
	return &ev, nil
}
