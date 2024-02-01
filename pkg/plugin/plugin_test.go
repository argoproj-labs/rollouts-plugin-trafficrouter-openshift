package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/argoproj-labs/rollouts-plugin-trafficrouter-openshift/pkg/mocks"
	"github.com/argoproj-labs/rollouts-plugin-trafficrouter-openshift/pkg/utils"

	pluginTypes "github.com/argoproj/argo-rollouts/utils/plugin/types"
	"github.com/openshift/client-go/route/clientset/versioned/fake"

	"github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	rolloutsPlugin "github.com/argoproj/argo-rollouts/rollout/trafficrouting/plugin/rpc"

	goPlugin "github.com/hashicorp/go-plugin"
	routev1 "github.com/openshift/api/route/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
)

var testHandshake = goPlugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "ARGO_ROLLOUTS_RPC_PLUGIN",
	MagicCookieValue: "trafficrouter",
}

var _ = Describe("Test TrafficRouter plugin for OpenShift route", func() {
	var (
		ctx         context.Context
		cancel      context.CancelFunc
		closeCh     chan struct{}
		routePlugin rolloutsPlugin.TrafficRouterPlugin
		fakeClient  *fake.Clientset
	)
	BeforeEach(func() {
		utils.InitLogger(slog.LevelDebug)

		ctx, cancel = context.WithCancel(context.Background())

		s := runtime.NewScheme()
		Expect(routev1.AddToScheme(s)).To(BeNil())

		fakeClient = fake.NewSimpleClientset(mocks.MakeObjects()...)
		routePluginImp := &RpcPlugin{
			routeClient: fakeClient,
		}

		// pluginMap is the map of plugins we can dispense.
		pluginName := "RpcTrafficRouterPlugin"
		var pluginMap = map[string]goPlugin.Plugin{
			pluginName: &rolloutsPlugin.RpcTrafficRouterPlugin{Impl: routePluginImp},
		}

		ch := make(chan *goPlugin.ReattachConfig, 1)
		closeCh = make(chan struct{})
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
			Fail("should've received reattach")
		}

		Expect(config).ToNot(BeNil())

		// Connect!
		c := goPlugin.NewClient(&goPlugin.ClientConfig{
			Cmd:             nil,
			HandshakeConfig: testHandshake,
			Plugins:         pluginMap,
			Reattach:        config,
		})
		client, err := c.Client()
		Expect(err).To(BeNil())

		// Pinging should work
		err = client.Ping()
		Expect(err).ToNot(HaveOccurred())

		// Kill which should do nothing
		c.Kill()
		err = client.Ping()
		Expect(err).ToNot(HaveOccurred())

		// Request the plugin
		rawPlugin, err := client.Dispense(pluginName)
		Expect(err).ToNot(HaveOccurred())

		routePlugin = rawPlugin.(rolloutsPlugin.TrafficRouterPlugin)
	})

	AfterEach(func() {
		cancel()
		<-closeCh
	})

	Context("Test SetWeight function", func() {
		It("should set the correct desired weight", func() {
			rollout := newRollout(mocks.StableServiceName, mocks.CanaryServiceName, mocks.RouteName)
			desiredWeight := int32(30)

			rpcErr := routePlugin.SetWeight(rollout, desiredWeight, []v1alpha1.WeightDestination{})
			Expect(rpcErr.HasError()).To(BeFalse())
			Expect(rpcErr.Error()).To(BeEmpty())

			var route *routev1.Route
			route, err := fakeClient.RouteV1().Routes(rollout.Namespace).Get(ctx, mocks.RouteName, metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())

			// verify if the weights have been updated in the route.
			Expect(100 - desiredWeight).To(Equal(*route.Spec.To.Weight))
			Expect(desiredWeight).To(Equal(*route.Spec.AlternateBackends[0].Weight))
		})

		It("should return an error if the plugin config is not present", func() {
			rollout := newRollout(mocks.StableServiceName, mocks.CanaryServiceName, mocks.RouteName)
			desiredWeight := int32(30)

			// remove the route config from the rollout
			rollout.Spec.Strategy.Canary.TrafficRouting.Plugins = map[string]json.RawMessage{}

			rpcErr := routePlugin.SetWeight(rollout, desiredWeight, []v1alpha1.WeightDestination{})
			Expect(rpcErr.HasError()).To(BeTrue())
			Expect(rpcErr.Error()).To(Equal("unexpected end of JSON input"))
		})

		It("should return an error if the rollout canary strategy is not defined", func() {
			rollout := newRollout(mocks.StableServiceName, mocks.CanaryServiceName, mocks.RouteName)
			desiredWeight := int32(30)

			rollout.Spec.Strategy.Canary = nil
			rpcErr := routePlugin.SetWeight(rollout, desiredWeight, []v1alpha1.WeightDestination{})
			Expect(rpcErr.HasError()).To(BeTrue())
			Expect(rpcErr.Error()).To(Equal("illegal parameter(s)"))
		})

		It("should return an error if the stable/canary service is not defined", func() {
			rollout := newRollout(mocks.StableServiceName, mocks.CanaryServiceName, mocks.RouteName)
			desiredWeight := int32(30)

			rollout.Spec.Strategy.Canary.CanaryService = ""
			rpcErr := routePlugin.SetWeight(rollout, desiredWeight, []v1alpha1.WeightDestination{})
			Expect(rpcErr.HasError()).To(BeTrue())
			Expect(rpcErr.Error()).To(Equal("illegal parameter(s)"))
		})

		It("should return an error if the specified route is not found", func() {
			rollout := newRollout(mocks.StableServiceName, mocks.CanaryServiceName, "test-route")
			desiredWeight := int32(30)

			rpcErr := routePlugin.SetWeight(rollout, desiredWeight, []v1alpha1.WeightDestination{})
			Expect(rpcErr.HasError()).To(BeTrue())
			Expect(rpcErr.Error()).To(Equal(`routes.route.openshift.io "test-route" not found`))
		})

		It("should return an error if the route update fails", func() {
			errMsg := "failed to update route"
			fakeClient.PrependReactor("update", "*", func(action testing.Action) (handled bool, ret runtime.Object, err error) {
				return true, nil, errors.New(errMsg)
			})

			rollout := newRollout(mocks.StableServiceName, mocks.CanaryServiceName, mocks.RouteName)
			desiredWeight := int32(30)

			rpcErr := routePlugin.SetWeight(rollout, desiredWeight, []v1alpha1.WeightDestination{})
			Expect(rpcErr.HasError()).To(BeTrue())
			Expect(rpcErr.Error()).To(Equal(errMsg))
		})

		It("should remove alternate backends if the desired weight is 0", func() {
			rollout := newRollout(mocks.StableServiceName, mocks.CanaryServiceName, mocks.RouteName)

			rpcErr := routePlugin.SetWeight(rollout, 0, []v1alpha1.WeightDestination{})
			Expect(rpcErr.HasError()).To(BeFalse())
			Expect(rpcErr.Error()).To(BeEmpty())

			var route *routev1.Route
			route, err := fakeClient.RouteV1().Routes(rollout.Namespace).Get(ctx, mocks.RouteName, metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())

			Expect(route.Spec.AlternateBackends).To(BeEmpty())
		})

		It("shouldn't update the route if the weight doesn't change", func() {
			rollout := newRollout(mocks.StableServiceName, mocks.CanaryServiceName, mocks.RouteName)
			desiredWeight := int32(80)

			rpcErr := routePlugin.SetWeight(rollout, desiredWeight, []v1alpha1.WeightDestination{})
			Expect(rpcErr.HasError()).To(BeFalse())
			Expect(rpcErr.Error()).To(BeEmpty())

			updated := false
			for _, action := range fakeClient.Actions() {
				GinkgoT().Log("Action", action.GetVerb())
				if action.GetVerb() == "update" {
					updated = true
					break
				}
			}
			Expect(updated).To(BeFalse())
		})
	})

	When("Type() is called", func() {
		It("should return the type of traffic routing reconciler", func() {
			reconcilerType := routePlugin.Type()
			Expect(reconcilerType).To(Equal(ControllerType))
		})
	})

	When("UpdateHash() is called", func() {
		It("should return nil", func() {
			rollout := newRollout(mocks.StableServiceName, mocks.CanaryServiceName, mocks.RouteName)
			rpcErr := routePlugin.UpdateHash(rollout, "abc", "def", []v1alpha1.WeightDestination{})
			Expect(rpcErr.HasError()).To(BeFalse())
			Expect(rpcErr.Error()).To(BeEmpty())
		})
	})

	When("SetHeader() is called", func() {
		It("should return nil", func() {
			rollout := newRollout(mocks.StableServiceName, mocks.CanaryServiceName, mocks.RouteName)
			headerRoute := &v1alpha1.SetHeaderRoute{}
			rpcErr := routePlugin.SetHeaderRoute(rollout, headerRoute)
			Expect(rpcErr.HasError()).To(BeFalse())
			Expect(rpcErr.Error()).To(BeEmpty())
		})
	})

	When("SetMirrorRoute() is called", func() {
		It("should return nil", func() {
			rollout := newRollout(mocks.StableServiceName, mocks.CanaryServiceName, mocks.RouteName)
			mirrotRoute := &v1alpha1.SetMirrorRoute{}
			rpcErr := routePlugin.SetMirrorRoute(rollout, mirrotRoute)
			Expect(rpcErr.HasError()).To(BeFalse())
			Expect(rpcErr.Error()).To(BeEmpty())
		})
	})

	When("VerifyWeight() is called", func() {
		It("should return nil", func() {
			rollout := newRollout(mocks.StableServiceName, mocks.CanaryServiceName, mocks.RouteName)
			desiredWeight := int32(30)
			rpcVerified, rpcErr := routePlugin.VerifyWeight(rollout, desiredWeight, []v1alpha1.WeightDestination{})
			Expect(rpcErr.HasError()).To(BeFalse())
			Expect(rpcErr.Error()).To(BeEmpty())

			Expect(rpcVerified).To(Equal(pluginTypes.NotImplemented))
		})
	})

	When("RemoveManagedRoutes() is called", func() {
		It("should return nil", func() {
			rollout := newRollout(mocks.StableServiceName, mocks.CanaryServiceName, mocks.RouteName)
			rpcErr := routePlugin.RemoveManagedRoutes(rollout)
			Expect(rpcErr.HasError()).To(BeFalse())
			Expect(rpcErr.Error()).To(BeEmpty())
		})
	})

})

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
