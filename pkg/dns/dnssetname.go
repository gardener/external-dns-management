/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 *
 */

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
