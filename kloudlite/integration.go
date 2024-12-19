package kloudlite

import (
	"github.com/kloudlite/operator/toolkit/operator"
	v1 "github.com/kloudlite/plugin-helm-chart/api/v1"
	"github.com/kloudlite/plugin-helm-chart/internal/controller"
)

func RegisterInto(mgr operator.Operator) {
	ev, err := controller.LoadEnv()
	if err != nil {
		panic(err)
	}
	mgr.AddToSchemes(v1.AddToScheme)
	mgr.RegisterControllers(
		&controller.HelmChartReconciler{Env: ev, YAMLClient: mgr.Operator().KubeYAMLClient()},
	)
}
