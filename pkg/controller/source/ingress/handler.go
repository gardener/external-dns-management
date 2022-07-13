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

package ingress

import (
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"
	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/source"
	networkingv1 "k8s.io/api/networking/v1"
	networkingv1beta1 "k8s.io/api/networking/v1beta1"
)

type IngressSource struct {
	source.DefaultDNSSource
}

func NewIngressSource(controller.Interface) (source.DNSSource, error) {
	return &IngressSource{DefaultDNSSource: source.NewDefaultDNSSource(nil)}, nil
}

func (this *IngressSource) GetDNSInfo(logger logger.LogContext, obj resources.Object, current *source.DNSCurrentState) (*source.DNSInfo, error) {
	info := &source.DNSInfo{Targets: this.GetTargets(obj)}
	hosts, err := this.extractRuleHosts(obj)
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
		return info, fmt.Errorf("annotated dns names %s not declared by ingress", del)
	}
	info.Names = dns.NewRecordSetNameSetFromStringSet(names, current.SetIdentifier())
	return info, nil
}

func (this *IngressSource) extractRuleHosts(obj resources.Object) ([]string, error) {
	hosts := []string{}
	switch data := obj.Data().(type) {
	case *networkingv1beta1.Ingress:
		for _, i := range data.Spec.Rules {
			hosts = append(hosts, i.Host)
		}
		return hosts, nil
	case *networkingv1.Ingress:
		for _, i := range data.Spec.Rules {
			hosts = append(hosts, i.Host)
		}
		return hosts, nil
	default:
		return nil, fmt.Errorf("unexpected ingress type: %#v", obj.Data())
	}
}

func (this *IngressSource) GetTargets(obj resources.Object) utils.StringSet {
	set := utils.StringSet{}
	switch data := obj.Data().(type) {
	case *networkingv1beta1.Ingress:
		for _, i := range data.Status.LoadBalancer.Ingress {
			if i.Hostname != "" && i.IP == "" {
				set.Add(i.Hostname)
			} else {
				if i.IP != "" {
					set.Add(i.IP)
				}
			}
		}
	case *networkingv1.Ingress:
		for _, i := range data.Status.LoadBalancer.Ingress {
			if i.Hostname != "" && i.IP == "" {
				set.Add(i.Hostname)
			} else {
				if i.IP != "" {
					set.Add(i.IP)
				}
			}
		}
	default:
	}
	return set
}
