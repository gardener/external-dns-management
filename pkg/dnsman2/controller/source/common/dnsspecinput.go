/*
 * SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package common

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/utils/ptr"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

// DNSSpecInput specifies names, targets, and policies for DNS records.
type DNSSpecInput struct {
	Names                     *utils.UniqueStrings
	TTL                       *int64
	Interval                  *int64
	Targets                   *utils.UniqueStrings
	Text                      *utils.UniqueStrings
	RoutingPolicy             *v1alpha1.RoutingPolicy
	IPStack                   string
	ResolveTargetsToAddresses *bool
	Ignore                    string
}

// GetDNSSpecInputForService gets the DNS spec input for a service of type loadbalancer.
func GetDNSSpecInputForService(log logr.Logger, svc *corev1.Service) (*DNSSpecInput, error) {
	dnsNames, ok := svc.Annotations[dns.AnnotationDNSNames]
	if !ok {
		log.V(5).Info("No DNS names annotation", "key", dns.AnnotationDNSNames)
		return nil, nil
	}
	if dnsNames == "" {
		return nil, fmt.Errorf("empty value for annotation %q", dns.AnnotationDNSNames)
	}

	names := utils.NewUniqueStrings()
	for _, name := range strings.Split(dnsNames, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if name == "*" {
			return nil, fmt.Errorf("domain name annotation value '*' is not allowed for service objects")
		}
		names.Add(name)
	}

	var resolveTargetsToAddresses *bool
	ipstack := ""
	targets := utils.NewUniqueStrings()
	for _, i := range svc.Status.LoadBalancer.Ingress {
		if i.Hostname != "" && i.IP == "" {
			if svc.Annotations[dns.AnnotationOpenStackLoadBalancerAddress] != "" {
				// Support for PROXY protocol on Openstack (which needs a hostname as ingress)
				// If the user sets the annotation `loadbalancer.openstack.org/hostname`, the
				// annotation `loadbalancer.openstack.org/load-balancer-address` contains the IP address.
				// This address can then be used to create a DNS record for the hostname specified both
				// in annotation `loadbalancer.openstack.org/hostname` and `dns.gardener.cloud/dnsnames`
				// see https://github.com/kubernetes/cloud-provider-openstack/blob/master/docs/openstack-cloud-controller-manager/expose-applications-using-loadbalancer-type-service.md#service-annotations
				targets.Add(svc.Annotations[dns.AnnotationOpenStackLoadBalancerAddress])
			} else {
				targets.Add(i.Hostname)
			}
		} else {
			if i.IP != "" {
				targets.Add(i.IP)
			}
		}
	}
	if svc.Annotations[dns.AnnotationIPStack] != "" {
		ipstack = svc.Annotations[dns.AnnotationIPStack]
	}
	if svc.Annotations[dns.AnnotationAwsLoadBalancerIpAddressType] == dns.AnnotationAwsLoadBalancerIpAddressTypeValueDualStack {
		ipstack = dns.AnnotationValueIPStackIPDualStack
	}
	if v := svc.Annotations[dns.AnnotatationResolveTargetsToAddresses]; v != "" {
		resolveTargetsToAddresses = ptr.To(v == "true")
	}

	return augmentFromCommonAnnotations(svc.Annotations, DNSSpecInput{
		Names:                     names,
		Targets:                   targets,
		IPStack:                   ipstack,
		ResolveTargetsToAddresses: resolveTargetsToAddresses,
	})
}

func augmentFromCommonAnnotations(annotations map[string]string, input DNSSpecInput) (*DNSSpecInput, error) {
	if len(annotations) == 0 {
		return &input, nil
	}

	if a := annotations[dns.AnnotationRoutingPolicy]; a != "" {
		policy := &v1alpha1.RoutingPolicy{}
		if err := json.Unmarshal([]byte(a), policy); err != nil {
			return nil, err
		}
		input.RoutingPolicy = policy
	}

	if a := annotations[dns.AnnotationTTL]; a != "" {
		ttl, err := strconv.ParseInt(a, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid TTL: %s", err)
		}
		if ttl != 0 {
			input.TTL = &ttl
		}
	}

	if a := annotations[dns.AnnotationIgnore]; a != "" {
		input.Ignore = a
	}

	return &input, nil
}

func modifyEntryFor(entry *v1alpha1.DNSEntry, cfg config.SourceControllerConfig, src *DNSSpecInput, name string) {
	if cfg.TargetClass != nil && !dns.IsDefaultClass(*cfg.TargetClass) {
		utils.SetAnnotation(entry, dns.AnnotationClass, *cfg.TargetClass)
	}
	entry.Spec.DNSName = name
	entry.Spec.Targets = src.Targets.ToSlice()
	entry.Spec.Text = src.Text.ToSlice()

	entry.Spec.TTL = src.TTL
	entry.Spec.RoutingPolicy = src.RoutingPolicy
	if src.IPStack != "" {
		utils.SetAnnotation(entry, dns.AnnotationIPStack, src.IPStack)
	}
	entry.Spec.ResolveTargetsToAddresses = src.ResolveTargetsToAddresses
	switch src.Ignore {
	case dns.AnnotationIgnoreValueTrue, dns.AnnotationIgnoreValueReconcile:
		utils.SetAnnotation(entry, dns.AnnotationIgnore, dns.AnnotationIgnoreValueReconcile)
	case dns.AnnotationIgnoreValueFull:
		utils.SetAnnotation(entry, dns.AnnotationIgnore, dns.AnnotationIgnoreValueFull)
	default:
		utils.RemoveAnnotation(entry, dns.AnnotationIgnore)
	}
}
