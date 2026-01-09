/*
 * SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package common

import (
	"fmt"
	"maps"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/utils/ptr"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/state"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

// DNSSpecInput specifies names, targets, and policies for DNS records.
type DNSSpecInput struct {
	Names                     *utils.UniqueStrings
	TTL                       *int64
	CNameLookupInterval       *int64
	Targets                   *utils.UniqueStrings
	Text                      *utils.UniqueStrings
	RoutingPolicy             *v1alpha1.RoutingPolicy
	IPStack                   string
	ResolveTargetsToAddresses *bool
	Ignore                    string
}

// GetDNSSpecInputForService gets the DNS spec input for a service of type loadbalancer.
func GetDNSSpecInputForService(log logr.Logger, state state.AnnotationState, gvk schema.GroupVersionKind, svc *corev1.Service) (*DNSSpecInput, error) {
	annotations := GetMergedAnnotation(gvk, state, svc)

	names, err := getDNSNamesFromAnnotations(log, annotations)
	if err != nil {
		return nil, err
	}
	if names == nil {
		return nil, nil // no DNS names specified means no need to create DNS entries
	}
	if names.Contains("*") {
		return nil, fmt.Errorf("domain name annotation value '*' is not allowed for service objects")
	}

	targets := utils.NewUniqueStrings()
	for _, i := range svc.Status.LoadBalancer.Ingress {
		if i.Hostname != "" && i.IP == "" {
			if annotations[dns.AnnotationOpenStackLoadBalancerAddress] != "" {
				// Support for PROXY protocol on Openstack (which needs a hostname as ingress)
				// If the user sets the annotation `loadbalancer.openstack.org/hostname`, the
				// annotation `loadbalancer.openstack.org/load-balancer-address` contains the IP address.
				// This address can then be used to create a DNS record for the hostname specified both
				// in annotation `loadbalancer.openstack.org/hostname` and `dns.gardener.cloud/dnsnames`
				// see https://github.com/kubernetes/cloud-provider-openstack/blob/master/docs/openstack-cloud-controller-manager/expose-applications-using-loadbalancer-type-service.md#service-annotations
				targets.Add(annotations[dns.AnnotationOpenStackLoadBalancerAddress])
			} else {
				targets.Add(i.Hostname)
			}
		} else {
			if i.IP != "" {
				targets.Add(i.IP)
			}
		}
	}

	ipStack := annotations[dns.AnnotationIPStack]
	if annotations[dns.AnnotationAwsLoadBalancerIpAddressType] == dns.AnnotationAwsLoadBalancerIpAddressTypeValueDualStack {
		ipStack = dns.AnnotationValueIPStackIPDualStack
	}

	return augmentFromCommonAnnotations(annotations, DNSSpecInput{
		Names:   names,
		Targets: targets,
		IPStack: ipStack,
	})
}

// GetDNSSpecInputForIngress gets the DNS spec input for an Ingress resource.
func GetDNSSpecInputForIngress(log logr.Logger, state state.AnnotationState, gvk schema.GroupVersionKind, ingress *networkingv1.Ingress) (*DNSSpecInput, error) {
	annotations := GetMergedAnnotation(gvk, state, ingress)

	names, err := getDNSNamesForIngress(log, ingress, annotations)
	if err != nil {
		return nil, err
	}
	if names == nil {
		return nil, nil
	}

	return augmentFromCommonAnnotations(annotations, DNSSpecInput{
		Names:   names,
		Targets: getTargetsForIngress(ingress),
		IPStack: annotations[dns.AnnotationIPStack],
	})
}

func getDNSNamesForIngress(log logr.Logger, ingress *networkingv1.Ingress, annotations map[string]string) (*utils.UniqueStrings, error) {
	annotatedNames, err := getDNSNamesFromAnnotations(log, annotations)
	if err != nil {
		return nil, err
	}
	if annotatedNames == nil {
		return nil, nil
	}

	all := annotatedNames.Contains("*")
	dnsNames := utils.NewUniqueStrings()
	for _, rule := range ingress.Spec.Rules {
		host := rule.Host
		if host != "" && (all || annotatedNames.Contains(host)) {
			dnsNames.Add(host)
		}
	}

	annotatedNames.Remove("*")
	diff := annotatedNames.Difference(dnsNames)
	if len(diff) > 0 {
		return nil, fmt.Errorf("annotated dns names %s not declared by ingress", strings.Join(diff, ", "))
	}
	return dnsNames, nil
}

func getTargetsForIngress(ingress *networkingv1.Ingress) *utils.UniqueStrings {
	ips := utils.NewUniqueStrings()
	hosts := utils.NewUniqueStrings()
	for _, ing := range ingress.Status.LoadBalancer.Ingress {
		if ing.IP != "" {
			ips.Add(ing.IP)
		}
		if ing.Hostname != "" {
			hosts.Add(ing.Hostname)
		}
	}
	if ips.Len() > 0 {
		return ips
	}
	return hosts
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

	if v := annotations[dns.AnnotationResolveTargetsToAddresses]; v != "" {
		input.ResolveTargetsToAddresses = ptr.To(v == "true")
	}

	if a := annotations[dns.AnnotationCNameLookupInterval]; a != "" {
		interval, err := strconv.ParseInt(a, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid CNameLookupInterval: %w", err)
		}
		if interval != 0 {
			input.CNameLookupInterval = &interval
		}
	}

	return &input, nil
}

func modifyEntryFor(entry *v1alpha1.DNSEntry, cfg config.SourceControllerConfig, src *DNSSpecInput, name string) {
	if cfg.TargetClass != nil && !dns.IsDefaultClass(*cfg.TargetClass) {
		utils.SetAnnotation(entry, dns.AnnotationClass, *cfg.TargetClass)
	}
	if cfg.TargetLabels != nil {
		for k, v := range cfg.TargetLabels {
			utils.SetLabel(entry, k, v)
		}
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
	entry.Spec.CNameLookupInterval = src.CNameLookupInterval
	switch src.Ignore {
	case dns.AnnotationIgnoreValueTrue, dns.AnnotationIgnoreValueReconcile:
		utils.SetAnnotation(entry, dns.AnnotationIgnore, dns.AnnotationIgnoreValueReconcile)
	case dns.AnnotationIgnoreValueFull:
		utils.SetAnnotation(entry, dns.AnnotationIgnore, dns.AnnotationIgnoreValueFull)
	default:
		utils.RemoveAnnotation(entry, dns.AnnotationIgnore)
	}
}

func getDNSNamesFromAnnotations(log logr.Logger, annotations map[string]string) (*utils.UniqueStrings, error) {
	dnsNames, ok := annotations[dns.AnnotationDNSNames]
	if !ok {
		log.V(5).Info("No DNS names annotation", "key", dns.AnnotationDNSNames)
		return nil, nil
	}
	if dnsNames == "" {
		return nil, fmt.Errorf("empty value for annotation %q", dns.AnnotationDNSNames)
	}

	names := utils.NewUniqueStrings()
	for name := range strings.SplitSeq(dnsNames, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		names.Add(name)
	}
	return names, nil
}

// GetMergedAnnotation gets the merged annotations for the given object.
func GetMergedAnnotation(gvk schema.GroupVersionKind, state state.AnnotationState, obj metav1.Object) map[string]string {
	annotations := map[string]string{}
	externalAnnotations, _, _ := state.GetResourceAnnotationStatus(BuildResourceReference(gvk, obj))
	maps.Copy(annotations, externalAnnotations)
	maps.Copy(annotations, obj.GetAnnotations())
	return annotations
}

// BuildResourceReference builds a ResourceReference for the given object.
func BuildResourceReference(gvk schema.GroupVersionKind, obj metav1.Object) v1alpha1.ResourceReference {
	return v1alpha1.ResourceReference{
		APIVersion: gvk.GroupVersion().String(),
		Kind:       gvk.Kind,
		Name:       obj.GetName(),
		Namespace:  obj.GetNamespace(),
	}
}
