package templates

import (
	v1 "github.com/kloudlite/plugin-helm-chart/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type HelmPipelineInstallJobVars struct {
	JobMetadata    metav1.ObjectMeta
	PodTolerations []corev1.Toleration
	PodAnnotations map[string]string

	Pipeline []v1.PipelineStep
}
