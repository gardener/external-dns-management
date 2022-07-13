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

type RecordSetName struct {
	// domain name of the record
	DNSName string
	// optional set identifier (used for record with routing policy)
	SetIdentifier string
}

func (n RecordSetName) WithDNSName(dnsName string) RecordSetName {
	return RecordSetName{DNSName: dnsName, SetIdentifier: n.SetIdentifier}
}

func (n RecordSetName) String() string {
	if n.SetIdentifier == "" {
		return n.DNSName
	}
	return n.DNSName + "#" + n.SetIdentifier
}

func (n RecordSetName) Align() RecordSetName {
	return n.WithDNSName(AlignHostname(n.DNSName))
}

func (n RecordSetName) Normalize() RecordSetName {
	return n.WithDNSName(NormalizeHostname(n.DNSName))
}

type RecordSetNameSet map[RecordSetName]struct{}

func NewRecordNameSet(names ...RecordSetName) RecordSetNameSet {
	set := RecordSetNameSet{}
	set.AddAll(names...)
	return set
}

func (s RecordSetNameSet) AddAll(names ...RecordSetName) {
	for _, name := range names {
		s.Add(name)
	}
}

func (s RecordSetNameSet) Add(name RecordSetName) {
	s[name] = struct{}{}
}

func (s RecordSetNameSet) Contains(name RecordSetName) bool {
	_, ok := s[name]
	return ok
}

func (s RecordSetNameSet) IsEmpty() bool {
	return len(s) == 0
}

func (s RecordSetNameSet) Remove(name RecordSetName) {
	delete(s, name)
}

func NewRecordSetNameSetFromStringSet(dnsNames utils.StringSet, setIdentifier string) RecordSetNameSet {
	set := RecordSetNameSet{}
	for dnsname := range dnsNames {
		set.Add(RecordSetName{
			DNSName:       dnsname,
			SetIdentifier: setIdentifier,
		})
	}
	return set
}
