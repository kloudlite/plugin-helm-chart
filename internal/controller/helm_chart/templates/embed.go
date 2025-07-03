package templates

import (
	"embed"
	"path/filepath"

	operator_templates "github.com/kloudlite/kloudlite/operator/toolkit/templates"
)

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

var (
	ParseBytes  = operator_templates.ParseBytes
	ParseBytes2 = operator_templates.ParseBytes2
)
