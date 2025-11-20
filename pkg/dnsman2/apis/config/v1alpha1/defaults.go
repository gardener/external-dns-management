// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"os"
	"time"

	"github.com/gardener/gardener/pkg/logger"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	componentbaseconfigv1alpha1 "k8s.io/component-base/config/v1alpha1"
	"k8s.io/utils/ptr"
)

func addDefaultingFuncs(scheme *runtime.Scheme) error {
	return RegisterDefaults(scheme)
}

// SetDefaults_DNSManagerConfiguration sets defaults for the configuration of the Gardener dns-controller-manager.
func SetDefaults_DNSManagerConfiguration(obj *DNSManagerConfiguration) {
	if obj.LogLevel == "" {
		obj.LogLevel = logger.InfoLevel
	}
	if obj.LogFormat == "" {
		obj.LogFormat = logger.FormatJSON
	}
	if obj.ClientConnection == nil {
		obj.ClientConnection = &ClientConnection{}
	}
	if obj.ControlPlaneClientConnection == nil {
		obj.ControlPlaneClientConnection = &ControlPlaneClientConnection{}
	}
	if obj.Class == "" {
		obj.Class = DefaultClass
	}
}

// SetDefaults_ClientConnection sets defaults for the primary client connection.
func SetDefaults_ClientConnection(obj *ClientConnection) {
	if obj.QPS == 0.0 {
		obj.QPS = 100.0
	}
	if obj.Burst == 0 {
		obj.Burst = 130
	}
	if obj.CacheResyncPeriod == nil {
		obj.CacheResyncPeriod = &metav1.Duration{Duration: time.Hour}
	}
}

// SetDefaults_ControlPlaneClientConnection sets defaults for the secondary client connection.
func SetDefaults_ControlPlaneClientConnection(obj *ControlPlaneClientConnection) {
	if obj.QPS == 0.0 {
		obj.QPS = 100.0
	}
	if obj.Burst == 0 {
		obj.Burst = 130
	}
	if obj.CacheResyncPeriod == nil {
		obj.CacheResyncPeriod = &metav1.Duration{Duration: time.Hour}
	}
}

// SetDefaults_LeaderElectionConfiguration sets defaults for the leader election of the Gardener dns-controller-manager.
func SetDefaults_LeaderElectionConfiguration(obj *componentbaseconfigv1alpha1.LeaderElectionConfiguration) {
	if obj.ResourceLock == "" {
		// Don't use a constant from the client-go resourcelock package here (resourcelock is not an API package, pulls
		// in some other dependencies and is thereby not suitable to be used in this API package).
		obj.ResourceLock = "leases"
	}

	componentbaseconfigv1alpha1.RecommendedDefaultLeaderElectionConfiguration(obj)

	if obj.ResourceNamespace == "" {
		obj.ResourceNamespace = getDefaultNamespace()
	}
	if obj.ResourceName == "" {
		obj.ResourceName = DefaultLockObjectName
	}
}

// SetDefaults_ServerConfiguration sets defaults for the server configuration.
func SetDefaults_ServerConfiguration(obj *ServerConfiguration) {
	if obj.HealthProbes == nil {
		obj.HealthProbes = &Server{}
	}
	if obj.HealthProbes.Port == 0 {
		obj.HealthProbes.Port = 2751
	}

	if obj.Metrics == nil {
		obj.Metrics = &Server{}
	}
	if obj.Metrics.Port == 0 {
		obj.Metrics.Port = 2753
	}
}

// SetDefaults_DNSProviderControllerConfig sets defaults for the DNSProviderControllerConfig object.
func SetDefaults_DNSProviderControllerConfig(obj *DNSProviderControllerConfig) {
	if obj.ConcurrentSyncs == nil {
		obj.ConcurrentSyncs = ptr.To(2)
	}
	if obj.SyncPeriod == nil {
		obj.SyncPeriod = &metav1.Duration{Duration: time.Hour}
	}
	if obj.ReconciliationTimeout == nil {
		obj.ReconciliationTimeout = &metav1.Duration{Duration: 2 * time.Minute}
	}
	if obj.Namespace == "" {
		obj.Namespace = getDefaultNamespace()
	}
	if obj.DefaultRateLimits == nil {
		obj.DefaultRateLimits = &RateLimiterOptions{
			Enabled: true,
			QPS:     10,
			Burst:   20,
		}
	}
	if obj.DefaultTTL == nil {
		obj.DefaultTTL = ptr.To[int64](300)
	}
}

// SetDefaults_DNSEntryControllerConfig sets defaults for the DNSEntryControllerConfig object.
func SetDefaults_DNSEntryControllerConfig(obj *DNSEntryControllerConfig) {
	if obj.ConcurrentSyncs == nil {
		obj.ConcurrentSyncs = ptr.To(5)
	}
	if obj.ReconciliationTimeout == nil {
		obj.ReconciliationTimeout = &metav1.Duration{Duration: 2 * time.Minute}
	}
}

// SetDefaults_SourceControllerConfig sets defaults for the SourceControllerConfig object.
func SetDefaults_SourceControllerConfig(obj *SourceControllerConfig) {
	if obj.ConcurrentSyncs == nil {
		obj.ConcurrentSyncs = ptr.To(5)
	}
	if ptr.Deref(obj.TargetNamespace, "") == "" {
		obj.TargetNamespace = ptr.To(getDefaultNamespace())
	}
}

func getDefaultNamespace() string {
	if ns := os.Getenv("POD_NAMESPACE"); ns != "" {
		return ns
	}
	return "default"
}
