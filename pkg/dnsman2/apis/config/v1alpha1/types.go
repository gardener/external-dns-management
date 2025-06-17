// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	componentbaseconfigv1alpha1 "k8s.io/component-base/config/v1alpha1"
)

// DefaultClass is the default dns-class
const DefaultClass = "gardendns"

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// DNSManagerConfiguration defines the configuration for the Gardener dns-controller-manager.
type DNSManagerConfiguration struct {
	metav1.TypeMeta `json:",inline"`
	// ClientConnection specifies the kubeconfig file and the client connection settings for primary
	// cluster containing the certificate and source resources the dns-controller-manager should work on.
	ClientConnection *ClientConnection `json:"clientConnection,omitempty"`
	// ControlPlaneClientConnection contains client connection configurations
	// for the cluster containing the provided DNSProviders.
	// If not set, the primary cluster is used.
	// +optional
	ControlPlaneClientConnection *ControlPlaneClientConnection `json:"controlPlaneClientConnection,omitempty"`
	// LeaderElection defines the configuration of leader election client.
	LeaderElection componentbaseconfigv1alpha1.LeaderElectionConfiguration `json:"leaderElection"`
	// LogLevel is the level/severity for the logs. Must be one of [info,debug,error].
	LogLevel string `json:"logLevel"`
	// LogFormat is the output format for the logs. Must be one of [text,json].
	LogFormat string `json:"logFormat"`
	// Server defines the configuration of the HTTP server.
	Server ServerConfiguration `json:"server"`
	// Debugging holds configuration for Debugging related features.
	// +optional
	Debugging *componentbaseconfigv1alpha1.DebuggingConfiguration `json:"debugging,omitempty"`
	// Controllers defines the configuration of the controllers.
	Controllers ControllerConfiguration `json:"controllers"`
	// Class is the "dns.gardener.cloud/class" the dns-controller-manager is responsible for.
	// If not set, the default class "gardendns" is used.
	Class string `json:"class"`
}

// ClientConnection contains client connection configurations
// for the primary cluster (certificates and source resources).
type ClientConnection struct {
	componentbaseconfigv1alpha1.ClientConnectionConfiguration
	// CacheResyncPeriod specifies the duration how often the cache for the cluster is resynced.
	CacheResyncPeriod *metav1.Duration `json:"cacheResyncPeriod"`
}

// ControlPlaneClientConnection contains client connection configurations
// for the cluster containing the provided issuers.
type ControlPlaneClientConnection struct {
	componentbaseconfigv1alpha1.ClientConnectionConfiguration
	// CacheResyncPeriod specifies the duration how often the cache for the cluster is resynced.
	CacheResyncPeriod *metav1.Duration `json:"cacheResyncPeriod"`
}

// ServerConfiguration contains details for the HTTP(S) servers.
type ServerConfiguration struct {
	// Webhooks is the configuration for the HTTPS webhook server.
	Webhooks Server `json:"webhooks"`
	// HealthProbes is the configuration for serving the healthz and readyz endpoints.
	// +optional
	HealthProbes *Server `json:"healthProbes,omitempty"`
	// Metrics is the configuration for serving the metrics endpoint.
	// +optional
	Metrics *Server `json:"metrics,omitempty"`
}

// Server contains information for HTTP(S) server configuration.
type Server struct {
	// BindAddress is the IP address on which to listen for the specified port.
	BindAddress string `json:"bindAddress"`
	// Port is the port on which to serve requests.
	Port int `json:"port"`
}

// ControllerConfiguration defines the configuration of the controllers.
type ControllerConfiguration struct {
	// DNSProvider is the configuration for the DNSProvider controller.
	DNSProvider DNSProviderControllerConfig `json:"dnsProvider"`
	// DNSEntry is the configuration for the DNSEntry controller.
	DNSEntry DNSEntryControllerConfig `json:"dnsEntry"`
}

// DNSProviderControllerConfig is the configuration for the DNSProvider controller.
type DNSProviderControllerConfig struct {
	// ConcurrentSyncs is the number of concurrent worker routines for this controller.
	// +optional
	ConcurrentSyncs *int `json:"concurrentSyncs,omitempty"`
	// SyncPeriod is the duration how often the controller performs its reconciliation.
	// +optional
	SyncPeriod *metav1.Duration `json:"syncPeriod,omitempty"`
	// Namespace is the namespace on the secondary cluster containing the provided DNSProviders.
	Namespace string `json:"namespace"`
	// EnabledProviderTypes is the list of DNS provider types that should be enabled.
	// If not set, all provider types are enabled.
	// +optional
	EnabledProviderTypes []string `json:"enabledProviderTypes,omitempty"`
	// DisabledProviderTypes is the list of DNS provider types that should be disabled.
	// If not set, no provider types are disabled.
	// +optional
	DisabledProviderTypes []string `json:"disabledProviderTypes,omitempty"`
	// DefaultRateLimits defines the rate limiter configuration for a DNSProvider account if not overridden by the DNSProvider.
	// +optional
	DefaultRateLimits *RateLimiterOptions `json:"defaultRateLimits,omitempty"`
	// DefaultTTL is the default TTL used for DNS entries if not specified explicitly. May be overridden by the DNSProvider.
	// +optional
	DefaultTTL *int64 `json:"defaultTTL,omitempty"`
	// ZoneCacheTTL is the TTL for the cache for the `GetZones` method.
	// +optional
	ZoneCacheTTL *metav1.Duration `json:"zoneCacheTTL,omitempty"`
}

// DNSEntryControllerConfig is the configuration for the DNSEntry controller.
type DNSEntryControllerConfig struct {
	// ConcurrentSyncs is the number of concurrent worker routines for this controller.
	// +optional
	ConcurrentSyncs *int `json:"concurrentSyncs,omitempty"`
	// SyncPeriod is the duration how often the controller performs its reconciliation.
	// +optional
	SyncPeriod *metav1.Duration `json:"syncPeriod,omitempty"`
	// MaxConcurrentLookups is the number of concurrent DNS lookups for the lookup processor.
	// +optional
	MaxConcurrentLookups *int `json:"maxConcurrentLookups,omitempty"`
	// DefaultCNAMELookupInterval is the default interval for CNAME lookups in seconds.
	// +optional
	DefaultCNAMELookupInterval *int64 `json:"defaultCNAMELookupInterval,omitempty"`
}

// RateLimiterOptions defines the rate limiter configuration.
type RateLimiterOptions struct {
	Enabled bool `json:"enabled"`
	QPS     int  `json:"qps"`
	Burst   int  `json:"burst"`
}

const (
	// DefaultLockObjectNamespace is the default lock namespace for leader election.
	DefaultLockObjectNamespace = "kube-system"
	// DefaultLockObjectName is the default lock name for leader election.
	DefaultLockObjectName = "gardener-dns-controller-manager-leader-election"
)
