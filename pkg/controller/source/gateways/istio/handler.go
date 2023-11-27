/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 *
 */

package istio

import (
	"fmt"
	"strings"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"
	"github.com/gardener/external-dns-management/pkg/controller/source/service"
	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/source"
	istionetworkingv1alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	istionetworkingv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ServiceLister interface {
	GetServices(selectors map[string]string) ([]resources.ObjectData, error)
}

type GatewaySource struct {
	source.DefaultDNSSource
	serviceLister ServiceLister
}

func NewGatewaySource(c controller.Interface) (source.DNSSource, error) {
	serviceLister, err := newServiceLister(c)
	if err != nil {
		return nil, err
	}
	return NewGatewaySourceWithServiceLister(serviceLister)
}

func NewGatewaySourceWithServiceLister(serviceLister ServiceLister) (*GatewaySource, error) {
	return &GatewaySource{serviceLister: serviceLister, DefaultDNSSource: source.NewDefaultDNSSource(nil)}, nil
}

func (s *GatewaySource) GetDNSInfo(logger logger.LogContext, obj resources.Object, current *source.DNSCurrentState) (*source.DNSInfo, error) {
	return s.GetDNSInfoData(logger, obj.Data(), current)
}

func (s *GatewaySource) GetDNSInfoData(logger logger.LogContext, obj resources.ObjectData, current *source.DNSCurrentState) (*source.DNSInfo, error) {
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
	info.Targets = s.GetTargets(logger, info.Names, obj)
	return info, nil
}

func (s *GatewaySource) extractServerHosts(obj resources.ObjectData) ([]string, error) {
	var hosts []string
	switch data := obj.(type) {
	case *istionetworkingv1beta1.Gateway:
		for _, server := range data.Spec.Servers {
			hosts = append(hosts, parsedHosts(server.Hosts)...)
		}
		return hosts, nil
	case *istionetworkingv1alpha3.Gateway:
		for _, server := range data.Spec.Servers {
			hosts = append(hosts, parsedHosts(server.Hosts)...)
		}
		return hosts, nil
	default:
		return nil, fmt.Errorf("unexpected istio gateway type: %#v", obj)
	}
}

func (s *GatewaySource) GetTargets(logger logger.LogContext, names dns.DNSNameSet, obj resources.ObjectData) utils.StringSet {
	if len(names) == 0 {
		return nil
	}
	selectors := s.getSelectors(obj)
	if len(selectors) == 0 {
		return nil
	}
	serviceObjects, err := s.serviceLister.GetServices(selectors)
	if err != nil {
		return nil
	}

	set := utils.StringSet{}
	for _, svc := range serviceObjects {
		subset, _, err := service.GetTargets(logger, svc, names)
		if err != nil {
			logger.Warnf("no targets for gateway %s/%s: %s", obj.GetNamespace(), obj.GetName(), err)
		}
		set.AddSet(subset)
	}
	return set
}

func (s *GatewaySource) getSelectors(obj resources.ObjectData) map[string]string {
	switch data := obj.(type) {
	case *istionetworkingv1beta1.Gateway:
		return data.Spec.Selector
	case *istionetworkingv1alpha3.Gateway:
		return data.Spec.Selector
	default:
		return nil
	}
}

type serviceLister struct {
	servicesResources resources.Interface
}

var _ ServiceLister = &serviceLister{}

func newServiceLister(c controller.Interface) (*serviceLister, error) {
	svcResources, err := c.GetMainCluster().Resources().GetByGK(resources.NewGroupKind("", "Service"))
	if err != nil {
		return nil, err
	}
	return &serviceLister{servicesResources: svcResources}, nil
}

func (s *serviceLister) GetServices(selectors map[string]string) ([]resources.ObjectData, error) {
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
