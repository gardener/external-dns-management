// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dns

const (
	// ControllerGroupDNSControllers is the group name for DNS controller resources.
	ControllerGroupDNSControllers = "dnscontrollers"
	// ControllerGroupDNSSources is the group name for DNS source resources.
	ControllerGroupDNSSources = "dnssources"
	// ControllerGroupReplication is the group name for replication resources.
	ControllerGroupReplication = "replication"

	// DefaultClass is the default DNS class used by the controller.
	DefaultClass = "gardendns"
	// AnnotationGroup is the base annotation group for DNS-related annotations.
	AnnotationGroup = "dns.gardener.cloud"
	// AnnotationClass is the annotation key for specifying the DNS class.
	AnnotationClass = AnnotationGroup + "/class"
	// AnnotationNotRateLimited is the annotation key to disable rate limiting.
	AnnotationNotRateLimited = AnnotationGroup + "/not-rate-limited"
	// AnnotationDNSNames is the annotation key for specifying DNS names.
	AnnotationDNSNames = AnnotationGroup + "/dnsnames"

	// FinalizerCompound is the finalizer for provider resources ("compound" to be backwards-compatible).
	FinalizerCompound = "dns.gardener.cloud/compound"

	// AnnotationIPStack is an optional annotation for DNSEntries to specify the IP stack.
	// Values are 'ipv4', 'dual-stack', and 'ipv6'. If not specified, 'ipv4' is assumed.
	// This annotation is currently only relevant for AWS-Route53 to generate alias target A and/or AAAA records.
	AnnotationIPStack = AnnotationGroup + "/ip-stack"
	// AnnotationValueIPStackIPv4 is the annotation value for specifying IPv4-only IP stack.
	AnnotationValueIPStackIPv4 = "ipv4"
	// AnnotationValueIPStackIPDualStack is the annotation value for specifying dual-stack (IPv4 and IPv6) IP stack.
	AnnotationValueIPStackIPDualStack = "dual-stack"
	// AnnotationValueIPStackIPv6 is the annotation value for specifying IPv6-only IP stack.
	AnnotationValueIPStackIPv6 = "ipv6"

	// AnnotationIgnore is an optional annotation for DNSEntries and source resources to ignore them on reconciliation.
	AnnotationIgnore = AnnotationGroup + "/ignore"
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
	AnnotationHardIgnore = AnnotationGroup + "/target-hard-ignore"
)
