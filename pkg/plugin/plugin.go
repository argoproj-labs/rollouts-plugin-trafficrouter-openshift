package plugin

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/argoproj-labs/rollouts-plugin-trafficrouter-openshift/pkg/utils"
	"github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	rolloutsPlugin "github.com/argoproj/argo-rollouts/rollout/trafficrouting/plugin/rpc"
	pluginTypes "github.com/argoproj/argo-rollouts/utils/plugin/types"
	routev1 "github.com/openshift/api/route/v1"
	"golang.org/x/exp/slog"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
)

// Type holds this controller type
const Type = "Openshift"

var _ rolloutsPlugin.TrafficRouterPlugin = (*RpcPlugin)(nil)

type RpcPlugin struct {
	IsTest        bool
	dynamicClient dynamic.Interface
	UpdatedRoute  *routev1.Route
}

// OpenshiftTrafficRouting defines the configuration required to use Openshift routes for traffic
type OpenshiftTrafficRouting struct {
	// Routes is an array of strings which refer to the names of the Routes used to route traffic to the service
	Routes []string `json:"routes" protobuf:"bytes,1,name=routes"`
}

func (r *RpcPlugin) InitPlugin() pluginTypes.RpcError {
	if r.IsTest {
		return pluginTypes.RpcError{}
	}

	cfg, err := utils.NewKubeConfig()
	if err != nil {
		return pluginTypes.RpcError{ErrorString: err.Error()}
	}

	r.dynamicClient, err = dynamic.NewForConfig(cfg)
	if err != nil {
		return pluginTypes.RpcError{ErrorString: err.Error()}
	}

	return pluginTypes.RpcError{}
}

// UpdateHash informs a traffic routing reconciler about new canary/stable pod hashes
func (r *RpcPlugin) UpdateHash(ro *v1alpha1.Rollout, canaryHash, stableHash string, additionalDestinations []v1alpha1.WeightDestination) pluginTypes.RpcError {
	return pluginTypes.RpcError{}
}

// SetWeight modifies Nginx Ingress resources to reach desired state
func (r *RpcPlugin) SetWeight(rollout *v1alpha1.Rollout, desiredWeight int32, additionalDestinations []v1alpha1.WeightDestination) pluginTypes.RpcError {
	if err := validateRolloutParameters(rollout); err != nil {
		return pluginTypes.RpcError{ErrorString: err.Error()}
	}

	openshift, err := getOpenshiftRouting(rollout)
	if err != nil {
		return pluginTypes.RpcError{ErrorString: err.Error()}
	}

	ctx := context.Background()

	for _, route := range openshift.Routes {
		slog.Debug("updating route", slog.String("name", route))

		if err := r.updateRoute(ctx, route, rollout, desiredWeight, rollout.Namespace); err != nil {
			slog.Error("failed to update route", slog.String("name", route), slog.Any("err", err))
			return pluginTypes.RpcError{ErrorString: err.Error()}
		}
		slog.Info("successfully updated route", slog.String("name", route))
	}
	return pluginTypes.RpcError{}
}

func (r *RpcPlugin) SetHeaderRoute(ro *v1alpha1.Rollout, headerRouting *v1alpha1.SetHeaderRoute) pluginTypes.RpcError {
	return pluginTypes.RpcError{}
}

func (r *RpcPlugin) SetMirrorRoute(ro *v1alpha1.Rollout, setMirrorRoute *v1alpha1.SetMirrorRoute) pluginTypes.RpcError {
	return pluginTypes.RpcError{}
}

// Verifies weight of routes given by rollout
func (r *RpcPlugin) VerifyWeight(rollout *v1alpha1.Rollout, desiredWeight int32, additionalDestinations []v1alpha1.WeightDestination) (pluginTypes.RpcVerified, pluginTypes.RpcError) {
	return pluginTypes.NotImplemented, pluginTypes.RpcError{}
}

func (r *RpcPlugin) RemoveManagedRoutes(ro *v1alpha1.Rollout) pluginTypes.RpcError {
	return pluginTypes.RpcError{}
}

func (r *RpcPlugin) Type() string {
	return Type
}

func getOpenshiftRouting(rollout *v1alpha1.Rollout) (*OpenshiftTrafficRouting, error) {
	var openshift OpenshiftTrafficRouting
	if err := json.Unmarshal(rollout.Spec.Strategy.Canary.TrafficRouting.Plugins["argoproj-labs/openshift"], &openshift); err != nil {
		return nil, err
	}
	return &openshift, nil
}

// Update default backend weight,
// remove alternateBackends if weight is 0,
// otherwise update alternateBackends
func (r *RpcPlugin) updateRoute(ctx context.Context, routeName string, rollout *v1alpha1.Rollout, desiredWeight int32, namespace string) error {
	// get the route in the given namespace
	openshiftRoute, err := r.getRoute(ctx, namespace, routeName)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			msg := fmt.Sprintf("Route %q not found", routeName)
			slog.Debug("OpenshiftRouteNotFound", msg)
		}
		return err
	}

	// update default backend weight if weight is different
	altWeight := 100 - desiredWeight
	if *openshiftRoute.Spec.To.Weight == altWeight {
		return nil
	}

	slog.Info("updating default backend weight to %d", altWeight)
	openshiftRoute.Spec.To.Weight = &altWeight
	if desiredWeight == 0 {
		slog.Info("deleting alternateBackends")
		openshiftRoute.Spec.AlternateBackends = nil
	} else {
		slog.Info("updating alternate backend weight to %d", desiredWeight)
		openshiftRoute.Spec.AlternateBackends = []routev1.RouteTargetReference{{
			Kind:   "Service",
			Name:   routeName,
			Weight: &desiredWeight,
		}}
	}

	// convert openshiftRoute into map[string]interface{} for Updating the Route using dynamicClient
	m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&openshiftRoute)
	if err != nil {
		return err
	}

	updated, err := r.dynamicClient.Resource(routev1.GroupVersion.WithResource("route")).Namespace(rollout.Namespace).Update(ctx, &unstructured.Unstructured{Object: m}, metav1.UpdateOptions{})
	if err != nil {
		return err
	}

	if r.IsTest {
		var route routev1.Route
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(updated.UnstructuredContent(), &route); err != nil {
			return err
		}
		r.UpdatedRoute = &route
	}
	return err
}

func (r *RpcPlugin) getRoute(ctx context.Context, namespace string, routeName string) (*routev1.Route, error) {
	unstr, err := r.dynamicClient.Resource(routev1.GroupVersion.WithResource("Route")).Namespace(namespace).Get(ctx, routeName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	var openshiftRoute routev1.Route
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstr.UnstructuredContent(), &openshiftRoute); err != nil {
		return nil, err
	}
	return &openshiftRoute, nil

}

func validateRolloutParameters(rollout *v1alpha1.Rollout) error {
	if rollout == nil || rollout.Spec.Strategy.Canary == nil || rollout.Spec.Strategy.Canary.StableService == "" || rollout.Spec.Strategy.Canary.CanaryService == "" {
		return fmt.Errorf("illegal parameter(s)")
	}
	return nil
}
