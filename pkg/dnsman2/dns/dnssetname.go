// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dns

import (
	"k8s.io/apimachinery/pkg/util/sets"
)

// DNSSetName represents a DNS name and an optional set identifier for routing policies.
type DNSSetName struct {
	// domain name of the record
	DNSName string
	// optional set identifier (used for record with routing policy)
	SetIdentifier string
}

// WithDNSName returns a copy of DNSSetName with the DNSName replaced by the given value.
func (n DNSSetName) WithDNSName(dnsName string) DNSSetName {
	return DNSSetName{DNSName: dnsName, SetIdentifier: n.SetIdentifier}
}

// String returns the string representation of DNSSetName, including the set identifier if present.
func (n DNSSetName) String() string {
	if n.SetIdentifier == "" {
		return n.DNSName
	}
	return n.DNSName + "#" + n.SetIdentifier
}

// EnsureTrailingDot returns a copy of DNSSetName with a trailing dot added to the DNSName.
func (n DNSSetName) EnsureTrailingDot() DNSSetName {
	return n.WithDNSName(EnsureTrailingDot(n.DNSName))
}

// Normalize returns a copy of DNSSetName with the DNSName normalized.
func (n DNSSetName) Normalize() DNSSetName {
	return n.WithDNSName(NormalizeDomainName(n.DNSName))
}

// DNSNameSet is a set of DNSSetName values.
type DNSNameSet = sets.Set[DNSSetName]

// NewDNSNameSetFromStringSet creates a DNSNameSet from a set of strings and a set identifier.
func NewDNSNameSetFromStringSet(dnsNames sets.Set[string], setIdentifier string) DNSNameSet {
	set := DNSNameSet{}
	for dnsname := range dnsNames {
		set.Insert(DNSSetName{
			DNSName:       dnsname,
			SetIdentifier: setIdentifier,
		})
	}
	return set
}
