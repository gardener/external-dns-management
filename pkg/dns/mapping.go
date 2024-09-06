// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dns

import (
	"strings"
)

////////////////////////////////////////////////////////////////////////////////
// Text Record ObjectName Mapping
////////////////////////////////////////////////////////////////////////////////

var TxtPrefix = "comment-"

func AlignHostname(host string) string {
	if strings.HasSuffix(host, ".") {
		return host
	}
	return host + "."
}

func NormalizeHostname(host string) string {
	if strings.HasPrefix(host, "\\052.") {
		host = "*" + host[4:]
	}
	if strings.HasSuffix(host, ".") {
		return host[:len(host)-1]
	}
	return host
}

func MapToProvider(rtype string, dnsset *DNSSet, _ string) (DNSSetName, *RecordSet) {
	rs := dnsset.Sets[rtype]
	return dnsset.Name, rs
}

func MapFromProvider(name DNSSetName, rs *RecordSet) (DNSSetName, *RecordSet) {
	dns := name.DNSName
	return name.WithDNSName(dns), rs
}
