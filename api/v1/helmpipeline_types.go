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
	job_helper "github.com/kloudlite/operator/toolkit/job-helper"
	"github.com/kloudlite/operator/toolkit/plugin"
	"github.com/kloudlite/operator/toolkit/reconciler"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HelmPipelineSpec defines the desired state of HelmPipeline.
type HelmPipelineSpec struct {
	HelmJobVars HelmJobVars     `json:"jobVars,omitempty"`
	Pipeline    []*PipelineStep `json:"pipeline"`
}

type ReleaseInfo struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

type PipelineStep struct {
	Chart      ChartInfo                       `json:"chart"`
	Release    ReleaseInfo                     `json:"release"`
	HelmValues map[string]apiextensionsv1.JSON `json:"helmValues"`
	Export     plugin.Export                   `json:"export,omitempty"`

	PreInstall  string `json:"preInstall,omitempty"`
	PostInstall string `json:"postInstall,omitempty"`

	PreUninstall  string `json:"preUninstall,omitempty"`
	PostUninstall string `json:"postUninstall,omitempty"`
}

func (p *HelmPipeline) EnsureGVK() {
	if p != nil {
		p.SetGroupVersionKind(GroupVersion.WithKind("HelmPipeline"))
	}
}

func (p *HelmPipeline) GetStatus() *reconciler.Status {
	return &p.Status.Status
}

func (p *HelmPipeline) GetEnsuredLabels() map[string]string {
	return map[string]string{}
}

func (p *HelmPipeline) GetEnsuredAnnotations() map[string]string {
	return map[string]string{}
}

type HelmPipelineStatus struct {
	reconciler.Status `json:",inline"`
	Phase             job_helper.JobPhase `json:"phase"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:JSONPath=".status.lastReconcileTime",name=Seen,type=date
// +kubebuilder:printcolumn:JSONPath=".metadata.annotations.kloudlite\\.io\\/operator\\.checks",name=Checks,type=string
// +kubebuilder:printcolumn:JSONPath=".metadata.annotations.kloudlite\\.io\\/operator\\.resource\\.ready",name=Ready,type=string
// +kubebuilder:printcolumn:JSONPath=".metadata.creationTimestamp",name=Age,type=date

// HelmPipeline is the Schema for the helmpipelines API.
type HelmPipeline struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HelmPipelineSpec   `json:"spec,omitempty"`
	Status HelmPipelineStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// HelmPipelineList contains a list of HelmPipeline.
type HelmPipelineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HelmPipeline `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HelmPipeline{}, &HelmPipelineList{})
}
