package plugin

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/argoproj-labs/rollouts-plugin-trafficrouter-openshift/pkg/mocks"
	"github.com/argoproj-labs/rollouts-plugin-trafficrouter-openshift/pkg/utils"

	"github.com/openshift/client-go/route/clientset/versioned/fake"

	"github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	rolloutsPlugin "github.com/argoproj/argo-rollouts/rollout/trafficrouting/plugin/rpc"

	goPlugin "github.com/hashicorp/go-plugin"
	routev1 "github.com/openshift/api/route/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"
)

var testHandshake = goPlugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "ARGO_ROLLOUTS_RPC_PLUGIN",
	MagicCookieValue: "trafficrouter",
}

func TestRunSuccessfully(t *testing.T) {
	utils.InitLogger(slog.LevelDebug)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := runtime.NewScheme()

	if err := routev1.AddToScheme(s); err != nil {
		t.Fatal("unable to add scheme")
	}

	fakeClient := fake.NewSimpleClientset(mocks.MakeObjects()...)
	rpcPluginImp := &RpcPlugin{
		routeClient: fakeClient,
	}

	// pluginMap is the map of plugins we can dispense.
	var pluginMap = map[string]goPlugin.Plugin{
		"RpcTrafficRouterPlugin": &rolloutsPlugin.RpcTrafficRouterPlugin{Impl: rpcPluginImp},
	}

	ch := make(chan *goPlugin.ReattachConfig, 1)
	closeCh := make(chan struct{})
	go goPlugin.Serve(&goPlugin.ServeConfig{
		HandshakeConfig: testHandshake,
		Plugins:         pluginMap,
		Test: &goPlugin.ServeTestConfig{
			Context:          ctx,
			ReattachConfigCh: ch,
			CloseCh:          closeCh,
		},
	})

	// We should get a config
	var config *goPlugin.ReattachConfig
	select {
	case config = <-ch:
	case <-time.After(2000 * time.Millisecond):
		t.Fatal("should've received reattach")
	}
	if config == nil {
		t.Fatal("config should not be nil")
	}

	// Connect!
	c := goPlugin.NewClient(&goPlugin.ClientConfig{
		Cmd:             nil,
		HandshakeConfig: testHandshake,
		Plugins:         pluginMap,
		Reattach:        config,
	})
	client, err := c.Client()
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	// Pinging should work
	if err := client.Ping(); err != nil {
		t.Fatalf("should not err: %s", err)
	}

	// Kill which should do nothing
	c.Kill()
	if err := client.Ping(); err != nil {
		t.Fatalf("should not err: %s", err)
	}

	if err := rpcPluginImp.InitPlugin(); err.HasError() {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		// Canceling should cause an exit
		cancel()
		<-closeCh
	})

	t.Run("SetWeight", func(t *testing.T) {
		rollout := newRollout(mocks.StableServiceName, mocks.CanaryServiceName, mocks.RouteName)
		desiredWeight := int32(30)

		if err := rpcPluginImp.SetWeight(rollout, desiredWeight, []v1alpha1.WeightDestination{}); err.HasError() {
			t.Fatal(err)
		}

		var route *routev1.Route
		route, err = rpcPluginImp.routeClient.RouteV1().Routes(rollout.Namespace).Get(ctx, mocks.RouteName, metav1.GetOptions{})
		if err != nil {
			t.Fatal(err)
		}

		assertWeights := func(t *testing.T, expected, got int32) {
			t.Helper()
			if expected != got {
				t.Errorf("weights don't match expected %d got %d", expected, got)
			}
		}

		// verify if the weights have been updated in the route.
		assertWeights(t, 100-desiredWeight, *route.Spec.To.Weight)
		assertWeights(t, desiredWeight, *route.Spec.AlternateBackends[0].Weight)

	})

}

func newRollout(stableSvc, canarySvc, routeName string) *v1alpha1.Rollout {
	contourConfig := OpenshiftTrafficRouting{
		Routes: []string{routeName},
	}
	encodedContourConfig, err := json.Marshal(contourConfig)
	if err != nil {
		slog.Error("marshal the route's config is failed", slog.Any("err", err))
		os.Exit(1)
	}

	return &v1alpha1.Rollout{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rollout",
			Namespace: "default",
		},
		Spec: v1alpha1.RolloutSpec{
			Strategy: v1alpha1.RolloutStrategy{
				Canary: &v1alpha1.CanaryStrategy{
					StableService: stableSvc,
					CanaryService: canarySvc,
					TrafficRouting: &v1alpha1.RolloutTrafficRouting{
						Plugins: map[string]json.RawMessage{
							"argoproj-labs/openshift": encodedContourConfig,
						},
					},
				},
			},
		},
	}
}
