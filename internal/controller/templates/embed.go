package templates

import (
	"embed"
	"path/filepath"

	operator_templates "github.com/kloudlite/operator/pkg/templates"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type InstallJobVars struct {
	Metadata metav1.ObjectMeta

	ObservabilityAnnotations map[string]string

	ReleaseName      string
	ReleaseNamespace string

	Image           string
	ImagePullPolicy string

	BackOffLimit int

	ServiceAccountName string

	Tolerations  []corev1.Toleration
	Affinity     corev1.Affinity
	NodeSelector map[string]string

	ChartRepoURL string
	ChartName    string
	ChartVersion string

	PreInstall  string
	PostInstall string

	HelmValuesYAML string
}

type UnInstallJobVars struct {
	Metadata metav1.ObjectMeta

	ObservabilityAnnotations map[string]string

	ReleaseName      string
	ReleaseNamespace string

	Image           string
	ImagePullPolicy string

	BackOffLimit int

	ServiceAccountName string

	Tolerations  []corev1.Toleration
	Affinity     corev1.Affinity
	NodeSelector map[string]string

	ChartRepoURL string
	ChartName    string
	ChartVersion string

	PreUninstall  string
	PostUninstall string
}

//go:embed *
var templatesDir embed.FS

type templateFile string

const (
	HelmInstallJobTemplate   templateFile = "./helm-install-job.yml.tpl"
	HelmUninstallJobTemplate templateFile = "./helm-uninstall-job.yml.tpl"
)

func Read(t templateFile) ([]byte, error) {
	return templatesDir.ReadFile(filepath.Join(string(t)))
}

var ParseBytes = operator_templates.ParseBytes
