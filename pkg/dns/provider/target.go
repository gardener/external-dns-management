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

package provider

import (
	"fmt"
	"github.com/gardener/external-dns-management/pkg/dns"
	"net"
)

////////////////////////////////////////////////////////////////////////////////
// DNS Target
////////////////////////////////////////////////////////////////////////////////

type Targets []Target

func (this Targets) Has(target Target) bool {
	for _, t := range this {
		if t.GetRecordType() == target.GetRecordType() &&
			t.GetHostName() == target.GetHostName() {
			return true
		}
	}
	return false
}

func (this Targets) DifferFrom(targets Targets) bool {
	if len(this) != len(targets) {
		return true
	}
	for _, t := range this {
		if !targets.Has(t) {
			return true
		}
	}
	return false
}

type Target interface {
	GetHostName() string
	GetRecordType() string
	GetEntry() *EntryVersion
	Description() string
}

type target struct {
	rtype string
	host  string
	entry *EntryVersion
}

func NewText(t string, entry *EntryVersion) Target {
	return NewTarget(dns.RS_TXT, fmt.Sprintf("%q", t), entry)
}

func NewTarget(ty string, ta string, entry *EntryVersion) Target {
	return &target{rtype: ty, host: ta, entry: entry}
}

func NewTargetFromEntryVersion(name string, entry *EntryVersion) Target {
	ip := net.ParseIP(name)
	if ip == nil {
		return NewTarget(dns.RS_CNAME, name, entry)
	} else {
		return NewTarget(dns.RS_A, name, entry)
	}
}

func (t *target) GetEntry() *EntryVersion { return t.entry }
func (t *target) GetHostName() string     { return t.host }
func (t *target) GetRecordType() string   { return t.rtype }
func (t *target) Description() string {
	if t.entry != nil {
		return t.entry.Description()
	}
	return t.GetHostName()
}

func (t *target) String() string {
	return fmt.Sprintf("%s(%s)", t.GetRecordType(), t.GetHostName())
}
