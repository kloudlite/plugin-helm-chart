/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/kloudlite/operator/toolkit/errors"
	rApi "github.com/kloudlite/operator/toolkit/reconciler"
	stepResult "github.com/kloudlite/operator/toolkit/reconciler/step-result"
	"github.com/kloudlite/plugin-helm-chart/constants"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/yaml"

	fn "github.com/kloudlite/operator/toolkit/functions"
	job_manager "github.com/kloudlite/operator/toolkit/job-helper"
	"github.com/kloudlite/operator/toolkit/kubectl"
	v1 "github.com/kloudlite/plugin-helm-chart/api/v1"
	"github.com/kloudlite/plugin-helm-chart/internal/controller/templates"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HelmChartReconciler reconciles a HelmChart object
type HelmChartReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	Env        *Env
	YAMLClient kubectl.YAMLClient

	templateInstallOrUpgradeJob []byte
	templateUninstallJob        []byte
}

// GetName implements reconciler.Reconciler.
func (r *HelmChartReconciler) GetName() string {
	return "plugin-helm-chart"
}

const (
	JobServiceAccountName = "pl-helm-job-sa"
)

// check names
const (
	DefaultsPatched           string = "defaults-patched"
	HelmJobRBACCreated        string = "job-rbac-created"
	HelmInstallJobCompleted   string = "helm-install-job-completed"
	HelmUninstallJobCompleted string = "helm-uninstall-job-completed"
	ProcessExports            string = "process-exports"
)

// +kubebuilder:rbac:groups=plugin-helm-chart.kloudlite.github.com,resources=helmcharts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=plugin-helm-chart.kloudlite.github.com,resources=helmcharts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=plugin-helm-chart.kloudlite.github.com,resources=helmcharts/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the HelmChart object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *HelmChartReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	req, err := rApi.NewRequest(ctx, r.Client, request.NamespacedName, &v1.HelmChart{})
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	req.PreReconcile()
	defer req.PostReconcile()

	if req.Object.GetDeletionTimestamp() != nil {
		if x := r.finalize(req); !x.ShouldProceed() {
			return x.ReconcilerResponse()
		}
		return ctrl.Result{}, nil
	}

	if step := req.ClearStatusIfAnnotated(); !step.ShouldProceed() {
		return step.ReconcilerResponse()
	}

	if step := req.EnsureLabelsAndAnnotations(); !step.ShouldProceed() {
		return step.ReconcilerResponse()
	}

	if step := req.EnsureFinalizers(rApi.CommonFinalizer); !step.ShouldProceed() {
		return step.ReconcilerResponse()
	}

	if step := req.EnsureCheckList([]rApi.CheckMeta{
		{Name: HelmJobRBACCreated, Title: "Helm Job RBAC created"},
		{Name: HelmInstallJobCompleted, Title: "Helm Install Job Created and Completed"},
	}); !step.ShouldProceed() {
		return step.ReconcilerResponse()
	}

	if step := r.ensureJobRBAC(req); !step.ShouldProceed() {
		return step.ReconcilerResponse()
	}

	if step := r.startInstallJob(req); !step.ShouldProceed() {
		return step.ReconcilerResponse()
	}

	if step := r.processExports(req); !step.ShouldProceed() {
		return step.ReconcilerResponse()
	}

	req.Object.Status.IsReady = true
	return ctrl.Result{}, nil
}

func (r *HelmChartReconciler) finalize(req *rApi.Request[*v1.HelmChart]) stepResult.Result {
	obj := req.Object
	check := rApi.NewRunningCheck("finalizing", req)

	deleteCheckList := []rApi.CheckMeta{
		{Name: HelmUninstallJobCompleted, Title: "Helm Uninstall Lifecycle Applied And Completed"},
	}

	if !slices.Equal(obj.Status.CheckList, deleteCheckList) {
		if step := req.EnsureCheckList(deleteCheckList); !step.ShouldProceed() {
			return step
		}
		return check.StillRunning(fmt.Errorf("updating checklist")).RequeueAfter(1 * time.Second)
	}

	if step := r.startUninstallJob(req); !step.ShouldProceed() {
		return step
	}

	return req.Finalize()
}

func (r *HelmChartReconciler) ensureJobRBAC(req *rApi.Request[*v1.HelmChart]) stepResult.Result {
	ctx, obj := req.Context(), req.Object
	check := rApi.NewRunningCheck(HelmJobRBACCreated, req)

	jobSvcAcc := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: JobServiceAccountName, Namespace: obj.Namespace}}

	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, jobSvcAcc, func() error {
		if jobSvcAcc.Annotations == nil {
			jobSvcAcc.Annotations = make(map[string]string, 1)
		}
		jobSvcAcc.Annotations[rApi.AnnotationDescriptionKey] = "Service account used by helm charts to run helm release jobs"
		return nil
	}); err != nil {
		return check.Failed(err)
	}

	crb := rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("%s-rb", JobServiceAccountName)}}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, &crb, func() error {
		if crb.Annotations == nil {
			crb.Annotations = make(map[string]string, 1)
		}
		crb.Annotations[rApi.AnnotationDescriptionKey] = "Cluster role binding used by helm charts to run helm release jobs"

		crb.RoleRef = rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "cluster-admin",
		}

		found := false
		for i := range crb.Subjects {
			if crb.Subjects[i].Namespace == obj.Namespace && crb.Subjects[i].Name == JobServiceAccountName {
				found = true
				break
			}
		}
		if !found {
			crb.Subjects = append(crb.Subjects, rbacv1.Subject{
				Kind:      "ServiceAccount",
				Name:      JobServiceAccountName,
				Namespace: obj.Namespace,
			})
		}
		return nil
	}); err != nil {
		return check.Failed(err)
	}

	return check.Completed()
}

func valuesToYaml(values map[string]apiextensionsv1.JSON) (string, error) {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}

	slices.Sort(keys)
	m := make(map[string]apiextensionsv1.JSON, len(values))
	for _, k := range keys {
		m[k] = values[k]
	}

	b, err := yaml.Marshal(m)
	if err != nil {
		return "", err
	}

	return string(b), nil
}

func getJobName(suffix string) string {
	return "helm-job-" + suffix
}

const (
	LabelHelmJobType        string = constants.ProjectDomain + "/helm.job-type"
	LabelHelmChartName      string = constants.ProjectDomain + "/helm.chart-name"
	LabelResourceGeneration string = constants.ProjectDomain + "/helm.resource-generation"
)

func (r *HelmChartReconciler) startInstallJob(req *rApi.Request[*v1.HelmChart]) stepResult.Result {
	ctx, obj := req.Context(), req.Object
	check := rApi.NewRunningCheck(HelmInstallJobCompleted, req)

	job := &batchv1.Job{}
	if err := r.Get(ctx, fn.NN(obj.Namespace, getJobName(obj.Name)), job); err != nil {
		if !apiErrors.IsNotFound(err) {
			return check.Failed(err)
		}
		job = nil
	}

	helmValues := obj.Spec.HelmValues

	if v, ok := obj.GetAnnotations()[constants.ForceReconcile]; ok && v == "true" {
		if helmValues == nil {
			helmValues = make(map[string]apiextensionsv1.JSON, 1)
		}
		b, _ := json.Marshal(map[string]any{"time": time.Now().Format(time.RFC3339)})
		helmValues["force-reconciled-at"] = apiextensionsv1.JSON{Raw: b}
		ann := obj.GetAnnotations()
		delete(ann, constants.ForceReconcile)
		obj.SetAnnotations(ann)
		if err := r.Update(ctx, obj); err != nil {
			return check.StillRunning(fmt.Errorf("waiting for reconcilation"))
		}
	}

	values, err := valuesToYaml(helmValues)
	if err != nil {
		return check.Failed(errors.NewEf(err, "converting helm values to YAML"))
	}

	if job == nil {
		jobVars := obj.Spec.HelmJobVars
		if jobVars == nil {
			jobVars = &v1.HelmJobVars{}
		}

		b, err := templates.ParseBytes(r.templateInstallOrUpgradeJob, templates.InstallJobVars{
			Metadata: metav1.ObjectMeta{
				Name:      getJobName(obj.Name),
				Namespace: obj.Namespace,
				Labels: map[string]string{
					LabelHelmJobType:        "install",
					LabelHelmChartName:      obj.Name,
					LabelResourceGeneration: fmt.Sprintf("%d", obj.Generation),
				},
				Annotations:     map[string]string{},
				OwnerReferences: []metav1.OwnerReference{fn.AsOwner(obj, true)},
			},
			ObservabilityAnnotations: fn.MapFilterWithPrefix(obj.GetAnnotations(), "kloudlite.io/observability."),
			ReleaseName:              obj.Name,
			ReleaseNamespace:         obj.Namespace,
			Image:                    r.Env.HelmJobImage,
			ImagePullPolicy:          "",
			BackOffLimit:             1,
			ServiceAccountName:       JobServiceAccountName,
			Tolerations:              jobVars.Tolerations,
			Affinity:                 corev1.Affinity{},
			NodeSelector:             jobVars.NodeSelector,
			ChartRepoURL:             obj.Spec.Chart.URL,
			ChartName:                obj.Spec.Chart.Name,
			ChartVersion:             obj.Spec.Chart.Version,
			PreInstall:               obj.Spec.PreInstall,
			PostInstall:              obj.Spec.PostInstall,
			HelmValuesYAML:           values,
		})
		if err != nil {
			return check.Failed(err)
		}

		// fmt.Printf("YAML: ---\n%s\n---\n", b)

		rr, err := r.YAMLClient.ApplyYAML(ctx, b)
		if err != nil {
			return check.Failed(err)
		}

		req.AddToOwnedResources(rr...)
		return check.StillRunning(fmt.Errorf("waiting for job to be created")).RequeueAfter(1 * time.Second)
	}

	isMyJob := job.Labels[LabelResourceGeneration] == fmt.Sprintf("%d", obj.Generation) && job.Labels[LabelHelmJobType] == "install"

	if !isMyJob {
		if !job_manager.HasJobFinished(ctx, r.Client, job) {
			return check.Failed(fmt.Errorf("waiting for previous jobs to finish execution"))
		}

		if err := job_manager.DeleteJob(ctx, r.Client, job.Namespace, job.Name); err != nil {
			return check.Failed(err)
		}

		return req.Done().RequeueAfter(1 * time.Second)
	}

	if !job_manager.HasJobFinished(ctx, r.Client, job) {
		return check.StillRunning(fmt.Errorf("waiting for running job to finish"))
	}

	fmt.Println("job status", job_manager.HasJobFinished(ctx, r.Client, job), "job", job.Name)

	check.Message = job_manager.GetTerminationLog(ctx, r.Client, job.Namespace, job.Name)
	if job.Status.Succeeded > 0 {
		return check.Completed()
	}

	return check.Failed(fmt.Errorf("install or upgrade job failed"))
}

func (r *HelmChartReconciler) processExports(req *rApi.Request[*v1.HelmChart]) stepResult.Result {
	ctx, obj := req.Context(), req.Object
	check := rApi.NewRunningCheck(ProcessExports, req)

	if obj.Export.Template == "" || obj.Export.ViaSecret == "" {
		req.Logger.Info("export.template or export.viaSecret field is not set, skipping export processing")
		return check.Completed()
	}

	valuesMap := struct {
		HelmReleaseName      string
		HelmReleaseNamespace string
	}{
		HelmReleaseName:      obj.Name,
		HelmReleaseNamespace: obj.Namespace,
	}

	m, err := obj.Export.ParseKV(ctx, r.Client, obj.Namespace, valuesMap)
	if err != nil {
		return check.Failed(errors.NewEf(err, ""))
	}

	if obj.Export.ViaSecret == "" {
		return check.Failed(fmt.Errorf("exports.viaSecret must be specified"))
	}

	exportSecret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: obj.Export.ViaSecret, Namespace: obj.Namespace}}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, exportSecret, func() error {
		exportSecret.Data = nil
		exportSecret.StringData = m
		return nil
	}); err != nil {
		return check.Failed(errors.NewEf(err, "creating/updating export secret"))
	}

	return check.Completed()
}

func (r *HelmChartReconciler) startUninstallJob(req *rApi.Request[*v1.HelmChart]) stepResult.Result {
	ctx, obj := req.Context(), req.Object
	check := rApi.NewRunningCheck(HelmUninstallJobCompleted, req)

	job, err := rApi.Get(ctx, r.Client, fn.NN(obj.Namespace, getJobName(obj.Name)), &batchv1.Job{})
	if err != nil {
		if !apiErrors.IsNotFound(err) {
			return check.Failed(err)
		}
		job = nil
	}

	if job == nil {
		jobVars := obj.Spec.HelmJobVars
		if jobVars == nil {
			jobVars = &v1.HelmJobVars{}
		}

		b, err := templates.ParseBytes(r.templateUninstallJob, templates.UnInstallJobVars{
			Metadata: metav1.ObjectMeta{
				Name:      getJobName(obj.Name),
				Namespace: obj.Namespace,
				Labels: map[string]string{
					LabelHelmJobType:        "uninstall",
					LabelHelmChartName:      obj.Name,
					LabelResourceGeneration: fmt.Sprintf("%d", obj.Generation),
				},
				Annotations:     map[string]string{},
				OwnerReferences: []metav1.OwnerReference{fn.AsOwner(obj, true)},
			},
			ObservabilityAnnotations: fn.MapFilterWithPrefix(obj.GetAnnotations(), "kloudlite.io/observability."),
			ReleaseName:              obj.Name,
			ReleaseNamespace:         obj.Namespace,
			Image:                    r.Env.HelmJobImage,
			ImagePullPolicy:          "",
			BackOffLimit:             0,
			ServiceAccountName:       JobServiceAccountName,
			Tolerations:              jobVars.Tolerations,
			Affinity:                 corev1.Affinity{},
			NodeSelector:             jobVars.NodeSelector,
			ChartRepoURL:             obj.Spec.Chart.URL,
			ChartName:                obj.Spec.Chart.Name,
			ChartVersion:             obj.Spec.Chart.Version,
			PreUninstall:             obj.Spec.PreInstall,
			PostUninstall:            obj.Spec.PostInstall,
		})
		if err != nil {
			return check.Failed(err)
		}

		rr, err := r.YAMLClient.ApplyYAML(ctx, b)
		if err != nil {
			if strings.HasSuffix(err.Error(), "unable to create new content in namespace testing-plugin-helm-chart because it is being terminated") {
				// NOTE: namespace is already getting deleted anyway, no need to run the job
				return check.Completed()
			}
			return check.Failed(err)
		}

		req.AddToOwnedResources(rr...)
		return check.StillRunning(fmt.Errorf("waiting for job to be created")).RequeueAfter(1 * time.Second)
	}

	isMyJob := job.Labels[LabelResourceGeneration] == fmt.Sprintf("%d", obj.Generation) && job.Labels[LabelHelmJobType] == "uninstall"

	if !isMyJob {
		if !job_manager.HasJobFinished(ctx, r.Client, job) {
			return check.Failed(fmt.Errorf("waiting for previous jobs to finish execution"))
		}

		// deleting that job
		if err := r.Delete(ctx, job, &client.DeleteOptions{
			GracePeriodSeconds: fn.New(int64(10)),
			Preconditions:      &metav1.Preconditions{},
			PropagationPolicy:  fn.New(metav1.DeletePropagationBackground),
		}); err != nil {
			return check.Failed(err)
		}

		return check.StillRunning(fmt.Errorf("deleting helm job")).RequeueAfter(1 * time.Second)
	}

	if !job_manager.HasJobFinished(ctx, r.Client, job) {
		return check.Failed(fmt.Errorf("waiting for job to finish execution"))
	}

	// check.Message = job_manager.GetTerminationLog(ctx, r.Client, job.Namespace, job.Name)
	if job.Status.Failed > 0 {
		return check.Failed(fmt.Errorf("helm deletion job failed"))
	}

	// deleting that job
	if err := r.Delete(ctx, job, &client.DeleteOptions{
		GracePeriodSeconds: fn.New(int64(10)),
		Preconditions:      &metav1.Preconditions{},
		PropagationPolicy:  fn.New(metav1.DeletePropagationBackground),
	}); err != nil {
		return check.Failed(err)
	}

	return check.Completed()
}

// SetupWithManager sets up the controller with the Manager.
func (r *HelmChartReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Client == nil {
		r.Client = mgr.GetClient()
	}

	if r.Scheme == nil {
		r.Scheme = mgr.GetScheme()
	}

	if r.YAMLClient == nil {
		return fmt.Errorf("yamlclient must be set")
	}

	if r.Env == nil {
		return fmt.Errorf("env must be set")
	}

	var err error

	r.templateInstallOrUpgradeJob, err = templates.Read(templates.HelmInstallJobTemplate)
	if err != nil {
		return err
	}

	r.templateUninstallJob, err = templates.Read(templates.HelmUninstallJobTemplate)
	if err != nil {
		return err
	}

	builder := ctrl.NewControllerManagedBy(mgr).For(&v1.HelmChart{}).Named("plugin-helm-chart")
	builder.Owns(&batchv1.Job{})
	builder.WithOptions(controller.Options{MaxConcurrentReconciles: r.Env.MaxConcurrentReconciles})
	builder.WithEventFilter(rApi.ReconcileFilter())
	return builder.Complete(r)
}
