// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dnsman2_test

import (
	"context"
	"fmt"
	"time"

	gardenerhealthz "github.com/gardener/gardener/pkg/healthz"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	"github.com/onsi/gomega/types"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	gatewayapisv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapisv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/app"
	"github.com/gardener/external-dns-management/pkg/dnsman2/app/appcontext"
	dnsmanclient "github.com/gardener/external-dns-management/pkg/dnsman2/client"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/handler/local"
)

var _ = Describe("Provider/Entry/Source collaboration tests", func() {
	const (
		sourceClusterID = "source-cluster-id"
	)

	var (
		mgrCancel       context.CancelFunc
		testRunID       string
		testNamespace   *corev1.Namespace
		sourceNamespace *corev1.Namespace
		provider1       *v1alpha1.DNSProvider
		provider1Secret *corev1.Secret

		checkForOwnedEntry = func(ownerPrefix string, ownerKey client.ObjectKey, target *string, dnsNames ...string) []*v1alpha1.DNSEntry {
			GinkgoHelper()

			var ownedEntries []*v1alpha1.DNSEntry
			entryList := &v1alpha1.DNSEntryList{}
			Eventually(func(g Gomega) {
				ownedEntries = nil
				g.Expect(testClient.List(ctx, entryList, client.InNamespace(testRunID))).To(Succeed())
				for _, entry := range entryList.Items {
					if entry.Annotations["resources.gardener.cloud/owners"] == fmt.Sprintf("%s:%s/%s/%s", sourceClusterID, ownerPrefix, ownerKey.Namespace, ownerKey.Name) {
						ownedEntries = append(ownedEntries, &entry)
					}
				}
				g.Expect(ownedEntries).To(HaveLen(len(dnsNames)), "unexpected number of owned DNSEntry objects in namespace %s", testRunID)
				for _, dnsName := range dnsNames {
					found := false
					for _, entry := range ownedEntries {
						if entry.Spec.DNSName == dnsName {
							g.Expect(entry.Status.ObservedGeneration).To(Equal(entry.Generation))
							g.Expect(entry.Spec.DNSName).To(Equal(dnsName))
							if target != nil {
								g.Expect(entry.Spec.Targets).To(Equal([]string{*target}))
								g.Expect(entry.Status.State).To(Equal("Ready"), "expected DNSEntry with DNSName %s to be Ready", dnsName)
								g.Expect(entry.Status.DNSName).To(PointTo(Equal(dnsName)), "expected DNSEntry with DNSName %s to have Status.DNSName set", dnsName)
								g.Expect(entry.Status.Targets).To(Equal(entry.Spec.Targets), "expected DNSEntry with DNSName %s to have Status.Targets equal to Spec.Targets", dnsName)
							} else {
								g.Expect(entry.Status.State).To(Equal("Invalid"), "expected DNSEntry with DNSName %s to be Invalid due to missing targets", dnsName)
							}
							found = true
							break
						}
					}
					g.Expect(found).To(BeTrue(), "expected DNSEntry with DNSName %s not found", dnsName)
				}
			}).Should(Succeed())
			return ownedEntries
		}

		checkForOwnedProvider = func(ownerKey client.ObjectKey, expectOwnedProvider bool) *v1alpha1.DNSProvider {
			GinkgoHelper()

			var ownedProvider *v1alpha1.DNSProvider
			providerList := &v1alpha1.DNSProviderList{}
			Eventually(func(g Gomega) {
				var owned []*v1alpha1.DNSProvider
				g.Expect(testClient.List(ctx, providerList, client.InNamespace(testRunID))).To(Succeed())
				for _, provider := range providerList.Items {
					if provider.Annotations["resources.gardener.cloud/owners"] == fmt.Sprintf("%s:dns.gardener.cloud/DNSProvider/%s/%s", sourceClusterID, ownerKey.Namespace, ownerKey.Name) {
						owned = append(owned, &provider)
					}
				}
				if expectOwnedProvider {
					g.Expect(owned).To(HaveLen(1), "unexpected number of owned DNSProvider objects in namespace %s", testRunID)
					ownedProvider = owned[0]
					g.Expect(ownedProvider.Status.ObservedGeneration).To(Equal(ownedProvider.Generation))
				} else {
					g.Expect(owned).To(BeEmpty(), "expected no owned DNSProvider objects in namespace %s", testRunID)
				}
			}).Should(Succeed())
			return ownedProvider
		}

		checkSourceEvents = func(key client.ObjectKey, matcher types.GomegaMatcher) {
			GinkgoHelper()

			events := &corev1.EventList{}
			Expect(sourceClient.List(ctx, events, client.InNamespace(sourceNamespace.Name))).To(Succeed())
			var matchedEvents []corev1.Event
			for _, event := range events.Items {
				if event.InvolvedObject.Namespace == key.Namespace && event.InvolvedObject.Name == key.Name {
					matchedEvents = append(matchedEvents, event)
				}
			}
			success, err := matcher.Match(matchedEvents)
			Expect(err).NotTo(HaveOccurred())
			Expect(success).To(BeTrue(), "events for %s did not match: %s", key, matcher.FailureMessage(matchedEvents))
		}
	)

	BeforeEach(func() {
		if debug {
			SetDefaultEventuallyTimeout(30 * time.Second)
		}

		By("Create test Namespace")
		testNamespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "dnsman2-",
			},
		}
		Expect(testClient.Create(ctx, testNamespace)).To(Succeed())
		log.Info("Created Namespace for test", "namespaceName", testNamespace.Name)
		testRunID = testNamespace.Name

		By("Create test Namespace on source cluster")
		sourceNamespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "source-",
			},
		}
		Expect(sourceClient.Create(ctx, sourceNamespace)).To(Succeed())
		log.Info("Created Namespace for test on source cluster", "namespaceName", sourceNamespace.Name)

		cfg := &config.DNSManagerConfiguration{
			LogLevel:  "debug",
			LogFormat: "text",
			Controllers: config.ControllerConfiguration{
				DNSProvider: config.DNSProviderControllerConfig{
					Namespace:  testRunID,
					DefaultTTL: ptr.To[int64](300),
				},
				DNSEntry: config.DNSEntryControllerConfig{
					ReconciliationDelayAfterUpdate: ptr.To(metav1.Duration{Duration: 10 * time.Millisecond}),
				},
				Source: config.SourceControllerConfig{
					TargetNamespace:        ptr.To(testRunID),
					TargetClusterID:        ptr.To("test-cluster"),
					SourceClusterID:        ptr.To(sourceClusterID),
					DNSProviderReplication: ptr.To(true),
				},
				SkipNameValidation: ptr.To(true),
			},
			DeployCRDs: ptr.To(true),
		}
		cfg.LeaderElection.LeaderElect = false

		By("setting up manager")
		mgr, err := manager.New(sourceRestConfig, manager.Options{
			Metrics: metricsserver.Options{
				BindAddress: "0",
			},
			Logger:                  log,
			Scheme:                  dnsmanclient.ClusterScheme,
			GracefulShutdownTimeout: ptr.To(5 * time.Second),
		})
		Expect(err).ToNot(HaveOccurred())

		controlPlaneCluster, err := cluster.New(controlPlaneRestConfig, func(opts *cluster.Options) {
			opts.Scheme = dnsmanclient.ClusterScheme
			opts.Logger = log

			// use dynamic rest mapper for secondary cluster, which will automatically rediscover resources on NoMatchErrors
			// but is rate-limited to not issue to many discovery calls (rate-limit shared across all reconciliations)
			opts.MapperProvider = apiutil.NewDynamicRESTMapper

			opts.Cache.DefaultNamespaces = map[string]cache.Config{cfg.Controllers.DNSProvider.Namespace: {}}
			opts.Cache.SyncPeriod = ptr.To(1 * time.Hour)

			opts.Client.Cache = &client.CacheOptions{
				DisableFor: []client.Object{
					&corev1.Event{},
				},
			}
		})
		Expect(err).ToNot(HaveOccurred())

		log.Info("Setting up ready check for control plane informer sync")
		Expect(mgr.AddReadyzCheck("control-plane-informer-sync", gardenerhealthz.NewCacheSyncHealthz(controlPlaneCluster.GetCache()))).To(Succeed())

		log.Info("Adding control plane cluster to manager")
		Expect(mgr.Add(controlPlaneCluster)).To(Succeed())

		log.Info("Adding field indexes to informers")
		Expect(app.AddAllFieldIndexesToCluster(ctx, controlPlaneCluster)).To(Succeed())

		Expect(app.DeployCRDs(ctx, log, mgr.GetConfig(), cfg)).To(Succeed())

		By("Adding controllers to manager")
		controllerSwitches := app.ControllerSwitches()
		controllerSwitches.Enabled = []string{"dnsprovider", "dnsentry", "service-source", "ingress-source", "dnsentry-source", "dnsprovider-source", "gatewayapiv1beta1-source", "gatewayapiv1-source"}
		Expect(controllerSwitches.Complete()).To(Succeed())
		addCtx := appcontext.NewAppContext(ctx, log, controlPlaneCluster, cfg)
		Expect(controllerSwitches.Completed().AddToManager(addCtx, mgr)).To(Succeed())

		var mgrContext context.Context
		mgrContext, mgrCancel = context.WithCancel(ctx)

		By("starting manager")
		go func() {
			defer GinkgoRecover()
			err := mgr.Start(mgrContext)
			Expect(err).NotTo(HaveOccurred())
		}()

		DeferCleanup(func() {
			By("stopping manager")
			mgrCancel()
		})

		mcfg := local.MockConfig{
			Account: testRunID,
			Zones: []local.MockZone{
				{DNSName: "first.example.com"},
				{DNSName: "second.example.com"},
			},
		}
		bytes, err := json.Marshal(&mcfg)
		Expect(err).NotTo(HaveOccurred())

		provider1Secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testRunID,
				Name:      "mock1-secret",
			},
			Data: map[string][]byte{},
			Type: corev1.SecretTypeOpaque,
		}
		Expect(testClient.Create(ctx, provider1Secret)).To(Succeed())
		DeferCleanup(func() {
			Expect(testClient.Delete(ctx, provider1Secret)).To(Succeed())
		})
		provider1 = &v1alpha1.DNSProvider{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testRunID,
				Name:      "mock1",
			},
			Spec: v1alpha1.DNSProviderSpec{
				Type:           "local",
				ProviderConfig: &runtime.RawExtension{Raw: bytes},
				SecretRef:      &corev1.SecretReference{Name: "mock1-secret", Namespace: testRunID},
			},
		}
		Expect(testClient.Create(ctx, provider1)).To(Succeed())
		DeferCleanup(func() {
			err := testClient.Delete(ctx, provider1)
			Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
			Eventually(func(g Gomega) {
				g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(provider1), provider1)).NotTo(Succeed())
			}).Should(Succeed())
		})

		Eventually(func(g Gomega) {
			g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(provider1), provider1)).To(Succeed())
			g.Expect(provider1.Status.State).To(Equal("Ready"))
		}).Should(Succeed())
	})

	It("should create an entry for an annotated service resource", func() {
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: sourceNamespace.Name,
				Name:      "test-service",
				Annotations: map[string]string{
					"dns.gardener.cloud/dnsnames": "test-service.first.example.com",
				},
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{{Name: "http", Port: 80, TargetPort: intstr.FromInt32(8080), Protocol: corev1.ProtocolTCP}},
				Type:  corev1.ServiceTypeLoadBalancer,
			},
		}
		Expect(sourceClient.Create(ctx, svc)).To(Succeed())
		checkForOwnedEntry("/Service", client.ObjectKeyFromObject(svc), nil, "test-service.first.example.com")

		By("Update service status")
		Expect(sourceClient.Get(ctx, client.ObjectKeyFromObject(svc), svc)).To(Succeed())
		patch := client.MergeFrom(svc.DeepCopy())
		svc.Status = corev1.ServiceStatus{
			LoadBalancer: corev1.LoadBalancerStatus{
				Ingress: []corev1.LoadBalancerIngress{
					{IP: "1.2.3.4"},
				},
			},
		}
		Expect(sourceClient.Status().Patch(ctx, svc, patch)).To(Succeed())
		checkForOwnedEntry("/Service", client.ObjectKeyFromObject(svc), ptr.To("1.2.3.4"), "test-service.first.example.com")
		checkSourceEvents(client.ObjectKeyFromObject(svc), ContainElements(
			MatchFields(IgnoreExtras, Fields{
				"Reason":  Equal("DNSEntryCreated"),
				"Message": MatchRegexp("test-service.first.example.com: created entry .* in control plane"),
			}),
			MatchFields(IgnoreExtras, Fields{
				"Reason":  Equal("DNSEntryInvalid"),
				"Message": Equal("test-service.first.example.com: no target or text specified"),
			}),
			MatchFields(IgnoreExtras, Fields{
				"Reason":  Equal("DNSEntryUpdated"),
				"Message": MatchRegexp("test-service.first.example.com: updated entry .* in control plane"),
			}),
			MatchFields(IgnoreExtras, Fields{
				"Reason":  Equal("DNSEntryReady"),
				"Message": Equal("test-service.first.example.com: dns entry active"),
			}),
		))

		By("Delete service resource")
		Expect(sourceClient.Delete(ctx, svc)).To(Succeed())
		Eventually(func(g Gomega) {
			err := sourceClient.Get(ctx, client.ObjectKeyFromObject(svc), svc)
			g.Expect(errors.IsNotFound(err)).To(BeTrue())
		}).To(Succeed())
		checkForOwnedEntry("/Service", client.ObjectKeyFromObject(svc), nil)
		checkSourceEvents(client.ObjectKeyFromObject(svc), ContainElements(
			MatchFields(IgnoreExtras, Fields{
				"Reason":  Equal("DNSEntryDeleted"),
				"Message": MatchRegexp("test-service.first.example.com: deleted entry .* in control plane"),
			}),
		))
	})

	It("should create an entry for an annotated ingress resource", func() {
		ingress := &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: sourceNamespace.Name,
				Name:      "test-ingress",
				Annotations: map[string]string{
					"dns.gardener.cloud/dnsnames": "*",
				},
			},
			Spec: networkingv1.IngressSpec{
				Rules: []networkingv1.IngressRule{
					{Host: "test-ingress.first.example.com"},
				},
			},
		}
		Expect(sourceClient.Create(ctx, ingress)).To(Succeed())
		checkForOwnedEntry("networking.k8s.io/Ingress", client.ObjectKeyFromObject(ingress), nil, "test-ingress.first.example.com")

		By("Update ingress status")
		Expect(sourceClient.Get(ctx, client.ObjectKeyFromObject(ingress), ingress)).To(Succeed())
		patch := client.MergeFrom(ingress.DeepCopy())
		ingress.Status = networkingv1.IngressStatus{
			LoadBalancer: networkingv1.IngressLoadBalancerStatus{
				Ingress: []networkingv1.IngressLoadBalancerIngress{
					{IP: "1.2.3.4"},
				},
			},
		}
		Expect(sourceClient.Status().Patch(ctx, ingress, patch)).To(Succeed())
		checkForOwnedEntry("networking.k8s.io/Ingress", client.ObjectKeyFromObject(ingress), ptr.To("1.2.3.4"), "test-ingress.first.example.com")
		checkSourceEvents(client.ObjectKeyFromObject(ingress), ContainElements(
			MatchFields(IgnoreExtras, Fields{
				"Reason":  Equal("DNSEntryCreated"),
				"Message": MatchRegexp("test-ingress.first.example.com: created entry .* in control plane"),
			}),
			MatchFields(IgnoreExtras, Fields{
				"Reason":  Equal("DNSEntryInvalid"),
				"Message": Equal("test-ingress.first.example.com: no target or text specified"),
			}),
			MatchFields(IgnoreExtras, Fields{
				"Reason":  Equal("DNSEntryUpdated"),
				"Message": MatchRegexp("test-ingress.first.example.com: updated entry .* in control plane"),
			}),
			MatchFields(IgnoreExtras, Fields{
				"Reason":  Equal("DNSEntryReady"),
				"Message": Equal("test-ingress.first.example.com: dns entry active"),
			}),
		))

		By("Delete ingress resource")
		Expect(sourceClient.Delete(ctx, ingress)).To(Succeed())
		Eventually(func(g Gomega) {
			err := sourceClient.Get(ctx, client.ObjectKeyFromObject(ingress), ingress)
			g.Expect(errors.IsNotFound(err)).To(BeTrue())
		}).To(Succeed())
		checkForOwnedEntry("networking.k8s.io/Ingress", client.ObjectKeyFromObject(ingress), nil)
		checkSourceEvents(client.ObjectKeyFromObject(ingress), ContainElements(
			MatchFields(IgnoreExtras, Fields{
				"Reason":  Equal("DNSEntryDeleted"),
				"Message": MatchRegexp("test-ingress.first.example.com: deleted entry .* in control plane"),
			}),
		))
	})

	It("should create an entry for an annotated Gateway v1beta1 resource", func() {
		gateway := &gatewayapisv1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: sourceNamespace.Name,
				Name:      "test-gateway",
				Annotations: map[string]string{
					"dns.gardener.cloud/dnsnames": "*",
				},
			},
			Spec: gatewayapisv1beta1.GatewaySpec{
				GatewayClassName: "default",
				Listeners: []gatewayapisv1beta1.Listener{
					{Name: "default", Protocol: gatewayapisv1.HTTPProtocolType, Hostname: ptr.To(gatewayapisv1beta1.Hostname("test-gateway.first.example.com")), Port: 80},
				},
			},
		}
		Expect(sourceClient.Create(ctx, gateway)).To(Succeed())
		checkForOwnedEntry("gateway.networking.k8s.io/Gateway", client.ObjectKeyFromObject(gateway), nil, "test-gateway.first.example.com")

		By("Update gateway status")
		Expect(sourceClient.Get(ctx, client.ObjectKeyFromObject(gateway), gateway)).To(Succeed())
		patch := client.MergeFrom(gateway.DeepCopy())
		gateway.Status = gatewayapisv1beta1.GatewayStatus{
			Addresses: []gatewayapisv1.GatewayStatusAddress{
				{Type: ptr.To(gatewayapisv1beta1.IPAddressType), Value: "1.2.3.4"},
			},
		}
		Expect(sourceClient.Status().Patch(ctx, gateway, patch)).To(Succeed())
		checkForOwnedEntry("gateway.networking.k8s.io/Gateway", client.ObjectKeyFromObject(gateway), ptr.To("1.2.3.4"), "test-gateway.first.example.com")
		checkSourceEvents(client.ObjectKeyFromObject(gateway), ContainElements(
			MatchFields(IgnoreExtras, Fields{
				"Reason":  Equal("DNSEntryCreated"),
				"Message": MatchRegexp("test-gateway.first.example.com: created entry .* in control plane"),
			}),
			MatchFields(IgnoreExtras, Fields{
				"Reason":  Equal("DNSEntryInvalid"),
				"Message": Equal("test-gateway.first.example.com: no target or text specified"),
			}),
			MatchFields(IgnoreExtras, Fields{
				"Reason":  Equal("DNSEntryUpdated"),
				"Message": MatchRegexp("test-gateway.first.example.com: updated entry .* in control plane"),
			}),
			MatchFields(IgnoreExtras, Fields{
				"Reason":  Equal("DNSEntryReady"),
				"Message": Equal("test-gateway.first.example.com: dns entry active"),
			}),
		))

		By("Delete gateway resource")
		Expect(sourceClient.Delete(ctx, gateway)).To(Succeed())
		Eventually(func(g Gomega) {
			err := sourceClient.Get(ctx, client.ObjectKeyFromObject(gateway), gateway)
			g.Expect(errors.IsNotFound(err)).To(BeTrue())
		}).To(Succeed())
		checkForOwnedEntry("gateway.networking.k8s.io/Gateway", client.ObjectKeyFromObject(gateway), nil)
		checkSourceEvents(client.ObjectKeyFromObject(gateway), ContainElements(
			MatchFields(IgnoreExtras, Fields{
				"Reason":  Equal("DNSEntryDeleted"),
				"Message": MatchRegexp("test-gateway.first.example.com: deleted entry .* in control plane"),
			}),
		))
	})

	It("should create an entry for an annotated Gateway v1 resource", func() {
		gateway := &gatewayapisv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: sourceNamespace.Name,
				Name:      "test-gateway",
				Annotations: map[string]string{
					"dns.gardener.cloud/dnsnames": "*",
				},
			},
			Spec: gatewayapisv1.GatewaySpec{
				GatewayClassName: "default",
				Listeners: []gatewayapisv1.Listener{
					{Name: "default", Protocol: gatewayapisv1.HTTPProtocolType, Hostname: ptr.To(gatewayapisv1.Hostname("test-gateway.first.example.com")), Port: 80},
				},
			},
		}
		Expect(sourceClient.Create(ctx, gateway)).To(Succeed())
		checkForOwnedEntry("gateway.networking.k8s.io/Gateway", client.ObjectKeyFromObject(gateway), nil, "test-gateway.first.example.com")

		By("Update gateway status")
		Expect(sourceClient.Get(ctx, client.ObjectKeyFromObject(gateway), gateway)).To(Succeed())
		patch := client.MergeFrom(gateway.DeepCopy())
		gateway.Status = gatewayapisv1.GatewayStatus{
			Addresses: []gatewayapisv1.GatewayStatusAddress{
				{Type: ptr.To(gatewayapisv1.IPAddressType), Value: "1.2.3.4"},
			},
		}
		Expect(sourceClient.Status().Patch(ctx, gateway, patch)).To(Succeed())
		checkForOwnedEntry("gateway.networking.k8s.io/Gateway", client.ObjectKeyFromObject(gateway), ptr.To("1.2.3.4"), "test-gateway.first.example.com")
		checkSourceEvents(client.ObjectKeyFromObject(gateway), ContainElements(
			MatchFields(IgnoreExtras, Fields{
				"Reason":  Equal("DNSEntryCreated"),
				"Message": MatchRegexp("test-gateway.first.example.com: created entry .* in control plane"),
			}),
			MatchFields(IgnoreExtras, Fields{
				"Reason":  Equal("DNSEntryInvalid"),
				"Message": Equal("test-gateway.first.example.com: no target or text specified"),
			}),
			MatchFields(IgnoreExtras, Fields{
				"Reason":  Equal("DNSEntryUpdated"),
				"Message": MatchRegexp("test-gateway.first.example.com: updated entry .* in control plane"),
			}),
			MatchFields(IgnoreExtras, Fields{
				"Reason":  Equal("DNSEntryReady"),
				"Message": Equal("test-gateway.first.example.com: dns entry active"),
			}),
		))

		By("Delete gateway resource")
		Expect(sourceClient.Delete(ctx, gateway)).To(Succeed())
		Eventually(func(g Gomega) {
			err := sourceClient.Get(ctx, client.ObjectKeyFromObject(gateway), gateway)
			g.Expect(errors.IsNotFound(err)).To(BeTrue())
		}).To(Succeed())
		checkForOwnedEntry("gateway.networking.k8s.io/Gateway", client.ObjectKeyFromObject(gateway), nil)
		checkSourceEvents(client.ObjectKeyFromObject(gateway), ContainElements(
			MatchFields(IgnoreExtras, Fields{
				"Reason":  Equal("DNSEntryDeleted"),
				"Message": MatchRegexp("test-gateway.first.example.com: deleted entry .* in control plane"),
			}),
		))
	})

	It("should create an entry for an source DNSEntry", func() {
		sourceEntry := &v1alpha1.DNSEntry{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: sourceNamespace.Name,
				Name:      "test-entry",
			},
			Spec: v1alpha1.DNSEntrySpec{
				DNSName: "test-entry.first.example.com",
			},
		}
		Expect(sourceClient.Create(ctx, sourceEntry)).To(Succeed())
		checkForOwnedEntry("dns.gardener.cloud/DNSEntry", client.ObjectKeyFromObject(sourceEntry), nil, "test-entry.first.example.com")

		By("Set targets")
		Expect(sourceClient.Get(ctx, client.ObjectKeyFromObject(sourceEntry), sourceEntry)).To(Succeed())
		patch := client.MergeFrom(sourceEntry.DeepCopy())
		sourceEntry.Spec.Targets = []string{"1.2.3.4"}
		Expect(sourceClient.Patch(ctx, sourceEntry, patch)).To(Succeed())
		checkForOwnedEntry("dns.gardener.cloud/DNSEntry", client.ObjectKeyFromObject(sourceEntry), ptr.To("1.2.3.4"), "test-entry.first.example.com")
		checkSourceEvents(client.ObjectKeyFromObject(sourceEntry), ContainElements(
			MatchFields(IgnoreExtras, Fields{
				"Reason":  Equal("DNSEntryCreated"),
				"Message": MatchRegexp("test-entry.first.example.com: created entry .* in control plane"),
			}),
			MatchFields(IgnoreExtras, Fields{
				"Reason":  Equal("DNSEntryInvalid"),
				"Message": Equal("test-entry.first.example.com: no target or text specified"),
			}),
			MatchFields(IgnoreExtras, Fields{
				"Reason":  Equal("DNSEntryUpdated"),
				"Message": MatchRegexp("test-entry.first.example.com: updated entry .* in control plane"),
			}),
			MatchFields(IgnoreExtras, Fields{
				"Reason":  Equal("DNSEntryReady"),
				"Message": Equal("test-entry.first.example.com: dns entry active"),
			}),
		))

		Expect(sourceClient.Get(ctx, client.ObjectKeyFromObject(sourceEntry), sourceEntry)).To(Succeed())
		Expect(sourceEntry.Status.ObservedGeneration).To(Equal(sourceEntry.Generation))
		Expect(sourceEntry.Status.State).To(Equal("Ready"))
		Expect(sourceEntry.Status.Message).To(PointTo(Equal("dns entry active")))
		Expect(sourceEntry.Status.DNSName).To(PointTo(Equal("test-entry.first.example.com")))
		Expect(sourceEntry.Status.Targets).To(Equal([]string{"1.2.3.4"}))
		Expect(sourceEntry.Status.ProviderType).To(PointTo(Equal("local")))
		Expect(sourceEntry.Status.Provider).NotTo(BeNil())
		Expect(sourceEntry.Status.Zone).NotTo(BeNil())
		Expect(sourceEntry.Status.LastUpdateTime).NotTo(BeNil())

		By("Delete source DNSEntry resource")
		Expect(sourceClient.Delete(ctx, sourceEntry)).To(Succeed())
		Eventually(func(g Gomega) {
			err := sourceClient.Get(ctx, client.ObjectKeyFromObject(sourceEntry), sourceEntry)
			g.Expect(errors.IsNotFound(err)).To(BeTrue())
		}).To(Succeed())
		checkForOwnedEntry("dns.gardener.cloud/DNSEntry", client.ObjectKeyFromObject(sourceEntry), nil)
		checkSourceEvents(client.ObjectKeyFromObject(sourceEntry), ContainElements(
			MatchFields(IgnoreExtras, Fields{
				"Reason":  Equal("DNSEntryDeleted"),
				"Message": MatchRegexp("test-entry.first.example.com: deleted entry .* in control plane"),
			}),
		))
	})

	It("should create a provider for an source DNSProvider", func() {
		sourceSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: sourceNamespace.Name,
				Name:      "test-provider-secret",
			},
			Data: map[string][]byte{},
			Type: corev1.SecretTypeOpaque,
		}
		Expect(sourceClient.Create(ctx, sourceSecret)).To(Succeed())

		mcfg := local.MockConfig{
			Account: testRunID + "-source",
			Zones: []local.MockZone{
				{DNSName: "source.example.com"},
			},
		}
		bytes, err := json.Marshal(&mcfg)
		Expect(err).NotTo(HaveOccurred())

		sourceProvider := &v1alpha1.DNSProvider{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: sourceNamespace.Name,
				Name:      "test-provider",
			},
			Spec: v1alpha1.DNSProviderSpec{
				Type:           "local",
				ProviderConfig: &runtime.RawExtension{Raw: bytes},
				SecretRef:      &corev1.SecretReference{Name: sourceSecret.Name, Namespace: sourceNamespace.Name}},
		}
		Expect(sourceClient.Create(ctx, sourceProvider)).To(Succeed())
		ownedProvider := checkForOwnedProvider(client.ObjectKeyFromObject(sourceProvider), true)

		Eventually(func(g Gomega) {
			g.Expect(sourceClient.Get(ctx, client.ObjectKeyFromObject(sourceProvider), sourceProvider)).To(Succeed())
			g.Expect(sourceProvider.Status.ObservedGeneration).To(Equal(sourceProvider.Generation))
			g.Expect(sourceProvider.Status.State).To(Equal("Ready"))
		}).Should(Succeed())

		Expect(sourceProvider.Status.Message).To(Equal(ownedProvider.Status.Message))
		Expect(sourceProvider.Status.Domains).To(Equal(ownedProvider.Status.Domains))
		Expect(sourceProvider.Status.Zones).To(Equal(ownedProvider.Status.Zones))
		Expect(sourceProvider.Status.DefaultTTL).To(Equal(ownedProvider.Status.DefaultTTL))
		Expect(sourceProvider.Status.RateLimit).To(Equal(ownedProvider.Status.RateLimit))
		Expect(sourceProvider.Status.LastUpdateTime).To(Equal(ownedProvider.Status.LastUpdateTime))

		By("Delete source DNSProvider resource")
		Expect(sourceClient.Delete(ctx, sourceProvider)).To(Succeed())
		Eventually(func(g Gomega) {
			err := sourceClient.Get(ctx, client.ObjectKeyFromObject(sourceProvider), sourceProvider)
			g.Expect(errors.IsNotFound(err)).To(BeTrue())
		}).To(Succeed())
		checkForOwnedProvider(client.ObjectKeyFromObject(sourceProvider), false)
	})
})
