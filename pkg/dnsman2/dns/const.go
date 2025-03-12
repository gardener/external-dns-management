// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dns

const (
	ControllerGroupDNSControllers = "dnscontrollers"
	ControllerGroupDNSSources     = "dnssources"
	ControllerGroupReplication    = "replication"

	DefaultClass             = "gardendns"
	AnnotationGroup          = "dns.gardener.cloud"
	AnnotationClass          = AnnotationGroup + "/class"
	AnnotationNotRateLimited = AnnotationGroup + "/not-rate-limited"
	AnnotationDNSNames       = AnnotationGroup + "/dnsnames"

	// FinalizerCompound is the finalizer for provider resources ("compound" to be backwards-compatible).
	FinalizerCompound = "dns.gardener.cloud/compound"

	// AnnotationIPStack is an optional annotation for DNSEntries to specify the IP stack.
	// Values are 'ipv4', 'dual-stack', and 'ipv6'. If not specified, 'ipv4' is assumed.
	// This annotation is currently only relevant for AWS-Route53 to generate alias target A and/or AAAA records.
	AnnotationIPStack                 = AnnotationGroup + "/ip-stack"
	AnnotationValueIPStackIPv4        = "ipv4"
	AnnotationValueIPStackIPDualStack = "dual-stack"
	AnnotationValueIPStackIPv6        = "ipv6"

	// AnnotationIgnore is an optional annotation for DNSEntries and source resources to ignore them on reconciliation.
	AnnotationIgnore = AnnotationGroup + "/ignore"
	// AnnotationHardIgnore is an optional annotation for a generated target DNSEntry to ignore it on reconciliation.
	// This annotation is not propagated from source objects to the target DNSEntry.
	// IMPORTANT NOTE: The entry is even ignored on deletion, so use with caution to avoid orphaned entries.
	AnnotationHardIgnore = AnnotationGroup + "/target-hard-ignore"
)
