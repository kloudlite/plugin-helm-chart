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

package v1

import (
	job_helper "github.com/kloudlite/kloudlite/operator/toolkit/job-helper"
	"github.com/kloudlite/kloudlite/operator/toolkit/plugin"
	rApi "github.com/kloudlite/kloudlite/operator/toolkit/reconciler"
	types "github.com/kloudlite/kloudlite/operator/toolkit/types"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ChartInfo struct {
	URL     string `json:"url"`
	Version string `json:"version,omitempty"`
	Name    string `json:"name"`
}

type HelmJobVars struct {
	NodeSelector map[string]string   `json:"nodeSelector,omitempty"`
	Tolerations  []corev1.Toleration `json:"tolerations,omitempty"`
	Affinity     *corev1.Affinity    `json:"affinity,omitempty"`

	Resources types.Resource `json:"resources,omitempty"`
}

// HelmChartSpec defines the desired state of HelmChart.
type HelmChartSpec struct {
	Chart ChartInfo `json:"chart"`

	HelmValues map[string]apiextensionsv1.JSON `json:"helmValues"`

	HelmJobVars *HelmJobVars `json:"jobVars,omitempty"`

	PreInstall  string `json:"preInstall,omitempty"`
	PostInstall string `json:"postInstall,omitempty"`

	PreUninstall  string `json:"preUninstall,omitempty"`
	PostUninstall string `json:"postUninstall,omitempty"`
}

// HelmChartStatus defines the observed state of HelmChart.
type HelmChartStatus struct {
	rApi.Status `json:",inline"`
	Phase       job_helper.JobPhase `json:"phase"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:JSONPath=".status.lastReconcileTime",name=Seen,type=date
// +kubebuilder:printcolumn:JSONPath=".metadata.annotations.kloudlite\\.io\\/operator\\.checks",name=Checks,type=string
// +kubebuilder:printcolumn:JSONPath=".metadata.annotations.kloudlite\\.io\\/operator\\.resource\\.ready",name=Ready,type=string
// +kubebuilder:printcolumn:JSONPath=".metadata.creationTimestamp",name=Age,type=date

// HelmChart is the Schema for the helmcharts API.
type HelmChart struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec HelmChartSpec `json:"spec,omitempty"`

	Export plugin.Export   `json:"export,omitempty"`
	Status HelmChartStatus `json:"status,omitempty"`
}

func (p *HelmChart) EnsureGVK() {
	if p != nil {
		p.SetGroupVersionKind(GroupVersion.WithKind("HelmChart"))
	}
}

func (p *HelmChart) GetStatus() *rApi.Status {
	return &p.Status.Status
}

func (p *HelmChart) GetEnsuredLabels() map[string]string {
	return map[string]string{}
}

func (p *HelmChart) GetEnsuredAnnotations() map[string]string {
	return map[string]string{}
}

type LocalSecretReference struct {
	SecretName string `json:"secretName"`
}

// +kubebuilder:object:root=true

// HelmChartList contains a list of HelmChart.
type HelmChartList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HelmChart `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HelmChart{}, &HelmChartList{})
}
