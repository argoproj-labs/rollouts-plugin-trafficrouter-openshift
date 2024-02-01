package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"log/slog"

	"github.com/argoproj-labs/rollouts-plugin-trafficrouter-openshift/pkg/utils"
	"github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	rolloutsPlugin "github.com/argoproj/argo-rollouts/rollout/trafficrouting/plugin/rpc"
	pluginTypes "github.com/argoproj/argo-rollouts/utils/plugin/types"
	routev1 "github.com/openshift/api/route/v1"
	openshiftclientset "github.com/openshift/client-go/route/clientset/versioned"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Type holds this controller type
const ControllerType = "Openshift"

var _ rolloutsPlugin.TrafficRouterPlugin = (*RpcPlugin)(nil)

type RpcPlugin struct {
	routeClient openshiftclientset.Interface
}

// OpenshiftTrafficRouting defines the configuration required to use Openshift routes for traffic
type OpenshiftTrafficRouting struct {
	// Routes is an array of strings which refer to the names of the Routes used to route traffic to the service
	Routes []string `json:"routes" protobuf:"bytes,1,name=routes"`
}

func (r *RpcPlugin) InitPlugin() pluginTypes.RpcError {
	cfg, err := utils.NewKubeConfig()
	if err != nil {
		return pluginTypes.RpcError{ErrorString: err.Error()}
	}

	r.routeClient, err = openshiftclientset.NewForConfig(cfg)
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
		slog.Info("updating route", slog.String("name", route))
		namespace := rollout.Namespace
		routeName := route
		if strings.Contains(route, "/") {
			namespace = strings.Split(route, "/")[0]
			routeName = strings.Split(route, "/")[1]
		}
		if err := r.updateRoute(ctx, routeName, rollout, desiredWeight, namespace); err != nil {
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
	return ControllerType
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
	openshiftRoute, err := r.routeClient.RouteV1().Routes(namespace).Get(ctx, routeName, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			msg := fmt.Sprintf("Route %q not found", routeName)
			slog.Error("OpenshiftRouteNotFound: " + msg)
		}
		return err
	}

	// update default backend weight if weight is different
	altWeight := 100 - desiredWeight
	if *openshiftRoute.Spec.To.Weight == altWeight {
		return nil
	}

	slog.Info("updating default backend weight to " + string(altWeight))
	openshiftRoute.Spec.To.Weight = &altWeight
	if desiredWeight == 0 {
		slog.Info("deleting alternateBackends")
		openshiftRoute.Spec.AlternateBackends = nil
	} else {
		slog.Info("updating alternate backend weight to " + string(desiredWeight))
		openshiftRoute.Spec.AlternateBackends = []routev1.RouteTargetReference{{
			Kind:   "Service",
			Name:   routeName,
			Weight: &desiredWeight,
		}}
	}

	_, err = r.routeClient.RouteV1().Routes(rollout.Namespace).Update(ctx, openshiftRoute, metav1.UpdateOptions{})
	if err != nil {
		return err
	}

	return err
}

func validateRolloutParameters(rollout *v1alpha1.Rollout) error {
	if rollout == nil || rollout.Spec.Strategy.Canary == nil || rollout.Spec.Strategy.Canary.StableService == "" || rollout.Spec.Strategy.Canary.CanaryService == "" {
		return fmt.Errorf("illegal parameter(s)")
	}
	return nil
}
