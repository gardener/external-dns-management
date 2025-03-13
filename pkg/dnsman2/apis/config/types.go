// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	componentbaseconfig "k8s.io/component-base/config"
	componentbaseconfigv1alpha1 "k8s.io/component-base/config/v1alpha1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// DNSManagerConfiguration defines the configuration for the Gardener dns-controller-manager.
type DNSManagerConfiguration struct {
	metav1.TypeMeta
	// ClientConnection specifies the kubeconfig file and the client connection settings for primary
	// cluster containing the certificate and source resources the dns-controller-manager should work on.
	ClientConnection *ClientConnection
	// ControlPlaneClientConnection contains client connection configurations
	// for the cluster containing the provided DNSProviders.
	// If not set, the primary cluster is used.
	ControlPlaneClientConnection *ControlPlaneClientConnection
	// LeaderElection defines the configuration of leader election client.
	LeaderElection componentbaseconfig.LeaderElectionConfiguration
	// LogLevel is the level/severity for the logs. Must be one of [info,debug,error].
	LogLevel string
	// LogFormat is the output format for the logs. Must be one of [text,json].
	LogFormat string
	// Server defines the configuration of the HTTP server.
	Server ServerConfiguration
	// Debugging holds configuration for Debugging related features.
	Debugging *componentbaseconfig.DebuggingConfiguration
	// Controllers defines the configuration of the controllers.
	Controllers ControllerConfiguration
	// Class is the "dns.gardener.cloud/class" the dns-controller-manager is responsible for.
	// If not set, the default class "gardendns" is used.
	Class string
}

// ClientConnection contains client connection configurations
// for the primary cluster (certificates and source resources).
type ClientConnection struct {
	componentbaseconfigv1alpha1.ClientConnectionConfiguration
	// CacheResyncPeriod specifies the duration how often the cache for the cluster is resynced.
	CacheResyncPeriod *metav1.Duration
}

// ControlPlaneClientConnection contains client connection configurations
// for the cluster containing the provided issuers.
type ControlPlaneClientConnection struct {
	componentbaseconfigv1alpha1.ClientConnectionConfiguration
	// CacheResyncPeriod specifies the duration how often the cache for the cluster is resynced.
	CacheResyncPeriod *metav1.Duration
}

// ServerConfiguration contains details for the HTTP(S) servers.
type ServerConfiguration struct {
	// Webhooks is the configuration for the HTTPS webhook server.
	Webhooks Server
	// HealthProbes is the configuration for serving the healthz and readyz endpoints.
	HealthProbes *Server
	// Metrics is the configuration for serving the metrics endpoint.
	Metrics *Server
}

// Server contains information for HTTP(S) server configuration.
type Server struct {
	// BindAddress is the IP address on which to listen for the specified port.
	BindAddress string
	// Port is the port on which to serve requests.
	Port int
}

// ControllerConfiguration defines the configuration of the controllers.
type ControllerConfiguration struct {
	// DNSProvider is the configuration for the DNSProvider controller.
	DNSProvider DNSProviderControllerConfig
}

// DNSProviderControllerConfig is the configuration for the DNSProvider controller.
type DNSProviderControllerConfig struct {
	// ConcurrentSyncs is the number of concurrent worker routines for this controller.
	ConcurrentSyncs *int
	// SyncPeriod is the duration how often the controller performs its reconciliation.
	SyncPeriod *metav1.Duration
	// Namespace is the namespace on the secondary cluster containing the provided DNSProviders.
	Namespace string
	// EnabledProviderTypes is the list of DNS provider types that should be enabled.
	// If not set, all provider types are enabled.
	EnabledProviderTypes []string
	// DisabledProviderTypes is the list of DNS provider types that should be disabled.
	// If not set, no provider types are disabled.
	DisabledProviderTypes []string
	// DefaultRateLimits defines the rate limiter configuration for a DNSProvider account if not overridden by the DNSProvider.
	DefaultRateLimits *RateLimiterOptions
	// DefaultTTL is the default TTL used for DNS entries if not specified explicitly. May be overridden by the DNSProvider.
	DefaultTTL *int64
	// ZoneCacheTTL is the TTL for the cache for the `GetZones` method.
	ZoneCacheTTL *metav1.Duration
}

// RateLimiterOptions defines the rate limiter configuration.
type RateLimiterOptions struct {
	Enabled bool
	QPS     int
	Burst   int
}
