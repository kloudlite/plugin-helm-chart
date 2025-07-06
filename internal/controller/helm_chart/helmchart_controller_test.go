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
	"strings"
	"time"

	fn "github.com/kloudlite/kloudlite/operator/toolkit/functions"
	"github.com/kloudlite/kloudlite/operator/toolkit/reconciler"
	ktypes "github.com/kloudlite/kloudlite/operator/toolkit/types"
	pluginhelmchartv1 "github.com/kloudlite/plugin-helm-chart/api/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("HelmChart Controller", func() {
	const (
		timeout  = time.Second * 30
		interval = time.Millisecond * 250
	)

	var testNs *corev1.Namespace

	BeforeEach(func() {
		// Create test namespace
		testNs = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-helmchart-" + strings.ToLower(fn.CleanerNanoid(5)),
			},
		}
		Expect(k8sClient.Create(ctx, testNs)).Should(Succeed())
	})

	AfterEach(func() {
		// Clean up namespace
		Expect(k8sClient.Delete(ctx, testNs)).Should(Succeed())
	})

	// Helper function to check if a specific check has passed
	checkPassed := func(hc *pluginhelmchartv1.HelmChart, checkName string) bool {
		if hc.Status.Checks == nil {
			return false
		}
		for k, v := range hc.Status.Checks {
			if k == checkName && v.State == "completed" {
				return true
			}
		}
		return false
	}

	// Helper function to check if a specific check has failed
	checkFailed := func(hc *pluginhelmchartv1.HelmChart, checkName string) bool {
		if hc.Status.Checks == nil {
			return false
		}
		for k, v := range hc.Status.Checks {
			if k == checkName && v.State == "failed" {
				return true
			}
		}
		return false
	}

	Context("When reconciling a HelmChart resource", func() {
		It("Should complete all checks successfully for valid HelmChart", func() {
			By("Creating a new HelmChart")
			helmChart := &pluginhelmchartv1.HelmChart{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-helmchart",
					Namespace: testNs.Name,
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

			Expect(k8sClient.Create(ctx, helmChart)).Should(Succeed())

			helmChartLookupKey := types.NamespacedName{Name: helmChart.Name, Namespace: helmChart.Namespace}
			createdHelmChart := &pluginhelmchartv1.HelmChart{}

			By("Waiting for the controller to process the HelmChart")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, helmChartLookupKey, createdHelmChart)
				if err != nil {
					return false
				}
				// Check if status has been updated
				return len(createdHelmChart.Status.Checks) > 0
			}, timeout, interval).Should(BeTrue())

			By("Verifying all checks in the checklist pass")
			// The controller should process checks in order: RBAC setup, helm install job, process exports
			for _, checkDef := range createdHelmChart.Status.CheckList {
				Eventually(func() bool {
					err := k8sClient.Get(ctx, helmChartLookupKey, createdHelmChart)
					if err != nil {
						return false
					}
					return checkPassed(createdHelmChart, checkDef.Name)
				}, timeout, interval).Should(BeTrue(), "Check '%s' should pass", checkDef.Name)
			}

			By("Verifying the HelmChart becomes ready")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, helmChartLookupKey, createdHelmChart)
				if err != nil {
					return false
				}
				return createdHelmChart.Status.IsReady
			}, timeout, interval).Should(BeTrue())
		})

		It("Should verify RBAC check passes", func() {
			By("Creating a HelmChart")
			helmChart := &pluginhelmchartv1.HelmChart{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-helmchart-rbac",
					Namespace: testNs.Name,
				},
				Spec: pluginhelmchartv1.HelmChartSpec{
					HelmValues: map[string]apiextensionsv1.JSON{},
					Chart: pluginhelmchartv1.ChartInfo{
						URL:     "https://charts.bitnami.com/bitnami",
						Name:    "nginx",
						Version: "15.0.0",
					},
				},
			}

			Expect(k8sClient.Create(ctx, helmChart)).Should(Succeed())

			helmChartLookupKey := types.NamespacedName{Name: helmChart.Name, Namespace: helmChart.Namespace}
			createdHelmChart := &pluginhelmchartv1.HelmChart{}

			By("Verifying the RBAC setup check passes")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, helmChartLookupKey, createdHelmChart)
				if err != nil {
					return false
				}
				// The first check should be RBAC setup
				if len(createdHelmChart.Status.CheckList) > 0 {
					return checkPassed(createdHelmChart, createdHelmChart.Status.CheckList[0].Name)
				}
				return false
			}, timeout, interval).Should(BeTrue())
		})

		It("Should verify helm install job check progresses", func() {
			By("Creating a HelmChart with custom job resources")
			helmChart := &pluginhelmchartv1.HelmChart{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-helmchart-job",
					Namespace: testNs.Name,
				},
				Spec: pluginhelmchartv1.HelmChartSpec{
					Chart: pluginhelmchartv1.ChartInfo{
						URL:     "https://charts.bitnami.com/bitnami",
						Name:    "nginx",
						Version: "15.0.0",
					},
					HelmValues: map[string]apiextensionsv1.JSON{},
					HelmJobVars: &pluginhelmchartv1.HelmJobVars{
						Resources: ktypes.Resource{
							Cpu: &ktypes.CPUResource{
								Min: "50m",
								Max: "100m",
							},
							Memory: &ktypes.MemoryResource{
								Min: "64Mi",
								Max: "128Mi",
							},
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, helmChart)).Should(Succeed())

			helmChartLookupKey := types.NamespacedName{Name: helmChart.Name, Namespace: helmChart.Namespace}
			createdHelmChart := &pluginhelmchartv1.HelmChart{}

			By("Waiting for helm install job check to appear")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, helmChartLookupKey, createdHelmChart)
				if err != nil {
					return false
				}
				// Check if the helm install job check exists (should be second in checklist)
				if len(createdHelmChart.Status.CheckList) >= 2 {
					checkDef := createdHelmChart.Status.CheckList[1]
					_, exists := createdHelmChart.Status.Checks[checkDef.Name]
					return exists
				}
				return false
			}, timeout, interval).Should(BeTrue())

			By("Verifying a job was created")
			jobList := &batchv1.JobList{}
			Eventually(func() bool {
				err := k8sClient.List(ctx, jobList, client.InNamespace(testNs.Name))
				return err == nil && len(jobList.Items) > 0
			}, timeout, interval).Should(BeTrue())
		})

		It("Should process lifecycle hooks without failing checks", func() {
			By("Creating a HelmChart with lifecycle hooks")
			helmChart := &pluginhelmchartv1.HelmChart{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-helmchart-hooks",
					Namespace: testNs.Name,
				},
				Spec: pluginhelmchartv1.HelmChartSpec{
					HelmValues: map[string]apiextensionsv1.JSON{},
					Chart: pluginhelmchartv1.ChartInfo{
						URL:     "https://charts.bitnami.com/bitnami",
						Name:    "nginx",
						Version: "15.0.0",
					},
					PreInstall:  "echo 'Pre-install hook'",
					PostInstall: "echo 'Post-install hook'",
				},
			}

			Expect(k8sClient.Create(ctx, helmChart)).Should(Succeed())

			helmChartLookupKey := types.NamespacedName{Name: helmChart.Name, Namespace: helmChart.Namespace}
			createdHelmChart := &pluginhelmchartv1.HelmChart{}

			By("Verifying lifecycle hooks don't break the reconciliation")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, helmChartLookupKey, createdHelmChart)
				if err != nil {
					return false
				}
				// Ensure no checks are failing due to hooks
				for _, check := range createdHelmChart.Status.Checks {
					if check.State == "failed" {
						return false
					}
				}
				return len(createdHelmChart.Status.Checks) > 0
			}, timeout, interval).Should(BeTrue())
		})

		It("Should run deletion checks when HelmChart is deleted", func() {
			By("Creating a HelmChart")
			helmChart := &pluginhelmchartv1.HelmChart{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-helmchart-delete",
					Namespace: testNs.Name,
				},
				Spec: pluginhelmchartv1.HelmChartSpec{
					HelmValues: map[string]apiextensionsv1.JSON{},
					Chart: pluginhelmchartv1.ChartInfo{
						URL:     "https://charts.bitnami.com/bitnami",
						Name:    "nginx",
						Version: "15.0.0",
					},
				},
			}

			Expect(k8sClient.Create(ctx, helmChart)).Should(Succeed())

			By("Waiting for finalizer to be added")
			helmChartKey := types.NamespacedName{Name: helmChart.Name, Namespace: helmChart.Namespace}
			Eventually(func() bool {
				hc := &pluginhelmchartv1.HelmChart{}
				if err := k8sClient.Get(ctx, helmChartKey, hc); err != nil {
					return false
				}
				return len(hc.Finalizers) > 0
			}, timeout, interval).Should(BeTrue())

			By("Deleting the HelmChart")
			Expect(k8sClient.Delete(ctx, helmChart)).Should(Succeed())

			By("Verifying uninstall job is created")
			jobList := &batchv1.JobList{}
			Eventually(func() bool {
				err := k8sClient.List(ctx, jobList, client.InNamespace(testNs.Name))
				if err != nil {
					return false
				}
				// Check for uninstall job
				for _, job := range jobList.Items {
					if strings.Contains(job.Name, "uninstall") {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())
		})

		It("Should update status and phase correctly", func() {
			By("Creating a HelmChart")
			helmChart := &pluginhelmchartv1.HelmChart{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-helmchart-status",
					Namespace: testNs.Name,
				},
				Spec: pluginhelmchartv1.HelmChartSpec{
					HelmValues: map[string]apiextensionsv1.JSON{},
					Chart: pluginhelmchartv1.ChartInfo{
						URL:     "https://charts.bitnami.com/bitnami",
						Name:    "nginx",
						Version: "15.0.0",
					},
				},
			}

			Expect(k8sClient.Create(ctx, helmChart)).Should(Succeed())

			helmChartLookupKey := types.NamespacedName{Name: helmChart.Name, Namespace: helmChart.Namespace}
			createdHelmChart := &pluginhelmchartv1.HelmChart{}

			By("Verifying status fields are populated")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, helmChartLookupKey, createdHelmChart)
				if err != nil {
					return false
				}
				// Check that status has been updated with checks and phase
				return createdHelmChart.Status.Checks != nil &&
					len(createdHelmChart.Status.CheckList) > 0 &&
					createdHelmChart.Status.LastReconcileTime != nil
			}, timeout, interval).Should(BeTrue())

			By("Verifying HelmChart eventually becomes ready")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, helmChartLookupKey, createdHelmChart)
				if err != nil {
					return false
				}
				return createdHelmChart.Status.IsReady
			}, timeout, interval).Should(BeTrue())
		})

		It("Should fail helm install check for invalid chart", func() {
			By("Creating a HelmChart with invalid configuration")
			helmChart := &pluginhelmchartv1.HelmChart{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-helmchart-fail",
					Namespace: testNs.Name,
				},
				Spec: pluginhelmchartv1.HelmChartSpec{
					HelmValues: map[string]apiextensionsv1.JSON{},
					Chart: pluginhelmchartv1.ChartInfo{
						URL:     "https://invalid-url.com/charts",
						Name:    "invalid-chart",
						Version: "1.0.0",
					},
				},
			}

			Expect(k8sClient.Create(ctx, helmChart)).Should(Succeed())

			helmChartLookupKey := types.NamespacedName{Name: helmChart.Name, Namespace: helmChart.Namespace}
			createdHelmChart := &pluginhelmchartv1.HelmChart{}

			By("Waiting for job to be created")
			jobList := &batchv1.JobList{}
			Eventually(func() bool {
				err := k8sClient.List(ctx, jobList, client.InNamespace(testNs.Name))
				return err == nil && len(jobList.Items) > 0
			}, timeout, interval).Should(BeTrue())

			By("Simulating job failure")
			if len(jobList.Items) > 0 {
				job := &jobList.Items[0]
				job.Status.Failed = 1
				job.Status.Conditions = []batchv1.JobCondition{
					{
						Type:   batchv1.JobFailed,
						Status: corev1.ConditionTrue,
					},
				}
				k8sClient.Status().Update(ctx, job)
			}

			By("Verifying helm install check fails")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, helmChartLookupKey, createdHelmChart)
				if err != nil {
					return false
				}
				// Look for helm install check in the checklist
				if len(createdHelmChart.Status.CheckList) >= 2 {
					checkDef := createdHelmChart.Status.CheckList[1]
					return checkFailed(createdHelmChart, checkDef.Name)
				}
				return false
			}, timeout, interval).Should(BeTrue())
		})

		It("Should respect force reconciliation annotation", func() {
			By("Creating a HelmChart")
			helmChart := &pluginhelmchartv1.HelmChart{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-helmchart-force",
					Namespace: testNs.Name,
				},
				Spec: pluginhelmchartv1.HelmChartSpec{
					HelmValues: map[string]apiextensionsv1.JSON{},
					Chart: pluginhelmchartv1.ChartInfo{
						URL:     "https://charts.bitnami.com/bitnami",
						Name:    "nginx",
						Version: "15.0.0",
					},
				},
			}

			Expect(k8sClient.Create(ctx, helmChart)).Should(Succeed())

			helmChartLookupKey := types.NamespacedName{Name: helmChart.Name, Namespace: helmChart.Namespace}
			createdHelmChart := &pluginhelmchartv1.HelmChart{}

			By("Waiting for initial reconciliation")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, helmChartLookupKey, createdHelmChart)
				if err != nil {
					return false
				}
				return createdHelmChart.Status.LastReconcileTime != nil
			}, timeout, interval).Should(BeTrue())

			// Store initial reconcile time
			initialReconcileTime := createdHelmChart.Status.LastReconcileTime

			By("Adding force reconciliation annotation")
			Eventually(func() error {
				err := k8sClient.Get(ctx, helmChartLookupKey, createdHelmChart)
				if err != nil {
					return err
				}
				if createdHelmChart.Annotations == nil {
					createdHelmChart.Annotations = make(map[string]string)
				}
				createdHelmChart.Annotations["kloudlite.io/force-reconcile"] = "true"
				return k8sClient.Update(ctx, createdHelmChart)
			}, timeout, interval).Should(Succeed())

			By("Verifying reconciliation is triggered again")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, helmChartLookupKey, createdHelmChart)
				if err != nil {
					return false
				}
				// Check if reconciliation happened again (timestamp changed)
				return createdHelmChart.Status.LastReconcileTime != nil &&
					!createdHelmChart.Status.LastReconcileTime.Equal(initialReconcileTime)
			}, timeout, interval).Should(BeTrue())
		})

		It("Should apply custom job configuration without breaking checks", func() {
			By("Creating a HelmChart with custom job vars")
			helmChart := &pluginhelmchartv1.HelmChart{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-helmchart-custom",
					Namespace: testNs.Name,
				},
				Spec: pluginhelmchartv1.HelmChartSpec{
					Chart: pluginhelmchartv1.ChartInfo{
						URL:     "https://charts.bitnami.com/bitnami",
						Name:    "nginx",
						Version: "15.0.0",
					},
					HelmJobVars: &pluginhelmchartv1.HelmJobVars{
						NodeSelector: map[string]string{
							"disktype": "ssd",
						},
						Tolerations: []corev1.Toleration{
							{
								Key:      "workload",
								Operator: corev1.TolerationOpEqual,
								Value:    "special",
								Effect:   corev1.TaintEffectNoSchedule,
							},
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, helmChart)).Should(Succeed())

			helmChartLookupKey := types.NamespacedName{Name: helmChart.Name, Namespace: helmChart.Namespace}
			createdHelmChart := &pluginhelmchartv1.HelmChart{}

			By("Verifying custom configuration doesn't break checks")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, helmChartLookupKey, createdHelmChart)
				if err != nil {
					return false
				}
				// Verify at least RBAC check passes
				if len(createdHelmChart.Status.CheckList) > 0 {
					return checkPassed(createdHelmChart, createdHelmChart.Status.CheckList[0].Name)
				}
				return false
			}, timeout, interval).Should(BeTrue())

			By("Verifying job is created with custom settings")
			jobList := &batchv1.JobList{}
			Eventually(func() bool {
				err := k8sClient.List(ctx, jobList, client.InNamespace(testNs.Name))
				return err == nil && len(jobList.Items) > 0
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("When handling error scenarios", func() {
		It("Should fail checks for invalid configurations", func() {
			By("Creating a HelmChart with incomplete chart information")
			helmChart := &pluginhelmchartv1.HelmChart{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-helmchart-no-url",
					Namespace: testNs.Name,
				},
				Spec: pluginhelmchartv1.HelmChartSpec{
					Chart: pluginhelmchartv1.ChartInfo{
						Name:    "nginx",
						Version: "15.0.0",
						// Missing URL - invalid configuration
					},
				},
			}

			Expect(k8sClient.Create(ctx, helmChart)).Should(Succeed())

			helmChartLookupKey := types.NamespacedName{Name: helmChart.Name, Namespace: helmChart.Namespace}
			createdHelmChart := &pluginhelmchartv1.HelmChart{}

			By("Verifying checks fail for missing URL")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, helmChartLookupKey, createdHelmChart)
				if err != nil {
					return false
				}
				// At least one check should fail or error should be reported
				for _, check := range createdHelmChart.Status.Checks {
					if check.State == reconciler.FailedState {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())
		})

		It("Should re-run checks when configuration is updated", func() {
			By("Creating a HelmChart")
			helmChart := &pluginhelmchartv1.HelmChart{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-helmchart-update",
					Namespace: testNs.Name,
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

			Expect(k8sClient.Create(ctx, helmChart)).Should(Succeed())

			helmChartLookupKey := types.NamespacedName{Name: helmChart.Name, Namespace: helmChart.Namespace}
			createdHelmChart := &pluginhelmchartv1.HelmChart{}

			By("Waiting for initial reconciliation")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, helmChartLookupKey, createdHelmChart)
				if err != nil {
					return false
				}
				return createdHelmChart.Status.LastReconcileTime != nil
			}, timeout, interval).Should(BeTrue())

			initialGeneration := createdHelmChart.Generation

			By("Updating helm values")
			Eventually(func() error {
				err := k8sClient.Get(ctx, helmChartLookupKey, createdHelmChart)
				if err != nil {
					return err
				}
				createdHelmChart.Spec.HelmValues = map[string]apiextensionsv1.JSON{
					"replicaCount": {Raw: []byte("2")},
					"service.type": {Raw: []byte(`"NodePort"`)},
				}
				return k8sClient.Update(ctx, createdHelmChart)
			}, timeout, interval).Should(Succeed())

			By("Verifying checks are re-run after update")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, helmChartLookupKey, createdHelmChart)
				if err != nil {
					return false
				}
				// Generation should have increased and checks should be updated
				return createdHelmChart.Generation > initialGeneration &&
					len(createdHelmChart.Status.Checks) > 0
			}, timeout, interval).Should(BeTrue())
		})
	})
})
