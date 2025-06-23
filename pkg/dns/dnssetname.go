// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dns

import "github.com/gardener/controller-manager-library/pkg/utils"

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

func (n DNSSetName) Align() DNSSetName {
	return n.WithDNSName(AlignHostname(n.DNSName))
}

func (n DNSSetName) Normalize() DNSSetName {
	return n.WithDNSName(NormalizeHostname(n.DNSName))
}

type DNSNameSet map[DNSSetName]struct{}

func NewDNSNameSet(names ...DNSSetName) DNSNameSet {
	set := DNSNameSet{}
	set.AddAll(names...)
	return set
}

func (s DNSNameSet) AddAll(names ...DNSSetName) {
	for _, name := range names {
		s.Add(name)
	}
}

func (s DNSNameSet) Add(name DNSSetName) {
	s[name] = struct{}{}
}

func (s DNSNameSet) Contains(name DNSSetName) bool {
	_, ok := s[name]
	return ok
}

func (s DNSNameSet) IsEmpty() bool {
	return len(s) == 0
}

func (s DNSNameSet) Remove(name DNSSetName) {
	delete(s, name)
}

func NewDNSNameSetFromStringSet(dnsNames utils.StringSet, setIdentifier string) DNSNameSet {
	set := DNSNameSet{}
	for dnsname := range dnsNames {
		set.Add(DNSSetName{
			DNSName:       dnsname,
			SetIdentifier: setIdentifier,
		})
	}
	return set
}
