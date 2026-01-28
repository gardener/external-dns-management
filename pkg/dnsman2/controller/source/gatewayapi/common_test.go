// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package gatewayapi

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayapisv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapisv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	dnsmanclient "github.com/gardener/external-dns-management/pkg/dnsman2/client"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/common"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/state"
)

var _ = Describe("Common", func() {
	Describe("#GetGVKV1beta1", func() {
		It("should return correct GVK for Gateway API v1beta1", func() {
			gvk := GetGVKV1beta1()
			Expect(gvk.Group).To(Equal("gateway.networking.k8s.io"))
			Expect(gvk.Version).To(Equal("v1beta1"))
			Expect(gvk.Kind).To(Equal("Gateway"))
		})
	})

	Describe("#GetGVKV1", func() {
		It("should return correct GVK for Gateway API v1", func() {
			gvk := GetGVKV1()
			Expect(gvk.Group).To(Equal("gateway.networking.k8s.io"))
			Expect(gvk.Version).To(Equal("v1"))
			Expect(gvk.Kind).To(Equal("Gateway"))
		})
	})

	Describe("#GetDNSSpecInput", func() {
		var (
			fakeClient client.Client
			reconciler *common.SourceReconciler[client.Object]
		)

		BeforeEach(func() {
			fakeClient = fake.NewClientBuilder().WithScheme(dnsmanclient.ClusterScheme).Build()
			reconciler = &common.SourceReconciler[client.Object]{}

			reconciler.Client = fakeClient
			reconciler.Log = logr.Discard()
			reconciler.State = state.GetState().GetAnnotationState()
		})

		It("should get the DNS spec input for Gateway v1beta1", func() {
			gateway := &gatewayapisv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway",
					Namespace: "default",
					Annotations: map[string]string{
						"dns.gardener.cloud/dnsnames": "wikipedia.org",
					},
				},
				Spec: gatewayapisv1beta1.GatewaySpec{
					Listeners: []gatewayapisv1beta1.Listener{
						{Hostname: ptr.To(gatewayapisv1beta1.Hostname("wikipedia.org"))},
					},
				},
				Status: gatewayapisv1beta1.GatewayStatus{
					Addresses: []gatewayapisv1.GatewayStatusAddress{
						{Type: ptr.To(gatewayapisv1beta1.IPAddressType), Value: "1.0.0.1"},
					},
				},
			}
			input, err := GetDNSSpecInput(context.Background(), reconciler, gateway)
			Expect(err).ToNot(HaveOccurred())
			Expect(input.Names.ToSlice()).To(Equal([]string{"wikipedia.org"}))
			Expect(input.Targets.ToSlice()).To(Equal([]string{"1.0.0.1"}))
		})

		It("should get the DNS spec input for Gateway v1", func() {
			gateway := &gatewayapisv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway",
					Namespace: "default",
					Annotations: map[string]string{
						"dns.gardener.cloud/dnsnames": "example.com",
					},
				},
				Spec: gatewayapisv1.GatewaySpec{
					Listeners: []gatewayapisv1.Listener{
						{Hostname: ptr.To(gatewayapisv1.Hostname("example.com"))},
					},
				},
				Status: gatewayapisv1.GatewayStatus{
					Addresses: []gatewayapisv1.GatewayStatusAddress{
						{Type: ptr.To(gatewayapisv1.IPAddressType), Value: "1.1.1.1"},
					},
				},
			}
			input, err := GetDNSSpecInput(context.Background(), reconciler, gateway)
			Expect(err).ToNot(HaveOccurred())
			Expect(input.Names.ToSlice()).To(Equal([]string{"example.com"}))
			Expect(input.Targets.ToSlice()).To(Equal([]string{"1.1.1.1"}))
		})
	})

	Describe("#ExtractGatewayKeys", func() {
		var (
			route *gatewayapisv1.HTTPRoute
		)

		BeforeEach(func() {
			route = &gatewayapisv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "route-namespace",
				},
				Spec: gatewayapisv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayapisv1.CommonRouteSpec{
						ParentRefs: []gatewayapisv1.ParentReference{
							{Group: ptr.To(gatewayapisv1.Group("gateway.networking.k8s.io")), Kind: ptr.To(gatewayapisv1.Kind("Gateway")), Name: "gateway-with-group-without-namespace"},
							{Group: ptr.To(gatewayapisv1.Group("gateway.networking.k8s.io")), Kind: ptr.To(gatewayapisv1.Kind("Gateway")), Name: "gateway-with-group-and-namespace", Namespace: ptr.To(gatewayapisv1.Namespace("gateway-namespace"))},
							{Group: ptr.To(gatewayapisv1.Group("networking.istio.io")), Kind: ptr.To(gatewayapisv1.Kind("Gateway")), Name: "istio-gateway"},
							{Name: "gateway-with-name-only"},
							{Name: "gateway-with-name-and-namespace", Namespace: ptr.To(gatewayapisv1.Namespace("gateway-namespace"))},
						},
					},
				},
			}
		})

		It("should extract gateway keys for matching group", func() {
			gvk := schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "Gateway"}
			gatewayKeys := ExtractGatewayKeys(gvk, route)
			Expect(gatewayKeys).To(Equal([]client.ObjectKey{
				{Name: "gateway-with-group-without-namespace", Namespace: "route-namespace"},
				{Name: "gateway-with-group-and-namespace", Namespace: "gateway-namespace"},
				{Name: "gateway-with-name-only", Namespace: "route-namespace"},
				{Name: "gateway-with-name-and-namespace", Namespace: "gateway-namespace"},
			}))
		})

		It("should extract gateway keys for refs without a group only when group does not match", func() {
			gvk := schema.GroupVersionKind{Group: "my.networking.io", Version: "v1alpha1", Kind: "Gateway"}
			gatewayKeys := ExtractGatewayKeys(gvk, route)
			Expect(gatewayKeys).To(Equal([]client.ObjectKey{
				{Name: "gateway-with-name-only", Namespace: "route-namespace"},
				{Name: "gateway-with-name-and-namespace", Namespace: "gateway-namespace"},
			}))
		})
	})

	Describe("#getDNSNames", func() {
		var (
			fakeClient client.Client
			reconciler *common.SourceReconciler[client.Object]
			gateway    *gatewayapisv1.Gateway
		)

		BeforeEach(func() {
			fakeClient = fake.NewClientBuilder().WithScheme(dnsmanclient.ClusterScheme).Build()
			reconciler = &common.SourceReconciler[client.Object]{}
			gateway = &gatewayapisv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-gateway",
					Namespace:   "default",
					Annotations: map[string]string{},
				},
				Spec: gatewayapisv1.GatewaySpec{
					Listeners: []gatewayapisv1.Listener{
						{Hostname: ptr.To(gatewayapisv1.Hostname("example.com"))},
						{Hostname: ptr.To(gatewayapisv1.Hostname("wikipedia.org"))},
					},
				},
			}

			reconciler.Client = fakeClient
			reconciler.Log = logr.Discard()
		})

		It("should get a single DNS name based on the annotation", func() {
			gateway.Annotations["dns.gardener.cloud/dnsnames"] = "wikipedia.org"
			names, err := getDNSNames(context.Background(), reconciler, gateway, gateway.Annotations)
			Expect(err).ToNot(HaveOccurred())
			Expect(names.ToSlice()).To(Equal([]string{"wikipedia.org"}))
		})

		It("should get all DNS names with a wildcard annotation", func() {
			gateway.Annotations["dns.gardener.cloud/dnsnames"] = "*"
			names, err := getDNSNames(context.Background(), reconciler, gateway, gateway.Annotations)
			Expect(err).ToNot(HaveOccurred())
			Expect(names.ToSlice()).To(Equal([]string{"example.com", "wikipedia.org"}))
		})

		It("should return an error if an annotated DNS name is not declared by the Gateway's listeners", func() {
			gateway.Annotations["dns.gardener.cloud/dnsnames"] = "notlistened.to"
			_, err := getDNSNames(context.Background(), reconciler, gateway, gateway.Annotations)
			Expect(err).To(MatchError("annotated dns names notlistened.to not declared by gateway.spec.listeners[].hostname"))
		})

		It("should get DNS names for Gateway v1beta1", func() {
			gatewayv1beta1 := &gatewayapisv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway",
					Namespace: "default",
					Annotations: map[string]string{
						"dns.gardener.cloud/dnsnames": "example.com,wikipedia.org",
					},
				},
				Spec: gatewayapisv1beta1.GatewaySpec{
					Listeners: []gatewayapisv1beta1.Listener{
						{Hostname: ptr.To(gatewayapisv1beta1.Hostname("example.com"))},
						{Hostname: ptr.To(gatewayapisv1beta1.Hostname("wikipedia.org"))},
					},
				},
			}
			names, err := getDNSNames(context.Background(), reconciler, gatewayv1beta1, gatewayv1beta1.Annotations)
			Expect(err).ToNot(HaveOccurred())
			Expect(names.ToSlice()).To(Equal([]string{"example.com", "wikipedia.org"}))
		})
	})

	Describe("#extractHosts", func() {
		var (
			fakeClient client.Client
		)

		BeforeEach(func() {
			fakeClient = fake.NewClientBuilder().WithScheme(dnsmanclient.ClusterScheme).Build()
			routev1beta1 := &gatewayapisv1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route-v1beta1",
					Namespace: "default",
				},
				Spec: gatewayapisv1beta1.HTTPRouteSpec{
					Hostnames: []gatewayapisv1beta1.Hostname{
						"foo.gardener.cloud",
						"example.com",
					},
					CommonRouteSpec: gatewayapisv1beta1.CommonRouteSpec{
						ParentRefs: []gatewayapisv1beta1.ParentReference{
							{Name: "gateway-v1beta1-with-route"},
						},
					},
				},
			}
			routev1 := &gatewayapisv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route-v1",
					Namespace: "default",
				},
				Spec: gatewayapisv1.HTTPRouteSpec{
					Hostnames: []gatewayapisv1.Hostname{
						"foo.wikipedia.org",
						"baz.bar.example.com",
						"gardener.cloud",
					},
					CommonRouteSpec: gatewayapisv1.CommonRouteSpec{
						ParentRefs: []gatewayapisv1.ParentReference{
							{Name: "gateway-v1-with-route"},
						},
					},
				},
			}
			Expect(fakeClient.Create(context.Background(), routev1beta1)).To(Succeed())
			Expect(fakeClient.Create(context.Background(), routev1)).To(Succeed())
		})

		It("should extract the hosts based on Gateway v1beta1 listeners", func() {
			gateway := &gatewayapisv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gateway-v1beta1-not-referenced-by-routes",
					Namespace: "default",
				},
				Spec: gatewayapisv1beta1.GatewaySpec{
					Listeners: []gatewayapisv1beta1.Listener{
						{Hostname: ptr.To(gatewayapisv1beta1.Hostname("wikipedia.org"))},
						{Hostname: ptr.To(gatewayapisv1beta1.Hostname("example.com"))},
					},
				},
			}
			hosts, err := extractHosts(context.Background(), fakeClient, gateway)
			Expect(err).ToNot(HaveOccurred())
			Expect(hosts).To(Equal([]string{"wikipedia.org", "example.com"}))
		})

		It("should extract the hosts based on Gateway v1 listeners", func() {
			gateway := &gatewayapisv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gateway-v1-not-referenced-by-routes",
					Namespace: "default",
				},
				Spec: gatewayapisv1.GatewaySpec{
					Listeners: []gatewayapisv1.Listener{
						{Hostname: ptr.To(gatewayapisv1.Hostname("wikipedia.org"))},
						{Hostname: ptr.To(gatewayapisv1.Hostname("example.com"))},
					},
				},
			}
			hosts, err := extractHosts(context.Background(), fakeClient, gateway)
			Expect(err).ToNot(HaveOccurred())
			Expect(hosts).To(Equal([]string{"wikipedia.org", "example.com"}))
		})

		It("should extract additional hosts from HTTPRoutes that reference the Gateway v1beta1", func() {
			gateway := &gatewayapisv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gateway-v1beta1-with-route",
					Namespace: "default",
				},
				Spec: gatewayapisv1beta1.GatewaySpec{
					Listeners: []gatewayapisv1beta1.Listener{
						{Hostname: ptr.To(gatewayapisv1beta1.Hostname("wikipedia.org"))},
						{Hostname: ptr.To(gatewayapisv1beta1.Hostname("*.gardener.cloud"))},
						{Hostname: ptr.To(gatewayapisv1beta1.Hostname("example.com"))},
					},
				},
			}
			hosts, err := extractHosts(context.Background(), fakeClient, gateway)
			Expect(err).ToNot(HaveOccurred())
			Expect(hosts).To(Equal([]string{"wikipedia.org", "*.gardener.cloud", "example.com"}))
		})

		It("should extract additional hosts from HTTPRoutes that reference the Gateway v1", func() {
			gateway := &gatewayapisv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gateway-v1-with-route",
					Namespace: "default",
				},
				Spec: gatewayapisv1.GatewaySpec{
					Listeners: []gatewayapisv1.Listener{
						{Hostname: ptr.To(gatewayapisv1.Hostname("*.wikipedia.org"))},
						{Hostname: ptr.To(gatewayapisv1.Hostname("*.baz.example.com"))},
					},
				},
			}
			hosts, err := extractHosts(context.Background(), fakeClient, gateway)
			Expect(err).ToNot(HaveOccurred())
			Expect(hosts).To(Equal([]string{"*.wikipedia.org", "*.baz.example.com", "baz.bar.example.com", "gardener.cloud"}))
		})
	})

	Describe("#getListeners", func() {
		It("should reject unknown gateway types", func() {
			_, err := getListeners(&v1.Pod{})
			Expect(err).To(MatchError("unknown gateway object: *v1.Pod"))
		})

		It("should return listeners from Gateway API v1beta1", func() {
			expected := []gatewayapisv1.Listener{
				{Name: "test-listener"},
			}
			gateway := &gatewayapisv1beta1.Gateway{
				Spec: gatewayapisv1beta1.GatewaySpec{
					Listeners: expected,
				},
			}
			actual, err := getListeners(gateway)
			Expect(err).ToNot(HaveOccurred())
			Expect(actual).To(Equal(expected))
		})

		It("should return listeners from Gateway API v1", func() {
			expected := []gatewayapisv1.Listener{
				{Name: "test-listener"},
			}
			gateway := &gatewayapisv1.Gateway{
				Spec: gatewayapisv1.GatewaySpec{
					Listeners: expected,
				},
			}
			actual, err := getListeners(gateway)
			Expect(err).ToNot(HaveOccurred())
			Expect(actual).To(Equal(expected))
		})
	})

	Describe("#listHTTPRoutes", func() {
		var (
			fakeClient client.Client
		)

		BeforeEach(func() {
			fakeClient = fake.NewClientBuilder().WithScheme(dnsmanclient.ClusterScheme).Build()

			createRouteV1beta1 := func(routeName, gatewayName string) *gatewayapisv1beta1.HTTPRoute {
				return &gatewayapisv1beta1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      routeName,
						Namespace: "default",
					},
					Spec: gatewayapisv1beta1.HTTPRouteSpec{
						CommonRouteSpec: gatewayapisv1beta1.CommonRouteSpec{
							ParentRefs: []gatewayapisv1beta1.ParentReference{
								{Name: gatewayapisv1beta1.ObjectName(gatewayName)},
							},
						},
					},
				}
			}

			createRouteV1 := func(routeName, gatewayName string) *gatewayapisv1.HTTPRoute {
				return &gatewayapisv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      routeName,
						Namespace: "default",
					},
					Spec: gatewayapisv1.HTTPRouteSpec{
						CommonRouteSpec: gatewayapisv1.CommonRouteSpec{
							ParentRefs: []gatewayapisv1.ParentReference{
								{Name: gatewayapisv1.ObjectName(gatewayName)},
							},
						},
					},
				}
			}

			Expect(fakeClient.Create(context.Background(), createRouteV1beta1("test-route-v1beta1-gateway-0", "gateway-0"))).To(Succeed())
			Expect(fakeClient.Create(context.Background(), createRouteV1beta1("test-route-v1beta1-gateway-1", "gateway-1"))).To(Succeed())
			Expect(fakeClient.Create(context.Background(), createRouteV1("test-route-v1-gateway-0", "gateway-0"))).To(Succeed())
			Expect(fakeClient.Create(context.Background(), createRouteV1("test-route-v1-gateway-1", "gateway-1"))).To(Succeed())
		})

		It("should list HTTPRoutes for Gateway v1alpha1", func() {
			gateway := &gatewayapisv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gateway-1",
					Namespace: "default",
				},
			}
			routes, err := listHTTPRoutes(context.Background(), fakeClient, gateway)
			Expect(err).ToNot(HaveOccurred())
			Expect(routes).To(HaveLen(1))
			Expect(routes[0].Name).To(Equal("test-route-v1beta1-gateway-1"))
		})

		It("should list HTTPRoutes for Gateway v1", func() {
			gateway := &gatewayapisv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gateway-0",
					Namespace: "default",
				},
			}
			routes, err := listHTTPRoutes(context.Background(), fakeClient, gateway)
			Expect(err).ToNot(HaveOccurred())
			Expect(routes).To(HaveLen(1))
			Expect(routes[0].Name).To(Equal("test-route-v1-gateway-0"))
		})

		It("should return an empty list if no HTTPRoute references the Gateway", func() {
			gateway := &gatewayapisv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "non-referenced-gateway",
					Namespace: "default",
				},
			}
			routes, err := listHTTPRoutes(context.Background(), fakeClient, gateway)
			Expect(err).ToNot(HaveOccurred())
			Expect(routes).To(BeEmpty())
		})
	})

	Describe("#listHTTPRoutesFor", func() {
		var (
			fakeClient client.Client
		)

		BeforeEach(func() {
			fakeClient = fake.NewClientBuilder().WithScheme(dnsmanclient.ClusterScheme).Build()
			routev1beta1 := &gatewayapisv1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route-v1beta1",
					Namespace: "default",
				},
			}
			routev1 := &gatewayapisv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route-v1",
					Namespace: "default",
				},
			}

			Expect(fakeClient.Create(context.Background(), routev1beta1)).To(Succeed())
			Expect(fakeClient.Create(context.Background(), routev1)).To(Succeed())
		})

		It("should reject unknown gateway types", func() {
			_, _, err := listHTTPRoutesFor(context.Background(), fakeClient, &v1.Pod{})
			Expect(err).To(MatchError("unknown gateway object: *v1.Pod"))
		})

		It("should list HTTPRoutes for Gateway API v1beta1", func() {
			gateway := &gatewayapisv1beta1.Gateway{}
			routes, gvk, err := listHTTPRoutesFor(context.Background(), fakeClient, gateway)
			Expect(err).NotTo(HaveOccurred())
			Expect(gvk).To(HaveValue(Equal(GetGVKV1beta1())))
			Expect(routes).To(HaveLen(1))
			Expect(routes[0].Name).To(Equal("test-route-v1beta1"))
		})

		It("should list HTTPRoutes for Gateway API v1", func() {
			gateway := &gatewayapisv1.Gateway{}
			routes, gvk, err := listHTTPRoutesFor(context.Background(), fakeClient, gateway)
			Expect(err).NotTo(HaveOccurred())
			Expect(gvk).To(HaveValue(Equal(GetGVKV1())))
			Expect(routes).To(HaveLen(1))
			Expect(routes[0].Name).To(Equal("test-route-v1"))
		})
	})

	Describe("#getTargets", func() {
		It("should return IP targets", func() {
			gateway := &gatewayapisv1.Gateway{
				Status: gatewayapisv1.GatewayStatus{
					Addresses: []gatewayapisv1.GatewayStatusAddress{
						{
							Type:  ptr.To(gatewayapisv1.IPAddressType),
							Value: "1.1.1.1",
						},
					},
				},
			}
			targets, err := getTargets(gateway)
			Expect(err).ToNot(HaveOccurred())
			Expect(targets.ToSlice()).To(Equal([]string{"1.1.1.1"}))
		})

		It("should return hostname targets", func() {
			gateway := &gatewayapisv1.Gateway{
				Status: gatewayapisv1.GatewayStatus{
					Addresses: []gatewayapisv1.GatewayStatusAddress{
						{
							Type:  ptr.To(gatewayapisv1.HostnameAddressType),
							Value: "wikipedia.org",
						},
					},
				},
			}
			targets, err := getTargets(gateway)
			Expect(err).ToNot(HaveOccurred())
			Expect(targets.ToSlice()).To(Equal([]string{"wikipedia.org"}))
		})

		It("should prefer IP over hostname targets", func() {
			gateway := &gatewayapisv1.Gateway{
				Status: gatewayapisv1.GatewayStatus{
					Addresses: []gatewayapisv1.GatewayStatusAddress{
						{
							Type:  ptr.To(gatewayapisv1.IPAddressType),
							Value: "1.1.1.1",
						},
						{
							Type:  ptr.To(gatewayapisv1.HostnameAddressType),
							Value: "wikipedia.org",
						},
					},
				},
			}
			targets, err := getTargets(gateway)
			Expect(err).ToNot(HaveOccurred())
			Expect(targets.ToSlice()).To(Equal([]string{"1.1.1.1"}))
		})

		It("should return targets from Gateway API v1beta1", func() {
			gateway := &gatewayapisv1beta1.Gateway{
				Status: gatewayapisv1beta1.GatewayStatus{
					Addresses: []gatewayapisv1.GatewayStatusAddress{
						{
							Type:  ptr.To(gatewayapisv1.IPAddressType),
							Value: "1.1.1.1",
						},
					},
				},
			}
			targets, err := getTargets(gateway)
			Expect(err).ToNot(HaveOccurred())
			Expect(targets.ToSlice()).To(Equal([]string{"1.1.1.1"}))
		})
	})

	Describe("#getStatusAddresses", func() {
		It("should reject unknown gateway types", func() {
			_, err := getStatusAddresses(&v1.Pod{})
			Expect(err).To(MatchError("unknown gateway object: *v1.Pod"))
		})

		It("should handle Gateway API v1beta1", func() {
			expected := []gatewayapisv1.GatewayStatusAddress{
				{
					Type:  ptr.To(gatewayapisv1.IPAddressType),
					Value: "1.1.1.1",
				},
			}
			gateway := &gatewayapisv1beta1.Gateway{
				Status: gatewayapisv1beta1.GatewayStatus{
					Addresses: expected,
				},
			}
			actual, err := getStatusAddresses(gateway)
			Expect(err).ToNot(HaveOccurred())
			Expect(actual).To(Equal(expected))
		})

		It("should handle Gateway API v1", func() {
			expected := []gatewayapisv1.GatewayStatusAddress{
				{
					Type:  ptr.To(gatewayapisv1.IPAddressType),
					Value: "1.1.1.1",
				},
			}
			gateway := &gatewayapisv1.Gateway{
				Status: gatewayapisv1.GatewayStatus{
					Addresses: expected,
				},
			}
			actual, err := getStatusAddresses(gateway)
			Expect(err).ToNot(HaveOccurred())
			Expect(actual).To(Equal(expected))
		})
	})
})
