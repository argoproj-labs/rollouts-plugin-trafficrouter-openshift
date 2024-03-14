package rollout

import (
	"context"
	"fmt"

	"github.com/argoproj-labs/rollouts-plugin-trafficrouter-openshift/tests/fixtures"
	. "github.com/onsi/gomega"

	rolloutsv1alpha1 "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	matcher "github.com/onsi/gomega/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
)

func HavePhase(status rolloutsv1alpha1.RolloutPhase) matcher.GomegaMatcher {
	return WithTransform(func(rollout *rolloutsv1alpha1.Rollout) bool {
		rolloutClient, err := fixtures.GetRolloutsClient()
		if err != nil {
			fmt.Println("failed to create the Rollout client", err)
			return false
		}

		ctx := context.Background()
		rollout, err = rolloutClient.ArgoprojV1alpha1().Rollouts(rollout.Namespace).Get(
			ctx,
			rollout.Name,
			metav1.GetOptions{},
		)
		if err != nil {
			fmt.Println("failed to get the Rollout", err)
			return false
		}

		if rollout.Status.Phase != status {
			fmt.Printf("Rollout phase mismatch, got %v, want %v\n", rollout.Status.Phase, status)
			return false
		}

		return true
	}, BeTrue())
}

func UpdateWithoutConflict(rollout *rolloutsv1alpha1.Rollout, updateFunc func(rollout *rolloutsv1alpha1.Rollout)) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		rolloutClient, err := fixtures.GetRolloutsClient()
		if err != nil {
			return err
		}

		ctx := context.Background()
		rollout, err := rolloutClient.ArgoprojV1alpha1().Rollouts(rollout.Namespace).Get(ctx, rollout.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		updateFunc(rollout)

		rollout.Spec.Template.Labels["test"] = "e2e"
		_, err = rolloutClient.ArgoprojV1alpha1().Rollouts(rollout.Namespace).Update(ctx, rollout, metav1.UpdateOptions{})
		return err
	})
}

func HasTransitionedToCanary(expectedReplicas int) matcher.GomegaMatcher {
	return WithTransform(func(rollout *rolloutsv1alpha1.Rollout) bool {
		ns := rollout.Namespace
		selector, err := metav1.LabelSelectorAsSelector(rollout.Spec.Selector)
		if err != nil {
			fmt.Println("failed to create a label selector", rollout.Spec.Selector.String())
			return false
		}

		k8sClient, err := fixtures.GetK8sClient()
		if err != nil {
			fmt.Println("failed to get K8s Client", err)
			return false
		}

		ctx := context.Background()
		rsList, err := k8sClient.AppsV1().ReplicaSets(ns).List(ctx, metav1.ListOptions{LabelSelector: selector.String()})
		if err != nil {
			fmt.Println("failed to list ReplicaSets", err)
			return false
		}

		if len(rsList.Items) != 2 {
			fmt.Printf("more than 2 ReplicaSets found for the selector: %v\n", selector.String())
			return false
		}

		canaryName := rollout.Spec.Strategy.Canary.CanaryService
		stableName := rollout.Spec.Strategy.Canary.StableService
		canaryService, err := k8sClient.CoreV1().Services(ns).Get(ctx, canaryName, metav1.GetOptions{})
		if err != nil {
			fmt.Println("failed to get canary service", err)
			return false
		}
		stableService, err := k8sClient.CoreV1().Services(ns).Get(ctx, stableName, metav1.GetOptions{})
		if err != nil {
			fmt.Println("failed to get stable servive", err)
			return false
		}

		for _, rs := range rsList.Items {
			if rs.ResourceVersion == "1" {
				if *rs.Spec.Replicas != 0 {
					fmt.Printf("expected the previous ReplicaSet to be scaled down: %s\n", rs.Name)
					return false
				}
			} else if rs.ResourceVersion == "2" {
				if rs.Spec.Template.Labels[rolloutsv1alpha1.DefaultRolloutUniqueLabelKey] !=
					stableService.Spec.Selector[rolloutsv1alpha1.DefaultRolloutUniqueLabelKey] {
					fmt.Printf("expected the stable service %s to point to the latest ReplicSet %s\n", stableName, rs.Name)
					return false
				}

				if rs.Spec.Template.Labels[rolloutsv1alpha1.DefaultRolloutUniqueLabelKey] !=
					canaryService.Spec.Selector[rolloutsv1alpha1.DefaultRolloutUniqueLabelKey] {
					fmt.Printf("expected the canary service %s to point to the latest ReplicSet %s\n", canaryName, rs.Name)
					return false
				}

				if *rs.Spec.Replicas != int32(expectedReplicas) {
					fmt.Printf("expected the latest ReplicaSet %s to have %d replicas", rs.Name, expectedReplicas)
					return false
				}
			}
		}

		return true
	}, BeTrue())
}
