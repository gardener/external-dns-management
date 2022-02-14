/*
 * Copyright 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 *
 */

package conversion

import (
	"reflect"
	"testing"

	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

func TestMarshalDNSSets(t *testing.T) {
	sets1 := dns.DNSSets{}
	rsb := dns.NewRecordSet(dns.RS_A, 100, []*dns.Record{{Value: "1.1.1.1"}, {Value: "1.1.1.2"}})
	rsc := dns.NewRecordSet(dns.RS_TXT, 200, []*dns.Record{{Value: "foo"}, {Value: "bar"}})
	sets1.AddRecordSet("b.a", rsb)
	sets1.AddRecordSet("c.a", rsc)
	table := []struct {
		name string
		sets dns.DNSSets
	}{
		{"empty", dns.DNSSets{}},
		{"sets1", sets1},
	}

	for _, item := range table {
		remote := MarshalDNSSets(item.sets)
		copy := UnmarshalDNSSets(remote)

		if !reflect.DeepEqual(item.sets, copy) {
			t.Errorf("dnssets mismatch item %s", item.name)
		}
	}
}

func TestMarshalChangeRequest(t *testing.T) {
	set := dns.NewDNSSet("a.b")
	set.UpdateGroup = "group1"
	set.SetMetaAttr(dns.ATTR_OWNER, "owner1")
	set.SetMetaAttr(dns.ATTR_PREFIX, "comment-")
	set.SetRecordSet(dns.RS_A, 100, "1.1.1.1", "1.1.1.2")
	table := []struct {
		name    string
		request *provider.ChangeRequest
	}{
		{"create", provider.NewChangeRequest(provider.R_CREATE, dns.RS_A, nil, set, nil)},
		{"update", provider.NewChangeRequest(provider.R_UPDATE, dns.RS_META, nil, set, nil)},
		{"delete", provider.NewChangeRequest(provider.R_DELETE, dns.RS_A, set, nil, nil)},
	}

	for _, item := range table {
		remote, err := MarshalChangeRequest(item.request)
		if err != nil {
			t.Errorf("MarshalChangeRequest failed: %w", err)
			continue
		}
		copy, err := UnmarshalChangeRequest(remote, nil)
		if err != nil {
			t.Errorf("UnmarshalChangeRequest failed: %w", err)
			continue
		}

		var add, del *dns.DNSSet
		if item.request.Addition != nil {
			add = item.request.Addition.Clone()
			add.Sets = map[string]*dns.RecordSet{item.request.Type: add.Sets[item.request.Type]}
		}
		if item.request.Deletion != nil {
			del = item.request.Deletion.Clone()
			del.Sets = map[string]*dns.RecordSet{item.request.Type: del.Sets[item.request.Type]}
		}
		expected := provider.NewChangeRequest(item.request.Action, item.request.Type, del, add, item.request.Done)
		if !reflect.DeepEqual(expected, copy) {
			t.Errorf("change request mismatch: %s", item.name)
		}
	}
}
