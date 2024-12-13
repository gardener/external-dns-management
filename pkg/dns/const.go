// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
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
	// AnnotationHardIgnore is an optional annotation for a generated target DNSEntry to ignore it on reconciliation.
	// This annotation is not propagated from source objects to the target DNSEntry.
	// IMPORTANT NOTE: The entry is even ignored on deletion, so use with caution to avoid orphaned entries.
	AnnotationHardIgnore = ANNOTATION_GROUP + "/target-hard-ignore"
)
