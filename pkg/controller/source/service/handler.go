// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"
	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/source"
	api "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

func GetTargets(_ logger.LogContext, obj resources.ObjectData, names dns.DNSNameSet) (*source.TargetExtraction, error) {
	svc := obj.(*api.Service)
	if svc.Spec.Type != api.ServiceTypeLoadBalancer {
		if len(names) == 0 {
			return nil, nil
		}
		return nil, fmt.Errorf("service is not of type LoadBalancer")
	}
	if len(names) == 1 {
		for name := range names {
			if name.DNSName == "*" {
				return nil, nil
			}
		}
	}
	ignore := false
	var resolveTargetsToAddresses *bool
	ipstack := ""
	set := utils.StringSet{}
	for _, i := range svc.Status.LoadBalancer.Ingress {
		if i.Hostname != "" && i.IP == "" {
			if svc.Annotations["loadbalancer.openstack.org/load-balancer-address"] != "" {
				// Support for PROXY protocol on Openstack (which needs a hostname as ingress)
				// If the user sets the annotation `loadbalancer.openstack.org/hostname`, the
				// annotation `loadbalancer.openstack.org/load-balancer-address` contains the IP address.
				// This address can then be used to create a DNS record for the hostname specified both
				// in annotation `loadbalancer.openstack.org/hostname` and `dns.gardener.cloud/dnsnames`
				// see https://github.com/kubernetes/cloud-provider-openstack/blob/master/docs/openstack-cloud-controller-manager/expose-applications-using-loadbalancer-type-service.md#service-annotations
				set.Add(svc.Annotations["loadbalancer.openstack.org/load-balancer-address"])
			} else {
				set.Add(i.Hostname)
			}
		} else {
			if i.IP != "" {
				set.Add(i.IP)
			}
		}
		if svc.Annotations[dns.AnnotationIPStack] != "" {
			ipstack = svc.Annotations[dns.AnnotationIPStack]
		}
		if svc.Annotations["service.beta.kubernetes.io/aws-load-balancer-ip-address-type"] == "dualstack" {
			ipstack = dns.AnnotationValueIPStackIPDualStack
		}
		if v := svc.Annotations[source.RESOLVE_TARGETS_TO_ADDRS_ANNOTATION]; v != "" {
			resolveTargetsToAddresses = ptr.To(v == "true")
		}
		if v := svc.Annotations[dns.AnnotationIgnore]; v != "" {
			ignore = v == "true"
		}
	}
	return &source.TargetExtraction{
		Targets:                   set,
		IPStack:                   ipstack,
		ResolveTargetsToAddresses: resolveTargetsToAddresses,
		Ignore:                    ignore,
	}, nil
}
