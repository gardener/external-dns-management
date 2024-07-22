// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package gatewayapi

import (
	"fmt"
	"strings"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	gatewayapisv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapisv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayapisv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/source"
)

type httpRouteLister interface {
	ListHTTPRoutes(gateway *resources.ObjectName) ([]resources.ObjectData, error)
}

type gatewaySource struct {
	source.DefaultDNSSource
	lister httpRouteLister
	state  *routesState
}

// NewGatewaySource is the DNSSource for gateways.gateway.networking.k8s.io resources.
func NewGatewaySource(c controller.Interface) (source.DNSSource, error) {
	lister, err := newServiceLister(c)
	if err != nil {
		return nil, err
	}
	state, err := getOrCreateSharedState(c)
	if err != nil {
		return nil, err
	}
	return newGatewaySourceWithRouteLister(lister, state)
}

func newGatewaySourceWithRouteLister(lister httpRouteLister, state *routesState) (source.DNSSource, error) {
	return &gatewaySource{lister: lister, DefaultDNSSource: source.NewDefaultDNSSource(nil), state: state}, nil
}

func (s *gatewaySource) Setup() error {
	routes, err := s.lister.ListHTTPRoutes(nil)
	if err != nil {
		return err
	}
	for _, route := range routes {
		gateways := extractGatewayNames(route)
		s.state.AddRoute(resources.NewObjectNameForData(route), gateways)
	}
	return nil
}

func (s *gatewaySource) GetDNSInfo(_ logger.LogContext, obj resources.ObjectData, current *source.DNSCurrentState) (*source.DNSInfo, error) {
	info := &source.DNSInfo{}
	hosts, err := s.extractServerHosts(obj)
	if err != nil {
		return nil, err
	}
	names := utils.StringSet{}
	all := current.AnnotatedNames.Contains("all") || current.AnnotatedNames.Contains("*")
	for _, host := range hosts {
		if host != "" && (all || current.AnnotatedNames.Contains(host)) {
			names.Add(host)
		}
	}
	_, del := current.AnnotatedNames.DiffFrom(names)
	del.Remove("all")
	del.Remove("*")
	if len(del) > 0 {
		return info, fmt.Errorf("annotated dns names %s not declared by gateway.spec.listeners[].hostname", del)
	}
	info.Names = dns.NewDNSNameSetFromStringSet(names, current.GetSetIdentifier())
	info.Targets = s.getTargets(obj)
	if v := obj.GetAnnotations()[source.RESOLVE_TARGETS_TO_ADDRS_ANNOTATION]; v != "" {
		info.ResolveTargetsToAddresses = ptr.To(v == "true")
	}
	if v := obj.GetAnnotations()[dns.AnnotationIgnore]; v != "" {
		info.Ignore = v == "true"
	}
	return info, nil
}

func (s *gatewaySource) extractServerHosts(obj resources.ObjectData) ([]string, error) {
	var hosts []string
	switch data := obj.(type) {
	case *gatewayapisv1.Gateway:
		for _, listener := range data.Spec.Listeners {
			if listener.Hostname != nil {
				hosts = append(hosts, string(*listener.Hostname))
			}
		}
	case *gatewayapisv1beta1.Gateway:
		for _, listener := range data.Spec.Listeners {
			if listener.Hostname != nil {
				hosts = append(hosts, string(*listener.Hostname))
			}
		}
	case *gatewayapisv1alpha2.Gateway:
		for _, listener := range data.Spec.Listeners {
			if listener.Hostname != nil {
				hosts = append(hosts, string(*listener.Hostname))
			}
		}
	default:
		return nil, fmt.Errorf("unexpected istio gateway type: %#v", obj)
	}

	routes, err := s.lister.ListHTTPRoutes(ptr.To(resources.NewObjectNameForData(obj)))
	if err != nil {
		return nil, err
	}

	addHost := func(hosts []string, host string) []string {
		for _, h := range hosts {
			if h == host {
				return hosts
			}
			if strings.HasPrefix(h, "*.") && strings.HasSuffix(host, h[1:]) && !strings.Contains(host[:len(host)-len(h)+1], ".") {
				return hosts
			}
		}
		return append(hosts, host)
	}

	for _, route := range routes {
		switch r := route.(type) {
		case *gatewayapisv1.HTTPRoute:
			for _, h := range r.Spec.Hostnames {
				hosts = addHost(hosts, string(h))
			}
		case *gatewayapisv1beta1.HTTPRoute:
			for _, h := range r.Spec.Hostnames {
				hosts = addHost(hosts, string(h))
			}
		case *gatewayapisv1alpha2.HTTPRoute:
			for _, h := range r.Spec.Hostnames {
				hosts = addHost(hosts, string(h))
			}
		}
	}
	return hosts, nil
}

func (s *gatewaySource) getTargets(obj resources.ObjectData) utils.StringSet {
	var (
		hostnames   []string
		ipAddresses []string
	)
	switch data := obj.(type) {
	case *gatewayapisv1.Gateway:
		for _, address := range data.Status.Addresses {
			t := address.Type
			switch {
			case t != nil && *t == gatewayapisv1.HostnameAddressType:
				hostnames = append(hostnames, address.Value)
			case t == nil || *t == gatewayapisv1.IPAddressType:
				ipAddresses = append(ipAddresses, address.Value)
			}
		}
	case *gatewayapisv1beta1.Gateway:
		for _, address := range data.Status.Addresses {
			t := address.Type
			switch {
			case t != nil && *t == gatewayapisv1beta1.HostnameAddressType:
				hostnames = append(hostnames, address.Value)
			case t == nil || *t == gatewayapisv1beta1.IPAddressType:
				ipAddresses = append(ipAddresses, address.Value)
			}
		}
	case *gatewayapisv1alpha2.Gateway:
		for _, address := range data.Status.Addresses {
			t := address.Type
			switch {
			case t != nil && *t == gatewayapisv1alpha2.HostnameAddressType:
				hostnames = append(hostnames, address.Value)
			case t == nil || *t == gatewayapisv1alpha2.IPAddressType:
				ipAddresses = append(ipAddresses, address.Value)
			}
		}
	}
	switch {
	case len(hostnames) == 1:
		return utils.NewStringSet(hostnames...)
	case len(ipAddresses) > 0:
		return utils.NewStringSet(ipAddresses...)
	case len(hostnames) > 1:
		return utils.NewStringSet(hostnames...)
	default:
		return nil
	}
}

var _ httpRouteLister = &httprouteLister{}

type httprouteLister struct {
	httprouteResources resources.Interface
}

func newServiceLister(c controller.Interface) (*httprouteLister, error) {
	httprouteResources, err := c.GetMainCluster().Resources().GetByGK(resources.NewGroupKind(Group, "HTTPRoute"))
	if err != nil {
		return nil, err
	}
	return &httprouteLister{httprouteResources: httprouteResources}, nil
}

func (l *httprouteLister) ListHTTPRoutes(gateway *resources.ObjectName) ([]resources.ObjectData, error) {
	objs, err := l.httprouteResources.List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var array []resources.ObjectData
	for _, obj := range objs {
		gateways := extractGatewayNames(obj.Data())
		for g := range gateways {
			if gateway == nil || g == *gateway {
				array = append(array, obj.Data())
			}
		}
	}
	return array, nil
}
