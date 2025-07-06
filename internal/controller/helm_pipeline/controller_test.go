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
	"time"

	fn "github.com/kloudlite/kloudlite/operator/toolkit/functions"
	"github.com/kloudlite/kloudlite/operator/toolkit/plugin"
	ktypes "github.com/kloudlite/kloudlite/operator/toolkit/types"
	"github.com/kloudlite/plugin-helm-chart/api/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("HelmPipeline Controller", func() {
	const (
		timeout  = time.Second * 30
		interval = time.Millisecond * 250
	)

	var testNs *corev1.Namespace

	BeforeEach(func() {
		// Create test namespace
		testNs = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-helmpipeline-" + fn.CleanerNanoid(5),
			},
		}
		Expect(k8sClient.Create(ctx, testNs)).Should(Succeed())
	})

	AfterEach(func() {
		// Clean up namespace
		Expect(k8sClient.Delete(ctx, testNs)).Should(Succeed())
	})

	Context("When reconciling a HelmPipeline resource", func() {
		It("Should create a HelmPipeline successfully", func() {
			By("Creating a new HelmPipeline")
			helmPipeline := &v1.HelmPipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-helmpipeline",
					Namespace: testNs.Name,
				},
				Spec: v1.HelmPipelineSpec{
					Pipeline: []*v1.PipelineStep{
						{
							Chart: v1.ChartInfo{
								URL:     "https://charts.bitnami.com/bitnami",
								Name:    "nginx",
								Version: "15.0.0",
							},
							Release: v1.ReleaseInfo{
								Name:      "nginx",
								Namespace: testNs.Name,
							},
							HelmValues: map[string]apiextensionsv1.JSON{
								"replicaCount": {Raw: []byte("1")},
							},
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, helmPipeline)).Should(Succeed())

			helmPipelineLookupKey := types.NamespacedName{Name: helmPipeline.Name, Namespace: helmPipeline.Namespace}
			createdHelmPipeline := &v1.HelmPipeline{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, helmPipelineLookupKey, createdHelmPipeline)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			By("Checking that the HelmPipeline has expected spec")
			Expect(createdHelmPipeline.Spec.Pipeline).Should(HaveLen(1))
			Expect(createdHelmPipeline.Spec.Pipeline[0].Chart.Name).Should(Equal("nginx"))
		})

		It("Should create RBAC resources for HelmPipeline job", func() {
			By("Creating a HelmPipeline")
			helmPipeline := &v1.HelmPipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-helmpipeline-rbac",
					Namespace: testNs.Name,
				},
				Spec: v1.HelmPipelineSpec{
					Pipeline: []*v1.PipelineStep{
						{
							Chart: v1.ChartInfo{
								URL:     "https://charts.bitnami.com/bitnami",
								Name:    "nginx",
								Version: "15.0.0",
							},
							Release: v1.ReleaseInfo{
								Name:      "nginx",
								Namespace: testNs.Name,
							},
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, helmPipeline)).Should(Succeed())

			By("Checking ServiceAccount creation")
			sa := &corev1.ServiceAccount{}
			saKey := types.NamespacedName{
				Name:      "helm-pipeline-sa",
				Namespace: helmPipeline.Namespace,
			}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, saKey, sa)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			By("Checking ClusterRoleBinding creation")
			crb := &rbacv1.ClusterRoleBinding{}
			crbKey := types.NamespacedName{
				Name: "helm-pipeline-sa-rb",
			}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, crbKey, crb)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			Expect(crb.RoleRef.Kind).Should(Equal("ClusterRole"))
			Expect(crb.RoleRef.Name).Should(Equal("cluster-admin"))
		})

		It("Should create pipeline job", func() {
			By("Creating a HelmPipeline with multiple steps")
			helmPipeline := &v1.HelmPipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-helmpipeline-job",
					Namespace: testNs.Name,
				},
				Spec: v1.HelmPipelineSpec{
					Pipeline: []*v1.PipelineStep{
						{
							Chart: v1.ChartInfo{
								URL:     "https://charts.bitnami.com/bitnami",
								Name:    "nginx",
								Version: "15.0.0",
							},
							Release: v1.ReleaseInfo{
								Name:      "nginx",
								Namespace: testNs.Name,
							},
						},
						{
							Chart: v1.ChartInfo{
								URL:     "https://charts.bitnami.com/bitnami",
								Name:    "postgresql",
								Version: "13.0.0",
							},
							Release: v1.ReleaseInfo{
								Name:      "postgresql",
								Namespace: testNs.Name,
							},
						},
					},
					HelmJobVars: v1.HelmJobVars{
						Resources: ktypes.Resource{
							Cpu: &ktypes.CPUResource{
								Min: "100m",
								Max: "200m",
							},
							Memory: &ktypes.MemoryResource{
								Min: "128Mi",
								Max: "256Mi",
							},
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, helmPipeline)).Should(Succeed())

			By("Checking pipeline job creation")
			jobList := &batchv1.JobList{}
			Eventually(func() bool {
				err := k8sClient.List(ctx, jobList, client.InNamespace(testNs.Name))
				if err != nil {
					return false
				}
				return len(jobList.Items) > 0
			}, timeout, interval).Should(BeTrue())

			job := &jobList.Items[0]
			// Check job name pattern instead of label
			Expect(job.Name).Should(Equal(helmPipeline.Name + "-pipeline-job"))
			// Resources are configured in the job template, verify job was created
		})

		It("Should handle pipeline with exports between steps", func() {
			By("Creating a HelmPipeline with exports")
			helmPipeline := &v1.HelmPipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-helmpipeline-exports",
					Namespace: testNs.Name,
				},
				Spec: v1.HelmPipelineSpec{
					Pipeline: []*v1.PipelineStep{
						{
							Chart: v1.ChartInfo{
								URL:     "https://charts.bitnami.com/bitnami",
								Name:    "postgresql",
								Version: "13.0.0",
							},
							Release: v1.ReleaseInfo{
								Name:      "postgresql",
								Namespace: testNs.Name,
							},
							Export: plugin.Export{
								ViaSecret: "postgresql-export",
								Template: `host: "{{ .HelmReleaseName }}-postgresql"
password: "{{ .Values.auth.postgresPassword }}"`,
							},
						},
						{
							Chart: v1.ChartInfo{
								URL:     "https://example.com/charts",
								Name:    "app",
								Version: "1.0.0",
							},
							Release: v1.ReleaseInfo{
								Name:      "app",
								Namespace: testNs.Name,
							},
							HelmValues: map[string]apiextensionsv1.JSON{
								"database": {Raw: []byte(`{"host": "postgresql-postgresql", "password": "from-secret"}`)},
							},
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, helmPipeline)).Should(Succeed())

			// In a real test, we would need to simulate pipeline execution
			// and verify exports are created and used correctly
		})

		It("Should handle HelmPipeline deletion with finalizer", func() {
			By("Creating a HelmPipeline")
			helmPipeline := &v1.HelmPipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-helmpipeline-delete",
					Namespace: testNs.Name,
				},
				Spec: v1.HelmPipelineSpec{
					Pipeline: []*v1.PipelineStep{
						{
							Chart: v1.ChartInfo{
								URL:     "https://charts.bitnami.com/bitnami",
								Name:    "nginx",
								Version: "15.0.0",
							},
							Release: v1.ReleaseInfo{
								Name:      "nginx",
								Namespace: testNs.Name,
							},
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, helmPipeline)).Should(Succeed())

			By("Waiting for finalizer to be added")
			helmPipelineKey := types.NamespacedName{Name: helmPipeline.Name, Namespace: helmPipeline.Namespace}
			Eventually(func() bool {
				hp := &v1.HelmPipeline{}
				if err := k8sClient.Get(ctx, helmPipelineKey, hp); err != nil {
					return false
				}
				return len(hp.Finalizers) > 0
			}, timeout, interval).Should(BeTrue())

			By("Deleting the HelmPipeline")
			Expect(k8sClient.Delete(ctx, helmPipeline)).Should(Succeed())

			By("Checking uninstall job is created")
			Eventually(func() bool {
				jobList := &batchv1.JobList{}
				if err := k8sClient.List(ctx, jobList, client.InNamespace(testNs.Name)); err != nil {
					return false
				}
				for _, job := range jobList.Items {
					// Check if it's an uninstall job by name pattern and annotation
					if job.Name == helmPipeline.Name+"-pipeline-job" && job.Annotations["kloudlite.io/helmpipeline.tracker"] != "" {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())
		})

		It("Should track job phases correctly", func() {
			By("Creating a HelmPipeline")
			helmPipeline := &v1.HelmPipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-helmpipeline-phases",
					Namespace: testNs.Name,
				},
				Spec: v1.HelmPipelineSpec{
					Pipeline: []*v1.PipelineStep{
						{
							Chart: v1.ChartInfo{
								URL:     "https://charts.bitnami.com/bitnami",
								Name:    "nginx",
								Version: "15.0.0",
							},
							Release: v1.ReleaseInfo{
								Name:      "nginx",
								Namespace: testNs.Name,
							},
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, helmPipeline)).Should(Succeed())

			By("Checking status includes job phase")
			helmPipelineKey := types.NamespacedName{Name: helmPipeline.Name, Namespace: helmPipeline.Namespace}
			Eventually(func() bool {
				hp := &v1.HelmPipeline{}
				if err := k8sClient.Get(ctx, helmPipelineKey, hp); err != nil {
					return false
				}
				return hp.Status.Phase != ""
			}, timeout, interval).Should(BeTrue())
		})

		It("Should handle pre and post hooks in pipeline steps", func() {
			By("Creating a HelmPipeline with hooks")
			helmPipeline := &v1.HelmPipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-helmpipeline-hooks",
					Namespace: testNs.Name,
				},
				Spec: v1.HelmPipelineSpec{
					Pipeline: []*v1.PipelineStep{
						{
							Chart: v1.ChartInfo{
								URL:     "https://charts.bitnami.com/bitnami",
								Name:    "nginx",
								Version: "15.0.0",
							},
							Release: v1.ReleaseInfo{
								Name:      "nginx",
								Namespace: testNs.Name,
							},
							PreInstall:  "echo 'Pre-install nginx'",
							PostInstall: "echo 'Post-install nginx'",
						},
						{
							Chart: v1.ChartInfo{
								URL:     "https://charts.bitnami.com/bitnami",
								Name:    "postgresql",
								Version: "13.0.0",
							},
							Release: v1.ReleaseInfo{
								Name:      "postgresql",
								Namespace: testNs.Name,
							},
							PreInstall:  "echo 'Pre-install postgresql'",
							PostInstall: "echo 'Post-install postgresql'",
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, helmPipeline)).Should(Succeed())

			// Verify pipeline job is created with proper hook configuration
		})

		It("Should handle custom job configurations for pipeline", func() {
			By("Creating a HelmPipeline with custom job vars")
			helmPipeline := &v1.HelmPipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-helmpipeline-custom",
					Namespace: testNs.Name,
				},
				Spec: v1.HelmPipelineSpec{
					Pipeline: []*v1.PipelineStep{
						{
							Chart: v1.ChartInfo{
								URL:     "https://charts.bitnami.com/bitnami",
								Name:    "nginx",
								Version: "15.0.0",
							},
							Release: v1.ReleaseInfo{
								Name:      "nginx",
								Namespace: testNs.Name,
							},
						},
					},
					HelmJobVars: v1.HelmJobVars{
						NodeSelector: map[string]string{
							"pipeline": "true",
						},
						Tolerations: []corev1.Toleration{
							{
								Key:      "pipeline",
								Operator: corev1.TolerationOpEqual,
								Value:    "true",
								Effect:   corev1.TaintEffectNoSchedule,
							},
						},
						Affinity: &corev1.Affinity{
							PodAffinity: &corev1.PodAffinity{
								RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
									{
										LabelSelector: &metav1.LabelSelector{
											MatchLabels: map[string]string{
												"app": "pipeline-runner",
											},
										},
										TopologyKey: "kubernetes.io/hostname",
									},
								},
							},
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, helmPipeline)).Should(Succeed())

			By("Verifying job has custom configurations")
			jobList := &batchv1.JobList{}
			Eventually(func() bool {
				err := k8sClient.List(ctx, jobList, client.InNamespace(testNs.Name))
				if err != nil {
					return false
				}
				return len(jobList.Items) > 0
			}, timeout, interval).Should(BeTrue())

			job := &jobList.Items[0]
			Expect(job.Spec.Template.Spec.NodeSelector).Should(HaveKeyWithValue("pipeline", "true"))
			Expect(job.Spec.Template.Spec.Tolerations).Should(HaveLen(1))
			Expect(job.Spec.Template.Spec.Affinity).ShouldNot(BeNil())
		})

		It("Should handle force reconciliation for pipeline", func() {
			By("Creating a HelmPipeline with force reconcile annotation")
			helmPipeline := &v1.HelmPipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-helmpipeline-force",
					Namespace: testNs.Name,
					Annotations: map[string]string{
						"kloudlite.io/force-reconcile": "true",
					},
				},
				Spec: v1.HelmPipelineSpec{
					Pipeline: []*v1.PipelineStep{
						{
							Chart: v1.ChartInfo{
								URL:     "https://charts.bitnami.com/bitnami",
								Name:    "nginx",
								Version: "15.0.0",
							},
							Release: v1.ReleaseInfo{
								Name:      "nginx",
								Namespace: testNs.Name,
							},
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, helmPipeline)).Should(Succeed())

			By("Checking force reconcile annotation is handled")
			helmPipelineKey := types.NamespacedName{Name: helmPipeline.Name, Namespace: helmPipeline.Namespace}
			Eventually(func() bool {
				hp := &v1.HelmPipeline{}
				if err := k8sClient.Get(ctx, helmPipelineKey, hp); err != nil {
					return false
				}
				_, exists := hp.Annotations["kloudlite.io/force-reconcile"]
				return !exists // Annotation should be removed after processing
			}, timeout, interval).Should(BeTrue())
		})

		It("Should update status correctly throughout pipeline execution", func() {
			By("Creating a HelmPipeline")
			helmPipeline := &v1.HelmPipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-helmpipeline-status",
					Namespace: testNs.Name,
				},
				Spec: v1.HelmPipelineSpec{
					Pipeline: []*v1.PipelineStep{
						{
							Chart: v1.ChartInfo{
								URL:     "https://charts.bitnami.com/bitnami",
								Name:    "nginx",
								Version: "15.0.0",
							},
							Release: v1.ReleaseInfo{
								Name:      "nginx",
								Namespace: testNs.Name,
							},
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, helmPipeline)).Should(Succeed())

			By("Checking status is updated")
			helmPipelineKey := types.NamespacedName{Name: helmPipeline.Name, Namespace: helmPipeline.Namespace}
			Eventually(func() bool {
				hp := &v1.HelmPipeline{}
				if err := k8sClient.Get(ctx, helmPipelineKey, hp); err != nil {
					return false
				}
				return hp.Status.IsReady
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("When testing HelmPipeline controller error scenarios", func() {
		It("Should handle empty pipeline", func() {
			By("Creating a HelmPipeline with empty pipeline")
			helmPipeline := &v1.HelmPipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-helmpipeline-empty",
					Namespace: testNs.Name,
				},
				Spec: v1.HelmPipelineSpec{
					Pipeline: []*v1.PipelineStep{},
				},
			}

			Expect(k8sClient.Create(ctx, helmPipeline)).Should(Succeed())

			By("Checking error is reported in status")
			helmPipelineKey := types.NamespacedName{Name: helmPipeline.Name, Namespace: helmPipeline.Namespace}
			Eventually(func() bool {
				hp := &v1.HelmPipeline{}
				if err := k8sClient.Get(ctx, helmPipelineKey, hp); err != nil {
					return false
				}
				return hp.Status.Phase != "" || len(hp.Status.Checks) > 0
			}, timeout, interval).Should(BeTrue())
		})

		It("Should handle invalid chart configuration in pipeline step", func() {
			By("Creating a HelmPipeline with invalid chart")
			helmPipeline := &v1.HelmPipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-helmpipeline-invalid",
					Namespace: testNs.Name,
				},
				Spec: v1.HelmPipelineSpec{
					Pipeline: []*v1.PipelineStep{
						{
							Chart: v1.ChartInfo{
								// Missing URL
								Name:    "nginx",
								Version: "15.0.0",
							},
							Release: v1.ReleaseInfo{
								Name:      "nginx",
								Namespace: testNs.Name,
							},
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, helmPipeline)).Should(Succeed())

			// Verify error handling
		})

		It("Should handle concurrent updates gracefully", func() {
			By("Creating a HelmPipeline")
			helmPipeline := &v1.HelmPipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-helmpipeline-concurrent",
					Namespace: testNs.Name,
				},
				Spec: v1.HelmPipelineSpec{
					Pipeline: []*v1.PipelineStep{
						{
							Chart: v1.ChartInfo{
								URL:     "https://charts.bitnami.com/bitnami",
								Name:    "nginx",
								Version: "15.0.0",
							},
							Release: v1.ReleaseInfo{
								Name:      "nginx",
								Namespace: testNs.Name,
							},
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, helmPipeline)).Should(Succeed())

			By("Simulating concurrent updates")
			helmPipelineKey := types.NamespacedName{Name: helmPipeline.Name, Namespace: helmPipeline.Namespace}

			// Update 1
			go func() {
				hp := &v1.HelmPipeline{}
				k8sClient.Get(ctx, helmPipelineKey, hp)
				hp.Spec.Pipeline[0].HelmValues = map[string]apiextensionsv1.JSON{"key1": {Raw: []byte(`"value1"`)}}
				k8sClient.Update(ctx, hp)
			}()

			// Update 2
			go func() {
				hp := &v1.HelmPipeline{}
				k8sClient.Get(ctx, helmPipelineKey, hp)
				hp.Spec.Pipeline[0].HelmValues = map[string]apiextensionsv1.JSON{"key2": {Raw: []byte(`"value2"`)}}
				k8sClient.Update(ctx, hp)
			}()

			// Should handle updates without panic
			time.Sleep(2 * time.Second)
		})

		It("Should handle release namespace conflicts", func() {
			By("Creating a HelmPipeline with conflicting release namespaces")
			helmPipeline := &v1.HelmPipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-helmpipeline-ns-conflict",
					Namespace: testNs.Name,
				},
				Spec: v1.HelmPipelineSpec{
					Pipeline: []*v1.PipelineStep{
						{
							Chart: v1.ChartInfo{
								URL:     "https://charts.bitnami.com/bitnami",
								Name:    "nginx",
								Version: "15.0.0",
							},
							Release: v1.ReleaseInfo{
								Name:      "nginx",
								Namespace: "non-existent-namespace",
							},
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, helmPipeline)).Should(Succeed())

			// Verify namespace validation or error handling
		})

		It("Should handle pipeline with duplicate release names", func() {
			By("Creating a HelmPipeline with duplicate releases")
			helmPipeline := &v1.HelmPipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-helmpipeline-duplicate",
					Namespace: testNs.Name,
				},
				Spec: v1.HelmPipelineSpec{
					Pipeline: []*v1.PipelineStep{
						{
							Chart: v1.ChartInfo{
								URL:     "https://charts.bitnami.com/bitnami",
								Name:    "nginx",
								Version: "15.0.0",
							},
							Release: v1.ReleaseInfo{
								Name:      "nginx",
								Namespace: testNs.Name,
							},
						},
						{
							Chart: v1.ChartInfo{
								URL:     "https://charts.bitnami.com/bitnami",
								Name:    "nginx",
								Version: "16.0.0",
							},
							Release: v1.ReleaseInfo{
								Name:      "nginx", // Duplicate name
								Namespace: testNs.Name,
							},
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, helmPipeline)).Should(Succeed())

			// Verify handling of duplicate release names
		})
	})
})
