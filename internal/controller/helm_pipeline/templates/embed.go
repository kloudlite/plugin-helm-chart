package templates

import (
	"embed"
	"path/filepath"

	operator_templates "github.com/kloudlite/operator/toolkit/templates"
)

//go:embed *
var templatesDir embed.FS

type templateFile string

const (
	HelmPipelineInstallJobSpec   templateFile = "./helm-pipeline-install-job-spec.yml.tpl"
	HelmPipelineUninstallJobSpec templateFile = "./helm-pipeline-uninstall-job-spec.yml.tpl"
)

func Read(t templateFile) ([]byte, error) {
	return templatesDir.ReadFile(filepath.Join(string(t)))
}

var ParseBytes = operator_templates.ParseBytes
