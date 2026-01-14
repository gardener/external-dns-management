// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dns

import (
	"fmt"
)

const (
	// ControllerGroupDNSControllers is the group name for DNS controller resources.
	ControllerGroupDNSControllers = "dnscontrollers"
	// ControllerGroupDNSSources is the group name for DNS source resources.
	ControllerGroupDNSSources = "dnssources"
	// ControllerGroupReplication is the group name for replication resources.
	ControllerGroupReplication = "replication"

	// DefaultClass is the default DNS class used by the controller.
	DefaultClass = "gardendns"
	// AnnotationClass is the annotation key for specifying the DNS class.
	AnnotationClass = "dns.gardener.cloud/class"
	// AnnotationTTL is the annotation key for specifying the TTL (Time To Live) for DNS records.
	AnnotationTTL = "dns.gardener.cloud/ttl"

	// AnnotationNotRateLimited is the annotation key to disable rate limiting.
	AnnotationNotRateLimited = "dns.gardener.cloud/not-rate-limited"
	// AnnotationDNSNames is the annotation key for specifying DNS names.
	AnnotationDNSNames = "dns.gardener.cloud/dnsnames"

	// FinalizerCompound is the finalizer for provider resources ("compound" to be backwards-compatible).
	FinalizerCompound = "dns.gardener.cloud/compound"
	// FinalizerReplication is the finalizer for provider resources (exact name needed for backward compatibility).
	FinalizerReplication = "garden.dns.gardener.cloud/dnsprovider-replication"
	// FinalizerSourceTemplate is the finalizer template to be filled with class and controller (exact name needed for backward compatibility).
	FinalizerSourceTemplate = "%s.dns.gardener.cloud/%s"

	// AnnotationIPStack is an optional annotation for DNSEntries to specify the IP stack.
	// Values are 'ipv4', 'dual-stack', and 'ipv6'. If not specified, 'ipv4' is assumed.
	// This annotation is currently only relevant for AWS-Route53 to generate alias target A and/or AAAA records.
	AnnotationIPStack = "dns.gardener.cloud/ip-stack"
	// AnnotationValueIPStackIPv4 is the annotation value for specifying IPv4-only IP stack.
	AnnotationValueIPStackIPv4 = "ipv4"
	// AnnotationValueIPStackIPDualStack is the annotation value for specifying dual-stack (IPv4 and IPv6) IP stack.
	AnnotationValueIPStackIPDualStack = "dual-stack"
	// AnnotationValueIPStackIPv6 is the annotation value for specifying IPv6-only IP stack.
	AnnotationValueIPStackIPv6 = "ipv6"

	// AnnotationIgnore is an optional annotation for DNSEntries and source resources to ignore them on reconciliation.
	AnnotationIgnore = "dns.gardener.cloud/ignore"
	// AnnotationIgnoreValueTrue is the value for the annotation to ignore the entry on reconciliation. Same as "reconcile".
	AnnotationIgnoreValueTrue = "true"
	// AnnotationIgnoreValueReconcile is the value for the annotation to ignore the entry on reconciliation. Same as "true".
	AnnotationIgnoreValueReconcile = "reconcile"
	// AnnotationIgnoreValueFull is the value for the annotation to ignore the entry on reconciliation and deletion.
	// IMPORTANT NOTE: The entry is even ignored on deletion. Use with caution to avoid orphaned entries!
	AnnotationIgnoreValueFull = "full"
	// AnnotationHardIgnore is an optional annotation for a generated target DNSEntry to ignore it on reconciliation.
	// This annotation is not propagated from source objects to the target DNSEntry.
	// IMPORTANT NOTE: The entry is even ignored on deletion, so use with caution to avoid orphaned entries.
	AnnotationHardIgnore = "dns.gardener.cloud/target-hard-ignore"

	// AnnotationRoutingPolicy is the annotation key for specifying the routing policy.
	AnnotationRoutingPolicy = "dns.gardener.cloud/routing-policy"
	// AnnotationResolveTargetsToAddresses is the annotation key for source objects to set the `.spec.resolveTargetsToAddresses` in the DNSEntry.
	AnnotationResolveTargetsToAddresses = "dns.gardener.cloud/resolve-targets-to-addresses"
	// AnnotationCNameLookupInterval is an optional annotation for source objects to set the `.spec.cnameLookupInterval` in the DNSEntry.
	AnnotationCNameLookupInterval = "dns.gardener.cloud/cname-lookup-interval"

	// AnnotationTargetEntry is an optional annotation for source DNSEntries to indicate the corresponding target DNSEntry.
	AnnotationTargetEntry = "dns.gardener.cloud/target-entry"

	// AnnotationValidationError is an optional annotation for replicated provider secrets to indicate a validation error.
	AnnotationValidationError = "dns.gardener.cloud/validation-error"

	// AnnotationServiceBetaGroup is the group for beta Service annotations.
	AnnotationServiceBetaGroup = "service.beta.kubernetes.io"
	// AnnotationAwsLoadBalancerIpAddressType is an optional annotation for AWS LoadBalancer Services to specify the IP address type.
	// Values are 'ipv4' and 'dual-stack'. If not specified, 'ipv4' is assumed.
	// Behaves similar to dns.gardener.cloud/ip-stack=dual-stack
	AnnotationAwsLoadBalancerIpAddressType = AnnotationServiceBetaGroup + "/aws-load-balancer-ip-address-type"
	// AnnotationAwsLoadBalancerIpAddressTypeValueDualStack is the value for the annotation to specify dual-stack IP address type.
	AnnotationAwsLoadBalancerIpAddressTypeValueDualStack = "dualstack"

	// AnnotationOpenStackLoadBalancerGroup is the group for OpenStack LoadBalancer Service annotations.
	AnnotationOpenStackLoadBalancerGroup = "loadbalancer.openstack.org"
	// AnnotationOpenStackLoadBalancerAddress is an optional annotation for OpenStack LoadBalancer Services to specify the load balancer address.
	// Support for PROXY protocol on Openstack (which needs a hostname as ingress)
	// If the user sets the annotation `loadbalancer.openstack.org/hostname`, the
	// annotation `loadbalancer.openstack.org/load-balancer-address` contains the IP address.
	// This address can then be used to create a DNS record for the hostname specified both
	// in annotation `loadbalancer.openstack.org/hostname` and `dns.gardener.cloud/dnsnames`
	// see https://github.com/kubernetes/cloud-provider-openstack/blob/master/docs/openstack-cloud-controller-manager/expose-applications-using-loadbalancer-type-service.md#service-annotations
	AnnotationOpenStackLoadBalancerAddress = AnnotationOpenStackLoadBalancerGroup + "/load-balancer-address"

	// AnnotationOwners is the annotation key to specify owners of a resource across namespaces and clusters.
	AnnotationOwners = "resources.gardener.cloud/owners"

	// RoleARN is a constant for the key in a provider secret that points to the role which should be assumed.
	RoleARN = "roleARN"
)

// ClassSourceFinalizer returns the finalizer string for the given DNS class and controller name.
func ClassSourceFinalizer(class, controller string) string {
	return fmt.Sprintf(FinalizerSourceTemplate, class, controller)
}
