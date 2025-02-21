// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dns

import (
	"k8s.io/apimachinery/pkg/util/sets"
)

type DNSSetName struct {
	// domain name of the record
	DNSName string
	// optional set identifier (used for record with routing policy)
	SetIdentifier string
}

func (n DNSSetName) WithDNSName(dnsName string) DNSSetName {
	return DNSSetName{DNSName: dnsName, SetIdentifier: n.SetIdentifier}
}

func (n DNSSetName) String() string {
	if n.SetIdentifier == "" {
		return n.DNSName
	}
	return n.DNSName + "#" + n.SetIdentifier
}

func (n DNSSetName) EnsureTrailingDot() DNSSetName {
	return n.WithDNSName(EnsureTrailingDot(n.DNSName))
}

func (n DNSSetName) Normalize() DNSSetName {
	return n.WithDNSName(NormalizeDomainName(n.DNSName))
}

type DNSNameSet = sets.Set[DNSSetName]

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
