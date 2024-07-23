// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package istio

import (
	"fmt"
	"strings"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"
	istionetworkingv1 "istio.io/client-go/pkg/apis/networking/v1"
	istionetworkingv1alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	istionetworkingv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/gardener/external-dns-management/pkg/controller/source/ingress"
	"github.com/gardener/external-dns-management/pkg/controller/source/service"
	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/source"
)

const (
	// TargetsAnnotation is the annotation used to specify the target IPs or names explicitly
	TargetsAnnotation = dns.ANNOTATION_GROUP + "/targets"
	// IngressTargetSourceAnnotation is the annotation used to determine if the gateway is implemented by an Ingress object
	// instead of a standard LoadBalancer service type
	IngressTargetSourceAnnotation = dns.ANNOTATION_GROUP + "/ingress"
)

type resourceLister interface {
	ListServices(selectors map[string]string) ([]resources.ObjectData, error)
	GetIngress(name resources.ObjectName) (resources.ObjectData, error)
	ListVirtualServices(gateway *resources.ObjectName) ([]resources.ObjectData, error)
}

type gatewaySource struct {
	source.DefaultDNSSource
	lister resourceLister
	state  *resourcesState
}

func NewGatewaySource(c controller.Interface) (source.DNSSource, error) {
	lister, err := newResourceLister(c)
	if err != nil {
		return nil, err
	}
	state, err := getOrCreateSharedState(c)
	if err != nil {
		return nil, err
	}
	return newGatewaySourceWithResourceLister(lister, state)
}

func newGatewaySourceWithResourceLister(lister resourceLister, state *resourcesState) (source.DNSSource, error) {
	return &gatewaySource{lister: lister, state: state, DefaultDNSSource: source.NewDefaultDNSSource(nil)}, nil
}

func (s *gatewaySource) Setup() error {
	virtualServices, err := s.lister.ListVirtualServices(nil)
	if err != nil {
		return err
	}
	for _, virtualService := range virtualServices {
		gateways := extractGatewayNames(virtualService)
		s.state.AddVirtualService(resources.NewObjectNameForData(virtualService), gateways)
	}
	return nil
}

func (s *gatewaySource) Deleted(logger logger.LogContext, key resources.ClusterObjectKey) {
	s.DefaultDNSSource.Deleted(logger, key)
}

func (s *gatewaySource) GetDNSInfo(logger logger.LogContext, obj resources.ObjectData, current *source.DNSCurrentState) (*source.DNSInfo, error) {
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
		return info, fmt.Errorf("annotated dns names %s not declared by gateway", del)
	}
	info.Names = dns.NewDNSNameSetFromStringSet(names, current.GetSetIdentifier())
	info.Targets = s.getTargets(logger, info.Names, obj)
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
	case *istionetworkingv1.Gateway:
		for _, server := range data.Spec.Servers {
			hosts = append(hosts, parsedHosts(server.Hosts)...)
		}
	case *istionetworkingv1beta1.Gateway:
		for _, server := range data.Spec.Servers {
			hosts = append(hosts, parsedHosts(server.Hosts)...)
		}
	case *istionetworkingv1alpha3.Gateway:
		for _, server := range data.Spec.Servers {
			hosts = append(hosts, parsedHosts(server.Hosts)...)
		}
	default:
		return nil, fmt.Errorf("unexpected istio gateway type: %#v", obj)
	}

	virtualServices, err := s.lister.ListVirtualServices(ptr.To(resources.NewObjectNameForData(obj)))
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

	for _, vsvc := range virtualServices {
		switch r := vsvc.(type) {
		case *istionetworkingv1.VirtualService:
			for _, h := range r.Spec.Hosts {
				hosts = addHost(hosts, h)
			}
		case *istionetworkingv1beta1.VirtualService:
			for _, h := range r.Spec.Hosts {
				hosts = addHost(hosts, h)
			}
		case *istionetworkingv1alpha3.VirtualService:
			for _, h := range r.Spec.Hosts {
				hosts = addHost(hosts, h)
			}
		}
	}
	return hosts, nil
}

func (s *gatewaySource) getTargets(logger logger.LogContext, names dns.DNSNameSet, obj resources.ObjectData) utils.StringSet {
	if len(names) == 0 {
		return nil
	}

	if targets := obj.GetAnnotations()[TargetsAnnotation]; targets != "" {
		targetSet := utils.NewStringSet()
		for _, target := range strings.Split(targets, ",") {
			targetSet.Add(target)
		}
		return targetSet
	}

	if ingressName := obj.GetAnnotations()[IngressTargetSourceAnnotation]; ingressName != "" {
		return s.getTargetsFromIngress(logger, ingressName, obj)
	}

	return s.getTargetsFromService(logger, names, obj)
}

func (s *gatewaySource) getTargetsFromIngress(logger logger.LogContext, ingressName string, obj resources.ObjectData) utils.StringSet {
	parts := strings.Split(ingressName, "/")
	var namespace, name string
	switch len(parts) {
	case 1:
		namespace = obj.GetNamespace()
		name = parts[0]
	case 2:
		namespace = parts[0]
		name = parts[1]
	default:
		logger.Warnf("invalid annotation %s: %s", IngressTargetSourceAnnotation, ingressName)
		return nil
	}
	key := resources.NewKey(ingress.MainResource, namespace, name)
	s.state.AddTargetSource(key, []resources.ObjectName{resources.NewObjectName(obj.GetNamespace(), obj.GetName())})
	ingressObj, err := s.lister.GetIngress(key.ObjectName())
	if err != nil {
		logger.Warnf("cannot retrieve source ingress %s: %s", key.ObjectName(), err)
		return nil
	}
	return ingress.GetTargets(ingressObj)
}

func (s *gatewaySource) getTargetsFromService(logger logger.LogContext, names dns.DNSNameSet, obj resources.ObjectData) utils.StringSet {
	selectors := s.getSelectors(obj)
	if len(selectors) == 0 {
		return nil
	}

	serviceObjects, err := s.lister.ListServices(selectors)
	if err != nil {
		return nil
	}

	for _, svc := range serviceObjects {
		key := resources.NewKey(service.MainResource, svc.GetNamespace(), svc.GetName())
		s.state.AddTargetSource(key, []resources.ObjectName{resources.NewObjectName(obj.GetNamespace(), obj.GetName())})
	}

	set := utils.StringSet{}
	for _, svc := range serviceObjects {
		extraction, err := service.GetTargets(logger, svc, names)
		if err != nil {
			logger.Warnf("no targets for gateway %s/%s: %s", obj.GetNamespace(), obj.GetName(), err)
		}
		if extraction != nil {
			set.AddSet(extraction.Targets)
		}
	}
	return set
}

func (s *gatewaySource) getSelectors(obj resources.ObjectData) map[string]string {
	switch data := obj.(type) {
	case *istionetworkingv1.Gateway:
		return data.Spec.Selector
	case *istionetworkingv1beta1.Gateway:
		return data.Spec.Selector
	case *istionetworkingv1alpha3.Gateway:
		return data.Spec.Selector
	default:
		return nil
	}
}

type stdResourceLister struct {
	servicesResources        resources.Interface
	ingressResources         resources.Interface
	virtualServicesResources resources.Interface
}

var _ resourceLister = &stdResourceLister{}

func newResourceLister(c controller.Interface) (*stdResourceLister, error) {
	svcResources, err := c.GetMainCluster().Resources().GetByGK(service.MainResource)
	if err != nil {
		return nil, err
	}
	ingressResources, err := c.GetMainCluster().Resources().GetByGK(ingress.MainResource)
	if err != nil {
		return nil, err
	}
	virtualServicesResources, err := c.GetMainCluster().Resources().GetByGK(GroupKindVirtualService)
	if err != nil {
		return nil, err
	}
	return &stdResourceLister{
		servicesResources:        svcResources,
		ingressResources:         ingressResources,
		virtualServicesResources: virtualServicesResources,
	}, nil
}

func (s *stdResourceLister) ListServices(selectors map[string]string) ([]resources.ObjectData, error) {
	ls, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchLabels: selectors})
	if err != nil {
		return nil, err
	}
	objs, err := s.servicesResources.ListCached(ls)
	if err != nil {
		return nil, err
	}
	var array []resources.ObjectData
	for _, obj := range objs {
		array = append(array, obj.Data())
	}
	return array, nil
}

func (s *stdResourceLister) GetIngress(name resources.ObjectName) (resources.ObjectData, error) {
	obj, err := s.ingressResources.Get(name)
	if err != nil {
		return nil, err
	}
	return obj.Data(), nil
}

func (s *stdResourceLister) ListVirtualServices(gateway *resources.ObjectName) ([]resources.ObjectData, error) {
	objs, err := s.virtualServicesResources.List(metav1.ListOptions{})
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

func parsedHosts(serverHosts []string) []string {
	var hosts []string
	for _, serverHost := range serverHosts {
		if serverHost == "*" {
			continue
		}
		parts := strings.Split(serverHost, "/")
		if len(parts) == 2 {
			// first part is namespace
			hosts = append(hosts, parts[1])
		} else if len(parts) == 1 {
			hosts = append(hosts, parts[0])
		}
	}
	return hosts
}
