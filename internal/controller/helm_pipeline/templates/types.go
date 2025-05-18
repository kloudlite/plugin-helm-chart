package templates

import (
	v1 "github.com/kloudlite/plugin-helm-chart/api/v1"
	corev1 "k8s.io/api/core/v1"
)

type HelmPipelineInstallJobSpecVars struct {
	PodAnnotations map[string]string
	PodTolerations []corev1.Toleration

	NodeSelector map[string]string

	ServiceAccountName string

	ImagePullPolicy string
	Image           string

	Pipeline []Pipeline

	BackOffLimit int
}

type Pipeline struct {
	*v1.PipelineStep
	HelmValuesYAML string
}

type HelmPipelineUninstallJobSpecVars struct {
	Pipeline []*v1.PipelineStep

	PodAnnotations map[string]string
	PodTolerations []corev1.Toleration

	NodeSelector map[string]string

	ServiceAccountName string

	ImagePullPolicy string
	Image           string

	BackOffLimit int
}
