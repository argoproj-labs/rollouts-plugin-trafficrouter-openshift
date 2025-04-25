package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/argoproj-labs/rollouts-plugin-trafficrouter-openshift/pkg/plugin"
	"github.com/argoproj-labs/rollouts-plugin-trafficrouter-openshift/tests/fixtures"
	rolloutFixture "github.com/argoproj-labs/rollouts-plugin-trafficrouter-openshift/tests/fixtures/rollout"
	routeFixture "github.com/argoproj-labs/rollouts-plugin-trafficrouter-openshift/tests/fixtures/route"
	rolloutsv1alpha1 "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	rolloutsclientset "github.com/argoproj/argo-rollouts/pkg/client/clientset/versioned"
	"github.com/argoproj/argo-rollouts/pkg/kubectl-argo-rollouts/cmd/promote"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	routeapi "github.com/openshift/api/route/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var _ = Describe("OpenShift Route Traffic Plugin Tests", func() {
	Context("Test the functionality of the Route plugin", func() {
		var (
			namespace   = fixtures.RolloutsE2ENamespace
			rolloutName = "rollouts-demo"

			k8sClient     kubernetes.Interface
			rolloutClient rolloutsclientset.Interface
			ctx           context.Context
		)

		BeforeEach(func() {
			var err error
			ctx = context.Background()

			err = fixtures.EnsureCleanState()
			Expect(err).ToNot(HaveOccurred())

			k8sClient, err = fixtures.GetK8sClient()
			Expect(err).ToNot(HaveOccurred())

			rolloutClient, err = fixtures.GetRolloutsClient()
			Expect(err).ToNot(HaveOccurred())
		})

		It("should initialize the canary weight to zero when a Rollout is created", func() {
			By("create a Route with predefined weights for stable and canary")
			err := fixtures.ApplyResources("../data/route_with_weights.yaml", namespace)
			Expect(err).ToNot(HaveOccurred())

			By("Route should retain the weights set for canary and stable")
			route := &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rollouts-demo",
					Namespace: namespace,
				},
			}
			Eventually(route, "30s", "1s").Should(routeFixture.HaveWeights(50, 50))

			By("apply the Rollout and verify if the weights have been reinitialized")
			err = fixtures.ApplyResources("../data/sample_rollout.yaml", namespace)
			Expect(err).ToNot(HaveOccurred())

			rollout, err := rolloutClient.ArgoprojV1alpha1().Rollouts(namespace).Get(
				ctx,
				rolloutName,
				metav1.GetOptions{},
			)
			Expect(err).ToNot(HaveOccurred())
			Eventually(rollout, "60s", "1s").Should(rolloutFixture.HavePhase(rolloutsv1alpha1.RolloutPhaseHealthy))

			Eventually(route, "30s", "1s").Should(routeFixture.HaveWeights(100, 0))
		})

		It("should handle a Rollout with multiple steps", func() {
			err := fixtures.ApplyResources("../data/single_route.yaml", namespace)
			Expect(err).ToNot(HaveOccurred())

			By("verify if Rollout is healthy")
			rollout, err := rolloutClient.ArgoprojV1alpha1().Rollouts(namespace).Get(
				ctx,
				rolloutName,
				metav1.GetOptions{},
			)
			Expect(err).ToNot(HaveOccurred())
			Eventually(rollout, "60s", "1s").Should(rolloutFixture.HavePhase(rolloutsv1alpha1.RolloutPhaseHealthy))

			By("verify if the Service and the ReplicaSet is managed by the Rollout")
			canaryServiceName := rollout.Spec.Strategy.Canary.CanaryService
			stableServiceName := rollout.Spec.Strategy.Canary.StableService
			canaryService, err := k8sClient.CoreV1().Services(namespace).Get(ctx, canaryServiceName, metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())
			stableService, err := k8sClient.CoreV1().Services(namespace).Get(ctx, stableServiceName, metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())

			Expect(canaryService.Annotations[rolloutsv1alpha1.ManagedByRolloutsKey]).To(Equal(rollout.Name))
			Expect(stableService.Annotations[rolloutsv1alpha1.ManagedByRolloutsKey]).To(Equal(rollout.Name))

			By("verify if the Route has the required weights")
			routeList := &plugin.OpenshiftTrafficRouting{}
			err = json.Unmarshal(rollout.Spec.Strategy.Canary.TrafficRouting.Plugins["argoproj-labs/openshift"], routeList)
			Expect(err).ToNot(HaveOccurred())

			route := &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      routeList.Routes[0],
					Namespace: namespace,
				},
			}
			Eventually(route, "30s", "1s").Should(routeFixture.HaveWeights(100, 0))

			By("verify if the ReplicaSets are created")
			selector, err := metav1.LabelSelectorAsSelector(rollout.Spec.Selector)
			Expect(err).ToNot(HaveOccurred())
			rsList, err := k8sClient.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector.String()})
			Expect(err).ToNot(HaveOccurred())
			Expect(rsList.Items).To(HaveLen(1))
			Expect(rsList.Items[0].Spec.Template.Labels[rolloutsv1alpha1.DefaultRolloutUniqueLabelKey]).To(Equal(stableService.Spec.Selector[rolloutsv1alpha1.DefaultRolloutUniqueLabelKey]))

			By("update the Rollout and verify if it has reached Paused phase")
			err = rolloutFixture.UpdateWithoutConflict(rollout, func(rollout *rolloutsv1alpha1.Rollout) {
				rollout.Spec.Template.Labels["test"] = "e2e"
			})
			Expect(err).ToNot(HaveOccurred())

			Eventually(rollout, "30s", "1s").Should(rolloutFixture.HavePhase(rolloutsv1alpha1.RolloutPhasePaused))

			By("verify if the traffic weight is updated in the Route")
			Eventually(route, "30s", "1s").Should(routeFixture.HaveWeights(80, 20))

			By("verify if the ReplicaSets are correctly divided between canary and stable")
			rsList, err = k8sClient.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector.String()})
			Expect(err).ToNot(HaveOccurred())
			Expect(rsList.Items).To(HaveLen(2))

			canaryService, err = k8sClient.CoreV1().Services(namespace).Get(ctx, canaryServiceName, metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())
			stableService, err = k8sClient.CoreV1().Services(namespace).Get(ctx, stableServiceName, metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())

			for _, rs := range rsList.Items {
				if rs.ResourceVersion == "1" {
					Expect(rs.Spec.Template.Labels[rolloutsv1alpha1.DefaultRolloutUniqueLabelKey]).To(Equal(
						stableService.Spec.Selector[rolloutsv1alpha1.DefaultRolloutUniqueLabelKey],
					))
					Expect(rs.Spec.Replicas).To(Equal(5))
				} else if rs.ResourceVersion == "2" {
					Expect(rs.Spec.Template.Labels[rolloutsv1alpha1.DefaultRolloutUniqueLabelKey]).To(Equal(
						canaryService.Spec.Selector[rolloutsv1alpha1.DefaultRolloutUniqueLabelKey],
					))
					Expect(rs.Spec.Replicas).To(Equal(1))
				}
			}

			By("promote the Rollout and verify if it has moved to next stage")
			rollout, err = promote.PromoteRollout(rolloutClient.ArgoprojV1alpha1().Rollouts(namespace),
				rolloutName, false, false, false)
			Expect(err).ToNot(HaveOccurred())

			Eventually(rollout, "30s", "1s").Should(rolloutFixture.HavePhase(rolloutsv1alpha1.RolloutPhasePaused))

			By("verify if the traffic weight is updated in the Route")
			Eventually(route, "20s", "1s").Should(routeFixture.HaveWeights(30, 70))

			By("verify if the ReplicaSets are correctly divided between canary and stable")
			rsList, err = k8sClient.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector.String()})
			Expect(err).ToNot(HaveOccurred())
			Expect(rsList.Items).To(HaveLen(2))

			canaryService, err = k8sClient.CoreV1().Services(namespace).Get(ctx, canaryServiceName, metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())
			stableService, err = k8sClient.CoreV1().Services(namespace).Get(ctx, stableServiceName, metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())

			for _, rs := range rsList.Items {
				if rs.ResourceVersion == "1" {
					Expect(rs.Spec.Template.Labels[rolloutsv1alpha1.DefaultRolloutUniqueLabelKey]).To(Equal(
						stableService.Spec.Selector[rolloutsv1alpha1.DefaultRolloutUniqueLabelKey],
					))
					Expect(rs.Spec.Replicas).To(Equal(5))
				} else if rs.ResourceVersion == "2" {
					Expect(rs.Spec.Template.Labels[rolloutsv1alpha1.DefaultRolloutUniqueLabelKey]).To(Equal(
						canaryService.Spec.Selector[rolloutsv1alpha1.DefaultRolloutUniqueLabelKey],
					))
					Expect(rs.Spec.Replicas).To(Equal(3))
				}
			}

			By("promote the Rollout for the final time and verify if the previous version is removed")
			rollout, err = promote.PromoteRollout(rolloutClient.ArgoprojV1alpha1().Rollouts(namespace),
				rolloutName, false, false, false)
			Expect(err).ToNot(HaveOccurred())

			Eventually(rollout, "30s", "1s").Should(rolloutFixture.HavePhase(rolloutsv1alpha1.RolloutPhaseHealthy))

			By("verify if the traffic weight is updated in the Route")
			Eventually(route, "20s", "1s").Should(routeFixture.HaveWeights(100, 0))

			By("verify if the old ReplicaSet is scaled down")
			Expect(rollout).Should(rolloutFixture.HasTransitionedToCanary(5))

		})

		It("should handle multiple Routes that are specified in the plugin", func() {
			err := fixtures.ApplyResources("../data/multiple_routes.yaml", namespace)
			Expect(err).ToNot(HaveOccurred())

			By("verify if Rollout is healthy")
			rollout, err := rolloutClient.ArgoprojV1alpha1().Rollouts(namespace).Get(
				ctx,
				rolloutName,
				metav1.GetOptions{},
			)
			Expect(err).ToNot(HaveOccurred())
			Eventually(rollout, "60s", "1s").Should(rolloutFixture.HavePhase(rolloutsv1alpha1.RolloutPhaseHealthy))

			By("verify if the Routes have the required weights before update")
			routeList := &plugin.OpenshiftTrafficRouting{}
			err = json.Unmarshal(rollout.Spec.Strategy.Canary.TrafficRouting.Plugins["argoproj-labs/openshift"], routeList)
			Expect(err).ToNot(HaveOccurred())
			Expect(routeList.Routes).To(HaveLen(2))

			routeA := &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      routeList.Routes[0],
					Namespace: namespace,
				},
			}
			Eventually(routeA, "30s", "1s").Should(routeFixture.HaveWeights(100, 0))

			routeB := &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      routeList.Routes[1],
					Namespace: namespace,
				},
			}
			Eventually(routeB, "30s", "1s").Should(routeFixture.HaveWeights(100, 0))

			By("update the Rollout and verify if it has reached Paused phase")
			err = rolloutFixture.UpdateWithoutConflict(rollout, func(rollout *rolloutsv1alpha1.Rollout) {
				rollout.Spec.Template.Labels["test"] = "e2e"
			})
			Expect(err).ToNot(HaveOccurred())
			Eventually(rollout, "30s", "1s").Should(rolloutFixture.HavePhase(rolloutsv1alpha1.RolloutPhasePaused))

			By("verify if the Routes have been updated with the correct weights")
			Eventually(routeA, "20s", "1s").Should(routeFixture.HaveWeights(50, 50))
			Eventually(routeB, "20s", "1s").Should(routeFixture.HaveWeights(50, 50))

			// There is a race condition in Rollouts where a call to PromoteRollout too quickly will cause 'Rollout phase mismatch, got Paused, want Healthy'
			// I don't see a way to know when it's possible to promote the Rollout, so for now just wait X seconds
			Eventually(rollout, "30s", "5s").Should(rolloutFixture.HaveExpectedObservedGeneration())
			time.Sleep(10 * time.Second)

			By("promote the Rollout for the final time and verify if the previous version is removed")
			rollout, err = promote.PromoteRollout(rolloutClient.ArgoprojV1alpha1().Rollouts(namespace),
				rolloutName, false, false, false)
			Expect(err).ToNot(HaveOccurred())

			Eventually(rollout, "2m", "5s").Should(rolloutFixture.HavePhase(rolloutsv1alpha1.RolloutPhaseHealthy))

			By("verify if the traffic weight is updated in the Route")
			Eventually(routeA, "20s", "1s").Should(routeFixture.HaveWeights(100, 0))
			Eventually(routeB, "20s", "1s").Should(routeFixture.HaveWeights(100, 0))

			By("verify if the old Replicaset is scaled down")
			Expect(rollout).Should(rolloutFixture.HasTransitionedToCanary(5))
		})

		It("should handle the Rollouts with Experiment and Analysis", func() {
			err := fixtures.ApplyResources("../data/route_with_experiment.yaml", namespace)
			Expect(err).ToNot(HaveOccurred())

			By("verify if Rollout is healthy")
			rollout, err := rolloutClient.ArgoprojV1alpha1().Rollouts(namespace).Get(
				ctx,
				rolloutName,
				metav1.GetOptions{},
			)
			Expect(err).ToNot(HaveOccurred())
			Eventually(rollout, "60s", "1s").Should(rolloutFixture.HavePhase(rolloutsv1alpha1.RolloutPhaseHealthy))

			By("verify if the Route has the required weights")
			routeList := &plugin.OpenshiftTrafficRouting{}
			err = json.Unmarshal(rollout.Spec.Strategy.Canary.TrafficRouting.Plugins["argoproj-labs/openshift"], routeList)
			Expect(err).ToNot(HaveOccurred())

			route := &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      routeList.Routes[0],
					Namespace: namespace,
				},
			}
			Eventually(route, "30s", "1s").Should(routeFixture.HaveWeights(100, 0))

			By("update the Rollout and verify if it has reached Paused phase")
			err = rolloutFixture.UpdateWithoutConflict(rollout, func(rollout *rolloutsv1alpha1.Rollout) {
				rollout.Spec.Template.Labels["test"] = "e2e"
			})
			Expect(err).ToNot(HaveOccurred())
			Eventually(rollout, "30s", "1s").Should(rolloutFixture.HavePhase(rolloutsv1alpha1.RolloutPhasePaused))

			By("verify if the Analysis was successful")
			analysisList, err := rolloutClient.ArgoprojV1alpha1().AnalysisRuns(namespace).List(ctx, metav1.ListOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(analysisList.Items).To(HaveLen(1))

			analysisRun := analysisList.Items[0]
			Expect(analysisRun.OwnerReferences[0].Name).To(Equal(rollout.Name))

			Eventually(func() bool {
				analysisRun, err := rolloutClient.ArgoprojV1alpha1().AnalysisRuns(namespace).Get(ctx, analysisRun.Name, metav1.GetOptions{})
				if err != nil {
					fmt.Println("failed to get AnalysisRun", err)
					return false
				}

				if analysisRun.Status.Phase != rolloutsv1alpha1.AnalysisPhaseSuccessful {
					fmt.Printf("AnalysisRun phase mismatch: got %s, want %s\n", analysisRun.Status.Phase, rolloutsv1alpha1.AnalysisPhaseSuccessful)
					return false
				}
				return true
			}, "30s", "1s").Should(BeTrue())

			By("promote the Rollout and verify if it has moved to next stage")
			_, err = promote.PromoteRollout(rolloutClient.ArgoprojV1alpha1().Rollouts(namespace),
				rolloutName, false, false, false)
			Expect(err).ToNot(HaveOccurred())

			Eventually(rollout, "60s", "1s").Should(rolloutFixture.HavePhase(rolloutsv1alpha1.RolloutPhaseHealthy))

			By("verify if the Experiment was successful")
			expList, err := rolloutClient.ArgoprojV1alpha1().Experiments(namespace).List(ctx, metav1.ListOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(expList.Items).To(HaveLen(1))

			exp := expList.Items[0]
			Expect(exp.OwnerReferences[0].Name).To(Equal(rollout.Name))
			Eventually(func() bool {
				exp, err := rolloutClient.ArgoprojV1alpha1().Experiments(namespace).Get(ctx, exp.Name, metav1.GetOptions{})
				if err != nil {
					fmt.Println("failed to get Experiment", err)
					return false
				}

				if exp.Status.Phase != rolloutsv1alpha1.AnalysisPhaseSuccessful {
					fmt.Printf("Experiment phase mismatch: got %s, want %s\n", exp.Status.Phase, rolloutsv1alpha1.AnalysisPhaseSuccessful)
					return false
				}
				return true
			}, "30s", "1s").Should(BeTrue())

			By("verify if the Rollouts is Healthy and the Route is pointing to the new pods")
			Eventually(rollout, "60s", "1s").Should(rolloutFixture.HavePhase(rolloutsv1alpha1.RolloutPhaseHealthy))
			Eventually(route, "30s", "1s").Should(routeFixture.HaveWeights(100, 0))
			Expect(rollout).Should(rolloutFixture.HasTransitionedToCanary(5))

		})
	})
})
