// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package dns

const (
	// DNSDefaultTTL is the default TTL for DNS records in seconds.
	DNSDefaultTTL = 300 // 300 seconds

	// DNSAnnotationKey used to preserve the original FQDN without prefix and replacements.
	OriginalFQDNAnnotationKey = "OriginalFQDN"

	// SetIdentifierKey used to preserve the identifier when handling records.
	SetIdentifierKey = "SetIdentifier"
)
