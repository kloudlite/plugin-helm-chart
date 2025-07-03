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

package helm_pipeline

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/yaml"

	fn "github.com/kloudlite/kloudlite/operator/toolkit/functions"
	job_helper "github.com/kloudlite/kloudlite/operator/toolkit/job-helper"
	"github.com/kloudlite/kloudlite/operator/toolkit/reconciler"
	"github.com/kloudlite/plugin-helm-chart/api/v1"
	"github.com/kloudlite/plugin-helm-chart/internal/controller/helm_pipeline/templates"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HelmPipelineReconciler reconciles a HelmPipeline object
type HelmPipelineReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Env    *Env

	templateInstallJobSpec   []byte
	templateUninstallJobSpec []byte
}

// GetName implements reconciler.Reconciler.
func (r *HelmPipelineReconciler) GetName() string {
	return "plugin-helm-pipeline"
}

const (
	jobTrackerAnnKey = "kloudlite.io/helmpipeline.tracker"
)

// +kubebuilder:rbac:groups=plugin-helm-chart.kloudlite.github.com,resources=helmpipelines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=plugin-helm-chart.kloudlite.github.com,resources=helmpipelines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=plugin-helm-chart.kloudlite.github.com,resources=helmpipelines/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the HelmPipeline object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *HelmPipelineReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	req, err := reconciler.NewRequest(ctx, r.Client, request.NamespacedName, &v1.HelmPipeline{})
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	req.PreReconcile()
	defer req.PostReconcile()

	return reconciler.ReconcileSteps(req, []reconciler.Step[*v1.HelmPipeline]{
		{
			Name:     "setup-k8s-job-RBAC",
			Title:    "Setup K8s Job RBAC",
			OnCreate: r.setupJobRBAC,
			OnDelete: nil,
		},
		{
			Name:     "setup-helm-pipeline-job",
			Title:    "Setup Helm Pipeline Job",
			OnCreate: r.createInstallJob,
			OnDelete: r.createUninstallJob,
		},
		{
			Name:     "setup-pipeline-exports",
			Title:    "Setup Helm Pipeline Job",
			OnCreate: r.processExports,
			OnDelete: r.cleanupExports,
		},
	})
}

const JobServiceAccountName = "helm-pipeline-sa"

func (r *HelmPipelineReconciler) setupJobRBAC(check *reconciler.Check[*v1.HelmPipeline], obj *v1.HelmPipeline) reconciler.StepResult {
	jobSvcAcc := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: JobServiceAccountName, Namespace: obj.Namespace}}

	if _, err := controllerutil.CreateOrUpdate(check.Context(), r.Client, jobSvcAcc, func() error {
		if jobSvcAcc.Annotations == nil {
			jobSvcAcc.Annotations = make(map[string]string, 1)
		}
		jobSvcAcc.Annotations[reconciler.AnnotationDescriptionKey] = "Service account used by helm pipeline controlller to run helm pipeline jobs"
		return nil
	}); err != nil {
		return check.Failed(err)
	}

	crb := rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("%s-rb", JobServiceAccountName)}}
	if _, err := controllerutil.CreateOrUpdate(check.Context(), r.Client, &crb, func() error {
		if crb.Annotations == nil {
			crb.Annotations = make(map[string]string, 1)
		}
		crb.Annotations[reconciler.AnnotationDescriptionKey] = "Cluster role binding used by helm pipeline to run helm pipeline jobs"

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

type jobOpType string

const (
	InstallOp   jobOpType = "install"
	UninstallOp jobOpType = "uninstall"
)

// runPipelineJob builds, creates (if needed) and tracks a Job from a template
// - opType must be install|uninstall
func (r *HelmPipelineReconciler) runPipelineJob(check *reconciler.Check[*v1.HelmPipeline], obj *v1.HelmPipeline, jobSpec []byte, op jobOpType) reconciler.StepResult {
	annVal := fmt.Sprintf("%d/%s", obj.GetGeneration(), op)
	name := fmt.Sprintf("%s-pipeline-job", obj.Name)

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
				Annotations:     fn.MapMerge(fn.MapFilterWithPrefix(obj.GetAnnotations(), "kloudlite.io/observability."), map[string]string{jobTrackerAnnKey: annVal}),
				OwnerReferences: []metav1.OwnerReference{fn.AsOwner(obj, true)},
			},
		}

		check.Logger().Warn("job.spec", "YAML", string(jobSpec))

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

func (r *HelmPipelineReconciler) createInstallJob(check *reconciler.Check[*v1.HelmPipeline], obj *v1.HelmPipeline) reconciler.StepResult {
	pipelineSteps := make([]templates.Pipeline, 0, len(obj.Spec.Pipeline))

	for _, pipeline := range obj.Spec.Pipeline {
		valuesYAML, err := valuesToYaml(pipeline.HelmValues)
		if err != nil {
			return check.Failed(err)
		}

		pipelineSteps = append(pipelineSteps, templates.Pipeline{
			PipelineStep:   pipeline,
			HelmValuesYAML: valuesYAML,
		})
	}

	b, err := templates.ParseBytes(r.templateInstallJobSpec, templates.HelmPipelineInstallJobSpecVars{
		PodAnnotations:     fn.MapFilterWithPrefix(obj.GetAnnotations(), reconciler.ObservabilityAnnotationKey),
		PodTolerations:     obj.Spec.HelmJobVars.Tolerations,
		NodeSelector:       obj.Spec.HelmJobVars.NodeSelector,
		ServiceAccountName: JobServiceAccountName,
		Image:              r.Env.HelmJobRunnerImage,
		Pipeline:           pipelineSteps,
		BackOffLimit:       1,
	})
	if err != nil {
		return check.Failed(err)
	}

	return r.runPipelineJob(check, obj, b, InstallOp)
}

func (r *HelmPipelineReconciler) createUninstallJob(check *reconciler.Check[*v1.HelmPipeline], obj *v1.HelmPipeline) reconciler.StepResult {
	b, err := templates.ParseBytes(r.templateUninstallJobSpec, templates.HelmPipelineUninstallJobSpecVars{
		Pipeline:           obj.Spec.Pipeline,
		PodAnnotations:     fn.MapFilterWithPrefix(obj.GetAnnotations(), reconciler.ObservabilityAnnotationKey),
		PodTolerations:     obj.Spec.HelmJobVars.Tolerations,
		NodeSelector:       obj.Spec.HelmJobVars.NodeSelector,
		ServiceAccountName: JobServiceAccountName,
		Image:              r.Env.HelmJobRunnerImage,
		BackOffLimit:       1,
	})
	if err != nil {
		return check.Failed(err)
	}

	return r.runPipelineJob(check, obj, b, UninstallOp)
}

func (r *HelmPipelineReconciler) processExports(check *reconciler.Check[*v1.HelmPipeline], obj *v1.HelmPipeline) reconciler.StepResult {
	hasUpdate := false
	for _, step := range obj.Spec.Pipeline {
		if step.Export.ViaSecret == "" {
			hasUpdate = true
			step.Export.ViaSecret = fmt.Sprintf("%s-exports", step.Release.Name)
		}
	}

	if hasUpdate {
		if err := r.Client.Update(check.Context(), obj); err != nil {
			return check.Failed(err)
		}

		return check.Abort("setting pipeline.export.viaSecret field to their default value")
	}

	for _, step := range obj.Spec.Pipeline {
		if step.Export.Template == "" || step.Export.ViaSecret == "" {
			continue
		}

		valuesMap := struct {
			HelmReleaseName      string
			HelmReleaseNamespace string
		}{
			HelmReleaseName:      step.Release.Name,
			HelmReleaseNamespace: step.Release.Namespace,
		}

		m, err := step.Export.ParseKV(check.Context(), r.Client, step.Release.Namespace, valuesMap)
		if err != nil {
			return check.Failed(errors.Join(errors.New("failed to parse export KVs"), err))
		}

		exportSecret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: step.Export.ViaSecret, Namespace: step.Release.Namespace}}
		if _, err := controllerutil.CreateOrUpdate(check.Context(), r.Client, exportSecret, func() error {
			exportSecret.StringData = m
			return nil
		}); err != nil {
			return check.Failed(errors.Join(errors.New("creating/updating export secret"), err))
		}
	}

	return check.Passed()
}

func (r *HelmPipelineReconciler) cleanupExports(check *reconciler.Check[*v1.HelmPipeline], obj *v1.HelmPipeline) reconciler.StepResult {
	for _, step := range obj.Spec.Pipeline {
		if step.Export.ViaSecret == "" {
			continue
		}

		if err := fn.DeleteAndWait(check.Context(), r.Client, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      step.Export.ViaSecret,
				Namespace: step.Release.Namespace,
			},
		}); err != nil {
			return check.Failed(err)
		}
	}

	return check.Passed()
}

// SetupWithManager sets up the controller with the Manager.
func (r *HelmPipelineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Client == nil {
		r.Client = mgr.GetClient()
	}

	if r.Scheme == nil {
		r.Scheme = mgr.GetScheme()
	}

	if r.Env == nil {
		return errors.New("env must be set by caller")
	}

	var err error

	r.templateInstallJobSpec, err = templates.Read(templates.HelmPipelineInstallJobSpec)
	if err != nil {
		return err
	}

	r.templateUninstallJobSpec, err = templates.Read(templates.HelmPipelineUninstallJobSpec)
	if err != nil {
		return err
	}

	builder := ctrl.NewControllerManagedBy(mgr).For(&v1.HelmPipeline{}).Named(r.GetName())
	builder.Owns(&batchv1.Job{})
	builder.WithOptions(controller.Options{MaxConcurrentReconciles: r.Env.MaxConcurrentReconciles})
	builder.WithEventFilter(reconciler.ReconcileFilter())
	return builder.Complete(r)
}
