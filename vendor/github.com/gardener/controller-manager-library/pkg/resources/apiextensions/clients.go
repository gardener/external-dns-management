/*
 * SPDX-FileCopyrightText: 2020 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 */

package apiextensions

import (
	"fmt"

	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"

	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/server"
)

////////////////////////////////////////////////////////////////////////////////
// WebHookClientSources

// WebhookClientConfig contains the information to make a TLS
// connection with the webhook.
// A similar type exists for admission and conversion webhooks, but these
// are different GO types. Thefore we introduce a dedicated type
// here that can be configured in a comman way and is then mapped
// to the webhook specific types.
type WebhookClientConfig struct {
	// `url` gives the location of the webhook, in standard URL form
	// (`scheme://host:port/path`). Exactly one of `url` or `service`
	// must be specified.
	//
	// The `host` should not refer to a service running in the cluster; use
	// the `service` field instead. The host might be resolved via external
	// DNS in some apiservers (e.g., `kube-apiserver` cannot resolve
	// in-cluster DNS as that would be a layering violation). `host` may
	// also be an IP address.
	//
	// Please note that using `localhost` or `127.0.0.1` as a `host` is
	// risky unless you take great care to run this webhook on all hosts
	// which run an apiserver which might need to make calls to this
	// webhook. Such installs are likely to be non-portable, i.e., not easy
	// to turn up in a new cluster.
	//
	// The scheme must be "https"; the URL must begin with "https://".
	//
	// A path is optional, and if present may be any string permissible in
	// a URL. You may use the path to pass an arbitrary string to the
	// webhook, for example, a cluster identifier.
	//
	// Attempting to use a user or basic auth e.g. "user:password@" is not
	// allowed. Fragments ("#...") and query parameters ("?...") are not
	// allowed, either.
	//
	// +optional
	URL *string

	// `service` is a reference to the service for this webhook. Either
	// `service` or `url` must be specified.
	//
	// If the webhook is running within the cluster, then you should use `service`.
	//
	// +optional
	Service *ServiceReference

	// `caBundle` is a PEM encoded CA bundle which will be used to validate the webhook's server certificate.
	// If unspecified, system trust roots on the apiserver are used.
	// +optional
	CABundle []byte
}

// ServiceReference holds a reference to Service.legacy.k8s.io
type ServiceReference struct {
	// `namespace` is the namespace of the service.
	// Required
	Namespace string
	// `name` is the name of the service.
	// Required
	Name string

	// `path` is an optional URL path which will be sent in any request to
	// this service.
	// +optional
	Path *string

	// If specified, the port on the service that hosting webhook.
	// `port` should be a valid port number (1-65535, inclusive).
	// +optional
	Port int32
}

func (this *ServiceReference) PortP() *int32 {
	if this.Port == 0 {
		return nil
	}
	p := this.Port
	return &p
}

////////////////////////////////////////////////////////////////////////////////

type WebhookClientConfigSource interface {
	WebhookClientConfig() *WebhookClientConfig
}

func (this *WebhookClientConfig) WebhookClientConfig() *WebhookClientConfig {
	return (*WebhookClientConfig)(this)
}

func NewURLWebhookClientConfig(url string, caBundle []byte) WebhookClientConfigSource {
	return &WebhookClientConfig{
		CABundle: caBundle,
		URL:      &url,
	}
}

func NewDNSWebhookClientConfig(dnsName string, path string, caBundle []byte, port ...int) WebhookClientConfigSource {
	url := fmt.Sprintf("https://%s/%s", dnsName, path)
	if len(port) > 0 && port[0] > 0 {
		url = fmt.Sprintf("https://%s:%d/%s", dnsName, port[0], path)
	}
	return NewURLWebhookClientConfig(url, caBundle)
}

func NewRuntimeServiceWebhookClientConfig(name resources.ObjectName, path string, caBundle []byte, port ...int) WebhookClientConfigSource {
	url := fmt.Sprintf("https://%s.%s/%s", name.Name(), name.Namespace(), path)
	if len(port) > 0 && port[0] > 0 {
		url = fmt.Sprintf("https://%s.%s:%d/%s", name.Name(), name.Namespace(), port[0], path)
	}
	return NewURLWebhookClientConfig(url, caBundle)
}

func NewServiceWebhookClientConfig(name resources.ObjectName, port int, path string, caBundle []byte) WebhookClientConfigSource {
	path = server.NormPath(path)
	return &WebhookClientConfig{
		CABundle: caBundle,
		Service: &ServiceReference{
			Namespace: name.Namespace(),
			Name:      name.Name(),
			Path:      &path,
			Port:      int32(port),
		},
	}
}

////////////////////////////////////////////////////////////////////////////////

func toClientConfig(cfg *WebhookClientConfig) *apiextensions.WebhookClientConfig {
	var svc *apiextensions.ServiceReference
	if cfg.Service != nil {
		svc = &apiextensions.ServiceReference{
			Namespace: cfg.Service.Namespace,
			Name:      cfg.Service.Name,
			Path:      cfg.Service.Path,
			Port:      cfg.Service.Port,
		}
	}
	return &apiextensions.WebhookClientConfig{
		URL:      cfg.URL,
		CABundle: append(cfg.CABundle[:0:0], cfg.CABundle...),
		Service:  svc,
	}
}
