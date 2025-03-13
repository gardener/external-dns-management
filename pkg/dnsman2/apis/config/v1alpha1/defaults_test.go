// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1_test

import (
	"time"

	"github.com/gardener/gardener/pkg/logger"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	componentbaseconfigv1alpha1 "k8s.io/component-base/config/v1alpha1"
	"k8s.io/utils/ptr"

	. "github.com/gardener/external-dns-management/pkg/dnsman2/apis/config/v1alpha1"
)

var _ = Describe("Defaults", func() {
	Describe("DNSManagerConfiguration", func() {
		var obj *DNSManagerConfiguration

		BeforeEach(func() {
			obj = &DNSManagerConfiguration{}
		})

		It("should correctly default the configuration", func() {
			SetObjectDefaults_DNSManagerConfiguration(obj)

			Expect(obj.LogLevel).To(Equal(logger.InfoLevel))
			Expect(obj.LogFormat).To(Equal(logger.FormatJSON))
			Expect(obj.Class).To(Equal(DefaultClass))

			Expect(obj.Server.HealthProbes.BindAddress).To(BeEmpty())
			Expect(obj.Server.HealthProbes.Port).To(Equal(2751))
			Expect(obj.Server.Metrics.BindAddress).To(BeEmpty())
			Expect(obj.Server.Metrics.Port).To(Equal(2753))
		})

		It("should not overwrite custom settings", func() {
			var (
				expectedLogLevel  = "foo"
				expectedLogFormat = "bar"
				expectedServer    = ServerConfiguration{
					HealthProbes: &Server{
						BindAddress: "baz",
						Port:        1,
					},
					Metrics: &Server{
						BindAddress: "bax",
						Port:        2,
					},
				}
			)

			obj.LogLevel = expectedLogLevel
			obj.LogFormat = expectedLogFormat
			obj.Server = expectedServer

			SetObjectDefaults_DNSManagerConfiguration(obj)

			Expect(obj.LogLevel).To(Equal(expectedLogLevel))
			Expect(obj.LogFormat).To(Equal(expectedLogFormat))
			Expect(obj.Server).To(Equal(expectedServer))
		})

		Describe("RuntimeClientConnection", func() {
			It("should not default ContentType and AcceptContentTypes", func() {
				SetObjectDefaults_DNSManagerConfiguration(obj)

				// ContentType fields will be defaulted by client constructors / controller-runtime based on whether a
				// given APIGroup supports protobuf or not. defaults must not touch these, otherwise the integelligent
				// logic will be overwritten
				Expect(obj.ClientConnection.ContentType).To(BeEmpty())
				Expect(obj.ClientConnection.AcceptContentTypes).To(BeEmpty())
			})

			It("should correctly default ClientConnection", func() {
				SetObjectDefaults_DNSManagerConfiguration(obj)

				Expect(obj.ClientConnection).NotTo(BeNil())
				Expect(obj.ClientConnection.ClientConnectionConfiguration).To(Equal(componentbaseconfigv1alpha1.ClientConnectionConfiguration{
					QPS:   100.0,
					Burst: 130,
				}))
				Expect(obj.ClientConnection.CacheResyncPeriod).To(Equal(&metav1.Duration{Duration: time.Hour}))
			})

			It("should correctly default ControlPlaneClientConnection", func() {
				SetObjectDefaults_DNSManagerConfiguration(obj)

				Expect(obj.ControlPlaneClientConnection).NotTo(BeNil())
				Expect(obj.ControlPlaneClientConnection.ClientConnectionConfiguration).To(Equal(componentbaseconfigv1alpha1.ClientConnectionConfiguration{
					QPS:   100.0,
					Burst: 130,
				}))
				Expect(obj.ControlPlaneClientConnection.CacheResyncPeriod).To(Equal(&metav1.Duration{Duration: time.Hour}))
			})
		})

		Describe("leader election settings", func() {
			It("should correctly default leader election settings", func() {
				SetObjectDefaults_DNSManagerConfiguration(obj)

				Expect(obj.LeaderElection).NotTo(BeNil())
				Expect(obj.LeaderElection.LeaderElect).To(PointTo(BeTrue()))
				Expect(obj.LeaderElection.LeaseDuration).To(Equal(metav1.Duration{Duration: 15 * time.Second}))
				Expect(obj.LeaderElection.RenewDeadline).To(Equal(metav1.Duration{Duration: 10 * time.Second}))
				Expect(obj.LeaderElection.RetryPeriod).To(Equal(metav1.Duration{Duration: 2 * time.Second}))
				Expect(obj.LeaderElection.ResourceLock).To(Equal("leases"))
				Expect(obj.LeaderElection.ResourceNamespace).To(Equal("kube-system"))
				Expect(obj.LeaderElection.ResourceName).To(Equal("gardener-dns-controller-manager-leader-election"))
			})

			It("should not overwrite custom settings", func() {
				expectedLeaderElection := componentbaseconfigv1alpha1.LeaderElectionConfiguration{
					LeaderElect:       ptr.To(true),
					ResourceLock:      "foo",
					RetryPeriod:       metav1.Duration{Duration: 40 * time.Second},
					RenewDeadline:     metav1.Duration{Duration: 41 * time.Second},
					LeaseDuration:     metav1.Duration{Duration: 42 * time.Second},
					ResourceNamespace: "other-garden-ns",
					ResourceName:      "lock-object",
				}
				obj.LeaderElection = expectedLeaderElection
				SetObjectDefaults_DNSManagerConfiguration(obj)

				Expect(obj.LeaderElection).To(Equal(expectedLeaderElection))
			})
		})

		Describe("Controller configuration", func() {
			Describe("DNSProvider controller", func() {
				It("should default the object", func() {
					obj := &DNSProviderControllerConfig{}

					SetDefaults_DNSProviderControllerConfig(obj)

					Expect(obj.ConcurrentSyncs).To(PointTo(Equal(1)))
					Expect(obj.SyncPeriod).To(PointTo(Equal(metav1.Duration{Duration: time.Hour})))
					Expect(obj.Namespace).To(Equal("default"))
					Expect(obj.DefaultRateLimits).To(Equal(&RateLimiterOptions{
						Enabled: true,
						QPS:     10,
						Burst:   20,
					}))
					Expect(obj.DefaultTTL).To(Equal(ptr.To[int64](300)))
				})

				It("should not overwrite existing values", func() {
					obj := &DNSProviderControllerConfig{
						ConcurrentSyncs: ptr.To(5),
						SyncPeriod:      &metav1.Duration{Duration: time.Second},
						Namespace:       "foo",
						DefaultRateLimits: &RateLimiterOptions{
							Enabled: false,
							QPS:     1,
							Burst:   2,
						},
					}

					SetDefaults_DNSProviderControllerConfig(obj)

					Expect(obj.ConcurrentSyncs).To(PointTo(Equal(5)))
					Expect(obj.SyncPeriod).To(PointTo(Equal(metav1.Duration{Duration: time.Second})))
					Expect(obj.Namespace).To(Equal("foo"))
					Expect(obj.DefaultRateLimits).To(Equal(&RateLimiterOptions{
						Enabled: false,
						QPS:     1,
						Burst:   2,
					}))
				})
			})
		})
	})
})
