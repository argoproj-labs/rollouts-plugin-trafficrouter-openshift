package mocks

import (
	routev1 "github.com/openshift/api/route/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	Namespace         = "default"
	StableServiceName = "argo-rollouts-stable"
	CanaryServiceName = "argo-rollouts-canary"

	RouteName               = "argo-rollouts"
	ValidRouteName          = "argo-rollouts-valid"
	OutdatedRouteName       = "argo-rollouts-outdated"
	InvalidRouteName        = "argo-rollouts-invalid"
	FalseConditionRouteName = "argo-rollouts-false-condition"

	RouteGeneration          = 1
	RouteDesiredWeight int32 = 20
)

func MakeObjects() []runtime.Object {
	route := newRoute(RouteName)

	validRoute := newRoute(ValidRouteName)

	invalidRoute := newRoute(InvalidRouteName)
	invalidRoute.Status = routev1.RouteStatus{}

	outdatedHttpProxy := newRoute(OutdatedRouteName)
	outdatedHttpProxy.Generation = RouteGeneration + 1

	falseConditionRoute := newRoute(FalseConditionRouteName)
	falseConditionRoute.Status = routev1.RouteStatus{}

	objs := []runtime.Object{
		route,
		validRoute,
		invalidRoute,
		outdatedHttpProxy,
		falseConditionRoute,
	}
	return objs
}

func newRoute(name string) *routev1.Route {
	var desiredWeight *int32
	desiredWeight = new(int32)
	*desiredWeight = RouteDesiredWeight

	var desiredAltWeight *int32
	desiredAltWeight = new(int32)
	*desiredAltWeight = 100 - RouteDesiredWeight

	return &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  Namespace,
			Generation: RouteGeneration,
		},
		Spec: routev1.RouteSpec{
			Host: "http://route.example.com",
			Port: &routev1.RoutePort{TargetPort: intstr.FromInt(8080)},
			To: routev1.RouteTargetReference{
				Weight: desiredWeight,
			},
			AlternateBackends: []routev1.RouteTargetReference{
				routev1.RouteTargetReference{
					Weight: desiredAltWeight,
				},
			},
		},
		Status: routev1.RouteStatus{
			Ingress: []routev1.RouteIngress{},
		},
	}
}
