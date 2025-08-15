// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dns

const (
	CONTROLLER_GROUP_DNS_CONTROLLERS = "dnscontrollers"
	CONTROLLER_GROUP_DNS_SOURCES     = "dnssources"
	CONTROLLER_GROUP_REPLICATION     = "replication"
)

const (
	DEFAULT_CLASS               = "gardendns"
	ANNOTATION_GROUP            = "dns.gardener.cloud"
	CLASS_ANNOTATION            = ANNOTATION_GROUP + "/class"
	REALM_ANNOTATION            = ANNOTATION_GROUP + "/realms"
	NOT_RATE_LIMITED_ANNOTATION = ANNOTATION_GROUP + "/not-rate-limited"
	DNS_ANNOTATION              = ANNOTATION_GROUP + "/dnsnames"
)

const OPT_SETUP = "setup"

const (
	// AnnotationIPStack is an optional annotation for DNSEntries to specify the IP stack.
	// Values are 'ipv4', 'dual-stack', and 'ipv6'. If not specified, 'ipv4' is assumed.
	// This annotation is currently only relevant for AWS-Route53 to generate alias target A and/or AAAA records.
	AnnotationIPStack                 = ANNOTATION_GROUP + "/ip-stack"
	AnnotationValueIPStackIPv4        = "ipv4"
	AnnotationValueIPStackIPDualStack = "dual-stack"
	AnnotationValueIPStackIPv6        = "ipv6"

	// AnnotationIgnore is an optional annotation for DNSEntries and source resources to ignore them on reconciliation.
	AnnotationIgnore = ANNOTATION_GROUP + "/ignore"
	// AnnotationIgnoreValueTrue is the value for the annotation to ignore the entry on reconciliation. Same as "reconcile".
	AnnotationIgnoreValueTrue = "true"
	// AnnotationIgnoreValueReconcile is the value for the annotation to ignore the entry on reconciliation. Same as "true".
	AnnotationIgnoreValueReconcile = "reconcile"
	// AnnotationIgnoreValueFull is the value for the annotation to ignore the entry on reconciliation and deletion.
	// IMPORTANT NOTE: The entry is even ignored on deletion. Use with caution to avoid orphaned entries!
	AnnotationIgnoreValueFull = "full"

	// AnnotationHardIgnore is an optional annotation for a generated target DNSEntry to ignore it on reconciliation.
	// It is enabled if the annotation value is "true".
	// This annotation is not propagated from source objects to the target DNSEntry.
	// IMPORTANT NOTE: The entry is even ignored on deletion. Use with caution to avoid orphaned entries!
	AnnotationHardIgnore = ANNOTATION_GROUP + "/target-hard-ignore"

	// AnnotationValidationError is an optional annotation for replicated provider secrets to indicate a validation error.
	AnnotationValidationError = ANNOTATION_GROUP + "/validation-error"

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
)
