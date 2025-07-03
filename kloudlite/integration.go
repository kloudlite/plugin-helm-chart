package kloudlite

import (
	"github.com/kloudlite/kloudlite/operator/toolkit/operator"
	v1 "github.com/kloudlite/plugin-helm-chart/api/v1"
	"github.com/kloudlite/plugin-helm-chart/internal/controller/helm_chart"
	"github.com/kloudlite/plugin-helm-chart/internal/controller/helm_pipeline"
)

func RegisterInto(mgr operator.Operator) {
	helmChartEnv, err := helm_chart.LoadEnv()
	if err != nil {
		panic(err)
	}

	helmPipelineEnv, err := helm_pipeline.LoadEnv()
	if err != nil {
		panic(err)
	}

	mgr.AddToSchemes(v1.AddToScheme)
	mgr.RegisterControllers(
		&helm_chart.HelmChartReconciler{
			Env: helmChartEnv,
		},
		&helm_pipeline.HelmPipelineReconciler{
			Env: helmPipelineEnv,
		},
	)
}
