// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package conversion

import (
	"reflect"
	"testing"

	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
	"github.com/gardener/external-dns-management/pkg/server/remote/common"
)

func TestMarshalDNSSets(t *testing.T) {
	sets1 := dns.DNSSets{}
	rsb := dns.NewRecordSet(dns.RS_A, 100, []*dns.Record{{Value: "1.1.1.1"}, {Value: "1.1.1.2"}})
	rsc1 := dns.NewRecordSet(dns.RS_TXT, 200, []*dns.Record{{Value: "foo"}, {Value: "bar"}})
	routingPolicy1 := &dns.RoutingPolicy{
		Type:       "weighted",
		Parameters: map[string]string{"weight": "1"},
	}
	rsc2 := dns.NewRecordSet(dns.RS_TXT, 200, []*dns.Record{{Value: "foo"}, {Value: "bla"}})
	routingPolicy2 := &dns.RoutingPolicy{
		Type:       "weighted",
		Parameters: map[string]string{"weight": "2"},
	}
	sets1.AddRecordSet(dns.DNSSetName{DNSName: "b.a"}, nil, rsb)
	sets1.AddRecordSet(dns.DNSSetName{DNSName: "c.a", SetIdentifier: "id1"}, routingPolicy1, rsc1)
	sets1.AddRecordSet(dns.DNSSetName{DNSName: "c.a", SetIdentifier: "id2"}, routingPolicy2, rsc2)
	table := []struct {
		name                 string
		sets                 dns.DNSSets
		expectedSizeVersion1 int
		expectedSizeVersion0 int
	}{
		{"empty", dns.DNSSets{}, 0, 0},
		{"sets1", sets1, 3, 1},
	}

	for _, item := range table {
		remote0 := MarshalDNSSets(item.sets, common.ProtocolVersion0)
		if len(remote0) != item.expectedSizeVersion0 {
			t.Errorf("version 0 size mismatch: %d != %d", len(remote0), item.expectedSizeVersion0)
		}
		remote := MarshalDNSSets(item.sets, common.ProtocolVersion1)
		if len(remote) != item.expectedSizeVersion1 {
			t.Errorf("version 0 size mismatch: %d != %d", len(remote), item.expectedSizeVersion1)
		}
		copy := UnmarshalDNSSets(remote)

		if !reflect.DeepEqual(item.sets, copy) {
			t.Errorf("dnssets mismatch item %s", item.name)
		}
	}
}

func TestMarshalChangeRequest(t *testing.T) {
	doTestMarshalChangeRequest(t, false)
}

func TestMarshalChangeRequestWithRoutingPolicy(t *testing.T) {
	doTestMarshalChangeRequest(t, true)
}

func doTestMarshalChangeRequest(t *testing.T, withPolicy bool) {
	var routingPolicy *dns.RoutingPolicy
	setIdentifier := ""
	if withPolicy {
		setIdentifier = "id1"
		routingPolicy = &dns.RoutingPolicy{
			Type:       dns.RoutingPolicyWeighted,
			Parameters: map[string]string{"weight": "100"},
		}
	}
	set := dns.NewDNSSet(dns.DNSSetName{DNSName: "b.a", SetIdentifier: setIdentifier}, routingPolicy)
	set.UpdateGroup = "group1"
	set.SetRecordSet(dns.RS_A, 100, "1.1.1.1", "1.1.1.2")
	table := []struct {
		name    string
		request *provider.ChangeRequest
	}{
		{"create", provider.NewChangeRequest(provider.R_CREATE, dns.RS_A, nil, set, nil)},
		{"update", provider.NewChangeRequest(provider.R_UPDATE, dns.RS_A, nil, set, nil)},
		{"delete", provider.NewChangeRequest(provider.R_DELETE, dns.RS_A, set, nil, nil)},
	}

	for _, item := range table {
		remote, err := MarshalChangeRequest(item.request)
		if err != nil {
			t.Errorf("MarshalChangeRequest failed: %s", err)
			continue
		}
		copy, err := UnmarshalChangeRequest(remote, nil)
		if err != nil {
			t.Errorf("UnmarshalChangeRequest failed: %s", err)
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
		expected.Done = nil
		if !reflect.DeepEqual(expected, copy) {
			t.Errorf("change request mismatch: %s", item.name)
		}
	}
}
