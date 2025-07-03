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
	"time"

	"github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	pluginhelmchartv1 "github.com/kloudlite/plugin-helm-chart/api/v1"
)

// TestHelpers provides utility functions for controller tests
type TestHelpers struct {
	K8sClient client.Client
	Ctx       context.Context
}

// WaitForJob waits for a job to be created with the given labels
func (h *TestHelpers) WaitForJob(namespace string, labels map[string]string, timeout time.Duration) (*batchv1.Job, error) {
	jobList := &batchv1.JobList{}
	err := h.waitFor(func() bool {
		if err := h.K8sClient.List(h.Ctx, jobList,
			client.InNamespace(namespace),
			client.MatchingLabels(labels)); err != nil {
			return false
		}
		return len(jobList.Items) > 0
	}, timeout)
	if err != nil {
		return nil, err
	}
	return &jobList.Items[0], nil
}

// WaitForHelmChartReady waits for a HelmChart to be ready
func (h *TestHelpers) WaitForHelmChartReady(name, namespace string, timeout time.Duration) error {
	key := types.NamespacedName{Name: name, Namespace: namespace}
	return h.waitFor(func() bool {
		hc := &pluginhelmchartv1.HelmChart{}
		if err := h.K8sClient.Get(h.Ctx, key, hc); err != nil {
			return false
		}
		return hc.Status.IsReady
	}, timeout)
}

// WaitForHelmPipelineReady waits for a HelmPipeline to be ready
func (h *TestHelpers) WaitForHelmPipelineReady(name, namespace string, timeout time.Duration) error {
	key := types.NamespacedName{Name: name, Namespace: namespace}
	return h.waitFor(func() bool {
		hp := &pluginhelmchartv1.HelmPipeline{}
		if err := h.K8sClient.Get(h.Ctx, key, hp); err != nil {
			return false
		}
		return hp.Status.IsReady
	}, timeout)
}

// WaitForSecret waits for a secret to be created
func (h *TestHelpers) WaitForSecret(name, namespace string, timeout time.Duration) (*corev1.Secret, error) {
	key := types.NamespacedName{Name: name, Namespace: namespace}
	secret := &corev1.Secret{}
	err := h.waitFor(func() bool {
		if err := h.K8sClient.Get(h.Ctx, key, secret); err != nil {
			return false
		}
		return true
	}, timeout)
	if err != nil {
		return nil, err
	}
	return secret, nil
}

// SimulateJobCompletion simulates a job completing successfully
func (h *TestHelpers) SimulateJobCompletion(job *batchv1.Job) error {
	job.Status.Conditions = []batchv1.JobCondition{
		{
			Type:               batchv1.JobComplete,
			Status:             corev1.ConditionTrue,
			LastProbeTime:      metav1.Now(),
			LastTransitionTime: metav1.Now(),
		},
	}
	job.Status.Succeeded = 1
	job.Status.CompletionTime = &metav1.Time{Time: time.Now()}

	return h.K8sClient.Status().Update(h.Ctx, job)
}

// SimulateJobFailure simulates a job failing
func (h *TestHelpers) SimulateJobFailure(job *batchv1.Job, reason string) error {
	job.Status.Conditions = []batchv1.JobCondition{
		{
			Type:               batchv1.JobFailed,
			Status:             corev1.ConditionTrue,
			LastProbeTime:      metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             reason,
			Message:            "Job failed: " + reason,
		},
	}
	job.Status.Failed = 1

	return h.K8sClient.Status().Update(h.Ctx, job)
}

// CreateHelmChartWithDefaults creates a HelmChart with sensible defaults
func (h *TestHelpers) CreateHelmChartWithDefaults(name, namespace string) *pluginhelmchartv1.HelmChart {
	return &pluginhelmchartv1.HelmChart{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: pluginhelmchartv1.HelmChartSpec{
			Chart: pluginhelmchartv1.ChartInfo{
				URL:     "https://charts.bitnami.com/bitnami",
				Name:    "nginx",
				Version: "15.0.0",
			},
			HelmValues: map[string]apiextensionsv1.JSON{
				"replicaCount": {Raw: []byte("1")},
			},
		},
	}
}

// CreateHelmPipelineWithDefaults creates a HelmPipeline with sensible defaults
func (h *TestHelpers) CreateHelmPipelineWithDefaults(name, namespace string) *pluginhelmchartv1.HelmPipeline {
	return &pluginhelmchartv1.HelmPipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: pluginhelmchartv1.HelmPipelineSpec{
			Pipeline: []*pluginhelmchartv1.PipelineStep{
				{
					Chart: pluginhelmchartv1.ChartInfo{
						URL:     "https://charts.bitnami.com/bitnami",
						Name:    "nginx",
						Version: "15.0.0",
					},
					Release: pluginhelmchartv1.ReleaseInfo{
						Name:      "nginx",
						Namespace: namespace,
					},
					HelmValues: map[string]apiextensionsv1.JSON{
						"replicaCount": {Raw: []byte("1")},
					},
				},
			},
		},
	}
}

// waitFor is a helper function that waits for a condition to be true
func (h *TestHelpers) waitFor(condition func() bool, timeout time.Duration) error {
	interval := 250 * time.Millisecond
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if condition() {
			return nil
		}
		time.Sleep(interval)
	}

	return gomega.StopTrying("timeout waiting for condition")
}

// CleanupNamespace deletes all resources in a namespace
func (h *TestHelpers) CleanupNamespace(namespace string) error {
	// Delete all HelmCharts
	if err := h.K8sClient.DeleteAllOf(h.Ctx, &pluginhelmchartv1.HelmChart{},
		client.InNamespace(namespace)); err != nil {
		return err
	}

	// Delete all HelmPipelines
	if err := h.K8sClient.DeleteAllOf(h.Ctx, &pluginhelmchartv1.HelmPipeline{},
		client.InNamespace(namespace)); err != nil {
		return err
	}

	// Delete all Jobs
	if err := h.K8sClient.DeleteAllOf(h.Ctx, &batchv1.Job{},
		client.InNamespace(namespace)); err != nil {
		return err
	}

	// Delete all Secrets
	if err := h.K8sClient.DeleteAllOf(h.Ctx, &corev1.Secret{},
		client.InNamespace(namespace)); err != nil {
		return err
	}

	return nil
}
