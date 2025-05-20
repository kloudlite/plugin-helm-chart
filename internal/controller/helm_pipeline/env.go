package helm_pipeline

import "github.com/codingconcepts/env"

type Env struct {
	MaxConcurrentReconciles int    `env:"MAX_CONCURRENT_RECONCILES" default:"5"`
	HelmPipelineJobImage    string `env:"HELM_PIPELINE_JOB_IMAGE"`
}

func LoadEnv() (*Env, error) {
	var ev Env
	if err := env.Set(&ev); err != nil {
		return nil, err
	}
	return &ev, nil
}
