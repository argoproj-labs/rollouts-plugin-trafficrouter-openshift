package route

import (
	"context"
	"fmt"

	"github.com/argoproj-labs/rollouts-plugin-trafficrouter-openshift/tests/fixtures"
	. "github.com/onsi/gomega"
	matcher "github.com/onsi/gomega/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	routeapi "github.com/openshift/api/route/v1"
)

func HaveWeights(stableWeight, canaryWeight int32) matcher.GomegaMatcher {
	return WithTransform(func(route *routeapi.Route) bool {
		routeClient, err := fixtures.GetRouteClient()
		if err != nil {
			fmt.Println("failed to create the Route client", err)
			return false
		}

		ctx := context.Background()
		route, err = routeClient.RouteV1().Routes(route.Namespace).Get(ctx, route.Name, metav1.GetOptions{})
		if err != nil {
			fmt.Println("failed to get the route", err)
			return false
		}

		if len(route.Spec.AlternateBackends) > 1 {
			fmt.Println("there shouldn't be more than one alternate backend")
			return false
		}

		if len(route.Spec.AlternateBackends) == 1 && *route.Spec.AlternateBackends[0].Weight != canaryWeight {
			fmt.Printf("Canary weight mismatch: got %d, want %d\n", *route.Spec.AlternateBackends[0].Weight, canaryWeight)
			return false
		}

		if *route.Spec.To.Weight != stableWeight {
			fmt.Printf("Stable weight mismatch: got %d, want %d\n", *route.Spec.To.Weight, stableWeight)
			return false
		}

		return true
	}, BeTrue())
}
