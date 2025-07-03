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

package helm_chart

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/kloudlite/kloudlite/operator/toolkit/errors"
	job_helper "github.com/kloudlite/kloudlite/operator/toolkit/job-helper"
	"github.com/kloudlite/kloudlite/operator/toolkit/reconciler"
	"github.com/kloudlite/plugin-helm-chart/constants"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/yaml"

	fn "github.com/kloudlite/kloudlite/operator/toolkit/functions"
	v1 "github.com/kloudlite/plugin-helm-chart/api/v1"
	"github.com/kloudlite/plugin-helm-chart/internal/controller/helm_chart/templates"
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

	Env *Env

	templateInstallOrUpgradeJob []byte
	templateUninstallJob        []byte
}

// GetName implements reconciler.Reconciler.
func (r *HelmChartReconciler) GetName() string {
	return "plugin-helm-chart"
}

const (
	JobServiceAccountName  = "pl-helm-job-sa"
	ClusterRoleBindingName = JobServiceAccountName + "-rb"
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
	req, err := reconciler.NewRequest(ctx, r.Client, request.NamespacedName, &v1.HelmChart{})
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	req.PreReconcile()
	defer req.PostReconcile()

	return reconciler.ReconcileSteps(req, []reconciler.Step[*v1.HelmChart]{
		{
			Name:     "setup k8s job RBAC",
			Title:    "Setup Kubernetes RBAC for running helm job",
			OnCreate: r.createHelmJobRBAC,
			OnDelete: nil,
		},
		{
			Name:     "setup helm release job",
			Title:    "Setup Helm Release Job",
			OnCreate: r.createHelmInstallJob,
			OnDelete: r.createHelmUninstallJob,
		},
		{
			Name:     "process exports",
			Title:    "Process helm chart exports",
			OnCreate: r.createProcessExports,
			OnDelete: r.deleteProcessedExports,
		},
	})
}

func (r *HelmChartReconciler) createHelmJobRBAC(check *reconciler.Check[*v1.HelmChart], obj *v1.HelmChart) reconciler.StepResult {
	jobSvcAcc := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: JobServiceAccountName, Namespace: obj.Namespace}}

	if _, err := controllerutil.CreateOrUpdate(check.Context(), r.Client, jobSvcAcc, func() error {
		if jobSvcAcc.Annotations == nil {
			jobSvcAcc.Annotations = make(map[string]string, 1)
		}
		jobSvcAcc.Annotations[reconciler.AnnotationDescriptionKey] = "Service account used by helm charts to run helm release jobs"
		return nil
	}); err != nil {
		return check.Failed(err)
	}

	crb := rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: ClusterRoleBindingName}}
	if _, err := controllerutil.CreateOrUpdate(check.Context(), r.Client, &crb, func() error {
		if crb.Annotations == nil {
			crb.Annotations = make(map[string]string, 1)
		}
		crb.Annotations[reconciler.AnnotationDescriptionKey] = "Cluster role binding used by helm charts to run helm release jobs"

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

	return check.Passed()
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

const (
	jobTrackerAnnKey string = constants.ProjectDomain + "/helmpipeline"
)

type jobOpType string

const (
	InstallOp   jobOpType = "install"
	UninstallOp jobOpType = "uninstall"
)

func (r *HelmChartReconciler) runHelmChartJob(check *reconciler.Check[*v1.HelmChart], obj *v1.HelmChart, jobSpec []byte, op jobOpType) reconciler.StepResult {
	annVal := fmt.Sprintf("%d/%s", obj.GetGeneration(), op)
	name := fmt.Sprintf("%s-helmchart-job", obj.Name)

	jt, err := job_helper.NewJobTracker(check.Context(), r.Client, job_helper.JobTrackerArgs{
		JobNamespace: obj.Namespace,
		JobName:      name,
		IsTargetJob: func(job *batchv1.Job) bool {
			return job.Annotations[jobTrackerAnnKey] == annVal
		},
	})
	if err != nil {
		if !apiErrors.IsNotFound(err) {
			return check.Failed(err)
		}
		job := &batchv1.Job{
			TypeMeta: metav1.TypeMeta{Kind: "Job", APIVersion: "batch/v1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:            name,
				Namespace:       obj.Namespace,
				Labels:          obj.GetLabels(),
				Annotations:     fn.MapMerge(fn.MapFilterWithPrefix(obj.GetAnnotations(), reconciler.ObservabilityAnnotationKey), map[string]string{jobTrackerAnnKey: annVal}),
				OwnerReferences: []metav1.OwnerReference{fn.AsOwner(obj, true)},
			},
		}
		if err := yaml.Unmarshal(jobSpec, &job.Spec); err != nil {
			return check.Failed(err)
		}
		if err := r.Client.Create(check.Context(), job); err != nil {
			return check.Failed(err)
		}

		return check.Abort("waiting for job to be created")
	}

	phase, msg, err := jt.StartTracking(check.Context())
	obj.Status.Phase = phase
	if err != nil {
		return check.Failed(err)
	}
	if phase != job_helper.JobPhaseSucceeded {
		return check.Abort(msg)
	}
	return check.Passed()
}

func (r *HelmChartReconciler) createHelmInstallJob(check *reconciler.Check[*v1.HelmChart], obj *v1.HelmChart) reconciler.StepResult {
	ctx := check.Context()

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
		if err := r.Client.Update(ctx, obj); err != nil {
			return check.Errored(err).RequeueAfter(1 * time.Second)
		}
	}

	values, err := valuesToYaml(helmValues)
	if err != nil {
		return check.Failed(errors.NewEf(err, "converting helm values to YAML"))
	}

	jobVars := obj.Spec.HelmJobVars
	if jobVars == nil {
		jobVars = &v1.HelmJobVars{}
	}

	b, err := templates.ParseBytes(r.templateInstallOrUpgradeJob, templates.HelmChartInstallJobSpecParams{
		PodAnnotations:     fn.MapFilterWithPrefix(obj.GetAnnotations(), reconciler.ObservabilityAnnotationKey),
		ReleaseName:        obj.Name,
		ReleaseNamespace:   obj.Namespace,
		Image:              r.Env.HelmJobRunnerImage,
		BackOffLimit:       1,
		ServiceAccountName: JobServiceAccountName,
		Tolerations:        jobVars.Tolerations,
		Affinity:           corev1.Affinity{},
		NodeSelector:       jobVars.NodeSelector,
		ChartRepoURL:       obj.Spec.Chart.URL,
		ChartName:          obj.Spec.Chart.Name,
		ChartVersion:       obj.Spec.Chart.Version,
		PreInstall:         obj.Spec.PreInstall,
		PostInstall:        obj.Spec.PostInstall,
		HelmValuesYAML:     values,
	})
	if err != nil {
		return check.Failed(err)
	}

	return r.runHelmChartJob(check, obj, b, InstallOp)
}

func (r *HelmChartReconciler) createHelmUninstallJob(check *reconciler.Check[*v1.HelmChart], obj *v1.HelmChart) reconciler.StepResult {
	jobVars := obj.Spec.HelmJobVars
	if jobVars == nil {
		jobVars = &v1.HelmJobVars{}
	}

	b, err := templates.ParseBytes(r.templateUninstallJob, templates.HelmChartUninstallJobSpecParams{
		PodAnnotations:     fn.MapFilterWithPrefix(obj.GetAnnotations(), "kloudlite.io/observability."),
		ReleaseName:        obj.Name,
		ReleaseNamespace:   obj.Namespace,
		Image:              r.Env.HelmJobRunnerImage,
		BackOffLimit:       1,
		ServiceAccountName: JobServiceAccountName,
		Tolerations:        jobVars.Tolerations,
		Affinity:           corev1.Affinity{},
		NodeSelector:       jobVars.NodeSelector,
		ChartRepoURL:       obj.Spec.Chart.URL,
		ChartName:          obj.Spec.Chart.Name,
		ChartVersion:       obj.Spec.Chart.Version,
		PreUninstall:       obj.Spec.PreUninstall,
		PostUninstall:      obj.Spec.PostUninstall,
	})
	if err != nil {
		return check.Failed(err)
	}

	return r.runHelmChartJob(check, obj, b, UninstallOp)
}

func (r *HelmChartReconciler) createProcessExports(check *reconciler.Check[*v1.HelmChart], obj *v1.HelmChart) reconciler.StepResult {
	ctx := check.Context()

	if obj.Export.Template == "" || obj.Export.ViaSecret == "" {
		check.Logger().Info("export.template or export.viaSecret field is not set, skipping export processing")
		return check.Passed()
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
		return check.Failed(errors.NewEf(err, "failed to parse KV"))
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

	return check.Passed()
}

func (r *HelmChartReconciler) deleteProcessedExports(check *reconciler.Check[*v1.HelmChart], obj *v1.HelmChart) reconciler.StepResult {
	if obj.Export.Template == "" || obj.Export.ViaSecret == "" {
		check.Logger().Info("export.template or export.viaSecret field is not set, skipping export deletion")
		return check.Passed()
	}

	if err := r.Delete(check.Context(), &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: obj.Export.ViaSecret, Namespace: obj.Namespace}}); err != nil {
		return check.Failed(err)
	}

	return check.Passed()
}

// SetupWithManager sets up the controller with the Manager.
func (r *HelmChartReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Client == nil {
		r.Client = mgr.GetClient()
	}

	if r.Scheme == nil {
		r.Scheme = mgr.GetScheme()
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
	builder.WithEventFilter(reconciler.ReconcileFilter())
	return builder.Complete(r)
}
