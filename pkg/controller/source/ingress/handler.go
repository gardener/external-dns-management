// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package ingress

import (
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"
	networkingv1 "k8s.io/api/networking/v1"
	networkingv1beta1 "k8s.io/api/networking/v1beta1"
	"k8s.io/utils/ptr"

	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/source"
)

type IngressSource struct {
	source.DefaultDNSSource
}

func NewIngressSource(controller.Interface) (source.DNSSource, error) {
	return &IngressSource{DefaultDNSSource: source.NewDefaultDNSSource(nil)}, nil
}

func (this *IngressSource) GetDNSInfo(_ logger.LogContext, obj resources.ObjectData, current *source.DNSCurrentState) (*source.DNSInfo, error) {
	info := &source.DNSInfo{Targets: GetTargets(obj)}
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
	info.Names = dns.NewDNSNameSetFromStringSet(names, current.GetSetIdentifier())
	info.IPStack = obj.GetAnnotations()[dns.AnnotationIPStack]
	if v := obj.GetAnnotations()[source.RESOLVE_TARGETS_TO_ADDRS_ANNOTATION]; v != "" {
		info.ResolveTargetsToAddresses = ptr.To(v == "true")
	}
	if v := obj.GetAnnotations()[dns.AnnotationIgnore]; v != "" {
		info.Ignore = v == "true"
	}
	return info, nil
}

func (this *IngressSource) extractRuleHosts(obj resources.ObjectData) ([]string, error) {
	hosts := []string{}
	switch data := obj.(type) {
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
		return nil, fmt.Errorf("unexpected ingress type: %#v", obj)
	}
}

func GetTargets(obj resources.ObjectData) utils.StringSet {
	set := utils.StringSet{}
	switch data := obj.(type) {
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
